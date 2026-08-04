// Copyright (c) 2026 Nokia. All rights reserved.

// Package kindrig is the shared kind cluster lifecycle for integration tests
// and demos (eng01-kind-test-demo-rig). It moves the cluster code the
// chatbot-mesh magefiles grew -- runner injection, create-from-config, reuse
// without ownership, teardown in all paths, image load, log export -- behind
// one API so every example runs the same rig instead of re-implementing it.
package kindrig

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Runner runs one kind subcommand and returns its combined output. Injected
// so cluster ownership is testable against a fake kind without a real cluster.
type Runner func(args ...string) ([]byte, error)

// ContextRunner runs one kind subcommand with cancellation and deadline
// propagation. Image delivery uses it because loading can block on the local
// container engine and must remain bounded by the scenario.
type ContextRunner func(context.Context, ...string) ([]byte, error)

// CommandRunner runs a host command and returns its combined output. Scenarios
// inject a runner carrying their kubeconfig; DefaultCommandRun uses the current
// environment.
type CommandRunner func(name string, args ...string) ([]byte, error)

// ReusePolicy controls what EnsureClusterWithOptions may do when kind lists the
// requested cluster but its Kubernetes API is unhealthy.
type ReusePolicy uint8

const (
	// PreserveUnhealthyCluster is the safe default: kindrig reports the failed
	// health check and never deletes a cluster whose ownership is unknown.
	PreserveUnhealthyCluster ReusePolicy = iota
	// RecreateUnhealthyOwnedCluster is an explicit assertion by the caller that
	// the fixed cluster name is dedicated to it. An unhealthy listed cluster may
	// be deleted and recreated; the returned Cluster is then marked Created so
	// normal cleanup deletes the replacement.
	RecreateUnhealthyOwnedCluster
)

// EnsureOptions customizes reuse validation. HealthRun is injectable for unit
// tests and callers that need a context-bound command runner.
type EnsureOptions struct {
	ReusePolicy ReusePolicy
	HealthRun   CommandRunner
}

// FailureEvidence describes the persistent diagnostics to collect when an
// owned cluster's scenario fails. Directory is the final artifact directory,
// and Namespaces limits kubectl collection to scenario-owned namespaces.
type FailureEvidence struct {
	Directory  string
	Namespaces []string
	Run        CommandRunner
}

// DefaultRun streams kind's output so a multi-minute create still reports
// progress live, while also capturing it for the caller.
func DefaultRun(args ...string) ([]byte, error) {
	return DefaultRunContext(context.Background(), args...)
}

// DefaultRunContext streams and captures a context-bound kind subcommand.
func DefaultRunContext(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "kind", args...)
	var buf bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stderr, &buf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &buf)
	err := cmd.Run()
	return buf.Bytes(), err
}

// DefaultCommandRun executes a diagnostic command in the current environment.
func DefaultCommandRun(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

// Cluster records whether this run created the cluster it is using. Only a
// cluster this run created may be deleted: the integration targets use fixed
// cluster names and reuse one that is already up, so deleting unconditionally
// destroys a developer or CI cluster the test did not create (GH-589).
type Cluster struct {
	Name    string
	Created bool
}

// EnsureCluster reuses an existing cluster or creates one from the checked-in
// configuration at configPath, recording which happened so Release can decide
// whether deletion is ours to perform. The configuration file is required
// (eng01): it is what pins the node image and port mappings so two machines
// produce the same cluster. A wait of zero skips kind's readiness wait, which
// a scenario needs when its node cannot become Ready until a CNI is installed
// after create.
func EnsureCluster(run Runner, name, configPath string, wait time.Duration) (Cluster, error) {
	return EnsureClusterWithOptions(run, name, configPath, wait, EnsureOptions{})
}

// EnsureClusterWithOptions is EnsureCluster with an explicit existing-cluster
// policy. Every listed cluster must pass a Kubernetes API readiness probe using
// a kubeconfig generated for that cluster before it can be reused.
func EnsureClusterWithOptions(
	run Runner,
	name, configPath string,
	wait time.Duration,
	options EnsureOptions,
) (Cluster, error) {
	if configPath == "" {
		return Cluster{}, fmt.Errorf("kind cluster %s: a checked-in config file is required (eng01)", name)
	}
	if _, err := os.Stat(configPath); err != nil {
		return Cluster{}, fmt.Errorf("kind cluster %s: config %s: %w", name, configPath, err)
	}
	if options.ReusePolicy > RecreateUnhealthyOwnedCluster {
		return Cluster{}, fmt.Errorf("kind cluster %s: unsupported reuse policy %d", name, options.ReusePolicy)
	}
	if Exists(run, name) {
		healthRun := options.HealthRun
		if healthRun == nil {
			healthRun = DefaultCommandRun
		}
		healthErr := checkClusterHealth(run, healthRun, name)
		if healthErr == nil {
			fmt.Printf("kind: reusing healthy pre-existing cluster %s; it will not be deleted\n", name)
			return Cluster{Name: name}, nil
		}
		if options.ReusePolicy != RecreateUnhealthyOwnedCluster {
			return Cluster{}, fmt.Errorf(
				"kind cluster %s is listed but cannot be reused: %w; refusing to delete a cluster "+
					"not explicitly owned by this caller; remediation: restore its API health, "+
					"choose another cluster name, or remove it manually with kind delete cluster --name %s",
				name, healthErr, name)
		}
		fmt.Printf("kind: dedicated cluster %s is unhealthy; deleting and recreating it\n", name)
		if output, err := run("delete", "cluster", "--name", name); err != nil {
			detail := strings.TrimSpace(string(output))
			if detail != "" {
				return Cluster{}, fmt.Errorf(
					"kind cluster %s failed %v; caller authorized recreation, but kind delete "+
						"cluster --name %s failed: %w: %s; remediation: restore the cluster API "+
						"or remove the dedicated cluster manually, then rerun",
					name, healthErr, name, err, detail)
			}
			return Cluster{}, fmt.Errorf(
				"kind cluster %s failed %v; caller authorized recreation, but kind delete cluster "+
					"--name %s failed: %w; remediation: restore the cluster API or remove the "+
					"dedicated cluster manually, then rerun",
				name, healthErr, name, err)
		}
		cluster, err := createCluster(run, name, configPath, wait)
		if err != nil {
			return Cluster{}, fmt.Errorf(
				"kind cluster %s failed %v; caller-authorized recreation could not create its "+
					"replacement: %w; remediation: resolve the create failure and rerun",
				name, healthErr, err)
		}
		return cluster, nil
	}
	return createCluster(run, name, configPath, wait)
}

func createCluster(run Runner, name, configPath string, wait time.Duration) (Cluster, error) {
	args := []string{"create", "cluster", "--name", name, "--config", configPath}
	if wait > 0 {
		args = append(args, "--wait", wait.String())
	}
	if output, err := run(args...); err != nil {
		detail := strings.TrimSpace(string(output))
		if detail != "" {
			return Cluster{}, fmt.Errorf("kind create cluster %s: %w: %s", name, err, detail)
		}
		return Cluster{}, fmt.Errorf("kind create cluster %s: %w", name, err)
	}
	return Cluster{Name: name, Created: true}, nil
}

const clusterHealthRequestTimeout = "10s"

func checkClusterHealth(kindRun Runner, commandRun CommandRunner, name string) error {
	kubeconfig, cleanup, err := Kubeconfig(kindRun, name)
	if err != nil {
		return fmt.Errorf(
			"health command kubectl --kubeconfig <generated for %s> --request-timeout=%s "+
				"get --raw=/readyz could not start: %w",
			name, clusterHealthRequestTimeout, err)
	}
	defer cleanup()

	args := []string{
		"--kubeconfig", kubeconfig,
		"--request-timeout=" + clusterHealthRequestTimeout,
		"get", "--raw=/readyz",
	}
	output, err := commandRun("kubectl", args...)
	if err == nil {
		return nil
	}
	detail := strings.TrimSpace(string(output))
	if detail != "" {
		return fmt.Errorf("health command kubectl %s failed: %w: %s",
			strings.Join(args, " "), err, detail)
	}
	return fmt.Errorf("health command kubectl %s failed: %w",
		strings.Join(args, " "), err)
}

// CaptureRun runs a kind subcommand and returns only its captured combined
// output without mirroring to the terminal. It is used for subcommands whose
// output is data to consume (like `get kubeconfig`) rather than progress to
// stream, so the kubeconfig is not leaked onto the terminal.
func CaptureRun(args ...string) ([]byte, error) {
	return exec.Command("kind", args...).CombinedOutput()
}

// Kubeconfig fetches the named cluster's kubeconfig via `kind get kubeconfig`
// and writes it to a private temporary file, returning its path and a cleanup
// func. Binding every Helm/kubectl/port-forward subprocess to this file (see
// BindKubeconfig) is what keeps a reused, pre-existing cluster's commands off
// the ambient current context, which can point at an unrelated cluster and
// cause a live integration to mutate it (GH-1341).
func Kubeconfig(run Runner, name string) (string, func(), error) {
	out, err := run("get", "kubeconfig", "--name", name)
	if err != nil {
		return "", nil, fmt.Errorf("kind get kubeconfig %s: %w: %s", name, err, strings.TrimSpace(string(out)))
	}
	if len(bytes.TrimSpace(out)) == 0 {
		return "", nil, fmt.Errorf("kind get kubeconfig %s: empty kubeconfig", name)
	}
	dir, err := os.MkdirTemp("", "kindrig-kubeconfig-*")
	if err != nil {
		return "", nil, fmt.Errorf("kind kubeconfig %s temp dir: %w", name, err)
	}
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, out, 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return "", nil, fmt.Errorf("write kind kubeconfig %s: %w", name, err)
	}
	return path, func() { _ = os.RemoveAll(dir) }, nil
}

// BindKubeconfig points the process (and thus every child kubectl/helm/kind
// subprocess that inherits the environment) at the given kubeconfig by setting
// KUBECONFIG. It returns a restore func that reinstates the prior value, so a
// scenario's cluster binding does not leak past its own lifetime.
func BindKubeconfig(path string) func() {
	previous, had := os.LookupEnv("KUBECONFIG")
	_ = os.Setenv("KUBECONFIG", path)
	return func() {
		if had {
			_ = os.Setenv("KUBECONFIG", previous)
			return
		}
		_ = os.Unsetenv("KUBECONFIG")
	}
}

// Release deletes the cluster only when this run created it. A cleanup failure
// is reported but not fatal: the target's own result is what matters.
func (c Cluster) Release(run Runner) {
	if !c.Created {
		if c.Name != "" {
			fmt.Printf("kind: leaving pre-existing cluster %s in place\n", c.Name)
		}
		return
	}
	if _, err := run("delete", "cluster", "--name", c.Name); err != nil {
		fmt.Printf("kind: delete cluster %s failed: %v\n", c.Name, err)
	}
}

// ReleaseAfter releases an owned cluster, first persisting failure evidence
// when the scenario failed. Reused clusters are not inspected or deleted.
// Evidence errors are reported but never replace the scenario's own result.
func (c Cluster) ReleaseAfter(run Runner, failed bool, evidence FailureEvidence) {
	if !c.Created {
		c.Release(run)
		return
	}
	if failed {
		if err := evidence.capture(run, c.Name); err != nil {
			fmt.Printf("kind: capture failure evidence for %s failed: %v\n", c.Name, err)
		}
	}
	c.Release(run)
}

func (e FailureEvidence) capture(kindRun Runner, cluster string) error {
	if e.Directory == "" {
		return fmt.Errorf("evidence directory is required")
	}
	if err := os.MkdirAll(e.Directory, 0o755); err != nil {
		return fmt.Errorf("create evidence directory: %w", err)
	}
	var captureErrors []error
	if err := ExportLogs(kindRun, cluster, filepath.Join(e.Directory, "kind")); err != nil {
		captureErrors = append(captureErrors, err)
	}
	if e.Run == nil {
		if len(e.Namespaces) > 0 {
			captureErrors = append(captureErrors, fmt.Errorf("kubectl diagnostic runner is required"))
		}
		return errors.Join(captureErrors...)
	}
	for _, namespace := range e.Namespaces {
		if err := e.captureNamespace(namespace); err != nil {
			captureErrors = append(captureErrors, err)
		}
	}
	return errors.Join(captureErrors...)
}

func (e FailureEvidence) captureNamespace(namespace string) error {
	base := "namespace-" + evidenceName(namespace)
	var captureErrors []error
	if err := e.captureCommand(base+"-describe.txt", "kubectl",
		"describe", "all", "-n", namespace); err != nil {
		captureErrors = append(captureErrors, err)
	}
	pods, err := e.Run("kubectl", "get", "pods", "-n", namespace, "-o", "name")
	if writeErr := writeDiagnostic(
		filepath.Join(e.Directory, base+"-pods.txt"), pods, err); writeErr != nil {
		captureErrors = append(captureErrors, writeErr)
	}
	if err != nil {
		captureErrors = append(captureErrors, fmt.Errorf("list pods in %s: %w", namespace, err))
		return errors.Join(captureErrors...)
	}
	for _, pod := range strings.Fields(string(pods)) {
		if err := e.captureCommand(
			base+"-"+evidenceName(pod)+"-logs.txt", "kubectl",
			"logs", "-n", namespace, pod, "--all-containers=true",
			"--prefix=true", "--tail=-1"); err != nil {
			captureErrors = append(captureErrors, err)
		}
	}
	return errors.Join(captureErrors...)
}

func (e FailureEvidence) captureCommand(filename, name string, args ...string) error {
	output, commandErr := e.Run(name, args...)
	path := filepath.Join(e.Directory, filename)
	if err := writeDiagnostic(path, output, commandErr); err != nil {
		return err
	}
	if commandErr != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), commandErr)
	}
	return nil
}

func writeDiagnostic(path string, output []byte, commandErr error) error {
	data := append([]byte(nil), output...)
	if commandErr != nil {
		data = append(data, []byte(fmt.Sprintf("\n[command failed: %v]\n", commandErr))...)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write diagnostic %s: %w", path, err)
	}
	return nil
}

var nonEvidenceName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)
var gitRevision = regexp.MustCompile(`^[0-9a-fA-F]{12,64}$`)

func evidenceName(value string) string {
	return strings.Trim(nonEvidenceName.ReplaceAllString(value, "-"), "-")
}

// CommitImage returns a local image reference tagged with the tested checkout's
// 12-character commit revision. The revision is returned for evidence output.
func CommitImage(repository, revision string) (image, shortRevision string, err error) {
	revision = strings.TrimSpace(revision)
	if strings.TrimSpace(repository) == "" {
		return "", "", fmt.Errorf("image repository is required")
	}
	if !gitRevision.MatchString(revision) {
		return "", "", fmt.Errorf("git revision %q must be 12-64 hexadecimal characters", revision)
	}
	shortRevision = strings.ToLower(revision[:12])
	return repository + ":" + shortRevision, shortRevision, nil
}

// Exists reports whether the named cluster is in kind's cluster list. An
// unreadable list reports absent: Ensure then attempts a create, whose own
// error surfaces, rather than silently reusing an unknown cluster.
func Exists(run Runner, name string) bool {
	out, err := run("get", "clusters")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(line) == name {
			return true
		}
	}
	return false
}

// LoadImage loads a locally built image into the named cluster's nodes through
// an injectable, context-aware kind runner.
func LoadImage(ctx context.Context, run ContextRunner, cluster, image string) error {
	output, err := run(ctx, "load", "docker-image", image, "--name", cluster)
	if err == nil {
		return nil
	}
	if contextErr := ctx.Err(); contextErr != nil {
		err = contextErr
	}
	if detail := strings.TrimSpace(string(output)); detail != "" {
		return fmt.Errorf("kind load docker-image %s into %s: %w: %s",
			image, cluster, err, detail)
	}
	return fmt.Errorf("kind load docker-image %s into %s: %w", image, cluster, err)
}

// ExportLogs exports the cluster's node and pod logs into destDir so a failed
// run leaves enough behind to diagnose without re-running (eng01).
func ExportLogs(run Runner, cluster, destDir string) error {
	if _, err := run("export", "logs", destDir, "--name", cluster); err != nil {
		return fmt.Errorf("kind export logs %s: %w", cluster, err)
	}
	return nil
}
