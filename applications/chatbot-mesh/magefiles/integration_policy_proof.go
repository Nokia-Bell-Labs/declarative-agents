// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

// The GH-502 defect was a route and a policy that disagreed: the ingress pointed
// browser traffic at the applier's apply Service, whose NetworkPolicy admits only
// creator-labelled pods. Every template was individually valid, so nothing short of
// a real request on a policy-enforcing cluster could see it.
//
// magefiles/provisioning_route_render_test.go pins what the chart says. This target
// measures what a cluster does (GH-682).
//
// The cluster must enforce NetworkPolicy or the whole proof is vacuous: where
// nothing is ever blocked, every negative assertion below passes for free. So this
// target asserts enforcement is live before asserting anything else, and an
// unenforcing cluster fails rather than skips -- a skip would be indistinguishable
// from a pass, which is the failure mode this proof exists to rule out.
//
// It also pins the CNI rather than taking whatever the cluster has. kindnet, which
// the other kind targets use, does implement NetworkPolicy (kindnetd v20260528 on
// kind v0.32), so the self-test alone would not have caught running on it -- but
// the applier's creator-only rule, which selects a named port, was observed to
// admit traffic under kindnet that Calico blocks. Two CNIs disagreeing about the
// same rule is exactly why the proof names the one it was written against instead
// of trusting the default (GH-682).

const (
	policyKindCluster = "da-chatbot-mesh-policy"
	policyMeshNS      = "mesh"
	policyIngressNS   = "traefik"
	policyRelease     = "rel"
	calicoVersion     = "v3.32.1"
	// calicoManifest is pinned to the immutable commit behind the v3.32.1 tag:
	// an unpinned CNI would let the proof's meaning drift with an upstream release.
	calicoManifestCommit = "0ca9d1b93644778cafdf1812f3dda02ac0c361e8"
	calicoManifest       = "https://raw.githubusercontent.com/projectcalico/calico/" +
		calicoManifestCommit + "/manifests/calico.yaml"
	policyProbeImageRepository = "docker.io/library/busybox"
	policyProbeImageVersion    = "1.36"
	// policyKindConfig is the checked-in cluster configuration: default CNI
	// disabled so Calico can take over, node image pinned (eng01).
	policyKindConfig = "testdata/kind-policy-config.yaml"
)

type calicoImage struct {
	Component  string
	Repository string
	Digests    map[string]string
}

// calicoImages is the complete image set named by the pinned calico.yaml.
// Digests are the architecture-specific manifests beneath each v3.32.1
// multi-platform index, so Docker verifies the exact bytes kind receives.
var calicoImages = []calicoImage{
	{
		Component:  "cni",
		Repository: "quay.io/calico/cni",
		Digests: map[string]string{
			"amd64": "sha256:3ef9bbb3fdb2b3194dff57d7d8496d5e18247afb59606dfc694ab88ed1fa9f86",
			"arm64": "sha256:f83ba4048763b8dbfa95f65b5094e8fb08b7326ce8d465111bb9da416ecb6bdb",
		},
	},
	{
		Component:  "node",
		Repository: "quay.io/calico/node",
		Digests: map[string]string{
			"amd64": "sha256:c061070a27292f8152ae6a0582078eb9059d1b6ed5e57c2052e5c22534734240",
			"arm64": "sha256:9da8e32d2d6f9405be1985f258842bfc808bbf5aca51091bdef8110fca722a1b",
		},
	},
	{
		Component:  "kube-controllers",
		Repository: "quay.io/calico/kube-controllers",
		Digests: map[string]string{
			"amd64": "sha256:df00967cbd6d88e1ff3123e1598895845622e2987928b4ebd9d8ac49aefe00c3",
			"arm64": "sha256:afa3429708de65af587ede22064a7abddf57082edd368066c24781e3b2d30cb5",
		},
	},
}

var policyProbeImageDigests = map[string]string{
	"amd64": "sha256:b7f3d86d6e84fc17718c48bcde1450807faa2d56704205c697b4bd5df7b9e29f",
	"arm64": "sha256:bd44eb136a95dcc8dc58995e43abc40a413f2e8e3d4a2aae6bccbe94686acb05",
}

// reachability is the observable a probe produces. It is deliberately two-valued:
// the assertions care whether a connection completed, not why it did not.
type reachability string

const (
	reached reachability = "REACHED"
	blocked reachability = "BLOCKED"
)

// policyProbe is one reachability claim about the authority boundary.
type policyProbe struct {
	Name    string
	FromNS  string
	FromPod string
	ToPod   string
	Port    int
	Want    reachability
	Why     string
}

// policyProbes are the facts the epic rests on. The two BLOCKED ones are the
// point: they are the assertions that pass for free on an unenforcing cluster,
// which is why enforcement is checked first.
func policyProbes() []policyProbe {
	return []policyProbe{
		{
			Name:   "ingress controller reaches the provisioning-workflow-orchestrator intent port",
			FromNS: policyIngressNS, FromPod: "controller", ToPod: "provisioning-workflow-orchestrator", Port: 18100, Want: reached,
			Why: "the intake is the panel's only provisioning entry (srd004 R1.5)",
		},
		{
			Name:   "ingress controller cannot reach the applier apply port",
			FromNS: policyIngressNS, FromPod: "controller", ToPod: "applier", Port: 18090, Want: blocked,
			Why: "the apply surface is creator-only; routing the browser here was GH-502 (srd006 R4.1)",
		},
		{
			Name:   "ingress controller cannot reach the creator instance port",
			FromNS: policyIngressNS, FromPod: "controller", ToPod: "creator", Port: 18110, Want: blocked,
			Why: "the creator is provisioning-workflow-orchestrator-facing only, and it is the one pod the applier admits (srd005 R5.4, GH-685)",
		},
		{
			Name:   "provisioning-workflow-orchestrator reaches the creator instance port",
			FromNS: policyMeshNS, FromPod: "provisioning-workflow-orchestrator", ToPod: "creator", Port: 18110, Want: reached,
			Why: "the control plane chain must still work (srd005 R1.1)",
		},
		{
			Name:   "creator reaches the applier apply port",
			FromNS: policyMeshNS, FromPod: "creator", ToPod: "applier", Port: 18090, Want: reached,
			Why: "the creator alone realizes an apply (srd006 R4.1)",
		},
	}
}

// standInPod is a pod carrying the labels and named container port a rendered
// NetworkPolicy selects on. The policies are the artifact under test; these pods
// only need the identity the policies name and a socket to connect to, so the
// proof needs no runtime image and no live agent.
type standInPod struct {
	Component string
	PortName  string
	Port      int
}

func policyStandIns() []standInPod {
	return []standInPod{
		{Component: "provisioning-workflow-orchestrator", PortName: "intent", Port: 18100},
		{Component: "creator", PortName: "instance", Port: 18110},
		{Component: "applier", PortName: "apply", Port: 18090},
	}
}

// PolicyProof proves the provisioning authority boundary on a cluster that
// enforces NetworkPolicy (GH-502, GH-682).
func (Integration) PolicyProof() error {
	profilesRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	if reason := policyProofSkipReason(); reason != "" {
		fmt.Printf("SKIP policyProof: %s\n", reason)
		return nil
	}
	return runPolicyProof(applicationChartDir(profilesRoot))
}

// policyProofSkipReason reports missing tooling. Absent tooling is a skip; an
// unenforcing cluster is not, because that is the condition the proof exists to
// rule out and skipping it would look identical to passing.
func policyProofSkipReason() string {
	for _, bin := range []string{"docker", "kind", "helm", "kubectl"} {
		if _, err := exec.LookPath(bin); err != nil {
			return fmt.Sprintf("%s not found on PATH", bin)
		}
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		return "docker daemon is not running"
	}
	return ""
}

type policyProofExecution struct {
	acquire func() (kindrig.Cluster, error)
	release func(kindrig.Cluster, bool)
	prove   func(string) error
}

func runPolicyProof(chartDir string) error {
	evidence := policyFailureEvidence(chartDir, kindrig.DefaultCommandRun)
	return executePolicyProof(chartDir, policyProofExecution{
		acquire: ensurePolicyCluster,
		release: func(cluster kindrig.Cluster, failed bool) {
			cluster.ReleaseAfter(kindrig.DefaultRun, failed, evidence)
		},
		prove: provePolicyBoundary,
	})
}

func policyFailureEvidence(
	chartDir string,
	run kindrig.CommandRunner,
) kindrig.FailureEvidence {
	return kindrig.FailureEvidence{
		Directory: policyEvidenceDirectory(chartDir),
		Namespaces: []string{
			"kube-system",
			policyMeshNS,
			policyIngressNS,
		},
		Run: policyDiagnosticRunner(run),
	}
}

// executePolicyProof registers cleanup before it observes bootstrapErr. Cluster
// creation can succeed before Calico delivery fails, in which case acquire
// deliberately returns an owned Cluster alongside the error.
func executePolicyProof(
	chartDir string,
	execution policyProofExecution,
) (result error) {
	cluster, bootstrapErr := execution.acquire()
	defer func() {
		execution.release(cluster, result != nil)
	}()
	if bootstrapErr != nil {
		return bootstrapErr
	}
	return execution.prove(chartDir)
}

func provePolicyBoundary(chartDir string) error {
	if err := assertPolicyEnforcementActive(); err != nil {
		return err
	}
	if err := applyRenderedPolicies(chartDir); err != nil {
		return err
	}
	if err := applyPolicyStandIns(); err != nil {
		return err
	}
	return assertPolicyProbes()
}

func policyEvidenceDirectory(chartDir string) string {
	return filepath.Join(
		filepath.Dir(chartDir),
		"build",
		"kind-evidence",
		policyKindCluster+"-"+time.Now().UTC().Format("20060102T150405.000000000Z"),
	)
}

// policyDiagnosticRunner binds kindrig's generic kubectl evidence commands to
// this proof's cluster rather than trusting the ambient current context.
func policyDiagnosticRunner(run kindrig.CommandRunner) kindrig.CommandRunner {
	return func(name string, args ...string) ([]byte, error) {
		if name == "kubectl" {
			args = append([]string{"--context", policyKubeContext()}, args...)
		}
		return run(name, args...)
	}
}

// ensurePolicyCluster reuses or creates the policy cluster. It uses its own name
// rather than the smoke cluster so the CNI is the one this proof was written
// against; reusing a cluster built with a different CNI would measure a different
// system. Reuse still passes through kindrig's generated-kubeconfig API health
// check. Ownership follows the same rule as the other targets -- only a cluster
// this run created may be deleted (GH-589).
//
// A reused cluster is taken on trust that it is this target's own. The self-test
// still runs against it, so a reused cluster that does not enforce is caught; a
// reused cluster that enforces differently is not, which is why the printed notice
// names the risk.
func ensurePolicyCluster() (kindrig.Cluster, error) {
	return ensurePolicyClusterWith(
		kindrig.DefaultRun,
		kindrig.DefaultCommandRun,
		runtime.GOARCH,
	)
}

func ensurePolicyClusterWith(
	kindRun kindrig.Runner,
	commandRun kindrig.CommandRunner,
	arch string,
) (kindrig.Cluster, error) {
	// The node stays NotReady until a CNI lands, so a Ready wait here would always
	// time out (wait 0). A reused cluster is nevertheless API-health checked by
	// EnsureCluster before it is returned.
	cluster, err := kindrig.EnsureCluster(
		kindRun, policyKindCluster, policyKindConfig, 0)
	if err != nil {
		return kindrig.Cluster{}, err
	}
	if !cluster.Created {
		fmt.Printf("kind: reusing pre-existing cluster %s; it will not be deleted. "+
			"If it was not created by this target its CNI may differ from %s\n",
			policyKindCluster, calicoManifest)
		return cluster, nil
	}

	fmt.Printf("policyProof: created %s with the default CNI disabled\n", policyKindCluster)
	fmt.Printf("policyProof: installing Calico %s from locally loaded images\n", calicoVersion)
	if err := installCalico(commandRun, cluster.Name, arch); err != nil {
		return cluster, err
	}
	fmt.Printf("policyProof: loading the policy probe image into %s\n", cluster.Name)
	if err := preloadPolicyProbeImage(commandRun, cluster.Name, arch); err != nil {
		return cluster, err
	}
	return cluster, nil
}

func installCalico(run kindrig.CommandRunner, cluster, arch string) error {
	if strings.TrimSpace(cluster) == "" {
		return fmt.Errorf("install Calico %s: kind cluster name is required", calicoVersion)
	}
	for _, image := range calicoImages {
		source, runtimeImage, err := calicoImageRefs(image, arch)
		if err != nil {
			return err
		}
		commands := [][]string{
			{"docker", "pull", "--platform", "linux/" + arch, source},
			{"docker", "tag", source, runtimeImage},
			{"kind", "load", "docker-image", runtimeImage, "--name", cluster},
		}
		for _, command := range commands {
			if err := runCalicoCommand(
				run, image.Component, command[0], command[1:]...,
			); err != nil {
				return err
			}
		}
	}

	contextArgs := []string{"--context", "kind-" + cluster}
	commands := []struct {
		component string
		args      []string
	}{
		{
			component: "manifest",
			args:      append(contextArgs, "apply", "-f", calicoManifest),
		},
		{
			component: "node rollout",
			args: append(contextArgs, "-n", "kube-system", "rollout", "status",
				"daemonset/calico-node", "--timeout=300s"),
		},
		{
			component: "kube-controllers rollout",
			args: append(contextArgs, "-n", "kube-system", "rollout", "status",
				"deployment/calico-kube-controllers", "--timeout=300s"),
		},
		{
			component: "node readiness",
			args: append(contextArgs, "wait", "--for=condition=Ready", "node",
				"--all", "--timeout=300s"),
		},
	}
	for _, command := range commands {
		if err := runCalicoCommand(run, command.component, "kubectl", command.args...); err != nil {
			return err
		}
	}
	return nil
}

func calicoImageRefs(image calicoImage, arch string) (source, runtimeImage string, err error) {
	digest, ok := image.Digests[arch]
	if !ok {
		return "", "", fmt.Errorf(
			"install Calico %s %s: no pinned digest for linux/%s",
			calicoVersion, image.Component, arch,
		)
	}
	runtimeImage = image.Repository + ":" + calicoVersion
	return runtimeImage + "@" + digest, runtimeImage, nil
}

func runCalicoCommand(
	run kindrig.CommandRunner,
	component, name string,
	args ...string,
) error {
	output, err := run(name, args...)
	if err == nil {
		return nil
	}
	command := strings.TrimSpace(name + " " + strings.Join(args, " "))
	if detail := strings.TrimSpace(string(output)); detail != "" {
		return fmt.Errorf(
			"install Calico %s %s: %s: %w: %s",
			calicoVersion, component, command, err, detail,
		)
	}
	return fmt.Errorf(
		"install Calico %s %s: %s: %w",
		calicoVersion, component, command, err,
	)
}

func preloadPolicyProbeImage(
	run kindrig.CommandRunner,
	cluster, arch string,
) error {
	source, runtimeImage, err := policyProbeImageRefs(arch)
	if err != nil {
		return err
	}
	commands := [][]string{
		{"docker", "pull", "--platform", "linux/" + arch, source},
		{"docker", "tag", source, runtimeImage},
		{"kind", "load", "docker-image", runtimeImage, "--name", cluster},
	}
	for _, command := range commands {
		output, commandErr := run(command[0], command[1:]...)
		if commandErr == nil {
			continue
		}
		invocation := strings.Join(command, " ")
		if detail := strings.TrimSpace(string(output)); detail != "" {
			return fmt.Errorf(
				"load policy probe image %s: %s: %w: %s",
				runtimeImage, invocation, commandErr, detail,
			)
		}
		return fmt.Errorf(
			"load policy probe image %s: %s: %w",
			runtimeImage, invocation, commandErr,
		)
	}
	return nil
}

func policyProbeImageRefs(arch string) (source, runtimeImage string, err error) {
	digest, ok := policyProbeImageDigests[arch]
	if !ok {
		return "", "", fmt.Errorf(
			"load policy probe image %s:%s: no pinned digest for linux/%s",
			policyProbeImageRepository, policyProbeImageVersion, arch,
		)
	}
	runtimeImage = policyProbeImageRepository + ":" + policyProbeImageVersion
	return runtimeImage + "@" + digest, runtimeImage, nil
}

// assertPolicyEnforcementActive proves the cluster actually enforces NetworkPolicy
// before anything is measured. It reaches a target, applies a default-deny, and
// requires the same request to stop working.
//
// Without this the target's two BLOCKED assertions pass on any cluster that
// ignores policy, and a green run would mean nothing. This is the one check that
// must fail rather than skip.
func assertPolicyEnforcementActive() error {
	fmt.Println("policyProof: verifying NetworkPolicy enforcement is active")
	// A fresh namespace per run. Reusing a fixed name races the previous run's
	// deletion: pods in a terminating namespace are unreachable, which the probe
	// below would misread as the target being broken rather than as leftover state.
	ns := fmt.Sprintf("policy-selftest-%d", os.Getpid())
	if err := kubectlPolicy("create", "namespace", ns); err != nil {
		return fmt.Errorf("create self-test namespace %s: %w", ns, err)
	}
	defer func() { _ = kubectlPolicy("delete", "namespace", ns, "--wait=false") }()

	if err := applyPolicyYAML(selfTestPodsYAML(ns)); err != nil {
		return err
	}
	if err := kubectlPolicy("-n", ns, "wait", "--for=condition=Ready", "pod", "--all", "--timeout=180s"); err != nil {
		return fmt.Errorf("self-test pods not ready: %w", err)
	}
	ip, err := podIP(ns, "selftest-target")
	if err != nil {
		return err
	}

	if got := probeReachability(ns, "selftest-client", ip, 8080); got != reached {
		return fmt.Errorf("policy self-test: the target was unreachable before any policy was applied (%s); the probe itself is broken, so no conclusion about enforcement is possible", got)
	}
	if err := applyPolicyYAML(selfTestDenyYAML(ns)); err != nil {
		return err
	}
	// Give the CNI a moment to program the deny before concluding it does not.
	time.Sleep(5 * time.Second)
	if got := probeReachability(ns, "selftest-client", ip, 8080); got != blocked {
		return fmt.Errorf("policy self-test: a default-deny NetworkPolicy did not block traffic (%s). This cluster does not enforce NetworkPolicy, so every negative assertion in this proof would pass vacuously. Recreate %s with a policy-enforcing CNI", got, policyKindCluster)
	}
	fmt.Println("policyProof: enforcement confirmed (deny blocked a reachable target)")
	return nil
}

// applyRenderedPolicies applies the chart's own NetworkPolicies. Rendering rather
// than hand-writing them is the point: a policy edited in the chart changes what
// this proof measures.
func applyRenderedPolicies(chartDir string) error {
	out, err := exec.Command("helm", "template", policyRelease, chartDir,
		"--set", "controlPlane.enabled=true").Output()
	if err != nil {
		return fmt.Errorf("helm template: %w", err)
	}
	policies := extractNetworkPolicies(string(out))
	if policies == "" {
		return fmt.Errorf("the rendered chart contains no NetworkPolicy; there is nothing to prove")
	}
	_ = kubectlPolicy("create", "namespace", policyMeshNS)
	_ = kubectlPolicy("create", "namespace", policyIngressNS)
	return applyPolicyYAML(policies, "-n", policyMeshNS)
}

// extractNetworkPolicies keeps only the NetworkPolicy documents from a rendered
// chart, so the proof applies the policies without scheduling the whole mesh.
func extractNetworkPolicies(rendered string) string {
	var kept []string
	for _, doc := range strings.Split(rendered, "\n---") {
		if strings.Contains(doc, "kind: NetworkPolicy") {
			kept = append(kept, doc)
		}
	}
	return strings.Join(kept, "\n---\n")
}

func applyPolicyStandIns() error {
	if err := applyPolicyYAML(standInPodsYAML()); err != nil {
		return err
	}
	if err := kubectlPolicy("-n", policyMeshNS, "wait", "--for=condition=Ready", "pod", "--all", "--timeout=180s"); err != nil {
		return fmt.Errorf("mesh stand-ins not ready: %w", err)
	}
	return kubectlPolicy("-n", policyIngressNS, "wait", "--for=condition=Ready", "pod/controller", "--timeout=180s")
}

func assertPolicyProbes() error {
	var failures []string
	for _, probe := range policyProbes() {
		ip, err := podIP(policyMeshNS, probe.ToPod)
		if err != nil {
			return err
		}
		got := probeReachability(probe.FromNS, probe.FromPod, ip, probe.Port)
		if got == probe.Want {
			fmt.Printf("  PASS  %s (%s)\n", probe.Name, got)
			continue
		}
		fmt.Printf("  FAIL  %s: got %s, want %s\n", probe.Name, got, probe.Want)
		failures = append(failures, fmt.Sprintf("%s: got %s, want %s -- %s", probe.Name, got, probe.Want, probe.Why))
	}
	if len(failures) > 0 {
		return fmt.Errorf("policy proof failed:\n  %s", strings.Join(failures, "\n  "))
	}
	fmt.Printf("policyProof: %d reachability facts hold on an enforcing cluster\n", len(policyProbes()))
	return nil
}

// probeReachability runs one connection attempt from inside a pod. A non-zero
// exit is reported as blocked: the assertions distinguish completed from not
// completed, and a timeout is the shape a NetworkPolicy drop takes.
func probeReachability(ns, pod, ip string, port int) reachability {
	url := fmt.Sprintf("http://%s:%d/", ip, port)
	cmd := exec.Command("kubectl", "--context", policyKubeContext(), "-n", ns, "exec", pod,
		"--", "wget", "-q", "-T", "6", "-O", "-", url)
	if err := cmd.Run(); err != nil {
		return blocked
	}
	return reached
}

func podIP(ns, pod string) (string, error) {
	out, err := exec.Command("kubectl", "--context", policyKubeContext(), "-n", ns,
		"get", "pod", pod, "-o", "jsonpath={.status.podIP}").Output()
	if err != nil {
		return "", fmt.Errorf("read %s/%s pod IP: %w", ns, pod, err)
	}
	ip := strings.TrimSpace(string(out))
	if ip == "" {
		return "", fmt.Errorf("%s/%s has no pod IP", ns, pod)
	}
	return ip, nil
}

func policyKubeContext() string { return "kind-" + policyKindCluster }

func kubectlPolicy(args ...string) error {
	full := append([]string{"--context", policyKubeContext()}, args...)
	cmd := exec.Command("kubectl", full...)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	return cmd.Run()
}

func applyPolicyYAML(manifest string, extra ...string) error {
	args := append([]string{"--context", policyKubeContext(), "apply", "-f", "-"}, extra...)
	cmd := exec.Command("kubectl", args...)
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	return cmd.Run()
}

// httpdPod renders a pod that serves its own name over HTTP on one named port.
// The port name matters: the rendered policies select named ports, which resolve
// against the pod's container port names.
func httpdPod(ns, name, portName string, port int, labels map[string]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "apiVersion: v1\nkind: Pod\nmetadata:\n  name: %s\n  namespace: %s\n  labels:\n", name, ns)
	for k, v := range labels {
		fmt.Fprintf(&b, "    %s: %s\n", k, v)
	}
	fmt.Fprintf(&b, "spec:\n  containers:\n    - name: srv\n      image: %s\n", policyProbeImageRepository+":"+policyProbeImageVersion)
	b.WriteString("      imagePullPolicy: Never\n")
	fmt.Fprintf(&b, "      command: [\"sh\",\"-c\",\"mkdir -p /w && echo %s > /w/index.html && httpd -f -p %d -h /w\"]\n", name, port)
	fmt.Fprintf(&b, "      ports:\n        - {name: %s, containerPort: %d}\n", portName, port)
	return b.String()
}

func sleeperPod(ns, name string, labels map[string]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "apiVersion: v1\nkind: Pod\nmetadata:\n  name: %s\n  namespace: %s\n  labels:\n", name, ns)
	for k, v := range labels {
		fmt.Fprintf(&b, "    %s: %s\n", k, v)
	}
	fmt.Fprintf(&b, "spec:\n  containers:\n    - name: c\n      image: %s\n", policyProbeImageRepository+":"+policyProbeImageVersion)
	b.WriteString("      imagePullPolicy: Never\n      command: [\"sleep\",\"3600\"]\n")
	return b.String()
}

// meshLabels are the selector labels the rendered policies match on, for one
// component. They must track the chart's selectorLabels helper.
func meshLabels(component string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":      "chatbot-mesh",
		"app.kubernetes.io/instance":  policyRelease,
		"app.kubernetes.io/component": component,
	}
}

func standInPodsYAML() string {
	var docs []string
	for _, pod := range policyStandIns() {
		docs = append(docs, httpdPod(policyMeshNS, pod.Component, pod.PortName, pod.Port, meshLabels(pod.Component)))
	}
	// The ingress controller lives in a namespace named traefik, which
	// Kubernetes auto-labels with kubernetes.io/metadata.name -- the label the
	// provisioning-workflow-orchestrator policy's namespaceSelector matches.
	docs = append(docs, sleeperPod(policyIngressNS, "controller",
		map[string]string{"app.kubernetes.io/name": "traefik"}))
	return strings.Join(docs, "---\n")
}

func selfTestPodsYAML(ns string) string {
	return strings.Join([]string{
		httpdPod(ns, "selftest-target", "http", 8080, map[string]string{"role": "target"}),
		sleeperPod(ns, "selftest-client", map[string]string{"role": "client"}),
	}, "---\n")
}

func selfTestDenyYAML(ns string) string {
	return fmt.Sprintf(`apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: selftest-deny
  namespace: %s
spec:
  podSelector:
    matchLabels:
      role: target
  policyTypes: [Ingress]
  ingress: []
`, ns)
}
