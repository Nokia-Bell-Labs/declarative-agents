// Copyright (c) 2026 Nokia. All rights reserved.

// The chatbot-mesh example builds and tests independently of agent-profiles. It
// self-governs its own specification corpus with the jurist, driven by the
// agent-core runtime it depends on as a platform. The runtime binary and the
// jurist profile are external platform tools, located by convention (sibling
// checkouts) or overridden by environment.
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	// agentCoreRootEnv points at the agent-core checkout the runtime binary and the
	// jurist tools are built and resolved from; it defaults to a sibling checkout.
	agentCoreRootEnv = "AGENT_CORE_ROOT"
	// juristProfileEnv points at the jurist agent profile (a platform validation
	// tool); it defaults to the jurist under a sibling agent-profiles checkout.
	juristProfileEnv = "JURIST_PROFILE"

	juristProfileRel = "agent-profiles/agents/jurist/profile.yaml"
)

// Audit runs the jurist over this example's specification corpus, so the example
// self-governs: load_corpus reads docs/SPECIFICATIONS.yaml, docs/specs, and
// agents; validate_specs runs the corpus consistency checks; a single error
// finding (a broken index path, touchpoint, or citation) fails the target. The
// jurist exits zero on a failing corpus, so the outcome is read from its terminal
// state, not the process exit code. Skips cleanly (does not fail) when the
// agent-core checkout or the jurist profile is not reachable, so a copied-out
// example without the platform tools still runs `mage`.
func Audit() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	coreRoot := envOrDefault(agentCoreRootEnv, siblingPath(root, "agent-core"))
	if !agentCoreAvailable(coreRoot) {
		fmt.Printf("SKIP audit: agent-core checkout not found at %s (set %s)\n", coreRoot, agentCoreRootEnv)
		return nil
	}
	juristProfile := envOrDefault(juristProfileEnv, siblingPath(root, juristProfileRel))
	if _, statErr := os.Stat(juristProfile); statErr != nil {
		fmt.Printf("SKIP audit: jurist profile not found at %s (set %s)\n", juristProfile, juristProfileEnv)
		return nil
	}
	binary, err := buildAgent(coreRoot)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(binary) }()
	return runJurist(binary, juristProfile, root, coreRoot)
}

func runJurist(binary, juristProfile, root, coreRoot string) error {
	cmd := exec.Command(binary,
		"--profile", juristProfile,
		"--directory", root,
		"--core-root", coreRoot,
	)
	cmd.Dir = root
	var out bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &out)
	cmd.Stderr = io.MultiWriter(os.Stderr, &out)
	runErr := cmd.Run()
	if runErr != nil {
		return fmt.Errorf("jurist run failed: %w", runErr)
	}
	ok, err := juristSucceeded(out.String())
	switch {
	case err != nil:
		return fmt.Errorf("audit: %w; see the report above", err)
	case !ok:
		return fmt.Errorf("audit: the jurist found errors in the example corpus at %s", filepath.Join(root, "docs", "specs"))
	default:
		fmt.Println("audit: example corpus validated with no errors")
		return nil
	}
}

// juristSucceeded reads a clean/failing outcome from a jurist report. The jurist
// exits zero even when it finds errors, so the outcome is taken from its terminal
// state line, not the process exit code. A report with neither terminal marker is
// an indeterminate run and returns an error.
func juristSucceeded(report string) (bool, error) {
	switch {
	case strings.Contains(report, "terminal state: failed") || strings.Contains(report, "status=failed"):
		return false, nil
	case strings.Contains(report, "terminal state: succeeded"):
		return true, nil
	default:
		return false, fmt.Errorf("the jurist did not reach a terminal state")
	}
}

// buildAgent builds the production agent binary from the agent-core checkout, the
// same binary the runtime image ships and the integration tests drive.
func buildAgent(coreRoot string) (string, error) {
	binary := filepath.Join(os.TempDir(), "chatbot-mesh-example-agent")
	cmd := exec.Command("go", "build", "-tags", "production", "-o", binary, "./cmd/agent")
	cmd.Dir = coreRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	fmt.Printf("building agent binary from %s...\n", coreRoot)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("build agent: %w", err)
	}
	return binary, nil
}

// agentCoreAvailable reports whether coreRoot looks like an agent-core module
// checkout buildAgent can compile from.
func agentCoreAvailable(coreRoot string) bool {
	info, err := os.Stat(filepath.Join(coreRoot, "go.mod"))
	return err == nil && !info.IsDir()
}

// siblingPath resolves rel against the repository root, two levels above the
// example directory (examples/chatbot-mesh), so sibling checkouts are found.
func siblingPath(exampleRoot, rel string) string {
	return filepath.Clean(filepath.Join(exampleRoot, "..", "..", rel))
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
