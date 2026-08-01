// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

const (
	codingHelmRelease        = "smoke"
	codingHelmNamespace      = "coding-agent-smoke"
	codingHelmCluster        = "da-coding-agent-smoke"
	codingHelmAgentImageRepo = "declarative-agents/coding-agent-smoke"
	codingHelmModelImageRepo = "declarative-agents/coding-model-smoke"
	codingHelmGoBaseImage = "golang:1.26-alpine"
	// codingHelmCollectorImage is built locally from the agent-core checkout
	// (kindrig.BuildAgentCoreImage); the published ghcr.io chart default is
	// not pullable from every environment.
	codingHelmCollectorImage = kindrig.DefaultAgentCoreImage
	codingHelmTraceID     = "0af7651916cd43dd8448eb211c80319c"
	codingHelmTraceparent = "00-" + codingHelmTraceID + "-b7ad6b7169203331-01"

	codingHelmClusterTimeout = 3 * time.Minute
	codingHelmInstallTimeout = 5 * time.Minute
	codingHelmReadyTimeout   = 2 * time.Minute
	codingHelmRequestTimeout = 2 * time.Minute
	codingHelmTraceTimeout   = 45 * time.Second
	codingHelmProbeTimeout   = 5 * time.Second
	codingHelmDiagTimeout    = 15 * time.Second
)

type codingHelmImages struct {
	Revision string
	Agent    string
	Model    string
}

func resolveCodingHelmImages(applicationRoot string) (codingHelmImages, error) {
	repositoryRoot := filepath.Clean(filepath.Join(applicationRoot, "..", ".."))
	commit, err := gitOutput(repositoryRoot, "rev-parse", "HEAD")
	if err != nil {
		return codingHelmImages{}, fmt.Errorf("resolve integration revision: %w", err)
	}
	agentImage, revision, err := kindrig.CommitImage(codingHelmAgentImageRepo, commit)
	if err != nil {
		return codingHelmImages{}, err
	}
	modelImage, _, err := kindrig.CommitImage(codingHelmModelImageRepo, commit)
	if err != nil {
		return codingHelmImages{}, err
	}
	return codingHelmImages{Revision: revision, Agent: agentImage, Model: modelImage}, nil
}

type codingSmokeRunner func(context.Context, string, ...string) ([]byte, error)

type codingSmokeEnvironment struct {
	kubeconfig string
}

func (environment codingSmokeEnvironment) run(
	ctx context.Context, name string, args ...string,
) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Env = os.Environ()
	if environment.kubeconfig != "" {
		command.Env = append(command.Env, "KUBECONFIG="+environment.kubeconfig)
	}
	return command.CombinedOutput()
}

type codingHelmInfrastructureError struct {
	Step        string
	Cause       error
	Output      string
	Diagnostics string
}

func (failure *codingHelmInfrastructureError) Error() string {
	message := fmt.Sprintf("helmSmoke infrastructure unavailable at %s: %v", failure.Step, failure.Cause)
	if output := strings.TrimSpace(failure.Output); output != "" {
		message += "\nprobe output:\n" + output
	}
	if failure.Diagnostics != "" {
		message += "\n" + failure.Diagnostics
	}
	return message
}

func (failure *codingHelmInfrastructureError) Unwrap() error { return failure.Cause }

type codingHelmSemanticError struct {
	Step        string
	Cause       error
	Diagnostics string
}

func (failure *codingHelmSemanticError) Error() string {
	message := fmt.Sprintf("helmSmoke %s semantic failure: %v", failure.Step, failure.Cause)
	if failure.Diagnostics != "" {
		message += "\n" + failure.Diagnostics
	}
	return message
}

func (failure *codingHelmSemanticError) Unwrap() error { return failure.Cause }

// HelmSmoke installs the packaged chart into a disposable kind cluster and
// proves the real planner -> executor -> critic path, shared workspace mutation,
// truthful readiness, and one connected trace. Only missing host prerequisites
// skip; failures after cluster acquisition are classified and returned.
func (Integration) HelmSmoke() error {
	roots, err := resolveIntegrationRoots()
	if err != nil {
		return err
	}
	if reason := codingHelmSmokeSkipReason(roots, codingSmokeEnvironment{}.run); reason != "" {
		fmt.Printf("SKIP helmSmoke: %s\n", reason)
		return nil
	}
	return runCodingHelmSmoke(roots)
}

func codingHelmSmokeSkipReason(roots integrationRoots, run codingSmokeRunner) string {
	for _, binary := range []string{"docker", "kind", "helm", "kubectl"} {
		if _, err := exec.LookPath(binary); err != nil {
			return binary + " not found on PATH"
		}
	}
	if reason := baseIntegrationSkipReason(roots); reason != "" {
		return reason
	}
	ctx, cancel := context.WithTimeout(context.Background(), codingHelmProbeTimeout)
	defer cancel()
	if output, err := run(ctx, "docker", "info"); err != nil {
		return fmt.Sprintf("Docker unavailable: %v: %s", err, strings.TrimSpace(string(output)))
	}
	for _, image := range []string{
		codingHelmGoBaseImage,
	} {
		ctx, cancel := context.WithTimeout(context.Background(), codingHelmProbeTimeout)
		output, err := run(ctx, "docker", "image", "inspect", image)
		cancel()
		if err != nil {
			return fmt.Sprintf("required local image %s unavailable (run docker pull %s): %v: %s",
				image, image, err, strings.TrimSpace(string(output)))
		}
	}
	return ""
}

func runCodingHelmSmoke(roots integrationRoots) (result error) {
	images, err := resolveCodingHelmImages(roots.Application)
	if err != nil {
		return err
	}
	kindConfig := filepath.Join(roots.Application, "helm", "ci", "kind-config.yaml")
	kindRun := func(args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), codingHelmClusterTimeout)
		defer cancel()
		return codingSmokeEnvironment{}.run(ctx, "kind", args...)
	}
	cluster, err := kindrig.EnsureCluster(
		kindRun, codingHelmCluster, kindConfig, 120*time.Second)
	if err != nil {
		return &codingHelmInfrastructureError{Step: "kind cluster acquisition", Cause: err}
	}
	kubeconfig, cleanupKubeconfig, err := codingKindKubeconfig(codingHelmCluster)
	if err != nil {
		cluster.ReleaseAfter(kindRun, true, kindrig.FailureEvidence{
			Directory: codingHelmEvidenceDir(roots.Application, images.Revision),
		})
		return &codingHelmInfrastructureError{Step: "kind kubeconfig", Cause: err}
	}
	defer cleanupKubeconfig()
	environment := codingSmokeEnvironment{kubeconfig: kubeconfig}
	defer func() {
		cleanupCodingHelmSmoke(
			environment, cluster, kindRun, result != nil,
			codingHelmEvidenceDir(roots.Application, images.Revision))
	}()

	if err := checkCodingHelmInfrastructure(environment.run); err != nil {
		return err
	}
	if err := prepareCodingHelmCluster(environment, cluster.Name, roots, images); err != nil {
		return classifyCodingHelmFailure(environment.run, "cluster preparation", err)
	}
	archiveDir, err := os.MkdirTemp("", "coding-agent-smoke-chart-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(archiveDir)
	if err := Package(); err != nil {
		return &codingHelmSemanticError{Step: "profile package", Cause: err}
	}
	archive, err := packageHelmChart(
		filepath.Join(roots.Application, "helm"),
		filepath.Join(roots.Application, filepath.FromSlash(defaultProfileOutput)),
		archiveDir,
	)
	if err != nil {
		return &codingHelmSemanticError{Step: "chart package", Cause: err}
	}
	if err := installCodingHelmChart(
		environment, archive, roots.Application, images.Agent); err != nil {
		return classifyCodingHelmFailure(environment.run, "Helm install", err)
	}
	if err := verifyCodingHelmRollouts(environment); err != nil {
		return classifyCodingHelmFailure(environment.run, "role readiness", err)
	}
	if err := seedCodingWorkspace(environment, roots.Application); err != nil {
		return classifyCodingHelmFailure(environment.run, "workspace seed", err)
	}
	forwards, err := startCodingHelmForwards(environment, false)
	if err != nil {
		return classifyCodingHelmFailure(environment.run, "port-forward", err)
	}
	defer forwards.stop()
	if err := verifyCodingHealthEndpoints(forwards.queryURL); err != nil {
		return classifyCodingHelmFailure(environment.run, "lifecycle health", err)
	}
	if err := submitCodingHelmRequest(); err != nil {
		return classifyCodingHelmFailure(environment.run, "planner request", err)
	}
	if err := verifyCodingWorkspaceAndVerdict(environment); err != nil {
		return classifyCodingHelmFailure(environment.run, "workspace and critic result", err)
	}
	if err := verifyCodingTrace(forwards.queryURL); err != nil {
		return classifyCodingHelmFailure(environment.run, "connected trace", err)
	}
	fmt.Printf("integration:helmSmoke PASS - revision %s packaged chart ran planner -> executor -> critic with shared workspace and trace %s\n",
		images.Revision, codingHelmTraceID)
	return nil
}

func codingKindKubeconfig(cluster string) (string, func(), error) {
	ctx, cancel := context.WithTimeout(context.Background(), codingHelmProbeTimeout)
	defer cancel()
	output, err := codingSmokeEnvironment{}.run(
		ctx, "kind", "get", "kubeconfig", "--name", cluster)
	if err != nil {
		return "", nil, fmt.Errorf("kind get kubeconfig: %w: %s", err, strings.TrimSpace(string(output)))
	}
	dir, err := os.MkdirTemp("", "coding-agent-kubeconfig-*")
	if err != nil {
		return "", nil, err
	}
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, output, 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return "", nil, err
	}
	return path, func() { _ = os.RemoveAll(dir) }, nil
}

func checkCodingHelmInfrastructure(run codingSmokeRunner) error {
	probes := []struct {
		step, name string
		args       []string
		want       string
	}{
		{"Docker engine", "docker", []string{"info"}, ""},
		{"host-to-Kubernetes API", "kubectl",
			[]string{"--request-timeout=5s", "get", "--raw=/readyz"}, "ok"},
	}
	for _, probe := range probes {
		ctx, cancel := context.WithTimeout(context.Background(), codingHelmProbeTimeout)
		output, err := run(ctx, probe.name, probe.args...)
		contextErr := ctx.Err()
		cancel()
		if contextErr != nil {
			err = contextErr
		}
		if err == nil && (probe.want == "" || strings.TrimSpace(string(output)) == probe.want) {
			continue
		}
		if err == nil {
			err = fmt.Errorf("probe returned %q, want %q", strings.TrimSpace(string(output)), probe.want)
		}
		return &codingHelmInfrastructureError{
			Step: probe.step, Cause: err, Output: string(output),
		}
	}
	return nil
}

// classifyCodingHelmFailure separates a cluster/API outage from a semantic
// defect and attaches bounded diagnostics. includeApplier adds the applier's
// logs to that bundle for the live tier; the smoke install has no applier, so it
// passes nothing and the bundle is unchanged.
func classifyCodingHelmFailure(
	run codingSmokeRunner, step string, cause error, includeApplier ...bool,
) error {
	diagnostics := collectCodingHelmDiagnostics(run, codingHelmDiagTimeout, includeApplier...)
	var infrastructure *codingHelmInfrastructureError
	if errors.As(cause, &infrastructure) {
		infrastructure.Diagnostics = diagnostics
		return infrastructure
	}
	if err := checkCodingHelmInfrastructure(run); errors.As(err, &infrastructure) {
		infrastructure.Diagnostics = diagnostics
		return infrastructure
	}
	return &codingHelmSemanticError{Step: step, Cause: cause, Diagnostics: diagnostics}
}

func collectCodingHelmDiagnostics(run codingSmokeRunner, timeout time.Duration, includeApplier ...bool) string {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	commands := []struct {
		label, name string
		args        []string
	}{
		{"helm status", "helm", []string{"status", codingHelmRelease, "-n", codingHelmNamespace}},
		{"helm history", "helm", []string{"history", codingHelmRelease, "-n", codingHelmNamespace}},
		{"workloads", "kubectl", []string{"get", "deploy,statefulset,job,pods,pvc", "-n", codingHelmNamespace, "-o", "wide"}},
		{"service endpoints", "kubectl", []string{"get", "service,endpoints,endpointslice", "-n", codingHelmNamespace, "-o", "wide"}},
		{"events", "kubectl", []string{"get", "events", "-n", codingHelmNamespace, "--sort-by=.metadata.creationTimestamp"}},
		{"planner logs", "kubectl", []string{"logs", "-n", codingHelmNamespace, "-l", "app.kubernetes.io/component=planner", "--tail=80"}},
		{"executor logs", "kubectl", []string{"logs", "-n", codingHelmNamespace, "-l", "app.kubernetes.io/component=executor", "--tail=80"}},
		{"critic logs", "kubectl", []string{"logs", "-n", codingHelmNamespace, "-l", "app.kubernetes.io/component=critic", "--tail=80"}},
		{"collector logs", "kubectl", []string{"logs", "-n", codingHelmNamespace, "-l", "app.kubernetes.io/component=collector", "--tail=80"}},
		{"model logs", "kubectl", []string{"logs", "-n", codingHelmNamespace, "-l", "app=coding-model", "--tail=80"}},
	}
	// The live applier tier enables the applier; the smoke install does not, so
	// its logs join the bundle only when the caller asks.
	if len(includeApplier) > 0 && includeApplier[0] {
		commands = append(commands, struct {
			label, name string
			args        []string
		}{"applier logs", "kubectl", []string{"logs", "-n", codingHelmNamespace, "-l", "app.kubernetes.io/component=applier", "--tail=80"}})
	}
	var report strings.Builder
	report.WriteString("helmSmoke bounded diagnostics:")
	for _, command := range commands {
		output, err := run(ctx, command.name, command.args...)
		fmt.Fprintf(&report, "\n\n== %s ==\n%s", command.label, strings.TrimSpace(string(output)))
		if err != nil {
			fmt.Fprintf(&report, "\n[diagnostic failed: %v]", err)
		}
	}
	return report.String()
}
