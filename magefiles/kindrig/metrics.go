// Copyright (c) 2026 Nokia. All rights reserved.

package kindrig

import (
	_ "embed"
	"fmt"
	"os"
	"runtime"
	"strings"
)

const (
	metricsServerImageRepository   = "registry.k8s.io/metrics-server/metrics-server"
	metricsServerRuntimeRepository = "kindrig/metrics-server"
	metricsServerImageVersion      = "v0.9.0"
	metricsServerImagePlaceholder  = "KINDRIG_METRICS_SERVER_IMAGE"
	metricsAPIService              = "v1beta1.metrics.k8s.io"
)

var metricsServerImageDigests = map[string]string{
	"amd64": "sha256:25d291fde59974547bac6f07fa9d6cf6f5bedd1f19d60c893311c5e741e0a42f",
	"arm64": "sha256:d0341e07a1cf2b751e684cba68e2a0c62d3c5d2d23501fd67dc384d517ec65de",
}

//go:embed metrics-server-kind.yaml
var metricsServerKindManifest string

// InstallMetricsServer installs pinned, test-only resource metrics into a kind
// cluster. It returns a cleanup that removes only resources this call installed.
// A healthy pre-existing metrics API is reused. An unhealthy existing API is
// refused rather than overwritten because its ownership is unknown.
func InstallMetricsServer(run CommandRunner, cluster string) (func() error, error) {
	if strings.TrimSpace(cluster) == "" {
		return nil, fmt.Errorf("install metrics-server: kind cluster name is required")
	}
	status, err := metricsAPIStatus(run)
	if err == nil && status == "True" {
		return func() error { return nil }, nil
	}
	if err == nil {
		return nil, fmt.Errorf("metrics API already exists but is not healthy: status %q", status)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "notfound") {
		return nil, fmt.Errorf("read existing metrics API: %w", err)
	}
	sourceImage, err := metricsServerImage(runtime.GOARCH)
	if err != nil {
		return nil, err
	}
	runtimeImage := metricsServerRuntimeRepository + ":" + metricsServerImageVersion
	manifest := strings.ReplaceAll(
		metricsServerKindManifest, metricsServerImagePlaceholder, runtimeImage)
	path, removeFile, err := writeMetricsManifest(manifest)
	if err != nil {
		return nil, err
	}
	commands := [][]string{
		{"docker", "pull", "--platform", "linux/" + runtime.GOARCH, sourceImage},
		{"docker", "tag", sourceImage, runtimeImage},
		{"kind", "load", "docker-image", runtimeImage, "--name", cluster},
		{"kubectl", "apply", "-f", path},
		{"kubectl", "rollout", "status", "deployment/metrics-server",
			"--namespace", "kube-system", "--timeout=180s"},
		{"kubectl", "wait", "--for=condition=Available",
			"apiservice/" + metricsAPIService, "--timeout=180s"},
	}
	for _, command := range commands {
		if err := runMetricsCommand(run, command); err != nil {
			_ = deleteMetricsManifest(run, path)
			removeFile()
			return nil, err
		}
	}
	return func() error {
		defer removeFile()
		return deleteMetricsManifest(run, path)
	}, nil
}

func metricsAPIStatus(run CommandRunner) (string, error) {
	out, err := run("kubectl", "get", "apiservice", metricsAPIService,
		"-o", `jsonpath={.status.conditions[?(@.type=="Available")].status}`)
	status := strings.TrimSpace(string(out))
	if err != nil {
		return status, fmt.Errorf("%w: %s", err, status)
	}
	return status, nil
}

func metricsServerImage(arch string) (string, error) {
	digest, ok := metricsServerImageDigests[arch]
	if !ok {
		return "", fmt.Errorf("install metrics-server: %s has no pinned digest for linux/%s",
			metricsServerImageVersion, arch)
	}
	return metricsServerImageRepository + ":" + metricsServerImageVersion + "@" + digest, nil
}

func runMetricsCommand(run CommandRunner, command []string) error {
	name, args := command[0], command[1:]
	out, err := run(name, args...)
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s",
			name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func writeMetricsManifest(manifest string) (string, func(), error) {
	file, err := os.CreateTemp("", "kindrig-metrics-server-*.yaml")
	if err != nil {
		return "", nil, fmt.Errorf("create metrics-server manifest: %w", err)
	}
	path := file.Name()
	cleanup := func() { _ = os.Remove(path) }
	if _, err := file.WriteString(manifest); err != nil {
		_ = file.Close()
		cleanup()
		return "", nil, fmt.Errorf("write metrics-server manifest: %w", err)
	}
	if err := file.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("close metrics-server manifest: %w", err)
	}
	return path, cleanup, nil
}

func deleteMetricsManifest(run CommandRunner, path string) error {
	out, err := run("kubectl", "delete", "-f", path, "--ignore-not-found")
	if err != nil {
		return fmt.Errorf("delete metrics-server manifest: %w: %s",
			err, strings.TrimSpace(string(out)))
	}
	return nil
}
