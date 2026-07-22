// Copyright (c) 2026 Nokia. All rights reserved.

// Package kindrig is the shared kind cluster lifecycle for integration tests
// and demos (eng01-kind-test-demo-rig). It moves the cluster code the
// chatbot-mesh magefiles grew -- runner injection, create-from-config, reuse
// without ownership, teardown in all paths, image load, log export -- behind
// one API so every example runs the same rig instead of re-implementing it.
package kindrig

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Runner runs one kind subcommand and returns its combined output. Injected
// so cluster ownership is testable against a fake kind without a real cluster.
type Runner func(args ...string) ([]byte, error)

// DefaultRun streams kind's output so a multi-minute create still reports
// progress live, while also capturing it for the caller.
func DefaultRun(args ...string) ([]byte, error) {
	cmd := exec.Command("kind", args...)
	var buf bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stderr, &buf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &buf)
	err := cmd.Run()
	return buf.Bytes(), err
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

// LoadImage loads a locally built image into the named cluster's nodes.
func LoadImage(cluster, image string) error {
	cmd := exec.Command("kind", "load", "docker-image", image, "--name", cluster)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kind load docker-image %s: %w", image, err)
	}
	return nil
}

// ExportLogs exports the cluster's node and pod logs into destDir so a failed
// run leaves enough behind to diagnose without re-running (eng01).
func ExportLogs(run Runner, cluster, destDir string) error {
	if _, err := run("export", "logs", destDir, "--name", cluster); err != nil {
		return fmt.Errorf("kind export logs %s: %w", cluster, err)
	}
	return nil
}
