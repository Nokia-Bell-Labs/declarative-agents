// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// The GH-502 defect was a route and a policy that disagreed: the ingress pointed
// browser traffic at the executor's apply Service, whose NetworkPolicy admits only
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
// the executor's creator-only rule, which selects a named port, was observed to
// admit traffic under kindnet that Calico blocks. Two CNIs disagreeing about the
// same rule is exactly why the proof names the one it was written against instead
// of trusting the default (GH-682).

const (
	policyKindCluster = "chatbot-mesh-policy"
	policyMeshNS      = "mesh"
	policyIngressNS   = "ingress-nginx"
	policyRelease     = "rel"
	// calicoManifest is pinned: an unpinned CNI would let the proof's meaning
	// drift with an upstream release.
	calicoManifest = "https://raw.githubusercontent.com/projectcalico/calico/v3.28.2/manifests/calico.yaml"
)

// policyKindConfig is the cluster kind creates. disableDefaultCNI drops kindnet,
// whose absence is what lets a policy-enforcing CNI take over; the podSubnet is
// Calico's expected default.
const policyKindConfig = `kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
networking:
  disableDefaultCNI: true
  podSubnet: "192.168.0.0/16"
`

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
			Name:   "ingress controller reaches the coordinator intent port",
			FromNS: policyIngressNS, FromPod: "controller", ToPod: "coordinator", Port: 18100, Want: reached,
			Why: "the intake is the panel's only provisioning entry (srd004 R1.5)",
		},
		{
			Name:   "ingress controller cannot reach the executor apply port",
			FromNS: policyIngressNS, FromPod: "controller", ToPod: "executor", Port: 18090, Want: blocked,
			Why: "the apply surface is creator-only; routing the browser here was GH-502 (srd006 R4.1)",
		},
		{
			Name:   "ingress controller cannot reach the creator instance port",
			FromNS: policyIngressNS, FromPod: "controller", ToPod: "creator", Port: 18110, Want: blocked,
			Why: "the creator is coordinator-facing only, and it is the one pod the executor admits (srd005 R5.4, GH-685)",
		},
		{
			Name:   "coordinator reaches the creator instance port",
			FromNS: policyMeshNS, FromPod: "coordinator", ToPod: "creator", Port: 18110, Want: reached,
			Why: "the control plane chain must still work (srd005 R1.1)",
		},
		{
			Name:   "creator reaches the executor apply port",
			FromNS: policyMeshNS, FromPod: "creator", ToPod: "executor", Port: 18090, Want: reached,
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
		{Component: "coordinator", PortName: "intent", Port: 18100},
		{Component: "creator", PortName: "instance", Port: 18110},
		{Component: "executor", PortName: "apply", Port: 18090},
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
	return runPolicyProof(exampleChartDir(profilesRoot))
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

func runPolicyProof(chartDir string) error {
	cluster, err := ensurePolicyCluster()
	if err != nil {
		return err
	}
	defer cluster.release(defaultKindRun)

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

// ensurePolicyCluster reuses or creates the policy cluster. It uses its own name
// rather than the smoke cluster so the CNI is the one this proof was written
// against; reusing a cluster built with a different CNI would measure a different
// system. Ownership follows the same rule as the other targets -- only a cluster
// this run created may be deleted (GH-589).
//
// A reused cluster is taken on trust that it is this target's own. The self-test
// still runs against it, so a reused cluster that does not enforce is caught; a
// reused cluster that enforces differently is not, which is why the printed notice
// names the risk.
func ensurePolicyCluster() (kindCluster, error) {
	if kindClusterExists(defaultKindRun, policyKindCluster) {
		fmt.Printf("kind: reusing pre-existing cluster %s; it will not be deleted. "+
			"If it was not created by this target its CNI may differ from %s\n",
			policyKindCluster, calicoManifest)
		return kindCluster{Name: policyKindCluster}, nil
	}

	configPath, cleanup, err := writeTempFile("kind-policy-*.yaml", policyKindConfig)
	if err != nil {
		return kindCluster{}, err
	}
	defer cleanup()

	fmt.Printf("policyProof: creating %s with the default CNI disabled\n", policyKindCluster)
	// The node stays NotReady until a CNI lands, so a Ready wait here would always
	// time out. Calico is installed next and the readiness wait follows it.
	if _, err := defaultKindRun("create", "cluster", "--name", policyKindCluster, "--config", configPath); err != nil {
		return kindCluster{}, fmt.Errorf("kind create cluster %s: %w", policyKindCluster, err)
	}
	cluster := kindCluster{Name: policyKindCluster, Created: true}

	fmt.Println("policyProof: installing Calico")
	if err := kubectlPolicy("apply", "-f", calicoManifest); err != nil {
		return cluster, fmt.Errorf("install calico: %w", err)
	}
	if err := kubectlPolicy("-n", "kube-system", "rollout", "status", "daemonset/calico-node", "--timeout=300s"); err != nil {
		return cluster, fmt.Errorf("calico rollout: %w", err)
	}
	if err := kubectlPolicy("wait", "--for=condition=Ready", "node", "--all", "--timeout=300s"); err != nil {
		return cluster, fmt.Errorf("node ready: %w", err)
	}
	return cluster, nil
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

func writeTempFile(pattern, content string) (string, func(), error) {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", func() {}, err
	}
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		return "", func() {}, err
	}
	if err := f.Close(); err != nil {
		return "", func() {}, err
	}
	return f.Name(), func() { _ = os.Remove(f.Name()) }, nil
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
	fmt.Fprintf(&b, "spec:\n  containers:\n    - name: srv\n      image: busybox:1.36\n")
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
	b.WriteString("spec:\n  containers:\n    - name: c\n      image: busybox:1.36\n      command: [\"sleep\",\"3600\"]\n")
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
	// The ingress controller lives in a namespace named ingress-nginx, which
	// Kubernetes auto-labels with kubernetes.io/metadata.name -- the label the
	// coordinator policy's namespaceSelector matches.
	docs = append(docs, sleeperPod(policyIngressNS, "controller",
		map[string]string{"app.kubernetes.io/name": "ingress-nginx"}))
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
