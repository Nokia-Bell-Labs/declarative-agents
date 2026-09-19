// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

const (
	// The live applier tier reuses the smoke release and scenario namespace on
	// da-platform so the runtime helpers apply unchanged. It runs after the smoke
	// has released the namespace, so the two never overlap.

	// The applier's exec declarations are written for helm 3, which the CLI donor
	// pins (applier.cliDonor.image, kindrig.CLIDonorHelmVersion). A helm-4 donor
	// would reject the --dry-run spelling outright.
	applierDeclaredHelmMajor = "3"

	// The live apply legs run a real helm upgrade, a real 120s kubectl rollout verify,
	// and (on the rollback leg) a helm rollback, so the client waits well past the
	// fake-tracer's 130s bound.
	applierLiveRequestTimeout = 4 * time.Minute
	applierLiveReadyTimeout   = 3 * time.Minute
	applierLiveClusterTimeout = 3 * time.Minute
	applierLiveInstallTimeout = 5 * time.Minute
	applierLiveChartConfigMap = smokeRelease + "-agent-architecture-applier-chart"
)

// ApplierLive proves the applier against a real cluster, which the fake-CLI tracer
// (integration:applier) cannot: that target drives recording stand-ins whose exit
// codes come from the scenario, so it is evidence about the machine and the arguments
// it builds, not about helm and kubectl behaving as the declarations assume
// (srd002-applier R5.3). This one loads the pinned CLI donor, installs the chart with
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
	// The curator, the collector, and the applier all run the locally built
	// agent-core image (GH-1368); the applier's helm and kubectl arrive from the
	// pinned CLI donor at pod start (GH-2222).
	revision := mustGitRevision(resolved.Application)

	// One instrumented chart directory serves both the host-side install and the
	// applier's mounted /chart, so Helm records and rolls back one coherent chart.
	// The chart is delivered to the applier pod as a volume, not baked into the
	// image (GH-1368): it is packaged to a tarball and provisioned in a ConfigMap
	// outside the Helm release so the release Secret does not store the archive
	// twice and exceed the API server's 1 MiB object limit.
	chartDir, cleanupChart, err := stageApplierLiveChart(resolved)
	if err != nil {
		return err
	}
	defer cleanupChart()
	chartArchive, cleanupArchive, err := packageApplierChart(chartDir)
	if err != nil {
		return fmt.Errorf("applier chart package: %w", err)
	}
	defer cleanupArchive()
	if err := assertApplierChartArchiveCarriesProfiles(chartArchive); err != nil {
		return fmt.Errorf("applier chart verification: %w", err)
	}

	scenario, err := acquireSmokeScenario(resolved.Application, "applierLive")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, scenario.release(result != nil)) }()
	environment := scenario.environment
	cluster := scenario.platform.Cluster

	// Reuse the smoke cluster preparation verbatim: it builds and loads the shared
	// agent-core image both workloads run.
	if err := prepareSmokeCluster(environment, cluster.Name, resolved); err != nil {
		return smokeFailure(environment.run, "cluster preparation", err)
	}

	// prepareSmokeCluster already built and loaded the agent-core image the
	// applier runs. Its helm and kubectl come from the pinned CLI donor, loaded
	// once per platform node and copied into the pod's read-only /opt/tools by the
	// cli-donor init container (GH-2222). The chart reaches the pod through the
	// externally provisioned ConfigMap named by applier.chartArchiveConfigMap.
	if err := kindrig.EnsureCLIDonorImage(smokeCommandRunner(environment), cluster.Name); err != nil {
		return fmt.Errorf("applier CLI donor: %w", err)
	}

	if err := installApplierLiveChart(environment, chartDir, chartArchive, resolved.Application, smokeCollectorImage, smokeCollectorImage); err != nil {
		return smokeFailure(environment.run, "Helm install", err)
	}
	if err := verifyApplierLiveRollouts(environment); err != nil {
		return smokeFailure(environment.run, "role readiness", err)
	}
	helmVersion, err := assertApplierCLIDonor(environment)
	if err != nil {
		return smokeFailure(environment.run, "applier CLI donor", err)
	}

	forward, err := forwardService(environment, smokeRelease+"-agent-architecture-applier",
		"18330:18330", "18331:18331")
	if err != nil {
		return smokeFailure(environment.run, "applier port-forward", err)
	}
	defer forward.stop()

	if err := assertApplierServesItsSurface(environment, resolved.Application); err != nil {
		return err
	}
	fmt.Printf("integration:applierLive PASS - revision %s the applier runs the agent-core runtime under test with "+
		"helm %s from the pinned CLI donor on a read-only /opt/tools, reads a real collector Deployment's rollout, "+
		"applies a values patch that moves the release to a new revision, compensates a post-verify stall with a "+
		"real helm rollback, and rejects a non-conforming patch against the real chart schema without touching it\n",
		revision, helmVersion)
	return nil
}

// stageApplierLiveChart assembles one chart directory carrying the profile closures
// and the applier profile. The host installs this directory, and packageApplierChart
// packages it to the tarball the applier mounts at /chart, so a values change
// re-renders one coherent instrumented chart (srd002-applier R2.2). It mirrors
// packageHelmChart's staging.
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
	if err := prepareChartProfiles(resolved.Application, resolved.Catalog, chart); err != nil {
		cleanup()
		return "", nil, err
	}
	// No curator UI shards here: the applier mounts this chart from an external
	// ConfigMap, and carrying the ~1.2 MiB gzipped UI would exceed that object's
	// limit (GH-1402). applierLive does not exercise the
	// curator UI (only the collector and applier are awaited), so the curator
	// renders without it (curatorUI.shards defaults empty).
	if err := validatePreparedProfiles(filepath.Join(chart, "profiles")); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("validate staged profiles: %w", err)
	}
	return chart, cleanup, nil
}

// assertApplierCLIDonor proves, inside the running applier container, what the
// donor pattern promises (GH-2222): helm resolves to the pinned donor release the
// exec declarations are written for, kubectl is present, and the container cannot
// replace the binaries it execs. It returns the helm version for the PASS line.
func assertApplierCLIDonor(environment smokeEnvironment) (string, error) {
	return kindrig.VerifyCLIDonor(applierDeclaredHelmMajor, func(args ...string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), applierLiveReadyTimeout)
		defer cancel()
		exec := append([]string{"exec", "--namespace", smokeNamespace,
			"deployment/" + smokeRelease + "-agent-architecture-applier", "-c", "applier", "--"}, args...)
		output, err := environment.run(ctx, "kubectl", exec...)
		return strings.TrimSpace(string(output)), err
	})
}

// packageApplierChart packages the staged chart directory into a gzipped tarball
// and returns its path. This is the chart the applier's `helm upgrade
// agent-architecture /chart` word installs, delivered through an out-of-release
// ConfigMap rather than baked into the image (GH-1368). It is the same
// instrumented chart the host installs, so an in-cluster upgrade re-renders one
// coherent chart.
func packageApplierChart(chartDir string) (string, func(), error) {
	dest, err := os.MkdirTemp("", "agent-architecture-applier-chart-tgz-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dest) }
	output, err := exec.Command("helm", "package", chartDir, "--destination", dest).CombinedOutput()
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("helm package applier chart: %w: %s", err, strings.TrimSpace(string(output)))
	}
	archive := filepath.Join(dest, "agent-architecture-0.1.0.tgz")
	if _, err := os.Stat(archive); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("packaged applier chart %s: %w", archive, err)
	}
	return archive, cleanup, nil
}

// assertApplierChartArchiveCarriesProfiles renders the packaged chart the applier
// will mount at /chart, using the host helm, and requires every mounted profile to
// appear. This is what an apply actually does: the applier runs helm upgrade
// agent-architecture /chart, which re-renders the co-generated topology
// (srd002-applier R2.2), so the ConfigMaps that render produces replace the live
// ones. If the mounted chart carried no profiles the render would be empty, the
// replacement would strip every profile, and no agent would survive its next
// restart.
func assertApplierChartArchiveCarriesProfiles(archive string) error {
	out, err := exec.Command("helm", "template", "agent-architecture", archive,
		"--set", "applier.enabled=true").CombinedOutput()
	if err != nil {
		return fmt.Errorf("render packaged applier chart: %w\n%s", err, out)
	}
	render := string(out)
	for _, key := range []string{
		"documentation-curator__profile.yaml",
		"agents__collector__profile.yaml",
		"applications__agent-architecture__applier__profile.yaml",
		"applications__catalog__applier__machine.yaml",
	} {
		if !strings.Contains(render, key) {
			return fmt.Errorf("the packaged applier chart renders no %s; an apply would replace the live "+
				"profiles ConfigMap with one missing it, and that agent would not come back from a restart", key)
		}
	}
	fmt.Println("applierLive: the chart the applier mounts at /chart renders every mounted profile")
	return nil
}

// installApplierLiveChart installs the instrumented chart directory with the applier
// enabled. It layers the kind footprint every cluster test shares, then the applier
// the others deliberately disable, and pins the locally built and loaded images. The
// chart the applier mounts at /chart is delivered through a ConfigMap provisioned
// outside the release, so no image bakes the chart and Helm does not duplicate
// the archive in its release Secret.
func installApplierLiveChart(
	environment smokeEnvironment, chartDir, chartArchive, applicationRoot, runtimeImage, applierImage string,
) error {
	repository, tag := splitImageRef(runtimeImage)
	collectorRepository, collectorTag := splitImageRef(smokeCollectorImage)
	applierRepository, applierTag := splitImageRef(applierImage)
	if err := provisionApplierChartConfigMap(environment.run, chartArchive); err != nil {
		return err
	}
	valueArgs := applierLiveValueArgs(
		applicationRoot, repository, tag,
		collectorRepository, collectorTag, applierRepository, applierTag,
	)
	ctx, cancel := context.WithTimeout(context.Background(), applierLiveInstallTimeout)
	defer cancel()
	args := append([]string{"install", smokeRelease, chartDir}, valueArgs...)
	args = append(args,
		// No --wait: the bounded curator never stays ready, so waiting on the whole
		// release would always time out. Readiness is asserted per workload below.
		"--timeout", applierLiveInstallTimeout.String(),
	)
	output, err := environment.run(ctx, "helm", args...)
	if err != nil {
		return fmt.Errorf("helm install: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func applierLiveValueArgs(
	applicationRoot, repository, tag,
	collectorRepository, collectorTag, applierRepository, applierTag string,
) []string {
	return []string{
		"--set", "applier.chartArchiveConfigMap=" + applierLiveChartConfigMap,
		"--namespace", smokeNamespace,
		"--values", filepath.Join(applicationRoot, "helm", "ci", "kind-values.yaml"),
		"--values", filepath.Join(applicationRoot, "helm", "ci", "kind-applier-values.yaml"),
		"--set", "image.repository=" + repository,
		"--set-string", "image.tag=" + tag,
		"--set", "collector.image.repository=" + collectorRepository,
		"--set-string", "collector.image.tag=" + collectorTag,
		"--set", "applier.image.repository=" + applierRepository,
		"--set-string", "applier.image.tag=" + applierTag,
	}
}

func provisionApplierChartConfigMap(run applierLiveRunner, chartArchive string) error {
	ctx, cancel := context.WithTimeout(context.Background(), applierLiveInstallTimeout)
	defer cancel()
	if output, err := run(ctx, "kubectl",
		"delete", "configmap", applierLiveChartConfigMap,
		"--namespace", smokeNamespace, "--ignore-not-found",
	); err != nil {
		return fmt.Errorf("clear stale applier chart ConfigMap: %w: %s",
			err, strings.TrimSpace(string(output)))
	}
	output, err := run(ctx, "kubectl",
		"create", "configmap", applierLiveChartConfigMap,
		"--namespace", smokeNamespace, "--from-file=chart.tgz="+chartArchive,
	)
	if err != nil {
		return fmt.Errorf("create applier chart ConfigMap: %w: %s",
			err, strings.TrimSpace(string(output)))
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
// image at a name never loaded into the node and sets progressDeadlineSeconds to 5, so
// the applier's helm_upgrade applies and returns, the new collector ReplicaSet is
// ErrImageNeverPull, ProgressDeadlineExceeded trips in seconds rather than consuming
// the 120s verify window, the applier runs helm rollback, and maps RolledBack to the
// distinct 500 response. The 130s machine_request budget retains headroom for that
// rollback and response (GH-1745, GH-1767).
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
