// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

const (
	// The live applier tier reuses the smoke release and namespace so the runtime
	// helpers apply unchanged; it stands its own dedicated cluster apart from the
	// smoke one so a cluster failure here reads as a cluster failure, not a smoke
	// regression.
	applierLiveCluster   = "da-agent-architecture-applier"
	applierLiveImageRepo = "declarative-agents/agent-architecture-applier"

	// The applier's exec declarations are written for helm 3, which applier.Dockerfile
	// pins (ARG HELM_VERSION). A helm-4 image would reject the --dry-run spelling
	// outright.
	applierDeclaredHelmMajor = "3"

	// The live apply legs run a real helm upgrade, a real 120s kubectl rollout verify,
	// and (on the rollback leg) a helm rollback, so the client waits well past the
	// fake-tracer's 130s bound.
	applierLiveRequestTimeout = 4 * time.Minute
	applierLiveReadyTimeout   = 3 * time.Minute
	applierLiveClusterTimeout = 3 * time.Minute
	applierLiveInstallTimeout = 5 * time.Minute
)

// ApplierLive proves the applier against a real cluster, which the fake-CLI tracer
// (integration:applier) cannot: that target drives recording stand-ins whose exit
// codes come from the scenario, so it is evidence about the machine and the arguments
// it builds, not about helm and kubectl behaving as the declarations assume
// (srd002-applier R5.3). This one builds the applier image, installs the chart with
// the applier enabled, and drives a values patch through the running applier so a
// real helm upgrade moves the release revision, a verify stall triggers a real helm
// rollback, and a non-conforming patch is rejected against the real chart schema.
//
// It is a separate target from integration:applier on purpose. That one runs anywhere
// in seconds; this one needs docker and kind, builds images, and stands up a cluster.
func (Integration) ApplierLive() error {
	resolved, err := resolveRootsFromWorkingDirectory()
	if err != nil {
		fmt.Printf("SKIP applierLive: %v\n", err)
		return nil
	}
	if reason := smokeSkipReason(resolved); reason != "" {
		fmt.Printf("SKIP applierLive: %s\n", reason)
		return nil
	}
	return runApplierLive(resolved)
}

func runApplierLive(resolved roots) (result error) {
	image, revision, err := kindrig.CommitImage(smokeImageRepo, mustGitRevision(resolved.Application))
	if err != nil {
		return err
	}
	applierImage, _, err := kindrig.CommitImage(applierLiveImageRepo, mustGitRevision(resolved.Application))
	if err != nil {
		return err
	}

	// One instrumented chart directory serves both the host-side install and the
	// applier image's /chart, so Helm records and rolls back one coherent chart.
	chartDir, cleanupChart, err := stageApplierLiveChart(resolved)
	if err != nil {
		return err
	}
	defer cleanupChart()

	kindConfig := filepath.Join(resolved.Application, "helm", "ci", "kind-config.yaml")
	kindRun := func(args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), applierLiveClusterTimeout)
		defer cancel()
		return smokeEnvironment{}.run(ctx, "kind", args...)
	}
	cluster, err := kindrig.EnsureCluster(kindRun, applierLiveCluster, kindConfig, 120*time.Second)
	if err != nil {
		return fmt.Errorf("applierLive kind cluster acquisition: %w", err)
	}
	kubeconfig, cleanupKubeconfig, err := smokeKubeconfig(applierLiveCluster)
	if err != nil {
		cluster.Release(kindRun)
		return fmt.Errorf("applierLive kubeconfig: %w", err)
	}
	defer cleanupKubeconfig()
	environment := smokeEnvironment{kubeconfig: kubeconfig}
	defer func() {
		cleanupHelmSmoke(environment, cluster, kindRun, result != nil)
	}()

	// Reuse the smoke cluster preparation verbatim: it builds and loads the runtime
	// and collector images and creates the namespace.
	if err := prepareSmokeCluster(environment, cluster.Name, resolved, image); err != nil {
		return smokeFailure(environment.run, "cluster preparation", err)
	}

	// The applier image is built FROM the runtime image prepareSmokeCluster just built
	// and carries the instrumented /chart, so the in-cluster upgrade renders the same
	// chart the host installs.
	if err := buildApplierImage(resolved.Application, chartDir, image, applierImage); err != nil {
		return fmt.Errorf("applier image build: %w", err)
	}
	if err := assertApplierImageCarriesItsTools(applierImage); err != nil {
		return fmt.Errorf("applier image verification: %w", err)
	}
	if err := loadApplierImage(cluster.Name, applierImage); err != nil {
		return fmt.Errorf("applier image load: %w", err)
	}

	if err := installApplierLiveChart(environment, chartDir, resolved.Application, image, applierImage); err != nil {
		return smokeFailure(environment.run, "Helm install", err)
	}
	if err := verifyApplierLiveRollouts(environment); err != nil {
		return smokeFailure(environment.run, "role readiness", err)
	}

	forward, err := forwardService(environment, smokeRelease+"-agent-architecture-applier",
		"18230:18230", "18231:18231")
	if err != nil {
		return smokeFailure(environment.run, "applier port-forward", err)
	}
	defer forward.stop()

	if err := assertApplierServesItsSurface(environment, resolved.Application); err != nil {
		return err
	}
	fmt.Printf("integration:applierLive PASS - revision %s the applier runs on kind from an image built on the "+
		"runtime under test, reads a real collector Deployment's rollout, applies a values patch that moves the "+
		"release to a new revision, compensates a post-verify stall with a real helm rollback, and rejects a "+
		"non-conforming patch against the real chart schema without touching it\n", revision)
	return nil
}

// stageApplierLiveChart assembles one chart directory carrying the profile closures
// and the applier profile. Both the host-side install and the applier image's /chart
// use this directory, so a values change re-renders one coherent instrumented chart
// (srd002-applier R2.2). It mirrors packageHelmChart's staging without packaging to a
// tgz, because the applier image needs a directory to COPY.
func stageApplierLiveChart(resolved roots) (string, func(), error) {
	stage, err := os.MkdirTemp("", "agent-architecture-applier-live-chart-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(stage) }
	chart := filepath.Join(stage, "agent-architecture")
	if err := stageChartSource(filepath.Join(resolved.Application, "helm"), chart); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("stage source chart: %w", err)
	}
	if err := prepareChartProfiles(resolved.Catalog, chart); err != nil {
		cleanup()
		return "", nil, err
	}
	if err := stageApplierProfile(resolved.Application, chart); err != nil {
		cleanup()
		return "", nil, err
	}
	if err := validatePreparedProfiles(filepath.Join(chart, "profiles")); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("validate staged profiles: %w", err)
	}
	return chart, cleanup, nil
}

// buildApplierImage builds the applier image from the locally built runtime image and
// the supplied staged chart rather than published artifacts, so the live tier runs
// the code under test the way the smoke does.
//
// The build context carries the *staged* chart, not the one in the repo: the
// Dockerfile's `COPY helm /chart` takes whatever the context has, and the chart in the
// repo carries no profiles (helm/profiles/ is gitignored and staged by helmPrepare).
// An image built from the unstaged chart renders an empty profiles ConfigMap, so an
// in-cluster upgrade would replace the live one and no agent would survive its next
// restart.
//
// TARGETARCH is passed explicitly. The Dockerfile defaults it to amd64, and a plain
// docker build on an arm64 host does not set it -- the result is an arm64 image
// carrying amd64 helm and kubectl, which crash the first time an exec word runs one.
func buildApplierImage(applicationRoot, chartDir, runtimeImage, image string) error {
	buildCtx, err := os.MkdirTemp("", "agent-architecture-applier-image-ctx-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(buildCtx) }()
	if err := copyDirTree(chartDir, filepath.Join(buildCtx, "helm")); err != nil {
		return fmt.Errorf("place the staged chart in the build context: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), applierLiveClusterTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "build",
		"-f", filepath.Join(applicationRoot, "applier.Dockerfile"),
		"--build-arg", "RUNTIME_IMAGE="+runtimeImage,
		"--build-arg", "TARGETARCH="+runtime.GOARCH,
		"-t", image, buildCtx)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker build %s: %w: %s", image, err, strings.TrimSpace(string(output)))
	}
	return nil
}

// assertApplierImageCarriesItsTools runs each assumption the exec declarations make
// about their own container inside the built image, so a missing or wrong-architecture
// tool fails here with a name rather than at runtime inside a pod.
func assertApplierImageCarriesItsTools(image string) error {
	probes := []struct {
		what string
		args []string
		want string
	}{
		{"helm", []string{"helm", "version", "--short"}, "v"},
		{"kubectl", []string{"kubectl", "version", "--client"}, "Client Version"},
		{"the chart at /chart", []string{"cat", "/chart/Chart.yaml"}, "agent-architecture"},
		{"the agent binary", []string{"agent", "--help"}, "profile"},
	}
	for _, probe := range probes {
		args := append([]string{"run", "--rm", "--entrypoint", probe.args[0], image}, probe.args[1:]...)
		out, err := exec.Command("docker", args...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("applier image does not carry %s: docker %s: %w\n%s",
				probe.what, strings.Join(probe.args, " "), err, out)
		}
		if !strings.Contains(string(out), probe.want) {
			return fmt.Errorf("applier image %s check did not report %q:\n%s", probe.what, probe.want, out)
		}
		fmt.Printf("applierLive: image carries %s\n", probe.what)
	}
	if err := assertApplierImageHelmMajor(image); err != nil {
		return err
	}
	return assertApplierImageChartCarriesProfiles(image)
}

// assertApplierImageHelmMajor proves the helm inside the image is the major the exec
// declarations are written for.
func assertApplierImageHelmMajor(image string) error {
	out, err := exec.Command("docker", "run", "--rm", "--entrypoint", "helm", image,
		"version", "--template", "{{.Version}}").CombinedOutput()
	if err != nil {
		return fmt.Errorf("read helm version from %s: %w\n%s", image, err, out)
	}
	version := strings.TrimSpace(string(out))
	major := strings.TrimPrefix(strings.SplitN(version, ".", 2)[0], "v")
	if major != applierDeclaredHelmMajor {
		return fmt.Errorf("the applier image ships helm %s, but its exec declarations are written for helm %s; "+
			"the flag spellings differ between majors and helm rejects an unknown flag",
			version, applierDeclaredHelmMajor)
	}
	fmt.Printf("applierLive: image ships helm %s, matching the declared flags\n", version)
	return nil
}

// assertApplierImageChartCarriesProfiles renders the chart the image ships, using the
// image's own helm, and requires every mounted profile to appear in the rendered
// profiles ConfigMaps. This is what an apply actually does: the applier runs helm
// upgrade <release> /chart, which re-renders the co-generated topology, so the
// ConfigMaps that render produces replace the live ones. If /chart carries an unstaged
// chart the render is empty and no agent survives its next restart.
func assertApplierImageChartCarriesProfiles(image string) error {
	out, err := exec.Command("docker", "run", "--rm", "--entrypoint", "helm", image,
		"template", "agent-architecture", "/chart", "--set", "applier.enabled=true").CombinedOutput()
	if err != nil {
		return fmt.Errorf("render /chart with the image's own helm: %w\n%s", err, out)
	}
	render := string(out)
	for _, key := range []string{
		"documentation-curator__profile.yaml",
		"agents__collector__profile.yaml",
		"agents__applier__profile.yaml",
	} {
		if !strings.Contains(render, key) {
			return fmt.Errorf("the chart at /chart renders no %s; an apply would replace the live profiles "+
				"ConfigMap with one missing it, and that agent would not come back from a restart", key)
		}
	}
	fmt.Println("applierLive: the chart at /chart renders every mounted profile")
	return nil
}

func loadApplierImage(cluster, image string) error {
	kindLoad := func(ctx context.Context, args ...string) ([]byte, error) {
		return smokeEnvironment{}.run(ctx, "kind", args...)
	}
	ctx, cancel := context.WithTimeout(context.Background(), applierLiveClusterTimeout)
	defer cancel()
	return kindrig.LoadImage(ctx, kindLoad, cluster, image)
}

// installApplierLiveChart installs the instrumented chart directory with the applier
// enabled. It layers the kind footprint every cluster test shares, then the applier
// the others deliberately disable, and pins the locally built and loaded images.
func installApplierLiveChart(
	environment smokeEnvironment, chartDir, applicationRoot, runtimeImage, applierImage string,
) error {
	repository, tag := splitImageRef(runtimeImage)
	collectorRepository, collectorTag := splitImageRef(smokeCollectorImage)
	applierRepository, applierTag := splitImageRef(applierImage)
	ctx, cancel := context.WithTimeout(context.Background(), applierLiveInstallTimeout)
	defer cancel()
	output, err := environment.run(ctx, "helm",
		"install", smokeRelease, chartDir,
		"--namespace", smokeNamespace,
		"--values", filepath.Join(applicationRoot, "helm", "ci", "kind-values.yaml"),
		"--values", filepath.Join(applicationRoot, "helm", "ci", "kind-applier-values.yaml"),
		"--set", "image.repository="+repository,
		"--set", "image.tag="+tag,
		"--set", "collector.image.repository="+collectorRepository,
		"--set", "collector.image.tag="+collectorTag,
		"--set", "applier.image.repository="+applierRepository,
		"--set", "applier.image.tag="+applierTag,
		// No --wait: the bounded curator never stays ready, so waiting on the whole
		// release would always time out. Readiness is asserted per workload below.
		"--timeout", applierLiveInstallTimeout.String(),
	)
	if err != nil {
		return fmt.Errorf("helm install: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// verifyApplierLiveRollouts waits for the two persistent servers the applier depends
// on -- the collector it verifies and the applier itself -- to reach readiness. The
// bounded curator is deliberately not waited on.
func verifyApplierLiveRollouts(environment smokeEnvironment) error {
	for _, component := range []string{"collector", "applier"} {
		if err := runSmokeCommand(environment, applierLiveReadyTimeout, "kubectl", "rollout", "status",
			"deployment/"+smokeRelease+"-agent-architecture-"+component, "-n", smokeNamespace,
			"--timeout=120s"); err != nil {
			return fmt.Errorf("%s readiness: %w", component, err)
		}
	}
	return nil
}

// assertApplierServesItsSurface proves the applier is an agent that started, not just
// a container that is running, and then drives the apply path the fake-CLI tracer
// cannot reach: a real helm upgrade, a real rollback, and a real schema rejection
// against a real release.
func assertApplierServesItsSurface(environment smokeEnvironment, applicationRoot string) error {
	if err := waitHTTP200(applierControlHealthURL, applierLiveReadyTimeout); err != nil {
		return smokeFailure(environment.run, "applier control health",
			fmt.Errorf("the applier control health never answered: %w", err))
	}
	fmt.Println("applierLive: the applier answers its control health")

	steps := []struct {
		name string
		op   func() error
	}{
		{"rollout read", assertLiveRolloutReads},
		{"upgrade", func() error { return assertLiveApplyChangesTheRelease(environment, applicationRoot) }},
		{"rollback", func() error { return assertLiveRollbackRestoresTheRelease(environment, applicationRoot) }},
		{"schema rejection", func() error { return assertLiveSchemaRejection(environment, applicationRoot) }},
		{"rollout recheck", assertLiveRolloutReads},
	}
	for _, step := range steps {
		if err := runApplierLiveApplyStep(environment.run, step.name, step.op); err != nil {
			return err
		}
	}
	return nil
}

// assertLiveRolloutReads checks a rollout read against a real Deployment. The phase is
// not pinned -- both complete and progressing are honest answers about a real cluster
// -- but a 502 would mean the applier could not reach the Deployment at all, and a
// zero desired or revision would mean it read something that is not there.
func assertLiveRolloutReads() error {
	status, body, err := applierHTTPWithTimeout(http.MethodGet, applierRolloutURL, "", applierLiveRequestTimeout)
	if err != nil {
		return fmt.Errorf("rollout read failed: %w", err)
	}
	if status != http.StatusOK {
		return fmt.Errorf("rollout read status = %d, want 200; the applier could not read the Deployment: %s",
			status, body)
	}
	var rollout struct {
		Phase    string `json:"phase"`
		Ready    int    `json:"ready"`
		Desired  int    `json:"desired"`
		Revision int    `json:"revision"`
	}
	if err := json.Unmarshal([]byte(body), &rollout); err != nil {
		return fmt.Errorf("decode rollout response: %w: %s", err, body)
	}
	if rollout.Phase != "complete" && rollout.Phase != "progressing" {
		return fmt.Errorf("rollout phase = %q, want complete or progressing: %s", rollout.Phase, body)
	}
	if rollout.Desired < 1 {
		return fmt.Errorf("rollout desired = %d; the counts did not come from a real Deployment: %s",
			rollout.Desired, body)
	}
	if rollout.Revision < 1 {
		return fmt.Errorf("rollout revision = %d; a deployed release has at least revision 1: %s",
			rollout.Revision, body)
	}
	fmt.Printf("applierLive: rollout read reports phase %s, %d/%d ready, revision %d from the live collector Deployment\n",
		rollout.Phase, rollout.Ready, rollout.Desired, rollout.Revision)
	return nil
}

// assertLiveApplyChangesTheRelease drives a conforming values patch through the apply
// endpoint and proves the release actually changed. A 200 is not evidence here: the
// fake-CLI tracer already returns one. What makes this different is the helm revision.
func assertLiveApplyChangesTheRelease(environment smokeEnvironment, applicationRoot string) error {
	before, err := helmReleaseRevision(environment)
	if err != nil {
		return err
	}
	fmt.Printf("applierLive: release at revision %d before the apply\n", before)

	patch, err := applierValuesPatchRequest(applicationRoot, "conforming.yaml")
	if err != nil {
		return err
	}
	status, body, err := applierHTTPWithTimeout(http.MethodPost, applierApplyURL, patch, applierLiveRequestTimeout)
	if err != nil {
		return fmt.Errorf("apply request failed: %w", err)
	}
	if status != http.StatusOK {
		return fmt.Errorf("apply status = %d, want 200: %s", status, body)
	}
	if !strings.Contains(body, `"status":"applied"`) {
		return fmt.Errorf("apply did not report applied: %s", body)
	}

	after, err := helmReleaseRevision(environment)
	if err != nil {
		return err
	}
	if after <= before {
		return fmt.Errorf("the release is still at revision %d after an apply reported success; "+
			"helm_upgrade did not reach the release, so the 200 proved nothing", after)
	}
	fmt.Printf("applierLive: the apply moved the release from revision %d to %d\n", before, after)
	return nil
}

// assertLiveRollbackRestoresTheRelease proves the compensating action with real Helm
// and kubectl (srd002-applier R3.2). The rollback-trigger patch repoints the collector
// image at a name never loaded into the node, so the applier's helm_upgrade applies
// and returns, the new collector ReplicaSet is ErrImageNeverPull, the applier's
// kubectl rollout status of the collector observes the 120s timeout, runs helm
// rollback, and maps RolledBack to the distinct 500 response.
//
// Helm rollback creates a new release revision; it does not move the revision number
// backwards. Restoration is proved by comparing the computed release values and by
// waiting for the collector Deployment to become ready again.
func assertLiveRollbackRestoresTheRelease(environment smokeEnvironment, applicationRoot string) error {
	beforeRevision, err := helmReleaseRevision(environment)
	if err != nil {
		return err
	}
	beforeValues, err := helmReleaseValues(environment)
	if err != nil {
		return err
	}
	patch, err := applierValuesPatchRequest(applicationRoot, "rollback-trigger.yaml")
	if err != nil {
		return err
	}
	status, body, err := applierHTTPWithTimeout(http.MethodPost, applierApplyURL, patch, applierLiveRequestTimeout)
	if err != nil {
		return fmt.Errorf("rollback-triggering apply request failed: %w", err)
	}
	if status != http.StatusInternalServerError {
		return fmt.Errorf("rollback-triggering apply status = %d, want 500: %s", status, body)
	}
	for _, want := range []string{`"error":"rolled_back"`, `"status":"rolled_back"`} {
		if !strings.Contains(body, want) {
			return fmt.Errorf("rollback response does not contain %s: %s", want, body)
		}
	}

	afterRevision, err := helmReleaseRevision(environment)
	if err != nil {
		return err
	}
	if afterRevision < beforeRevision+2 {
		return fmt.Errorf("release revision moved from %d to %d, want an upgrade and a rollback revision",
			beforeRevision, afterRevision)
	}
	afterValues, err := helmReleaseValues(environment)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(afterValues, beforeValues) {
		beforeJSON, _ := json.Marshal(beforeValues)
		afterJSON, _ := json.Marshal(afterValues)
		return fmt.Errorf("helm rollback did not restore the prior computed values:\nbefore: %s\nafter:  %s",
			beforeJSON, afterJSON)
	}

	if err := runSmokeCommand(environment, applierLiveReadyTimeout,
		"kubectl", "rollout", "status",
		"deployment/"+smokeRelease+"-agent-architecture-collector",
		"-n", smokeNamespace, "--timeout", applierLiveReadyTimeout.String()); err != nil {
		return fmt.Errorf("collector Deployment did not recover after rollback: %w", err)
	}
	fmt.Printf("applierLive: real helm rollback restored revision %d values in new revision %d and recovered the collector\n",
		beforeRevision, afterRevision)
	return nil
}

// assertLiveSchemaRejection closes the loop the local dry-run opened: the same
// non-conforming document, now against a real release on a cluster. The release must
// not move -- a rejected patch applies nothing (srd002-applier R2.1).
func assertLiveSchemaRejection(environment smokeEnvironment, applicationRoot string) error {
	before, err := helmReleaseRevision(environment)
	if err != nil {
		return err
	}
	patch, err := applierValuesPatchRequest(applicationRoot, "non-conforming.yaml")
	if err != nil {
		return err
	}
	status, body, err := applierHTTPWithTimeout(http.MethodPost, applierApplyURL, patch, applierLiveRequestTimeout)
	if err != nil {
		return fmt.Errorf("apply request failed: %w", err)
	}
	if status != http.StatusBadRequest {
		return fmt.Errorf("a non-conforming patch returned %d, want 400: %s", status, body)
	}
	if !strings.Contains(body, "validate_rejected") {
		return fmt.Errorf("the rejection did not report validate_rejected: %s", body)
	}
	after, err := helmReleaseRevision(environment)
	if err != nil {
		return err
	}
	if after != before {
		return fmt.Errorf("the release moved from revision %d to %d on a rejected patch; "+
			"a schema rejection must apply nothing", before, after)
	}
	fmt.Printf("applierLive: the non-conforming patch was rejected and left the release at revision %d\n", after)
	return nil
}

// applierValuesPatchRequest reads a shared fixture and wraps it as the apply request
// the caller would send (srd002-applier R1.4). The fixtures are the ones the local
// dry-run tier validates, so both tiers exercise the same documents.
func applierValuesPatchRequest(applicationRoot, fixture string) (string, error) {
	path := filepath.Join(applicationRoot, "testdata", "integration", "applier-values", fixture)
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read values fixture %s: %w", fixture, err)
	}
	request := map[string]string{"schema_version": "1", "content": string(content)}
	encoded, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// helmReleaseRevision reads the release's current revision, which is what a real
// upgrade moves and a rejected patch leaves alone.
func helmReleaseRevision(environment smokeEnvironment) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), smokeProbeTimeout)
	defer cancel()
	out, err := environment.run(ctx, "helm", "get", "metadata", smokeRelease,
		"-n", smokeNamespace, "-o", "json")
	if err != nil {
		return 0, fmt.Errorf("helm get metadata %s: %w\n%s", smokeRelease, err, out)
	}
	var metadata struct {
		Revision int `json:"revision"`
	}
	if err := json.Unmarshal(out, &metadata); err != nil {
		return 0, fmt.Errorf("decode helm metadata: %w: %s", err, out)
	}
	return metadata.Revision, nil
}

// helmReleaseValues reads the fully computed values so a rollback is compared by
// released state, not by its ever-increasing numeric revision.
func helmReleaseValues(environment smokeEnvironment) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), smokeProbeTimeout)
	defer cancel()
	out, err := environment.run(ctx, "helm", "get", "values", smokeRelease,
		"-n", smokeNamespace, "--all", "-o", "json")
	if err != nil {
		return nil, fmt.Errorf("helm get values %s: %w\n%s", smokeRelease, err, out)
	}
	var values map[string]any
	if err := json.Unmarshal(out, &values); err != nil {
		return nil, fmt.Errorf("decode helm values: %w: %s", err, out)
	}
	return values, nil
}
