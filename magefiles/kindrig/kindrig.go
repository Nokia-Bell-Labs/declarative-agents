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
	if configPath == "" {
		return Cluster{}, fmt.Errorf("kind cluster %s: a checked-in config file is required (eng01)", name)
	}
	if _, err := os.Stat(configPath); err != nil {
		return Cluster{}, fmt.Errorf("kind cluster %s: config %s: %w", name, configPath, err)
	}
	if Exists(run, name) {
		fmt.Printf("kind: reusing pre-existing cluster %s; it will not be deleted\n", name)
		return Cluster{Name: name}, nil
	}
	args := []string{"create", "cluster", "--name", name, "--config", configPath}
	if wait > 0 {
		args = append(args, "--wait", wait.String())
	}
	if _, err := run(args...); err != nil {
		return Cluster{}, fmt.Errorf("kind create cluster %s: %w", name, err)
	}
	return Cluster{Name: name, Created: true}, nil
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
