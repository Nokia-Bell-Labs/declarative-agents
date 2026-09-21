// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package kindrig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// The deploy words are written in helm 3's spellings. helm 4 still accepts all
// of them: it deprecates --atomic in favour of --rollback-on-failure and
// prefers --dry-run=client over a bare --dry-run, and warns about both rather
// than refusing them. A bare --dry-run also still simulates under helm 4,
// which is the part that matters, since helm 4 redefines --dry-run as a string
// whose default is "none".
//
// So the local major is worth a word to the operator and is not worth gating
// on. A later helm will drop the deprecated spellings, and the warning is what
// makes that arrive as a sentence rather than as six failing words.
const declaredHelmMajor = "3"

var helmVersionPattern = regexp.MustCompile(`v(\d+)\.\d+`)

// DeployAgent locates the agent a deploy target runs, mirroring DiagnoseAgent.
type DeployAgent struct {
	Binary   string
	Profile  string
	CoreRoot string
	// Cleanup releases whatever holds the binary, once the run is over.
	Cleanup func()
}

// DeployRequest is one application's deploy or undeploy.
type DeployRequest struct {
	// Cluster is the running cluster this release goes to. It is confirmed
	// before any Helm word runs.
	Cluster string
	// ApplicationRoot is where build/deploy/<release> is rendered.
	ApplicationRoot string
	// Coordinates are this application's resolved deploy coordinates. The
	// caller leaves Kubeconfig and OverridesPath empty; Deploy fills both from
	// the cluster and the workspace it owns.
	Coordinates DeployCoordinates
	// Overrides is the decided values document, seeded through --request and
	// written to the workspace by write_overrides.
	Overrides string
	// Agent runs the machine. A zero value is an error: unlike a diagnosis, a
	// deploy that does not run has produced nothing.
	Agent DeployAgent
	// CatalogRoot is the shipped catalog this run binds its machine from.
	CatalogRoot string
	// HelmVersion overrides the local helm version probe. Tests set it.
	HelmVersion func() (string, error)
	// KubeconfigPath, when set, names a kubeconfig file the deploy uses
	// instead of asking kind for the cluster's. It is how the GCP rig
	// (eng08) points this same machine at GKE: the machine, the words, and
	// the coordinates are unchanged, only where the credential comes from.
	KubeconfigPath string
	// ProbeCluster overrides the reachability probe used with
	// KubeconfigPath. Tests set it; the default asks the API server for
	// /readyz through the provided kubeconfig.
	ProbeCluster func(kubeconfig string) error
}

// Deploy installs or upgrades one release by running the catalog applier's
// deploy machine host-side (srd022 R6).
//
// Unlike Diagnose, which reports and never gates, this gates: a failure
// terminal fails the caller's build, because a deploy that did not come up is
// not a result anyone should build on.
func Deploy(request DeployRequest) error {
	return runDeployMachine(request, "deploy", "Deployed")
}

// Undeploy removes one release. An already-absent release reaches Absent,
// which is a success terminal, so a teardown of nothing exits zero.
func Undeploy(request DeployRequest) error {
	return runDeployMachine(request, "undeploy", "Removed", "Absent")
}

// runDeployMachine preflights the cluster and the local helm, renders this
// application's coordinates, and runs the named machine.
func runDeployMachine(request DeployRequest, verb string, succeeded ...string) error {
	if strings.TrimSpace(request.Cluster) == "" {
		return fmt.Errorf("%s: no cluster named", verb)
	}
	// The undeploy machine reads a failing helm_history as an absent release.
	// That is only safe once an unreachable cluster has been ruled out, so this
	// check is part of the machine's correctness contract rather than a
	// convenience (srd022 R6.5). A caller-provided kubeconfig names a cluster
	// kind does not know, so its probe asks that cluster's API server instead.
	if request.KubeconfigPath == "" {
		if !Exists(CaptureRun, request.Cluster) {
			return fmt.Errorf("%s: cluster %s is not running", verb, request.Cluster)
		}
	} else {
		probe := request.ProbeCluster
		if probe == nil {
			probe = probeClusterReadyz
		}
		if err := probe(request.KubeconfigPath); err != nil {
			return fmt.Errorf("%s: cluster behind %s is not reachable: %w",
				verb, request.KubeconfigPath, err)
		}
	}
	// A version mismatch is reported and does not stop the run; only a probe
	// that could not execute at all aborts, since that means helm is unusable.
	if err := warnOnHelmMajor(request.HelmVersion); err != nil {
		return fmt.Errorf("%s: %w", verb, err)
	}
	if request.Agent.Binary == "" || request.Agent.Profile == "" {
		return fmt.Errorf("%s: no agent resolved; a deploy that does not run has produced nothing", verb)
	}
	if request.Agent.Cleanup != nil {
		defer request.Agent.Cleanup()
	}

	destination := DeployRenderDirectory(request.ApplicationRoot, request.Coordinates.Release)
	workspace := DeployWorkspace(request.ApplicationRoot, request.Coordinates.Release)
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return fmt.Errorf("%s: create workspace %s: %w", verb, workspace, err)
	}
	kubeconfig, err := stageDeployKubeconfig(request, destination)
	if err != nil {
		return fmt.Errorf("%s: %w", verb, err)
	}
	coordinates := request.Coordinates
	coordinates.Kubeconfig = kubeconfig
	coordinates.OverridesPath = filepath.Join(workspace, overridesFileName)
	declarations, err := RenderDeployDeclarations(
		filepath.Join(request.CatalogRoot, "agents", "applier", "deploy-declarations.yaml"),
		coordinates, destination)
	if err != nil {
		return fmt.Errorf("%s: %w", verb, err)
	}
	profile, err := RenderDeployProfile(RenderedProfile{
		Name:                 "applier-" + verb + "-request",
		Release:              coordinates.Release,
		Namespace:            coordinates.Namespace,
		MachinePath:          filepath.Join(request.CatalogRoot, "agents", "applier", verb+"-machine.yaml"),
		ToolsPath:            filepath.Join(request.CatalogRoot, "agents", "applier", verb+"-tools.yaml"),
		ApplyDeclarations:    filepath.Join(request.CatalogRoot, "agents", "applier", "apply-declarations.yaml"),
		RenderedDeclarations: declarations,
	}, destination)
	if err != nil {
		return fmt.Errorf("%s: %w", verb, err)
	}

	seed, err := writeDeployRequest(destination, request.Overrides)
	if err != nil {
		return fmt.Errorf("%s: %w", verb, err)
	}
	fmt.Printf("%s: rendered %s\n", verb, destination)
	return request.Agent.run(verb, profile, workspace, seed, succeeded)
}

// stageDeployKubeconfig stages the caller's kubeconfig when one is named and
// the kind cluster's otherwise, so both rigs share one render layout.
func stageDeployKubeconfig(request DeployRequest, destination string) (string, error) {
	if request.KubeconfigPath == "" {
		return stageKubeconfig(request.Cluster, destination)
	}
	data, err := os.ReadFile(request.KubeconfigPath)
	if err != nil {
		return "", fmt.Errorf("read kubeconfig %s: %w", request.KubeconfigPath, err)
	}
	staged := filepath.Join(destination, "kubeconfig")
	if err := os.WriteFile(staged, data, 0o600); err != nil {
		return "", fmt.Errorf("stage kubeconfig %s: %w", staged, err)
	}
	return staged, nil
}

// probeClusterReadyz asks the API server behind a kubeconfig for /readyz.
func probeClusterReadyz(kubeconfig string) error {
	out, err := exec.Command("kubectl", "--kubeconfig", kubeconfig,
		"get", "--raw", "/readyz").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// stageKubeconfig copies the cluster's kubeconfig into the render directory
// and returns that path.
//
// kindrig.Kubeconfig writes to a temporary directory it deletes when the run
// returns, and the rendered argv names whatever path it was given. Pointing the
// words at the temporary copy left every rendered command in
// build/deploy/<release> referring to a file that no longer existed, so the
// one thing the render directory exists for -- reading and re-running the exact
// command that ran -- failed with an unrelated "cluster unreachable". Staging
// it beside the declarations keeps the rendered argv reproducible after the run.
//
// The file carries cluster credentials, so it is written 0600, and every
// application's build tree is gitignored.
func stageKubeconfig(cluster, destination string) (string, error) {
	source, release, err := Kubeconfig(CaptureRun, cluster)
	if err != nil {
		return "", err
	}
	defer release()
	data, err := os.ReadFile(source)
	if err != nil {
		return "", fmt.Errorf("read kubeconfig for %s: %w", cluster, err)
	}
	staged := filepath.Join(destination, "kubeconfig")
	if err := os.WriteFile(staged, data, 0o600); err != nil {
		return "", fmt.Errorf("stage kubeconfig %s: %w", staged, err)
	}
	return staged, nil
}

// writeDeployRequest writes the seed the machine starts from. write_overrides
// reads path and content out of the parameters object by name.
func writeDeployRequest(destination, overrides string) (string, error) {
	body, err := json.Marshal(map[string]any{
		"parameters": map[string]string{
			"path":    overridesFileName,
			"content": overrides,
		},
	})
	if err != nil {
		return "", fmt.Errorf("encode deploy request: %w", err)
	}
	path := filepath.Join(destination, "request.json")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return "", fmt.Errorf("write deploy request %s: %w", path, err)
	}
	return path, nil
}

// run executes the agent and gates on its exit status. The agent exits non-zero
// on a failure terminal, and for a deploy that is the build's answer.
func (a DeployAgent) run(verb, profile, workspace, request string, succeeded []string) error {
	cmd := exec.Command(a.Binary,
		"--profile", profile,
		"--directory", workspace,
		"--request", request,
		"--core-root", a.CoreRoot)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	runErr := cmd.Run()
	if runErr == nil {
		fmt.Printf("%s: reached one of %s\n", verb, strings.Join(succeeded, ", "))
		return nil
	}
	var exitErr *exec.ExitError
	if !errors.As(runErr, &exitErr) {
		return fmt.Errorf("run %s agent: %w", verb, runErr)
	}
	return fmt.Errorf(
		"%s: the machine reached a failure terminal; read the run above and the rendered argv in %s",
		verb, filepath.Dir(profile))
}

// warnOnHelmMajor tells the operator when the local helm is not the major the
// deploy words were written for. It does not gate: every flag still parses, so
// refusing to run would block a deploy that works.
//
// A probe that cannot run at all is a different matter and is returned as an
// error, because it means helm is missing or unusable and every word is about
// to fail anyway. An unrecognizable version string is only a warning: helm
// answered, so it is there, and the words will run.
func warnOnHelmMajor(probe func() (string, error)) error {
	warning, err := helmMajorWarning(probe)
	if err != nil {
		return err
	}
	if warning != "" {
		fmt.Println(warning)
	}
	return nil
}

// helmMajorWarning is what warnOnHelmMajor has to say, as a value: empty when
// the local helm is the declared major. Returning the sentence rather than
// printing it is what makes it observable — a proof that read it off stdout
// had to swap the process-global os.Stdout, which two parallel tests then did
// at once and read each other's output (GH-2460).
func helmMajorWarning(probe func() (string, error)) (string, error) {
	if probe == nil {
		probe = localHelmVersion
	}
	version, err := probe()
	if err != nil {
		return "", err
	}
	match := helmVersionPattern.FindStringSubmatch(version)
	if len(match) < 2 {
		return fmt.Sprintf(
			"deploy: local helm version %q is not recognizable; the deploy words are written for helm %s",
			strings.TrimSpace(version), declaredHelmMajor), nil
	}
	if match[1] == declaredHelmMajor {
		return "", nil
	}
	return fmt.Sprintf(
		"deploy: local helm is major version %s and the deploy words are written for helm %s. "+
			"helm %s accepts them and warns: --atomic is deprecated for --rollback-on-failure, "+
			"and a bare --dry-run for --dry-run=client. A later helm will drop them, and the "+
			"words move together with the applier's pinned CLI donor, not on their own.",
		match[1], declaredHelmMajor, match[1]), nil
}

func localHelmVersion() (string, error) {
	out, err := exec.Command("helm", "version", "--short").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("run helm version: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}
