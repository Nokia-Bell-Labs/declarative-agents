// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package kindrig

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PlatformClusterName is the shared cluster-services tier every namespaced
// scenario runs on (GH-2215).
const PlatformClusterName = "da-platform"

// PlatformConformanceScenario owns the throwaway namespace the conformance
// suite exercises, da-platform-conformance.
const PlatformConformanceScenario = "platform-conformance"

const (
	platformClusterWait        = 5 * time.Minute
	platformConformanceHost    = "conformance.localhost"
	platformConformanceRelease = "platform-conformance"
	platformHostPlaceholder    = "KINDRIG_CONFORMANCE_HOST"
	platformTracingPlaceholder = "KINDRIG_PLATFORM_TRACING_CONFIG"
	platformObjectsPlaceholder = "KINDRIG_PLATFORM_OBJECTS"
	// platformObjectsDirName is the stable host directory holding the fake-GCS
	// filesystem backend. It lives under the user cache, outside any cluster,
	// so retained objects survive da-platform deletion and recreation and are
	// removed only by platform:reset (srd008 R4.3, R5; #2477 R3, R6).
	platformObjectsDirName = "da-platform-objects"
	platformWaitTimeout    = "120s"
	// platformCommandTimeout bounds each command the platform runner issues.
	// Every kubectl wait the rig declares is shorter, so only a stalled command
	// (a registry that never answers a pull) reaches it (GH-2226).
	platformCommandTimeout = 5 * time.Minute
)

//go:embed platform-kind-config.yaml
var platformKindConfig []byte

//go:embed platform-conformance.yaml
var platformConformanceManifest string

//go:embed platform-tracing.yaml
var platformTracingConfig []byte

// PlatformOptions configures StartPlatform. The zero value streams kind output,
// binds commands to a private kubeconfig for da-platform, and captures no
// failure evidence.
type PlatformOptions struct {
	// KindRun runs kind subcommands for acquisition, evidence, and teardown.
	KindRun Runner
	// Bind returns a command runner bound to the named cluster's kubeconfig and
	// a cleanup for that binding.
	Bind func(cluster string) (CommandRunner, func(), error)
	// EvidenceDirectory receives kind logs and namespace diagnostics when boot,
	// conformance, or a later caller-reported failure ends the platform.
	EvidenceDirectory string

	boot           func(CommandRunner, string) error
	conformance    func(CommandRunner, string) error
	healthRun      CommandRunner
	commandTimeout time.Duration
}

// Platform is a running, conformance-checked da-platform cluster. Run is bound
// to its kubeconfig. Stop must run on every exit path of the platform's owner.
type Platform struct {
	Cluster Cluster
	Run     CommandRunner

	kindRun  Runner
	evidence FailureEvidence
	unbind   func()
	stopped  bool
}

// StartPlatform acquires da-platform fresh, installs the shared infrastructure,
// and runs the conformance suite. A leftover da-platform from an interrupted
// run is deleted and recreated (GH-2137). When any step fails the cluster is
// released with evidence before StartPlatform returns, so a platform failure is
// reported before, and separately from, any application gate.
func StartPlatform(options PlatformOptions) (*Platform, error) {
	options = options.withDefaults()
	cluster, err := EnsurePlatformCluster(options.KindRun)
	if err != nil {
		return nil, err
	}
	run, unbind, err := options.Bind(cluster.Name)
	if err != nil {
		cluster.Release(options.KindRun)
		return nil, fmt.Errorf("bind %s commands: %w", cluster.Name, err)
	}
	platform := &Platform{
		Cluster: cluster,
		Run:     run,
		kindRun: options.KindRun,
		evidence: FailureEvidence{
			Directory:  options.EvidenceDirectory,
			Namespaces: platformEvidenceNamespaces(),
			Run:        run,
		},
		unbind: unbind,
	}
	for _, phase := range []struct {
		name string
		run  func(CommandRunner, string) error
	}{
		{"platform-boot", options.boot},
		{"platform-conformance", options.conformance},
	} {
		started := time.Now()
		if err := phase.run(run, cluster.Name); err != nil {
			LogPhase(cluster.Name, phase.name, "failed", started, "")
			platform.Stop(true)
			return nil, fmt.Errorf("%s %s: %w", cluster.Name, phase.name, err)
		}
		LogPhase(cluster.Name, phase.name, "passed", started, "")
	}
	return platform, nil
}

// AcquirePlatform is how an application's integration targets reach the tier.
// A listed da-platform belongs to whoever created it (a release run or a
// developer) and is reused without ownership once its data plane answers, so
// Stop leaves it running. With none listed, AcquirePlatform starts an owned
// platform through StartPlatform, which Stop deletes.
func AcquirePlatform(options PlatformOptions) (*Platform, error) {
	options = options.withDefaults()
	if !Exists(options.KindRun, PlatformClusterName) {
		return StartPlatform(options)
	}
	run, unbind, err := options.Bind(PlatformClusterName)
	if err != nil {
		return nil, fmt.Errorf("bind running %s commands: %w", PlatformClusterName, err)
	}
	if err := VerifyDataPlane(run); err != nil {
		unbind()
		return nil, fmt.Errorf(
			"running %s is not ready: %w; remediation: wait for its owner to finish, "+
				"or remove it with kind delete cluster --name %s and rerun",
			PlatformClusterName, err, PlatformClusterName)
	}
	fmt.Printf("kind: reusing running platform cluster %s; it will not be deleted\n",
		PlatformClusterName)
	return &Platform{
		Cluster: Cluster{Name: PlatformClusterName},
		Run:     run,
		kindRun: options.KindRun,
		evidence: FailureEvidence{
			Directory:  options.EvidenceDirectory,
			Namespaces: platformEvidenceNamespaces(),
			Run:        run,
		},
		unbind: unbind,
	}, nil
}

// Stop releases the platform: an owned cluster is deleted, after evidence
// capture when failed is true and an evidence directory was configured. A
// reused cluster is left in place. Stop is idempotent.
func (p *Platform) Stop(failed bool) {
	if p == nil || p.stopped {
		return
	}
	p.stopped = true
	if failed && p.evidence.Directory != "" {
		p.Cluster.ReleaseAfter(p.kindRun, true, p.evidence)
	} else {
		p.Cluster.Release(p.kindRun)
	}
	if p.unbind != nil {
		p.unbind()
	}
}

// UpPlatform brings up the persistent developer da-platform and leaves it
// running. With none listed it starts one (StartPlatform), deleting it only if
// boot or conformance fails. A listed, healthy da-platform is reused without
// ownership: its node image store is pruned of unreferenced images, the shared
// infrastructure is re-applied, and the conformance suite runs again; a failure
// leaves it in place for inspection. An unhealthy listed cluster is refused, not
// deleted. A release deletes and recreates da-platform (GH-2137), so a developer
// runs UpPlatform again afterwards.
func UpPlatform(options PlatformOptions) (Cluster, error) {
	options = options.withDefaults()
	if !Exists(options.KindRun, PlatformClusterName) {
		platform, err := StartPlatform(options)
		if err != nil {
			return Cluster{}, err
		}
		return platform.Detach(), nil
	}
	cluster, err := ensureListedPlatform(options)
	if err != nil {
		return Cluster{}, err
	}
	run, unbind, err := options.Bind(cluster.Name)
	if err != nil {
		return Cluster{}, fmt.Errorf("bind %s commands: %w", cluster.Name, err)
	}
	defer unbind()
	for _, phase := range []struct {
		name string
		run  func(CommandRunner, string) error
	}{
		{"platform-image-prune", prunePlatformImages},
		{"platform-boot", options.boot},
		{"platform-conformance", options.conformance},
	} {
		started := time.Now()
		if err := phase.run(run, cluster.Name); err != nil {
			LogPhase(cluster.Name, phase.name, "failed", started, "")
			return Cluster{}, fmt.Errorf("%s %s: %w", cluster.Name, phase.name, err)
		}
		LogPhase(cluster.Name, phase.name, "passed", started, "")
	}
	return cluster, nil
}

// DownPlatform stops da-platform compute and no other cluster. It refuses while
// a managed application namespace remains, so an application is never torn down
// as a side effect of a platform shutdown; the first release has no force
// bypass (#2477 R6, R7). The retained local object store is preserved: only
// platform:reset deletes it (srd008 R4.3). bind may be nil, which skips the
// namespace check (a caller with no cluster binding).
func DownPlatform(run Runner, bind func(string) (CommandRunner, func(), error)) error {
	if !Exists(run, PlatformClusterName) {
		fmt.Printf("platform: cluster %s does not exist\n", PlatformClusterName)
		return nil
	}
	if bind != nil {
		command, cleanup, err := bind(PlatformClusterName)
		if err == nil {
			defer cleanup()
			namespaces, nsErr := managedApplicationNamespaces(command)
			if nsErr == nil && len(namespaces) > 0 {
				return fmt.Errorf(
					"platform:down refuses: managed application namespaces still present (%s); "+
						"run app:down for each before platform:down",
					strings.Join(namespaces, ", "))
			}
		}
	}
	if output, err := run("delete", "cluster", "--name", PlatformClusterName); err != nil {
		return fmt.Errorf("delete %s: %w: %s", PlatformClusterName, err,
			strings.TrimSpace(string(output)))
	}
	fmt.Printf("platform: stopped cluster %s; retained object store preserved (platform:reset deletes it)\n",
		PlatformClusterName)
	return nil
}

// ensureListedPlatform reuses a listed da-platform through the health-checked,
// ownership-preserving branch of EnsureClusterWithOptions.
func ensureListedPlatform(options PlatformOptions) (Cluster, error) {
	path, cleanup, err := stagePlatformKindConfig()
	if err != nil {
		return Cluster{}, err
	}
	defer cleanup()
	return EnsureClusterWithOptions(options.KindRun, PlatformClusterName, path,
		platformClusterWait, EnsureOptions{
			ReusePolicy: PreserveUnhealthyCluster,
			HealthRun:   options.healthRun,
		})
}

// prunePlatformImages removes images no container references from the node's
// containerd store, which a long-lived developer platform accumulates as each
// revision is kind-loaded.
func prunePlatformImages(run CommandRunner, cluster string) error {
	return runChecked(run, "docker", "exec", cluster+"-control-plane",
		"crictl", "rmi", "--prune")
}

// Detach hands the platform's cluster to a caller that manages its lifecycle
// itself, dropping the kubeconfig binding without deleting anything. The caller
// releases the returned Cluster with Release or ReleaseAfter, which delete it
// only when this acquisition created it.
func (p *Platform) Detach() Cluster {
	if p == nil {
		return Cluster{}
	}
	p.stopped = true
	if p.unbind != nil {
		p.unbind()
	}
	return p.Cluster
}

// EnsurePlatformCluster creates da-platform from the checked-in
// platform-kind-config.yaml with FreshOwnedCluster semantics: any listed
// da-platform is a leftover and is deleted first, and the result is owned.
func EnsurePlatformCluster(run Runner) (Cluster, error) {
	path, cleanup, err := stagePlatformKindConfig()
	if err != nil {
		return Cluster{}, err
	}
	defer cleanup()
	return EnsureFreshCluster(run, PlatformClusterName, path, platformClusterWait)
}

// stagePlatformKindConfig writes the kind config with its tracing mount and
// its stable object directory resolved, for the duration of one acquisition.
func stagePlatformKindConfig() (string, func(), error) {
	tracingPath, err := stagePlatformTracingConfig()
	if err != nil {
		return "", nil, err
	}
	objectsPath, err := PlatformObjectsDir()
	if err != nil {
		return "", nil, err
	}
	config := strings.ReplaceAll(string(platformKindConfig), platformTracingPlaceholder, tracingPath)
	config = strings.ReplaceAll(config, platformObjectsPlaceholder, objectsPath)
	path, cleanup, err := writeTempManifest("kindrig-platform-*.yaml", config)
	if err != nil {
		return "", nil, fmt.Errorf("stage %s kind config: %w", PlatformClusterName, err)
	}
	return path, cleanup, nil
}

// PlatformObjectsDir is the stable host directory the fake-GCS filesystem
// backend is mounted from. It is created on first use and lives outside any
// cluster, so it survives da-platform deletion and recreation. platform:reset
// is the only operation that deletes it (srd008 R4.3; #2477 R3, R6).
func PlatformObjectsDir() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve %s object directory: %w", PlatformClusterName, err)
	}
	dir := filepath.Join(cache, "kindrig", platformObjectsDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create %s object directory: %w", PlatformClusterName, err)
	}
	return dir, nil
}

// stagePlatformTracingConfig writes the API-server tracing configuration to the
// user cache directory. The node bind-mounts it, and a restarted node remounts
// it, so it cannot live in a temporary directory the host may clean.
func stagePlatformTracingConfig() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("stage %s tracing config: %w", PlatformClusterName, err)
	}
	dir := filepath.Join(cache, "kindrig")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("stage %s tracing config: %w", PlatformClusterName, err)
	}
	path := filepath.Join(dir, "platform-tracing.yaml")
	if err := os.WriteFile(path, platformTracingConfig, 0o644); err != nil {
		return "", fmt.Errorf("stage %s tracing config: %w", PlatformClusterName, err)
	}
	return path, nil
}

// BootPlatform installs the cluster-wide infrastructure scenarios share: the
// pinned Traefik ingress controller and the pinned metrics-server. Application
// infrastructure (Dolt, Chroma, Ollama) stays in the application charts. Each
// install logs one phase per step and pulls its image only when the pinned
// digest is not already local; the platform runner bounds every command.
func BootPlatform(run CommandRunner, cluster string) error {
	if err := InstallIngress(run, cluster); err != nil {
		return fmt.Errorf("install ingress: %w", err)
	}
	// The platform is deleted whole, so the per-install cleanup is not needed.
	if _, err := InstallMetricsServer(run, cluster); err != nil {
		return fmt.Errorf("install metrics-server: %w", err)
	}
	// fake-GCS is a platform boot service, not a per-application add-on: one
	// filesystem-backed object store every application namespace binds by
	// endpoint (srd008 R2; #2477 R1). Its cleanup is not needed because the
	// platform is deleted whole and its data lives on the host mount.
	if _, err := InstallFakeGCS(run, cluster); err != nil {
		return fmt.Errorf("install fake-gcs: %w", err)
	}
	return nil
}

// managedApplicationNamespaces lists the da-<application> namespaces on the
// platform, excluding the platform's own conformance namespace, so a teardown
// can refuse while applications still run on it (#2477 R7).
func managedApplicationNamespaces(run CommandRunner) ([]string, error) {
	out, err := run("kubectl", "get", "namespaces",
		"-l", "app.kubernetes.io/managed-by=apprig",
		"-o", "jsonpath={.items[*].metadata.name}")
	if err != nil {
		return nil, fmt.Errorf("list managed application namespaces: %w: %s",
			err, strings.TrimSpace(string(out)))
	}
	var namespaces []string
	for _, name := range strings.Fields(string(out)) {
		if name != "" {
			namespaces = append(namespaces, name)
		}
	}
	return namespaces, nil
}

// ResetPlatform is the explicit destructive operation that deletes the retained
// local object store. It refuses while da-platform is running or a managed
// application namespace remains, so retained data is never deleted as a side
// effect of an ordinary teardown; there is no force bypass in this release
// (srd008 R4.3; #2477 R6, R4).
func ResetPlatform(run Runner, bind func(string) (CommandRunner, func(), error)) error {
	if Exists(run, PlatformClusterName) {
		if bind != nil {
			if command, cleanup, err := bind(PlatformClusterName); err == nil {
				defer cleanup()
				if namespaces, nsErr := managedApplicationNamespaces(command); nsErr == nil && len(namespaces) > 0 {
					return fmt.Errorf(
						"platform:reset refuses: managed application namespaces still present (%s); "+
							"run app:down for each, then platform:down, before reset",
						strings.Join(namespaces, ", "))
				}
			}
		}
		return fmt.Errorf(
			"platform:reset refuses while %s is running; run platform:down first, then platform:reset",
			PlatformClusterName)
	}
	dir, err := PlatformObjectsDir()
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("platform:reset: delete object store %s: %w", dir, err)
	}
	fmt.Printf("platform:reset: deleted retained local object store %s\n", dir)
	return nil
}

// PlatformConformance proves the cluster tier before any application gate:
// the data plane and /readyz answer, the storage provisioner binds a claim,
// Traefik routes an Ingress to a Service, and a torn-down scenario namespace
// leaves no namespace or volume behind. The error names the failed check.
func PlatformConformance(run CommandRunner, cluster string) (result error) {
	if err := VerifyDataPlane(run); err != nil {
		return fmt.Errorf("readiness: %w", err)
	}
	namespace, err := PrepareScenarioNamespace(
		run, PlatformConformanceScenario, platformConformanceRelease)
	if err != nil {
		return fmt.Errorf("namespace churn: %w", err)
	}
	released := false
	defer func() {
		if !released {
			result = errors.Join(result, namespace.Release())
		}
		result = errors.Join(result, resetCurrentNamespace(run))
	}()

	manifest := strings.ReplaceAll(platformConformanceManifest,
		traefikImagePlaceholder, traefikRuntimeRepository+":"+traefikImageVersion)
	manifest = strings.ReplaceAll(manifest, platformHostPlaceholder, platformConformanceHost)
	path, removeManifest, err := writeTempManifest("kindrig-platform-conformance-*.yaml", manifest)
	if err != nil {
		return err
	}
	defer removeManifest()
	if err := runChecked(run, "kubectl", "apply", "--namespace", namespace.Name, "-f", path); err != nil {
		return fmt.Errorf("apply conformance workload: %w", err)
	}

	if err := runChecked(run, "kubectl", "wait", "--namespace", namespace.Name,
		"--for=jsonpath={.status.phase}=Bound", "persistentvolumeclaim/conformance-claim",
		"--timeout="+platformWaitTimeout); err != nil {
		return fmt.Errorf("storage provisioner: %w", err)
	}
	volume, err := run("kubectl", "get", "persistentvolumeclaim", "conformance-claim",
		"--namespace", namespace.Name, "-o", "jsonpath={.spec.volumeName}")
	if err != nil || strings.TrimSpace(string(volume)) == "" {
		return fmt.Errorf("storage provisioner: bound claim names no volume: %v: %s", err, volume)
	}

	if err := runChecked(run, "kubectl", "rollout", "status", "deployment/conformance-echo",
		"--namespace", namespace.Name, "--timeout="+platformWaitTimeout); err != nil {
		return fmt.Errorf("ingress route: backend rollout: %w", err)
	}
	if err := runChecked(run, "docker", "exec", cluster+"-control-plane",
		"curl", "--fail", "--silent", "--show-error", "--max-time", "5",
		"--retry", "30", "--retry-delay", "1", "--retry-all-errors",
		"--header", "Host: "+platformConformanceHost, "http://127.0.0.1/ping"); err != nil {
		return fmt.Errorf("ingress route: Traefik did not route %s/ping: %w",
			platformConformanceHost, err)
	}

	released = true
	if err := namespace.Release(); err != nil {
		return fmt.Errorf("namespace churn: %w", err)
	}
	if err := runChecked(run, "kubectl", "wait", "--for=delete",
		"persistentvolume/"+strings.TrimSpace(string(volume)),
		"--timeout="+platformWaitTimeout); err != nil {
		return fmt.Errorf("namespace churn: volume %s outlived its namespace: %w",
			strings.TrimSpace(string(volume)), err)
	}
	return nil
}

// PlatformCommandBinding binds kubectl/docker commands to a cluster's private
// kubeconfig, the same binding StartPlatform uses. platform:down and
// platform:reset pass it so their managed-namespace check runs against the
// platform rather than the ambient context (#2477 R7, R9).
func PlatformCommandBinding(cluster string) (CommandRunner, func(), error) {
	commands, cleanup, err := ClusterCommands(CaptureRun, cluster)
	if err != nil {
		return nil, nil, err
	}
	return boundedCommandRunner(commands.RunContext, platformCommandTimeout), cleanup, nil
}

func (o PlatformOptions) withDefaults() PlatformOptions {
	if o.KindRun == nil {
		o.KindRun = DefaultRun
	}
	if o.commandTimeout == 0 {
		o.commandTimeout = platformCommandTimeout
	}
	if o.Bind == nil {
		timeout := o.commandTimeout
		o.Bind = func(cluster string) (CommandRunner, func(), error) {
			commands, cleanup, err := ClusterCommands(CaptureRun, cluster)
			if err != nil {
				return nil, nil, err
			}
			return boundedCommandRunner(commands.RunContext, timeout), cleanup, nil
		}
	}
	if o.boot == nil {
		o.boot = BootPlatform
	}
	if o.conformance == nil {
		o.conformance = PlatformConformance
	}
	return o
}

func platformEvidenceNamespaces() []string {
	conformance, _ := ScenarioNamespaceName(PlatformConformanceScenario)
	return []string{"kube-system", "traefik", conformance}
}

// resetCurrentNamespace points the bound kubeconfig back at default, since the
// conformance namespace no longer exists once the suite returns.
func resetCurrentNamespace(run CommandRunner) error {
	if err := runChecked(run, "kubectl", "config", "set-context", "--current",
		"--namespace", "default"); err != nil {
		return fmt.Errorf("reset current namespace: %w", err)
	}
	return nil
}

func runChecked(run CommandRunner, name string, args ...string) error {
	if output, err := run(name, args...); err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "),
			err, strings.TrimSpace(string(output)))
	}
	return nil
}

func writeTempManifest(pattern, manifest string) (string, func(), error) {
	file, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", nil, fmt.Errorf("create manifest: %w", err)
	}
	path := file.Name()
	cleanup := func() { _ = os.Remove(path) }
	if _, err := file.WriteString(manifest); err != nil {
		_ = file.Close()
		cleanup()
		return "", nil, fmt.Errorf("write manifest: %w", err)
	}
	if err := file.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("close manifest: %w", err)
	}
	return path, cleanup, nil
}

// boundedCommandRunner runs each command under its own deadline, so a command
// that never returns fails with the deadline named instead of stalling the
// release that booted the platform (GH-2226).
func boundedCommandRunner(
	run func(context.Context, string, ...string) ([]byte, error),
	timeout time.Duration,
) CommandRunner {
	return func(name string, args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		output, err := run(ctx, name, args...)
		if ctx.Err() == context.DeadlineExceeded {
			return output, fmt.Errorf("%s %s: no result within %s: %w",
				name, strings.Join(args, " "), timeout, ctx.Err())
		}
		return output, err
	}
}
