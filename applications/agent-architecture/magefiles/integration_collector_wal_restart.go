// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const walRestartPath = "/data/wal/traces.wal"

type walRestartRecord struct {
	Key       string `json:"key"`
	Checksum  string `json:"checksum"`
	State     string `json:"state"`
	StagePath string `json:"stage_path"`
}

// CollectorWALRestart proves GH-2491 AC3 through the shipped chart on the live
// da-platform. It interrupts fake GCS after the collector has durably recorded
// a pending object, replaces the pod, and proves the persistent WAL replays the
// exact pre-recorded key once when intake resumes.
func (Integration) CollectorWALRestart() error {
	resolved, err := resolveRootsFromWorkingDirectory()
	if err != nil {
		fmt.Printf("SKIP collectorWALRestart: %v\n", err)
		return nil
	}
	if reason := smokeSkipReason(resolved); reason != "" {
		fmt.Printf("SKIP collectorWALRestart: %s\n", reason)
		return nil
	}
	return runCollectorWALRestart(resolved)
}

func runCollectorWALRestart(resolved roots) (result error) {
	scenario, err := acquireSmokeScenario(resolved.Application, "collectorWALRestart")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, scenario.release(result != nil)) }()
	lease, err := kindrig.AcquireAgentCoreImageLease(
		resolved.Core, resolved.Image, smokeScenarioName+"-wal-restart")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, lease.Release()) }()
	environment := scenario.environment
	if err := prepareSmokeCluster(environment, scenario.platform.Cluster.Name, resolved.Image); err != nil {
		return smokeFailure(environment.run, "cluster preparation", err)
	}

	bucket := fmt.Sprintf("wal-restart-%d", time.Now().UTC().UnixNano())
	if err := createFakeGCSBucket(environment, bucket); err != nil {
		return smokeFailure(environment.run, "bucket provisioning", err)
	}
	defer func() { result = errors.Join(result, purgeFakeGCSBucket(environment, bucket)) }()

	archiveDir, err := os.MkdirTemp("", "agent-architecture-wal-chart-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(archiveDir) }()
	archive, err := packageHelmChart(filepath.Join(resolved.Application, "helm"), resolved.Catalog, archiveDir)
	if err != nil {
		return fmt.Errorf("collectorWALRestart chart package: %w", err)
	}
	if err := installWALRestartChart(
		environment, archive, resolved.Application, resolved.Image, bucket); err != nil {
		return smokeFailure(environment.run, "Helm install", err)
	}
	deployment := smokeRelease + "-agent-architecture-collector"
	if err := rolloutCollector(environment, deployment); err != nil {
		return smokeFailure(environment.run, "collector readiness", err)
	}

	if err := scaleFakeGCS(environment, 0); err != nil {
		return smokeFailure(environment.run, "stop fake GCS", err)
	}
	fakeGCSRestored := false
	defer func() {
		if !fakeGCSRestored {
			result = errors.Join(result, restoreFakeGCS(environment))
		}
	}()

	if err := exportCollectorTrace(environment, "before-pod-restart"); err != nil {
		return smokeFailure(environment.run, "export pre-restart trace", err)
	}
	pod, pending, err := waitPendingWAL(environment, deployment, 45*time.Second)
	if err != nil {
		return smokeFailure(environment.run, "observe pending WAL", err)
	}
	if err := runSmokeCommand(environment, smokeReadyTimeout, "kubectl", "delete", "pod/"+pod,
		"-n", smokeNamespace, "--wait=true"); err != nil {
		return smokeFailure(environment.run, "replace collector pod", err)
	}
	if err := rolloutCollector(environment, deployment); err != nil {
		return smokeFailure(environment.run, "WAL-backed readiness during bucket outage", err)
	}
	replacement, err := collectorPod(environment, deployment)
	if err != nil {
		return err
	}
	if replacement == pod {
		return fmt.Errorf("collector pod was not replaced: still %s", pod)
	}
	survived, err := readPendingWAL(environment, replacement)
	if err != nil {
		return err
	}
	if survived.Key != pending.Key || survived.Checksum != pending.Checksum {
		return fmt.Errorf("persistent WAL identity changed across pod replacement: before=%+v after=%+v", pending, survived)
	}

	if err := restoreFakeGCS(environment); err != nil {
		return smokeFailure(environment.run, "restore fake GCS", err)
	}
	fakeGCSRestored = true
	if err := exportCollectorTrace(environment, "after-pod-restart"); err != nil {
		return smokeFailure(environment.run, "export post-restart trace", err)
	}
	objects, err := waitForObjectReplay(environment, bucket, pending.Key, 60*time.Second)
	if err != nil {
		return smokeFailure(environment.run, "wait for immutable replay", err)
	}
	if occurrences(objects, pending.Key) != 1 {
		return fmt.Errorf("pending key %q appears %d times in objects %v, want exactly once",
			pending.Key, occurrences(objects, pending.Key), objects)
	}
	if len(objects) != 2 {
		return fmt.Errorf("objects after replay = %v, want pending and post-restart batches", objects)
	}
	if err := waitWALCleared(environment, replacement, 30*time.Second); err != nil {
		return err
	}
	fmt.Printf("integration:collectorWALRestart PASS - pod %s replaced by %s with key %s pending; readiness stayed healthy during outage and replay created exactly one immutable object\n",
		pod, replacement, pending.Key)
	return nil
}

func installWALRestartChart(
	environment smokeEnvironment,
	archive, applicationRoot, image, bucket string,
) error {
	repository, tag := splitImageRef(image)
	ctx, cancel := context.WithTimeout(context.Background(), smokeInstallTimeout)
	defer cancel()
	args := []string{
		"install", smokeRelease, archive, "--namespace", smokeNamespace,
		"--values", filepath.Join(applicationRoot, "helm", "ci", "kind-values.yaml"),
		"--set", "image.repository=" + repository, "--set-string", "image.tag=" + tag,
		"--set", "collector.image.repository=" + repository, "--set-string", "collector.image.tag=" + tag,
		"--set", "collector.storage.backend=object",
		"--set-string", "collector.storage.bucketName=" + bucket,
		"--set-string", "collector.storage.endpoint=" + kindrig.FakeGCSEndpoint,
		"--set-string", "collector.storage.prefix=wal-restart",
		"--set-string", "collector.storage.walCapacity=64Mi",
		"--timeout", smokeInstallTimeout.String(),
	}
	output, err := environment.run(ctx, "helm", args...)
	if err != nil {
		return fmt.Errorf("helm install: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func rolloutCollector(environment smokeEnvironment, deployment string) error {
	return runSmokeCommand(environment, smokeReadyTimeout, "kubectl", "rollout", "status",
		"deployment/"+deployment, "-n", smokeNamespace, "--timeout=90s")
}

func collectorPod(environment smokeEnvironment, deployment string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), smokeProbeTimeout)
	defer cancel()
	out, err := environment.run(ctx, "kubectl", "get", "pods", "-n", smokeNamespace,
		"-l", "app.kubernetes.io/component=collector", "-o", "jsonpath={.items[0].metadata.name}")
	if err != nil {
		return "", fmt.Errorf("get collector pod for %s: %w: %s", deployment, err, strings.TrimSpace(string(out)))
	}
	pod := strings.TrimSpace(string(out))
	if pod == "" {
		return "", fmt.Errorf("deployment %s has no collector pod", deployment)
	}
	return pod, nil
}

func scaleFakeGCS(environment smokeEnvironment, replicas int) error {
	return runSmokeCommand(environment, smokeReadyTimeout, "kubectl", "scale", "deployment/fake-gcs",
		"-n", "fake-gcs", fmt.Sprintf("--replicas=%d", replicas))
}

func restoreFakeGCS(environment smokeEnvironment) error {
	if err := scaleFakeGCS(environment, 1); err != nil {
		return err
	}
	return runSmokeCommand(environment, smokeReadyTimeout, "kubectl", "rollout", "status",
		"deployment/fake-gcs", "-n", "fake-gcs", "--timeout=90s")
}

func readPendingWAL(environment smokeEnvironment, pod string) (walRestartRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), smokeProbeTimeout)
	defer cancel()
	out, err := environment.run(ctx, "kubectl", "exec", "-n", smokeNamespace, pod, "--",
		"sh", "-c", "test ! -f "+walRestartPath+" || cat "+walRestartPath)
	if err != nil {
		return walRestartRecord{}, fmt.Errorf("read WAL from %s: %w: %s", pod, err, strings.TrimSpace(string(out)))
	}
	var pending walRestartRecord
	for _, line := range bytes.Split(bytes.TrimSpace(out), []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var record walRestartRecord
		if err := json.Unmarshal(line, &record); err != nil {
			return walRestartRecord{}, fmt.Errorf("decode WAL from %s: %w", pod, err)
		}
		if record.State == "pending" {
			pending = record
		}
	}
	if pending.Key == "" {
		return walRestartRecord{}, fmt.Errorf("pod %s has no pending WAL record", pod)
	}
	return pending, nil
}

func waitPendingWAL(environment smokeEnvironment, deployment string, timeout time.Duration) (string, walRestartRecord, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		pod, err := collectorPod(environment, deployment)
		if err == nil {
			record, readErr := readPendingWAL(environment, pod)
			if readErr == nil {
				return pod, record, nil
			}
			lastErr = readErr
		} else {
			lastErr = err
		}
		time.Sleep(250 * time.Millisecond)
	}
	return "", walRestartRecord{}, fmt.Errorf("no pending WAL within %s: %w", timeout, lastErr)
}

func waitWALCleared(environment smokeEnvironment, pod string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if _, err := readPendingWAL(environment, pod); err != nil && strings.Contains(err.Error(), "no pending WAL") {
			return nil
		} else {
			if err != nil {
				lastErr = err
			} else {
				lastErr = errors.New("WAL still has a pending record")
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("pending WAL not cleared on %s within %s: %w", pod, timeout, lastErr)
}

func exportCollectorTrace(environment smokeEnvironment, service string) error {
	local, err := freeLocalPort()
	if err != nil {
		return err
	}
	forward, err := forwardService(environment, smokeRelease+"-agent-architecture-collector", local+":4317")
	if err != nil {
		return err
	}
	defer forward.stop()
	time.Sleep(500 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, err := grpc.NewClient("127.0.0.1:"+local, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	hash := sha256.Sum256([]byte(service))
	now := uint64(time.Now().UnixNano())
	request := &coltracepb.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{{
		Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{{
			Key: "service.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: service}},
		}}},
		ScopeSpans: []*tracepb.ScopeSpans{{Spans: []*tracepb.Span{{
			TraceId: hash[:16], SpanId: hash[16:24], Name: service,
			StartTimeUnixNano: now, EndTimeUnixNano: now + uint64(time.Millisecond),
		}}}},
	}}}
	_, err = coltracepb.NewTraceServiceClient(conn).Export(ctx, request, grpc.WaitForReady(true))
	return err
}

func forwardFakeGCS(environment smokeEnvironment) (*portForward, string, error) {
	local, err := freeLocalPort()
	if err != nil {
		return nil, "", err
	}
	command := exec.Command("kubectl", "port-forward", "-n", "fake-gcs", "service/fake-gcs", local+":4443")
	command.Env = append(os.Environ(), "KUBECONFIG="+environment.kubeconfig)
	command.Stdout, command.Stderr = os.Stderr, os.Stderr
	if err := command.Start(); err != nil {
		return nil, "", err
	}
	return &portForward{command: command}, "http://127.0.0.1:" + local, nil
}

func fakeGCSRequest(base, method, path string, body []byte) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < 20; attempt++ {
		request, err := http.NewRequest(method, base+path, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		if len(body) > 0 {
			request.Header.Set("Content-Type", "application/json")
		}
		response, err := (&http.Client{Timeout: 3 * time.Second}).Do(request)
		if err == nil {
			data, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
			_ = response.Body.Close()
			if response.StatusCode >= 200 && response.StatusCode < 300 && readErr == nil {
				return data, nil
			}
			lastErr = fmt.Errorf("%s %s status %d: %s", method, path, response.StatusCode, strings.TrimSpace(string(data)))
		} else {
			lastErr = err
		}
		time.Sleep(250 * time.Millisecond)
	}
	return nil, lastErr
}

func createFakeGCSBucket(environment smokeEnvironment, bucket string) error {
	forward, base, err := forwardFakeGCS(environment)
	if err != nil {
		return err
	}
	defer forward.stop()
	_, err = fakeGCSRequest(base, http.MethodPost, "/storage/v1/b?project=wal-restart",
		[]byte(fmt.Sprintf(`{"name":%q}`, bucket)))
	return err
}

func listFakeGCSObjects(base, bucket string) ([]string, error) {
	data, err := fakeGCSRequest(base, http.MethodGet, "/storage/v1/b/"+url.PathEscape(bucket)+"/o", nil)
	if err != nil {
		return nil, err
	}
	var response struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(response.Items))
	for _, item := range response.Items {
		keys = append(keys, item.Name)
	}
	return keys, nil
}

func waitForObjectReplay(environment smokeEnvironment, bucket, key string, timeout time.Duration) ([]string, error) {
	forward, base, err := forwardFakeGCS(environment)
	if err != nil {
		return nil, err
	}
	defer forward.stop()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		objects, listErr := listFakeGCSObjects(base, bucket)
		if listErr == nil && occurrences(objects, key) == 1 && len(objects) >= 2 {
			return objects, nil
		}
		lastErr = listErr
		time.Sleep(500 * time.Millisecond)
	}
	return nil, fmt.Errorf("key %q not replayed within %s: %w", key, timeout, lastErr)
}

func purgeFakeGCSBucket(environment smokeEnvironment, bucket string) error {
	forward, base, err := forwardFakeGCS(environment)
	if err != nil {
		return err
	}
	defer forward.stop()
	objects, err := listFakeGCSObjects(base, bucket)
	if err != nil {
		return err
	}
	for _, key := range objects {
		if _, err := fakeGCSRequest(base, http.MethodDelete,
			"/storage/v1/b/"+url.PathEscape(bucket)+"/o/"+url.PathEscape(key), nil); err != nil {
			return err
		}
	}
	_, err = fakeGCSRequest(base, http.MethodDelete, "/storage/v1/b/"+url.PathEscape(bucket), nil)
	return err
}

func occurrences(values []string, want string) int {
	count := 0
	for _, value := range values {
		if value == want {
			count++
		}
	}
	return count
}
