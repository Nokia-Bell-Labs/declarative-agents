// Copyright (c) 2026 Nokia. All rights reserved.

// The chatbot-mesh application self-governs its specification corpus with the
// catalog specification critic, driven by the agent-core runtime it depends on as a platform.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/catalog/catalogroot"
	"gopkg.in/yaml.v3"
)

const (
	demoConfigFile   = "demo.yaml"
	agentCoreDirName = "agent-core"

	specificationCriticProfileRel = "agents/specification-critic/profile.yaml"
)

// demoConfig carries the optional, declarative overrides the chatbot-mesh
// magefiles read from demo.yaml. Every field is optional: an absent file or an
// unset field falls back to the monorepo default. Overriding a value means
// editing this declaration, never an environment variable (GH-1251).
type demoConfig struct {
	CatalogRoot       string `yaml:"catalog_root"`
	CoreRoot          string `yaml:"core_root"`
	HelmDist          string `yaml:"helm_dist"`
	SpecCriticProfile string `yaml:"spec_critic_profile"`

	// Collector ingress endpoints: where the observability targets bind the
	// collector and where the integration targets dial it.
	OTELGRPCPort         string `yaml:"otel_grpc_port"`
	CollectorControlPort string `yaml:"collector_control_port"`
	CollectorMonitorPort string `yaml:"collector_monitor_port"`
	CollectorQueryPort   string `yaml:"collector_query_port"`
	CollectorBindHost    string `yaml:"collector_bind_host"`

	// InferenceTimeout bounds one model call in the integration tracers, as a
	// Go duration string ("45s", "3m"). Empty means the shipped default.
	InferenceTimeout string `yaml:"inference_timeout"`

	// IntegrationOTLPEndpoint is where integration launches export live
	// telemetry when the target does not choose an endpoint itself. Empty
	// keeps those launches file-only.
	IntegrationOTLPEndpoint string `yaml:"integration_otlp_endpoint"`
}

// loadDemoConfig reads demo.yaml from the application root. A missing file is the
// zero-configuration path and yields an empty config, not an error.
func loadDemoConfig(applicationRoot string) (demoConfig, error) {
	var config demoConfig
	data, err := os.ReadFile(filepath.Join(applicationRoot, demoConfigFile))
	if err != nil {
		if os.IsNotExist(err) {
			return config, nil
		}
		return config, fmt.Errorf("read %s: %w", demoConfigFile, err)
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return config, fmt.Errorf("parse %s: %w", demoConfigFile, err)
	}
	return config, nil
}

// loadDemoConfigOrEmpty returns the parsed demo.yaml, or an empty config when the
// file is absent or unreadable. It suits the resolver helpers below, which fall
// back to the monorepo default rather than threading an error through every
// magefile call site.
func loadDemoConfigOrEmpty(applicationRoot string) demoConfig {
	config, _ := loadDemoConfig(applicationRoot)
	return config
}

// demoCoreRoot resolves the agent-core checkout: demo.yaml core_root when set
// (relative values anchored at the application root), otherwise the monorepo
// sibling two levels above the application root.
func demoCoreRoot(applicationRoot string) string {
	if coreRoot := loadDemoConfigOrEmpty(applicationRoot).CoreRoot; coreRoot != "" {
		if filepath.IsAbs(coreRoot) {
			return filepath.Clean(coreRoot)
		}
		return filepath.Join(applicationRoot, coreRoot)
	}
	return siblingPath(applicationRoot, agentCoreDirName)
}

// demoHelmDist resolves the chart package output directory: demo.yaml helm_dist
// when set (relative values anchored at the application root), otherwise
// helm/dist under the application root.
func demoHelmDist(applicationRoot string) string {
	if helmDist := loadDemoConfigOrEmpty(applicationRoot).HelmDist; helmDist != "" {
		if filepath.IsAbs(helmDist) {
			return filepath.Clean(helmDist)
		}
		return filepath.Join(applicationRoot, helmDist)
	}
	return filepath.Join(applicationRoot, "helm", "dist")
}

// demoSpecCriticProfile resolves the specification-critic validator profile:
// demo.yaml spec_critic_profile when set (relative values anchored at the
// application root), otherwise the canonical profile under the catalog root.
func demoSpecCriticProfile(applicationRoot, catalogRoot string) string {
	if profile := loadDemoConfigOrEmpty(applicationRoot).SpecCriticProfile; profile != "" {
		if filepath.IsAbs(profile) {
			return filepath.Clean(profile)
		}
		return filepath.Join(applicationRoot, profile)
	}
	return filepath.Join(catalogRoot, filepath.FromSlash(specificationCriticProfileRel))
}

// Audit runs the specification critic over this application's specification corpus, so the application
// self-governs: load_corpus reads docs/SPECIFICATIONS.yaml, docs/specs, and
// agents; validate_specs runs the corpus consistency checks; a single error
// finding (a broken index path, touchpoint, or citation) fails the target. The
// outcome is read from the specification critic's report rather than its exit code: a failing
// corpus makes the specification critic reach a failed terminal, which exits 2 (agent-core
// srd018 R6.1/R6.2, GH-683), and agentRunCompleted accepts that as a completed
// run because the report -- not the exit status -- names which checks failed.
// Audit is the self-governance gate: it fails clearly when the agent-core
// runtime or the specification-critic validator profile is not reachable, rather than
// skipping, so a copied-out application without the platform tools reports an
// honest failure instead of a false green.
//
// The gate has four steps, each answering a question the one before it cannot:
// the specification critic validates the corpus, the boot smoke proves every profile starts,
// the evidence resolver proves each named test exists, and the evidence run
// proves the tests a suite claims as implemented actually pass.
func Audit() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	catalogRoot, err := resolveCatalogRoot("chatbot-mesh audit", root)
	if err != nil {
		return err
	}
	if _, err := resolveChatbotComposition(root, catalogRoot); err != nil {
		return fmt.Errorf("audit application composition: %w", err)
	}
	coreRoot, specificationCriticProfile, err := resolveAuditTools(root, catalogRoot)
	if err != nil {
		return err
	}
	binary, err := buildAgent(coreRoot)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(binary) }()
	if err := runSpecificationCritic(binary, specificationCriticProfile, root, coreRoot); err != nil {
		return err
	}
	// The specification critic validates the specification corpus, not whether an agent can
	// start, so preflight every mesh profile through the runtime's own load path.
	profiles, err := meshProfiles(root)
	if err != nil {
		return err
	}
	corpusRuntime, cleanupCorpusRuntime, err := stageCorpusIngestRuntime(root)
	if err != nil {
		return err
	}
	defer cleanupCorpusRuntime()
	sourceCorpusProfile := filepath.Join(root, filepath.FromSlash(chromaIngestProfile))
	for index, profile := range profiles {
		if filepath.Clean(profile) == sourceCorpusProfile {
			profiles[index] = filepath.Join(
				corpusRuntime, filepath.FromSlash(chromaIngestProfile))
		}
	}
	if err := bootSmokeProfiles(defaultSmokeRun, binary, coreRoot, profiles); err != nil {
		return err
	}
	evidenceProfile := filepath.Join(filepath.Dir(specificationCriticProfile), "audit-profile.yaml")
	if _, err := os.Stat(evidenceProfile); err != nil {
		return fmt.Errorf("audit: specification-critic test-evidence profile not found at %s: %w", evidenceProfile, err)
	}
	return runSpecificationCritic(binary, evidenceProfile, root, coreRoot)
}

// resolveAuditTools locates the agent-core runtime checkout and the specification-critic
// validator profile the self-governance audit requires. Both are mandatory: the
// gate cannot validate the corpus without the runtime that executes the specification critic
// or the validator profile itself, so a missing tool is a clear failure, not a
// skip. Skip behavior is reserved for explicitly optional integration targets.
func resolveAuditTools(root, catalogRoot string) (coreRoot, specificationCriticProfile string, err error) {
	coreRoot = demoCoreRoot(root)
	if !agentCoreAvailable(coreRoot) {
		return "", "", fmt.Errorf("audit: agent-core checkout not found at %s (set core_root in %s); the self-governance gate requires the agent-core runtime", coreRoot, demoConfigFile)
	}
	specificationCriticProfile = demoSpecCriticProfile(root, catalogRoot)
	if _, statErr := os.Stat(specificationCriticProfile); statErr != nil {
		return "", "", fmt.Errorf("audit: specification-critic validator profile not found at %s (set spec_critic_profile in %s); the self-governance gate requires its validator", specificationCriticProfile, demoConfigFile)
	}
	return coreRoot, specificationCriticProfile, nil
}

// resolveCatalogRoot resolves the catalog source root from the declared
// demo.yaml catalog_root against the application owner's startup directory, or
// by repository discovery. It never changes process CWD; child commands
// continue to run from the chatbot-mesh root.
func resolveCatalogRoot(owner, startupCWD string) (string, error) {
	startupCWD, err := filepath.Abs(filepath.Clean(startupCWD))
	if err != nil {
		return "", fmt.Errorf("%s: resolve startup working directory: %w", owner, err)
	}
	resolution, err := catalogroot.Resolve(
		owner,
		startupCWD,
		loadDemoConfigOrEmpty(startupCWD).CatalogRoot,
		catalogroot.DiscoveryCandidates(startupCWD)...,
	)
	if err != nil {
		return "", err
	}
	return resolution.Path, nil
}

func runSpecificationCritic(binary, specificationCriticProfile, root, coreRoot string) error {
	cmd := exec.Command(binary,
		"--profile", specificationCriticProfile,
		"--directory", root,
		"--core-root", coreRoot,
	)
	cmd.Dir = root
	var out bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &out)
	cmd.Stderr = io.MultiWriter(os.Stderr, &out)
	// A corpus with findings drives the specification critic to a failure terminal, which
	// exits non-zero; that is a completed run reporting failure, not a broken
	// invocation (srd018-cli-flag-contract R6). Only a run the binary could
	// not complete is an error here; the outcome itself comes from the report.
	if runErr := cmd.Run(); !agentRunCompleted(runErr) {
		return fmt.Errorf("specification-critic run failed: %w", runErr)
	}
	ok, err := specificationCriticSucceeded(out.String())
	switch {
	case err != nil:
		return fmt.Errorf("audit: %w; see the report above", err)
	case !ok:
		return fmt.Errorf("audit: the specification critic found errors in the application corpus at %s", filepath.Join(root, "docs", "specs"))
	default:
		fmt.Printf("audit: specification-critic profile %s completed with no errors\n", filepath.Base(specificationCriticProfile))
		return nil
	}
}

// specificationCriticSucceeded reads a clean/failing outcome from a specification-critic report. The
// outcome is taken from the terminal state line rather than the exit code:
// the exit code now distinguishes a failed terminal from a failed invocation
// (srd018 R6), but the report is what names which checks failed. A report with
// neither terminal marker is an indeterminate run and returns an error.
func specificationCriticSucceeded(report string) (bool, error) {
	switch {
	case strings.Contains(report, "terminal state: failed") || strings.Contains(report, "status=failed"):
		return false, nil
	case strings.Contains(report, "terminal state: succeeded"):
		return true, nil
	default:
		return false, fmt.Errorf("the specification critic did not reach a terminal state")
	}
}

// buildAgent builds the production agent binary from the agent-core checkout, the
// same binary the runtime image ships and the integration tests drive.
func buildAgent(coreRoot string) (string, error) {
	binary := filepath.Join(os.TempDir(), "chatbot-mesh-application-agent")
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
// applications/chatbot-mesh owner root.
func siblingPath(applicationRoot, rel string) string {
	return filepath.Clean(filepath.Join(applicationRoot, "..", "..", rel))
}

// Collector ingress defaults. OTLP gRPC and the query surface are shared with
// the agent-architecture demo's local collector, so the two cannot run at once
// unless one is repointed in demo.yaml.
const (
	otelGRPCPortDefault         = "4317"
	collectorControlPortDefault = "18191"
	collectorMonitorPortDefault = "18192"
	collectorQueryPortDefault   = "18193"
	collectorBindHostDefault    = "0.0.0.0"
)

// observabilitySettings names the collector ingress endpoints the magefiles
// use: the ports and bind host the observability targets hand the collector
// child process, and the loopback ports the integration targets dial.
type observabilitySettings struct {
	OTELGRPCPort string
	ControlPort  string
	MonitorPort  string
	QueryPort    string
	BindHost     string
}

// observabilitySettingsFrom resolves the collector ingress endpoints from a
// demo.yaml config, filling unset fields with the shipped defaults.
func observabilitySettingsFrom(config demoConfig) observabilitySettings {
	return observabilitySettings{
		OTELGRPCPort: firstNonEmpty(config.OTELGRPCPort, otelGRPCPortDefault),
		ControlPort:  firstNonEmpty(config.CollectorControlPort, collectorControlPortDefault),
		MonitorPort:  firstNonEmpty(config.CollectorMonitorPort, collectorMonitorPortDefault),
		QueryPort:    firstNonEmpty(config.CollectorQueryPort, collectorQueryPortDefault),
		BindHost:     firstNonEmpty(config.CollectorBindHost, collectorBindHostDefault),
	}
}

// demoObservability resolves the collector ingress endpoints from demo.yaml in
// the mage invocation directory, the application root every target runs from.
func demoObservability() observabilitySettings {
	root, err := os.Getwd()
	if err != nil {
		return observabilitySettingsFrom(demoConfig{})
	}
	return observabilitySettingsFrom(loadDemoConfigOrEmpty(root))
}

// demoIntegrationOTLPEndpoint resolves the live-export endpoint for
// integration launches whose target does not choose one: demo.yaml
// integration_otlp_endpoint, or empty, which keeps the launch file-only.
func demoIntegrationOTLPEndpoint() string {
	root, err := os.Getwd()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(loadDemoConfigOrEmpty(root).IntegrationOTLPEndpoint)
}

// exitMachineFailed is the agent's exit code for a run whose machine reached a
// failure terminal, as distinct from 1, which means the binary could not
// complete a run at all (srd018-cli-flag-contract R6).
const exitMachineFailed = 2

// agentRunCompleted reports whether an agent invocation completed a run,
// including one whose machine reached a failure terminal.
func agentRunCompleted(runErr error) bool {
	if runErr == nil {
		return true
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return exitErr.ExitCode() == exitMachineFailed
	}
	return false
}
