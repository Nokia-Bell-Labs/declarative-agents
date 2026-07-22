// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/magefile/mage/mg"
	"gopkg.in/yaml.v3"
)

// integrationHTTPRequestTimeout bounds a probe: "is this service up?", a
// question a reachable service answers immediately and an unreachable one must
// fail fast on. It is not a bound on model work. Inference calls run on
// integrationInferenceTimeout instead, because sharing one constant conflates
// two different questions and only breaks under load, which is exactly when a
// tracer runs (GH-709).
const integrationHTTPRequestTimeout = 2 * time.Second

// integrationInferenceTimeoutDefault bounds one model call. Embedding an 8B
// model measures 0.15s warm but 1.91s under the tracer's own ingest load on an
// M2 Max, and a cold 4.7GB load on a fresh machine is slower still, so the
// bound is sized for model work rather than for a healthy round trip.
const integrationInferenceTimeoutDefault = 120 * time.Second

// integrationInferenceTimeoutEnv overrides the inference bound, so a slower host
// or a larger model does not require a code change.
const integrationInferenceTimeoutEnv = "MESH_INFERENCE_TIMEOUT"

var integrationHTTPClient = &http.Client{Timeout: integrationHTTPRequestTimeout}

var integrationInferenceClient = &http.Client{Timeout: integrationInferenceTimeout()}

// integrationInferenceTimeout returns the configured inference bound, falling
// back to the default when the override is unset or unparseable. A bad value
// must not silently become a 2s bound and resurrect GH-709.
func integrationInferenceTimeout() time.Duration {
	raw := strings.TrimSpace(os.Getenv(integrationInferenceTimeoutEnv))
	if raw == "" {
		return integrationInferenceTimeoutDefault
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed <= 0 {
		return integrationInferenceTimeoutDefault
	}
	return parsed
}

// Integration groups the example's end-to-end tracer targets. Each starts real
// services (a Chroma container, the mesh agents, an external Ollama) and skips
// cleanly (does not fail) when the toolchain or a configured model is
// unavailable, so the group stays runnable in a checkout without them.
type Integration mg.Namespace

// requireProfilePaths returns an error naming the first relative profile path
// under root that does not exist, so a target fails loudly on a bad repoint
// rather than skipping for the wrong reason.
func requireProfilePaths(root string, rels ...string) error {
	for _, rel := range rels {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			return fmt.Errorf("required profile path %s: %w", rel, err)
		}
	}
	return nil
}

// startDetachedAgent launches an agent profile as a long-running subprocess with
// its OTel spans written to tracePath, and returns a stop function. stop(kill=false)
// waits up to 15s for a graceful exit after the caller has requested a lifecycle
// exit; stop(kill=true) force-kills. The trace file is the caller's to read and
// remove, so an integration can assert each agent's spans after its graceful exit
// flushes them.
func startDetachedAgent(binary, profilesRoot, coreRoot, profile, tracePath string) (func(kill bool) error, error) {
	return startDetachedAgentWithTimeout(binary, profilesRoot, coreRoot, profile, tracePath, 15*time.Second)
}

func startDetachedAgentWithTimeout(binary, profilesRoot, coreRoot, profile, tracePath string, gracefulWait time.Duration) (func(kill bool) error, error) {
	profilePath := profile
	if !filepath.IsAbs(profilePath) {
		profilePath = filepath.Join(profilesRoot, profile)
	}
	cmd := exec.Command(binary,
		"--profile", profilePath,
		"--directory", os.TempDir(),
		"--core-root", coreRoot,
		"--otel-log-file", tracePath,
	)
	cmd.Dir = profilesRoot
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", profile, err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	return func(kill bool) error {
		if kill {
			if err := cmd.Process.Kill(); err != nil {
				return fmt.Errorf("kill %s: %w", profile, err)
			}
			_ = <-done // a signal exit is expected after an explicit force-kill.
			return nil
		}
		select {
		case err := <-done:
			if err != nil {
				return fmt.Errorf("%s exited during graceful shutdown: %w", profile, err)
			}
			return nil
		case <-time.After(gracefulWait):
			killErr := cmd.Process.Kill()
			waitErr := <-done
			if killErr != nil {
				return fmt.Errorf("%s did not stop within %s; kill failed: %w", profile, gracefulWait, killErr)
			}
			if waitErr != nil {
				return fmt.Errorf("%s did not stop within %s (forced process exit: %v)", profile, gracefulWait, waitErr)
			}
			return fmt.Errorf("%s did not stop within %s", profile, gracefulWait)
		}
	}, nil
}

func waitHTTPStatus(url string, want int, timeout time.Duration) error {
	return waitHTTPStatusWithClient(integrationHTTPClient, url, want, timeout)
}

func waitHTTPStatusWithClient(client *http.Client, url string, want int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		remaining := time.Until(deadline)
		ctx, cancel := context.WithTimeout(context.Background(), min(integrationHTTPRequestTimeout, remaining))
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err == nil {
			var resp *http.Response
			resp, err = client.Do(req)
			if err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode == want {
					cancel()
					return nil
				}
				lastErr = fmt.Errorf("status %d", resp.StatusCode)
			}
		}
		cancel()
		if err == nil {
			remaining = time.Until(deadline)
			if remaining > 0 {
				time.Sleep(min(100*time.Millisecond, remaining))
			}
			continue
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = context.DeadlineExceeded
	}
	return fmt.Errorf("wait for %s status %d: %w", url, want, lastErr)
}

func requestHTTP(method, url, body string) ([]byte, int, error) {
	return requestHTTPWithClient(integrationHTTPClient, method, url, body)
}

func requestHTTPWithClient(client *http.Client, method, url, body string) ([]byte, int, error) {
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	return data, resp.StatusCode, err
}

// requestInference issues a call that runs model inference, on the inference
// bound rather than the probe bound. work names the model or operation so a
// timeout reports which work exceeded its bound, at which endpoint, and how
// long it waited: without that, a slow model and a dead service produce the
// same bare "context deadline exceeded" (GH-709 R3).
func requestInference(method, url, body, work string) ([]byte, int, error) {
	started := time.Now()
	data, status, err := requestHTTPWithClient(integrationInferenceClient, method, url, body)
	if err != nil {
		return nil, 0, fmt.Errorf("%s at %s failed after %s (inference timeout %s, override with %s): %w",
			work, url, time.Since(started).Round(time.Millisecond),
			integrationInferenceTimeout(), integrationInferenceTimeoutEnv, err)
	}
	return data, status, nil
}

func readIntegrationYAML(path, label string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", label, err)
	}
	if err := yaml.Unmarshal(data, out); err != nil {
		return fmt.Errorf("parse %s: %w", label, err)
	}
	return nil
}

// freeLoopbackAddr binds an ephemeral loopback port and returns its address, so a
// tracer can hand a real free address to a subprocess it launches.
func freeLoopbackAddr() (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer listener.Close()
	return listener.Addr().String(), nil
}

// writeExecutable writes a script to path with the executable bit set, for the
// fake helm/kubectl binaries a tracer puts on PATH.
func writeExecutable(path, script, label string) error {
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		return fmt.Errorf("write %s: %w", label, err)
	}
	return nil
}
