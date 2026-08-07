// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The observer's monitor fan-in dials a discovered pod's address at a port the
// pod publishes as a label, and its REST client authorizes that port against a
// literal allowlist (srd028 R14.7). That allowlist restates the chart's
// per-component monitor ports, because the profile ConfigMap mounts declarations
// verbatim and srd008 R1.6 bars an environment variable, so nothing in the
// runtime can notice the two disagreeing. A port changed in values.yaml simply
// stops being reachable, at request time, on a cluster.
//
// These assert over the rendered chart rather than the template text, and they
// assert the three facts that have to agree: a workload serving a monitor port
// publishes that port as its label, the observer's allowlist covers it, and the
// allowlist claims nothing the chart does not serve (GH-1471).

// monitorWorkload is the part of a rendered workload this check reads.
type monitorWorkload struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name   string            `yaml:"name"`
		Labels map[string]string `yaml:"labels"`
	} `yaml:"metadata"`
	Spec struct {
		Template struct {
			Metadata struct {
				Labels map[string]string `yaml:"labels"`
			} `yaml:"metadata"`
			Spec struct {
				Containers []struct {
					Name  string `yaml:"name"`
					Ports []struct {
						Name          string `yaml:"name"`
						ContainerPort int    `yaml:"containerPort"`
					} `yaml:"ports"`
				} `yaml:"containers"`
			} `yaml:"spec"`
		} `yaml:"template"`
	} `yaml:"spec"`
}

// monitorPortLabel is the dot-free pod label carrying a workload's monitor port.
// The selector grammar walks maps and splits on dots, so neither the container
// port array nor a dotted label key is addressable; this label is how the port
// reaches the fan-in (srd008 R2.1).
const monitorPortLabel = "monitorPort"

// servedMonitorPort is one workload's monitor surface as the chart renders it.
type servedMonitorPort struct {
	Workload      string
	ContainerPort int
	Label         string
}

// renderMonitorWorkloads templates the chart with every agent component enabled
// and returns the workloads that serve a monitor port.
func renderMonitorWorkloads(t *testing.T, sets ...string) []servedMonitorPort {
	t.Helper()
	args := append([]string{
		"observer.enabled=true",
		"applier.enabled=true",
		"controlPlane.enabled=true",
	}, sets...)
	var served []servedMonitorPort
	for _, doc := range renderChartDocs(t, args...) {
		port, ok := containerMonitorPort(doc)
		if !ok {
			continue
		}
		served = append(served, servedMonitorPort{
			Workload:      doc.Metadata.Name,
			ContainerPort: port,
			Label:         doc.Spec.Template.Metadata.Labels[monitorPortLabel],
		})
	}
	if len(served) == 0 {
		t.Fatal("no workload in the rendered chart serves a monitor port")
	}
	sort.Slice(served, func(i, j int) bool { return served[i].Workload < served[j].Workload })
	return served
}

func containerMonitorPort(doc monitorWorkload) (int, bool) {
	for _, container := range doc.Spec.Template.Spec.Containers {
		for _, port := range container.Ports {
			if port.Name == "monitor" {
				return port.ContainerPort, true
			}
		}
	}
	return 0, false
}

// helmTemplateOutput renders the chart with the given --set overrides and returns
// the raw multi-document output. renderMesh and renderChartDocs decode different
// projections of the same render, so the invocation lives here once.
func helmTemplateOutput(t *testing.T, sets ...string) string {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	args := []string{"template", "rel", findChartDir(t)}
	for _, set := range sets {
		args = append(args, "--set", set)
	}
	out, err := exec.Command("helm", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	return string(out)
}

// renderChartDocs templates the chart and decodes each manifest into the shape
// this check reads.
func renderChartDocs(t *testing.T, sets ...string) []monitorWorkload {
	t.Helper()
	out := helmTemplateOutput(t, sets...)
	var docs []monitorWorkload
	for _, chunk := range strings.Split(out, "\n---") {
		var doc monitorWorkload
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

// observerAllowedMonitorPorts reads the observer's authorized fan-in ports from
// its REST declaration. The file is not plain YAML until load-time expansion --
// ${OBSERVER_CONTROL_PORT:-18201} puts a brace inside a flow sequence -- so the
// ${NAME:-default} references are replaced with their defaults first, which is
// what the runtime resolves them to when nothing sets them.
func observerAllowedMonitorPorts(t *testing.T) []int {
	t.Helper()
	path := filepath.Join(findApplicationRoot(t), "agents", "observer", "rest.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read observer rest declaration: %v", err)
	}
	var declaration struct {
		Rest struct {
			Limits map[string]struct {
				Network struct {
					Ports []int `yaml:"ports"`
				} `yaml:"network"`
			} `yaml:"limits"`
		} `yaml:"rest"`
	}
	if err := yaml.Unmarshal(expandDeclarationDefaults(raw), &declaration); err != nil {
		t.Fatalf("parse observer rest declaration: %v", err)
	}
	ports := declaration.Rest.Limits["agent_monitor_client"].Network.Ports
	if len(ports) == 0 {
		t.Fatal("observer agent_monitor_client declares no port allowlist")
	}
	return ports
}

var declarationDefault = regexp.MustCompile(`\$\{[A-Za-z_][A-Za-z0-9_]*(?::-([^}]*))?\}`)

// expandDeclarationDefaults substitutes each ${NAME:-default} reference with its
// default, mirroring what the runtime resolves when the environment sets nothing.
func expandDeclarationDefaults(raw []byte) []byte {
	return declarationDefault.ReplaceAllFunc(raw, func(match []byte) []byte {
		groups := declarationDefault.FindSubmatch(match)
		return groups[1]
	})
}

func findApplicationRoot(t *testing.T) string {
	t.Helper()
	return filepath.Dir(findChartDir(t))
}

// monitorPortViolations reports every disagreement between what the chart serves
// and what the observer authorizes, so one failure names all of them.
func monitorPortViolations(served []servedMonitorPort, allowed []int) []string {
	authorized := make(map[int]bool, len(allowed))
	for _, port := range allowed {
		authorized[port] = false
	}
	var violations []string
	for _, workload := range served {
		if _, declared := authorized[workload.ContainerPort]; !declared {
			violations = append(violations, fmt.Sprintf(
				"workload %s serves monitor port %d, absent from the observer's agent_monitor_client allowlist",
				workload.Workload, workload.ContainerPort))
			continue
		}
		authorized[workload.ContainerPort] = true
	}
	for _, port := range allowed {
		if !authorized[port] {
			violations = append(violations, fmt.Sprintf(
				"the observer allows monitor port %d, which no rendered workload serves", port))
		}
	}
	sort.Strings(violations)
	return violations
}

// TestObserverAllowlistCoversEveryRenderedMonitorPort pins the shipped chart: the
// fan-in can reach every agent, and the allowlist claims nothing more.
func TestObserverAllowlistCoversEveryRenderedMonitorPort(t *testing.T) {
	served := renderMonitorWorkloads(t)
	violations := monitorPortViolations(served, observerAllowedMonitorPorts(t))
	for _, violation := range violations {
		t.Error(violation)
	}
}

// TestObserverAllowlistDetectsAChangedChartPort is the guard on the guard. A port
// moved in values.yaml without the allowlist following is the drift this check
// exists to catch, so prove it catches it rather than passing vacuously.
func TestObserverAllowlistDetectsAChangedChartPort(t *testing.T) {
	served := renderMonitorWorkloads(t, "chatbot.ports.monitor=19999")
	violations := monitorPortViolations(served, observerAllowedMonitorPorts(t))
	if len(violations) == 0 {
		t.Fatal("moving the chatbot monitor port left the allowlist check silent")
	}
	joined := strings.Join(violations, "\n")
	if !strings.Contains(joined, "19999") {
		t.Errorf("violations do not name the moved port 19999:\n%s", joined)
	}
}

// TestObserverAllowlistDetectsAnUnservedPort covers the other direction: an entry
// that outlives the component it authorized.
func TestObserverAllowlistDetectsAnUnservedPort(t *testing.T) {
	served := renderMonitorWorkloads(t)
	violations := monitorPortViolations(served, append(observerAllowedMonitorPorts(t), 18099))
	joined := strings.Join(violations, "\n")
	if !strings.Contains(joined, "18099") {
		t.Errorf("a stray allowlist port went unreported:\n%s", joined)
	}
}

// TestEveryMonitorWorkloadPublishesItsPortLabel closes the third gap. The
// allowlist agreeing with the chart is not enough: the fan-in reads the port from
// the pod label, so a workload whose label disagrees with its container port
// sends the observer to the wrong port on the right pod.
func TestEveryMonitorWorkloadPublishesItsPortLabel(t *testing.T) {
	for _, workload := range renderMonitorWorkloads(t) {
		if workload.Label == "" {
			t.Errorf("workload %s serves monitor port %d but publishes no %s label",
				workload.Workload, workload.ContainerPort, monitorPortLabel)
			continue
		}
		if want := fmt.Sprint(workload.ContainerPort); workload.Label != want {
			t.Errorf("workload %s labels %s=%s but serves monitor port %s",
				workload.Workload, monitorPortLabel, workload.Label, want)
		}
	}
}
