// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os/exec"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// This is the check that would have caught GH-502. Each template was individually
// correct there -- the ingress named a real Service, the executor NetworkPolicy
// named a real selector -- and only the two read together showed the browser route
// was dead: an ingress-controller pod carries no creator label, so it could never
// reach the apply port the ingress pointed it at.
//
// So these assert over the rendered chart, not the template text, and they assert
// the route and the policy agree (srd003 R4.4/R4.5, srd004 R1.5, srd006 R4.1/R4.4).

type renderedDoc struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name   string            `yaml:"name"`
		Labels map[string]string `yaml:"labels"`
	} `yaml:"metadata"`
	Spec struct {
		// Ingress
		Rules []struct {
			HTTP struct {
				Paths []struct {
					Path    string `yaml:"path"`
					Backend struct {
						Service struct {
							Name string `yaml:"name"`
							Port struct {
								Name string `yaml:"name"`
							} `yaml:"port"`
						} `yaml:"service"`
					} `yaml:"backend"`
				} `yaml:"paths"`
			} `yaml:"http"`
		} `yaml:"rules"`
		// NetworkPolicy
		PodSelector struct {
			MatchLabels map[string]string `yaml:"matchLabels"`
		} `yaml:"podSelector"`
		Ingress []struct {
			From []struct {
				PodSelector *struct {
					MatchLabels map[string]string `yaml:"matchLabels"`
				} `yaml:"podSelector"`
				NamespaceSelector *struct {
					MatchLabels map[string]string `yaml:"matchLabels"`
				} `yaml:"namespaceSelector"`
			} `yaml:"from"`
			Ports []struct {
				Port any `yaml:"port"`
			} `yaml:"ports"`
		} `yaml:"ingress"`
	} `yaml:"spec"`
}

// renderMesh templates the chart with the given --set overrides and splits the
// multi-document output.
func renderMesh(t *testing.T, sets ...string) []renderedDoc {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chartDir := findChartDir(t)
	args := []string{"template", "rel", chartDir}
	for _, s := range sets {
		args = append(args, "--set", s)
	}
	out, err := exec.Command("helm", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}

	var docs []renderedDoc
	for _, chunk := range strings.Split(string(out), "\n---") {
		var doc renderedDoc
		if err := yaml.Unmarshal([]byte(chunk), &doc); err != nil {
			continue // a chunk that is not a manifest, such as the NOTES preamble
		}
		if doc.Kind != "" {
			docs = append(docs, doc)
		}
	}
	if len(docs) == 0 {
		t.Fatal("no manifests parsed from the rendered chart")
	}
	return docs
}

func findByKindComponent(docs []renderedDoc, kind, component string) *renderedDoc {
	for i := range docs {
		if docs[i].Kind == kind && docs[i].Metadata.Labels["app.kubernetes.io/component"] == component {
			return &docs[i]
		}
	}
	return nil
}

// provisioningBackend returns the Service name and port the /provisioning prefix
// routes to, and whether the rule is present at all.
func provisioningBackend(docs []renderedDoc) (service, port string, found bool) {
	for _, doc := range docs {
		if doc.Kind != "Ingress" {
			continue
		}
		for _, rule := range doc.Spec.Rules {
			for _, p := range rule.HTTP.Paths {
				if p.Path == "/provisioning" {
					return p.Backend.Service.Name, p.Backend.Service.Port.Name, true
				}
			}
		}
	}
	return "", "", false
}

// TestProvisioningIngressRoutesToTheCoordinator pins the route half. The panel's
// intake is the coordinator (srd004 R1.5); pointing this prefix at the executor's
// apply Service is the GH-502 defect.
func TestProvisioningIngressRoutesToTheCoordinator(t *testing.T) {
	docs := renderMesh(t, "controlPlane.enabled=true")

	service, port, found := provisioningBackend(docs)
	if !found {
		t.Fatal("no /provisioning ingress rule rendered with the control plane enabled")
	}
	if !strings.HasSuffix(service, "-coordinator") {
		t.Errorf("/provisioning routes to %q; the panel's intake is the coordinator, and routing it at the executor is what GH-502 fixed", service)
	}
	if strings.Contains(service, "executor") {
		t.Errorf("/provisioning routes to the executor Service %q, whose NetworkPolicy admits only creator-labelled pods", service)
	}
	if port != "intent" {
		t.Errorf("/provisioning targets port %q, want intent", port)
	}
}

// TestExecutorApplyStaysCreatorOnly pins the policy half. The executor policy was
// never the defect and must not be widened to admit the ingress controller --
// that would delete the authority boundary rather than route around it.
func TestExecutorApplyStaysCreatorOnly(t *testing.T) {
	docs := renderMesh(t, "controlPlane.enabled=true")

	policy := findByKindComponent(docs, "NetworkPolicy", "executor")
	if policy == nil {
		t.Fatal("no executor NetworkPolicy rendered")
	}
	if len(policy.Spec.Ingress) == 0 {
		t.Fatal("executor NetworkPolicy admits nothing; the apply surface would be unreachable even by the creator")
	}

	for _, rule := range policy.Spec.Ingress {
		for _, from := range rule.From {
			if from.NamespaceSelector != nil {
				t.Errorf("executor policy admits a namespaceSelector %v; the apply surface is creator-only (srd006 R4.1)", from.NamespaceSelector.MatchLabels)
			}
			if from.PodSelector == nil {
				t.Error("executor policy has a from entry with no podSelector, which would widen the apply surface")
				continue
			}
			if got := from.PodSelector.MatchLabels["app.kubernetes.io/component"]; got != "creator" {
				t.Errorf("executor policy admits component %q on the apply surface, want creator only", got)
			}
		}
		for _, p := range rule.Ports {
			if p.Port != "apply" {
				t.Errorf("executor policy opens port %v, want apply only", p.Port)
			}
		}
	}
}

// TestCoordinatorAdmitsOnlyTheIngressControllerOnIntent proves the new opening is
// narrow. The intake is browser-reachable by design; the monitor and control
// ports are not, and an open port would be as wrong here as a widened executor
// policy.
func TestCoordinatorAdmitsOnlyTheIngressControllerOnIntent(t *testing.T) {
	docs := renderMesh(t, "controlPlane.enabled=true")

	policy := findByKindComponent(docs, "NetworkPolicy", "coordinator")
	if policy == nil {
		t.Fatal("no coordinator NetworkPolicy rendered; the intake would be open to the whole cluster")
	}
	if len(policy.Spec.Ingress) != 1 {
		t.Fatalf("coordinator policy has %d ingress rules, want exactly 1", len(policy.Spec.Ingress))
	}

	rule := policy.Spec.Ingress[0]
	if len(rule.From) != 1 {
		t.Fatalf("coordinator policy admits %d sources, want exactly the ingress controller", len(rule.From))
	}
	from := rule.From[0]
	if from.NamespaceSelector == nil || len(from.NamespaceSelector.MatchLabels) == 0 {
		t.Error("coordinator policy admits any namespace; the ingress controller's namespace must be named")
	}
	if from.PodSelector == nil || len(from.PodSelector.MatchLabels) == 0 {
		t.Error("coordinator policy admits any pod in the namespace; the ingress controller must be named")
	}

	if len(rule.Ports) != 1 {
		t.Fatalf("coordinator policy opens %d ports, want intent only", len(rule.Ports))
	}
	if rule.Ports[0].Port != "intent" {
		t.Errorf("coordinator policy opens port %v, want intent", rule.Ports[0].Port)
	}
	for _, port := range []string{"control", "monitor"} {
		for _, p := range rule.Ports {
			if p.Port == port {
				t.Errorf("coordinator policy opens %q to the ingress controller; only the intake is browser-reachable", port)
			}
		}
	}
}

// TestNoProvisioningRouteWithoutTheControlPlane covers the disabled case. Without
// the coordinator there is no legitimate intake, and rendering a route to a
// surface nothing may reach is exactly what made GH-502 invisible: the manifest
// looked complete while the path was dead.
func TestNoProvisioningRouteWithoutTheControlPlane(t *testing.T) {
	docs := renderMesh(t) // controlPlane.enabled defaults to false

	if service, _, found := provisioningBackend(docs); found {
		t.Errorf("/provisioning renders to %q with the control plane disabled; no intake exists to serve it", service)
	}
	if policy := findByKindComponent(docs, "NetworkPolicy", "coordinator"); policy != nil {
		t.Error("a coordinator NetworkPolicy rendered with the control plane disabled")
	}
	// The executor may still be enabled on its own; its policy must be unaffected.
	if policy := findByKindComponent(docs, "NetworkPolicy", "executor"); policy == nil {
		t.Error("the executor NetworkPolicy stopped rendering when the control plane is disabled")
	}
}

// TestCreatorInstanceIsCoordinatorOnly pins the policy GH-685 added. The creator
// is the only pod the executor admits to its apply surface, so an unconstrained
// instance port was the widest remaining path to apply: reach the creator and it
// reaches the executor for you. The GH-682 kind proof measured that reachability
// before this policy existed.
func TestCreatorInstanceIsCoordinatorOnly(t *testing.T) {
	docs := renderMesh(t, "controlPlane.enabled=true")

	policy := findByKindComponent(docs, "NetworkPolicy", "creator")
	if policy == nil {
		t.Fatal("no creator NetworkPolicy rendered; the instance port is open to the whole cluster")
	}
	if len(policy.Spec.Ingress) != 1 {
		t.Fatalf("creator policy has %d ingress rules, want exactly 1", len(policy.Spec.Ingress))
	}
	rule := policy.Spec.Ingress[0]
	for _, from := range rule.From {
		if from.NamespaceSelector != nil {
			t.Errorf("creator policy admits a namespaceSelector %v; the instance port is coordinator-only", from.NamespaceSelector.MatchLabels)
		}
		if from.PodSelector == nil {
			t.Error("creator policy has a from entry with no podSelector")
			continue
		}
		if got := from.PodSelector.MatchLabels["app.kubernetes.io/component"]; got != "coordinator" {
			t.Errorf("creator policy admits component %q, want coordinator only", got)
		}
	}
	if len(rule.Ports) != 1 || rule.Ports[0].Port != "instance" {
		t.Errorf("creator policy opens %v, want the instance port only", rule.Ports)
	}
}
