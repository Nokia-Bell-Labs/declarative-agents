// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package otlp

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/objectstore"
)

// The live proof's backend, pinned by version and digest per ENG01 C2 and the
// same image the objectstore family's live proof uses.
const (
	spoolFakeGCSImageVersion = "1.56.1"
	spoolFakeGCSImageDigest  = "sha256:797ce226d62f947c009dc40246b30cfb456b8473d8241407f9d6f2c04e4d69ef"
	spoolFakeGCSImage        = "docker.io/fsouza/fake-gcs-server:" +
		spoolFakeGCSImageVersion + "@" + spoolFakeGCSImageDigest
	spoolFakeGCSContainer = "agent-core-otlp-spool-fake-gcs"
)

// startSpoolFakeGCS runs the pinned fake-gcs-server for the collector spool
// proof. It skips naming docker where docker is absent (ENG01), replaces a
// leftover container, seeds the fixture bucket, and cleans up on every path.
func startSpoolFakeGCS(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not on PATH: the collector object-spool live proof needs it")
	}
	_ = exec.Command("docker", "rm", "-f", spoolFakeGCSContainer).Run()
	out, err := exec.Command("docker", "run", "-d", "--name", spoolFakeGCSContainer,
		"-p", "127.0.0.1:0:4443", spoolFakeGCSImage,
		"-scheme", "http", "-port", "4443").CombinedOutput()
	if err != nil {
		t.Fatalf("docker run fake-gcs-server: %v\n%s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", spoolFakeGCSContainer).Run() })

	portOut, err := exec.Command("docker", "port", spoolFakeGCSContainer, "4443/tcp").Output()
	if err != nil {
		t.Fatalf("docker port: %v", err)
	}
	address := strings.TrimSpace(strings.Split(string(portOut), "\n")[0])
	endpoint := "http://" + address + "/storage/v1/"
	waitForFakeGCS(t, endpoint)

	body := strings.NewReader(`{"name":"agents"}`)
	response, err := http.Post(endpoint+"b?project=test", "application/json", body)
	if err != nil {
		t.Fatalf("create fixture bucket: %v", err)
	}
	_ = response.Body.Close()
	return endpoint
}

func waitForFakeGCS(t *testing.T, endpoint string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		response, err := http.Get(endpoint + "b?project=test")
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		if time.Now().After(deadline) {
			logs, _ := exec.Command("docker", "logs", spoolFakeGCSContainer).CombinedOutput()
			t.Fatalf("fake-gcs-server never answered at %s; logs:\n%s", endpoint, logs)
		}
		time.Sleep(300 * time.Millisecond)
	}
}

// srd008 AC2: the collector object backend end to end over gs:// against a real
// server — WAL-first upload, idempotent retry, separate trace and metric
// objects, and a different-content collision refused rather than overwritten.
func TestCollectorObjectSpoolAgainstFakeGCS(t *testing.T) {
	endpoint := startSpoolFakeGCS(t)
	connection := objectstore.ConnectionConfig{BucketURL: "gs://agents", Endpoint: endpoint}
	store := newObjectStore(t, connection, filepath.Join(t.TempDir(), "collector.wal"))
	ctx := context.Background()

	trace, err := store.persist(ctx, spoolBatch{signal: "trace", payload: tracePayload(t, "chatbot", 2)})
	if err != nil {
		t.Fatalf("persist trace: %v", err)
	}
	if trace.walState != walStateCommitted || !bucketHas(t, store.opener, connection, trace.objectKey) {
		t.Fatalf("trace object not committed: %+v", trace)
	}

	// Idempotent retry: the same batch resolves to the same key with no error.
	retry, err := store.persist(ctx, spoolBatch{signal: "trace", payload: tracePayload(t, "chatbot", 2)})
	if err != nil || retry.objectKey != trace.objectKey {
		t.Fatalf("retry = %+v (%v), want the same key idempotently", retry, err)
	}

	metric, err := store.persist(ctx, spoolBatch{signal: "metric", payload: []byte(`{"resourceMetrics":[]}`)})
	if err != nil {
		t.Fatalf("persist metric: %v", err)
	}
	if filepath.Dir(trace.objectKey) == filepath.Dir(metric.objectKey) {
		t.Fatalf("trace and metric share a family: %q vs %q", trace.objectKey, metric.objectKey)
	}

	if err := store.putObject(ctx, trace.objectKey, []byte("different"), "cafebabe"); err == nil {
		t.Fatal("a different payload overwrote an existing key on the live backend")
	}
	fmt.Println("collector object-spool live proof PASS - gs:// WAL-first, idempotent, integrity-guarded")
}
