// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package kindrig

import (
	"fmt"
	"strings"
	"testing"
)

// #2477 R11: a degraded application never makes the platform unhealthy, and a
// degraded platform surface does.
func TestPlatformHealthySeparatesApplicationFromPlatform(t *testing.T) {
	healthyPlatform := []ComponentHealth{
		{Name: "data-plane", State: HealthOK},
		{Name: "object-store", State: HealthOK},
		{Name: "ingress", State: HealthOK},
	}
	appDegraded := PlatformHealthReport{
		Platform:     healthyPlatform,
		Applications: []ComponentHealth{{Name: "da-chatbot-mesh", State: HealthDegraded, Detail: "workload chatbot is not Available"}},
	}
	if !appDegraded.PlatformHealthy() {
		t.Error("a degraded application made the platform unhealthy; R11 requires they be distinct")
	}

	platformDegraded := PlatformHealthReport{
		Platform: []ComponentHealth{
			{Name: "data-plane", State: HealthOK},
			{Name: "object-store", State: HealthDegraded, Detail: "deployment is not Available"},
			{Name: "ingress", State: HealthOK},
		},
	}
	if platformDegraded.PlatformHealthy() {
		t.Error("a degraded platform surface reported the platform healthy")
	}
}

// respondingRunner maps a command's joined argv to a canned (output, error).
func respondingRunner(responses map[string]struct {
	out string
	err error
}) CommandRunner {
	return func(name string, args ...string) ([]byte, error) {
		key := name + " " + strings.Join(args, " ")
		for prefix, response := range responses {
			if strings.HasPrefix(key, prefix) {
				return []byte(response.out), response.err
			}
		}
		return nil, fmt.Errorf("unexpected command: %s", key)
	}
}

// #2477 R11: object-store degradation is a platform-surface finding; a managed
// application whose workload is not Available is an application finding, listed
// apart from the platform.
func TestDeploymentAndApplicationHealthProbes(t *testing.T) {
	ok := deploymentHealth(respondingRunner(map[string]struct {
		out string
		err error
	}{"kubectl get deployment fake-gcs": {out: "True"}}), "object-store", fakeGCSDeployment, fakeGCSNamespace)
	if ok.State != HealthOK {
		t.Fatalf("available deployment = %+v, want ok", ok)
	}
	degraded := deploymentHealth(respondingRunner(map[string]struct {
		out string
		err error
	}{"kubectl get deployment fake-gcs": {out: "False"}}), "object-store", fakeGCSDeployment, fakeGCSNamespace)
	if degraded.State != HealthDegraded {
		t.Fatalf("unavailable deployment = %+v, want degraded", degraded)
	}

	apps := applicationHealth(respondingRunner(map[string]struct {
		out string
		err error
	}{
		"kubectl get namespaces":                              {out: "da-chatbot-mesh"},
		"kubectl get deployments --namespace da-chatbot-mesh": {out: "chatbot=True collector=False "},
	}))
	if len(apps) != 1 || apps[0].Name != "da-chatbot-mesh" || apps[0].State != HealthDegraded {
		t.Fatalf("application health = %+v, want one degraded da-chatbot-mesh", apps)
	}
	if !strings.Contains(apps[0].Detail, "collector") {
		t.Errorf("degraded application detail does not name the workload: %q", apps[0].Detail)
	}
}

// bucketAwareRunner answers fake-GCS reads with per-bucket content, so the
// isolation check sees each bucket return only its own object.
func bucketAwareRunner(content map[string]string) CommandRunner {
	return func(name string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		for bucket, body := range content {
			if strings.Contains(joined, "/b/"+bucket+"/o/") && strings.Contains(joined, "alt=media") {
				return []byte(body), nil
			}
		}
		return []byte("{}"), nil
	}
}

// #2477 R5, AC6: two application buckets holding the same key never cross-read;
// verifyObjectStorage passes when each returns its own bytes and refuses when a
// bucket returns the other's.
func TestVerifyObjectStorageProvesBucketIsolation(t *testing.T) {
	isolated := bucketAwareRunner(map[string]string{
		"conformance-app-a": "alpha-bucket-a",
		"conformance-app-b": "beta-bucket-b",
	})
	if err := verifyObjectStorage(isolated, PlatformClusterName); err != nil {
		t.Fatalf("isolated buckets refused: %v", err)
	}
	crossed := bucketAwareRunner(map[string]string{
		"conformance-app-a": "same-bytes",
		"conformance-app-b": "same-bytes",
	})
	err := verifyObjectStorage(crossed, PlatformClusterName)
	if err == nil || !strings.Contains(err.Error(), "not isolated") {
		t.Fatalf("cross-read was not caught: %v", err)
	}
}
