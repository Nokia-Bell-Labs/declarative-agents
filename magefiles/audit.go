// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

const patternLanguagePath = "design-patterns/pattern-language.yaml"

type statFunc func(string) (os.FileInfo, error)
type auditRunner func(string) error
type patternCheckRunner func(string, string, string) error

type patternLanguageFile struct {
	Patterns []patternWithInvariants `yaml:"patterns"`
}

type patternWithInvariants struct {
	ID         string             `yaml:"id"`
	Invariants []patternInvariant `yaml:"invariants"`
}

type patternInvariant struct {
	ID        string                 `yaml:"id"`
	Statement string                 `yaml:"statement"`
	Check     *patternInvariantCheck `yaml:"check"`
}

type patternInvariantCheck struct {
	Kind                    string   `yaml:"kind"`
	Command                 string   `yaml:"command"`
	Issue                   string   `yaml:"issue"`
	Reason                  string   `yaml:"reason"`
	NegativeTest            string   `yaml:"negative_test"`
	Module                  string   `yaml:"module"`
	AdapterPackages         []string `yaml:"adapter_packages"`
	ProviderImports         []string `yaml:"provider_imports"`
	RootTypes               []string `yaml:"root_types"`
	WholeValueFuncs         []string `yaml:"whole_value_functions"`
	DocumentationOnlyFields []string `yaml:"documentation_only_fields"`
}

type executablePatternCheck struct {
	invariantID  string
	command      string
	negativeTest string
}

type patternInvariantSummary struct {
	total      int
	executable int
	pending    int
	manual     int
}

// Audit first validates repository-wide document placement and actor-role
// realization, warms the agent build cache once, then runs mage audit in each
// sub-module and participating example module concurrently.
func Audit() error {
	if err := runDocumentPlacementAudit(); err != nil {
		return err
	}
	if err := runPatternInvariantAudit(); err != nil {
		return err
	}
	if err := runAgentRoleRealizationAudit(); err != nil {
		return err
	}
	if err := warmAgentBuild(); err != nil {
		return err
	}
	return auditSubModules(auditParticipants(), os.Stat, runMageAudit)
}

func runDocumentPlacementAudit() error {
	cmd := exec.Command("go", "test", ".", "-count=1", "-run", "^TestDocumentPlacement")
	cmd.Dir = "magefiles"
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("document placement audit: %w", err)
	}
	return nil
}

func runAgentRoleRealizationAudit() error {
	cmd := exec.Command("go", "test", ".", "-run", "^TestAgentRoleRealization")
	cmd.Dir = "magefiles"
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("agent role-realization audit: %w", err)
	}
	return nil
}

func runPatternInvariantAudit() error {
	return checkPatternInvariants(".", runPatternInvariantCommand, os.Stdout)
}

func checkPatternInvariants(root string, run patternCheckRunner, output io.Writer) error {
	language, err := loadPatternInvariants(filepath.Join(root, patternLanguagePath))
	if err != nil {
		return err
	}
	checks, summary, err := validatePatternInvariants(language)
	fmt.Fprintf(output, "manual-invariant count: %d\n", summary.manual)
	if err != nil {
		return err
	}
	fmt.Fprintf(output, "pattern invariants: %d total, %d executable, %d pending\n",
		summary.total, summary.executable, summary.pending)
	for _, check := range checks {
		if err := run(root, check.command, check.negativeTest); err != nil {
			return fmt.Errorf("pattern invariant %s: %w", check.invariantID, err)
		}
	}
	return nil
}

func loadPatternInvariants(path string) (patternLanguageFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return patternLanguageFile{}, fmt.Errorf("read pattern invariants: %w", err)
	}
	var language patternLanguageFile
	if err := yaml.Unmarshal(data, &language); err != nil {
		return patternLanguageFile{}, fmt.Errorf("parse pattern invariants: %w", err)
	}
	return language, nil
}

func validatePatternInvariants(language patternLanguageFile) ([]executablePatternCheck, patternInvariantSummary, error) {
	var summary patternInvariantSummary
	var checks []executablePatternCheck
	seen := make(map[string]bool)
	for _, pattern := range language.Patterns {
		if len(pattern.Invariants) == 0 {
			return nil, summary, fmt.Errorf("pattern %s has no invariants", pattern.ID)
		}
		for _, invariant := range pattern.Invariants {
			summary.total++
			check, class, err := validatePatternInvariant(invariant, seen)
			if err != nil {
				return nil, summary, err
			}
			switch class {
			case "manual":
				summary.manual++
			case "pending":
				summary.executable++
				summary.pending++
			case "executable":
				summary.executable++
				checks = append(checks, *check)
			}
		}
	}
	return checks, summary, nil
}

func validatePatternInvariant(
	invariant patternInvariant,
	seen map[string]bool,
) (*executablePatternCheck, string, error) {
	if invariant.ID == "" {
		return nil, "", fmt.Errorf("pattern invariant has no id")
	}
	if seen[invariant.ID] {
		return nil, "", fmt.Errorf("duplicate pattern invariant %s", invariant.ID)
	}
	seen[invariant.ID] = true
	if invariant.Statement == "" {
		return nil, "", fmt.Errorf("pattern invariant %s has no statement", invariant.ID)
	}
	if invariant.Check == nil {
		return nil, "", fmt.Errorf("pattern invariant %s has no check block", invariant.ID)
	}
	return classifyPatternInvariant(invariant)
}

func classifyPatternInvariant(invariant patternInvariant) (*executablePatternCheck, string, error) {
	check := invariant.Check
	switch check.Kind {
	case "manual":
		if check.Reason == "" {
			return nil, "", fmt.Errorf("manual pattern invariant %s has no reason", invariant.ID)
		}
		return nil, "manual", nil
	case "executable":
		if check.NegativeTest == "" {
			return nil, "", fmt.Errorf("executable pattern invariant %s has no negative test", invariant.ID)
		}
		if check.Command == "" {
			if check.Issue == "" {
				return nil, "", fmt.Errorf("executable pattern invariant %s has no command or issue", invariant.ID)
			}
			return nil, "pending", nil
		}
		if !strings.Contains(check.Command, check.NegativeTest) {
			return nil, "", fmt.Errorf(
				"executable pattern invariant %s command does not select negative test %s",
				invariant.ID, check.NegativeTest)
		}
		return &executablePatternCheck{
			invariantID:  invariant.ID,
			command:      check.Command,
			negativeTest: check.NegativeTest,
		}, "executable", nil
	default:
		return nil, "", fmt.Errorf("pattern invariant %s has unknown check kind %q", invariant.ID, check.Kind)
	}
}

func runPatternInvariantCommand(root, command, negativeTest string) error {
	var transcript bytes.Buffer
	stream := io.MultiWriter(os.Stdout, &transcript)
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = root
	cmd.Stdout = stream
	cmd.Stderr = stream
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("check command failed: %w", err)
	}
	runMarker := "=== RUN   " + negativeTest
	passMarker := "--- PASS: " + negativeTest
	if !strings.Contains(transcript.String(), runMarker) ||
		!strings.Contains(transcript.String(), passMarker) {
		return fmt.Errorf("negative test %s did not run and pass", negativeTest)
	}
	return nil
}

// warmAgentBuild compiles the agent-core production binary once so that every
// module audit which rebuilds it (buildAgent) links from a warm Go build cache
// rather than racing to compile the same packages cold under parallel dispatch.
// The compiled artifact is discarded; only the cache it primes matters. A
// failure here is a genuine agent-core breakage every agent-building module
// would hit, so it fails fast with one clear error instead of many concurrent
// identical ones.
func warmAgentBuild() error {
	tmp, err := os.MkdirTemp("", "audit-warm-agent-*")
	if err != nil {
		return fmt.Errorf("warm agent build: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	out := filepath.Join(tmp, "agent")
	fmt.Println("warming agent build cache...")
	cmd := exec.Command("go", "build", "-tags", "production", "-o", out, "./cmd/agent")
	cmd.Dir = agentCoreModule
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("warm agent build: %w", err)
	}
	return nil
}

// auditSubModules dispatches mage audit to every runnable module concurrently,
// bounded by auditConcurrency. Module audits are independent processes, so the
// only shared state is the first-error latch. The skip decision runs serially
// first so its messages and stat errors stay deterministic and ordered.
func auditSubModules(modules []string, stat statFunc, run auditRunner) error {
	return auditSubModulesLimited(modules, auditConcurrency(), stat, run)
}

// maxConcurrentModuleAudits is intentionally lower than NumCPU. Each module
// audit runs multi-core Go builds, tests, profile boot smokes, and evidence
// agents; fanning every module out at once starves process-spawning tests whose
// own deadlines are meaningful (#1587).
const maxConcurrentModuleAudits = 2

// auditConcurrency bounds how many module audits run at once.
func auditConcurrency() int {
	n := runtime.NumCPU()
	if n < 1 {
		return 1
	}
	if n < maxConcurrentModuleAudits {
		return n
	}
	return maxConcurrentModuleAudits
}

func auditSubModulesLimited(modules []string, limit int, stat statFunc, run auditRunner) error {
	if limit < 1 {
		limit = 1
	}
	runnable, err := runnableAuditModules(modules, stat)
	if err != nil {
		return err
	}

	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	for _, mod := range runnable {
		wg.Add(1)
		sem <- struct{}{}
		go func(mod string) {
			defer wg.Done()
			defer func() { <-sem }()
			fmt.Printf("=== %s audit ===\n", mod)
			if err := run(mod); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("audit in %s: %w", mod, err)
				}
				mu.Unlock()
			}
		}(mod)
	}
	wg.Wait()
	return firstErr
}

// runnableAuditModules filters modules to those exposing a mage entrypoint,
// preserving the original serial skip semantics: a missing magefiles/ falls
// back to magefile.go, and only IsNotExist for both is a skip. Any other stat
// error aborts before dispatch.
func runnableAuditModules(modules []string, stat statFunc) ([]string, error) {
	var runnable []string
	for _, mod := range modules {
		mageDir := filepath.Join(mod, "magefiles")
		if _, err := stat(mageDir); err != nil {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("stat %s: %w", mageDir, err)
			}
			mageFile := filepath.Join(mod, "magefile.go")
			if _, fileErr := stat(mageFile); fileErr != nil {
				if os.IsNotExist(fileErr) {
					fmt.Printf("skipping %s (no magefiles/ or magefile.go)\n", mod)
					continue
				}
				return nil, fmt.Errorf("stat %s: %w", mageFile, fileErr)
			}
		}
		runnable = append(runnable, mod)
	}
	return runnable, nil
}

func runMageAudit(dir string) error {
	cmd := exec.Command("mage", "audit")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
