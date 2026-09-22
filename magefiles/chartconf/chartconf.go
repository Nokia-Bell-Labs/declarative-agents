// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package chartconf checks rendered Helm manifests against the chart
// conformance rules ENG01 numbers and srd005-chart-conformance requires.
//
// The package takes rendered YAML and returns findings. It renders nothing
// and reads no chart, so every rule is testable from a string literal and the
// caller owns how a chart reaches a manifest (srd005 R1 through R5, R8).
package chartconf

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// RepositoryImagePrefixes name images this host produces rather than pulls.
// Two kinds qualify. The first is what this checkout builds. The second is a
// rig-local retag: ENG01 has the host pull a digest-pinned upstream image,
// retag it under a rig-local name, and kind-load that tag with
// imagePullPolicy Never, so the node never reaches a registry. Both carry a
// tag this repository sets and a digest that means nothing outside this
// machine, so R2.2 exempts them from the digest rule R2.1 puts on images the
// cluster pulls. The digest that matters for a retag is pinned on the
// upstream source in the chart defaults, where R2.1 does check it.
var RepositoryImagePrefixes = []string{
	"ghcr.io/nokia-bell-labs/declarative-agents/",
	"declarative-agents/",
	"kindrig/",
}

// Finding is one rule violation, carrying everything srd005 R8.1 requires a
// report to name: where it was rendered from, what it is, and which rule it
// breaks.
type Finding struct {
	Chart    string `yaml:"chart"`
	Overlay  string `yaml:"overlay"`
	Rule     string `yaml:"rule"`
	Resource string `yaml:"resource"`
	Value    string `yaml:"value"`
	Detail   string `yaml:"detail,omitempty"`
}

// Key identifies a finding for baseline matching. Detail carries the prose
// half of a message and is left out, so rewording a message does not
// invalidate a baseline entry.
func (f Finding) Key() string {
	return strings.Join([]string{f.Chart, f.Overlay, f.Rule, f.Resource, f.Value}, "|")
}

func (f Finding) String() string {
	message := fmt.Sprintf("%s [%s] %s: %s %q", f.Chart, f.Overlay, f.Rule, f.Resource, f.Value)
	if f.Detail != "" {
		message += " — " + f.Detail
	}
	return message
}

// Document is one rendered Kubernetes object, parsed loosely: a Service's
// selector is a flat map and a workload's nests under matchLabels, so both
// read through the same struct.
type Document struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name   string            `yaml:"name"`
		Labels map[string]string `yaml:"labels"`
	} `yaml:"metadata"`
	Spec struct {
		Type     string         `yaml:"type"`
		Selector map[string]any `yaml:"selector"`
		Ports    []struct {
			Port       int    `yaml:"port"`
			TargetPort any    `yaml:"targetPort"`
			Name       string `yaml:"name"`
		} `yaml:"ports"`
		Template  podTemplate `yaml:"template"`
		JobTarget struct {
			Template podTemplate `yaml:"template"`
		} `yaml:"jobTemplate"`
		Containers     []Container `yaml:"containers"`
		InitContainers []Container `yaml:"initContainers"`
	} `yaml:"spec"`
}

type podTemplate struct {
	Metadata struct {
		Labels map[string]string `yaml:"labels"`
	} `yaml:"metadata"`
	Spec struct {
		Containers     []Container `yaml:"containers"`
		InitContainers []Container `yaml:"initContainers"`
	} `yaml:"spec"`
}

// Container is the subset of a container spec the rules read.
type Container struct {
	Name           string   `yaml:"name"`
	Image          string   `yaml:"image"`
	PullPolicy     string   `yaml:"imagePullPolicy"`
	Command        []string `yaml:"command"`
	Args           []string `yaml:"args"`
	ReadinessProbe any      `yaml:"readinessProbe"`
	Ports          []struct {
		Name          string `yaml:"name"`
		ContainerPort int    `yaml:"containerPort"`
	} `yaml:"ports"`
}

// IsAgentWorkload reports whether a container runs the agent binary. Every
// agent workload is launched with a `--profile` argument that selects the
// mounted program; init and tool-donor containers (cli-donor, stage-chart,
// wait-for-models) run a shell command and carry none, and a non-agent
// workload such as a contrib OpenTelemetry gateway carries none either. The
// `--profile` argument is therefore the signal srd005 R9.2 asks the gate to
// classify on, so only agent main containers are checked against R9.1.
func (c Container) IsAgentWorkload() bool {
	for _, token := range append(append([]string{}, c.Command...), c.Args...) {
		if token == "--profile" || strings.HasPrefix(token, "--profile=") {
			return true
		}
	}
	return false
}

// Resource names a document the way a reader finds it again.
func (d Document) Resource() string {
	if d.Metadata.Name == "" {
		return d.Kind
	}
	return d.Kind + "/" + d.Metadata.Name
}

// podLabels returns the labels of the pods this document creates, or nil when
// it creates none.
func (d Document) podLabels() map[string]string {
	if len(d.Spec.Template.Metadata.Labels) > 0 {
		return d.Spec.Template.Metadata.Labels
	}
	if len(d.Spec.JobTarget.Template.Metadata.Labels) > 0 {
		return d.Spec.JobTarget.Template.Metadata.Labels
	}
	if d.Kind == "Pod" {
		return d.Metadata.Labels
	}
	return nil
}

// containers returns every container the document declares, init containers
// included, whichever shape carries them.
func (d Document) containers() []Container {
	var all []Container
	for _, group := range [][]Container{
		d.Spec.Template.Spec.Containers, d.Spec.Template.Spec.InitContainers,
		d.Spec.JobTarget.Template.Spec.Containers, d.Spec.JobTarget.Template.Spec.InitContainers,
		d.Spec.Containers, d.Spec.InitContainers,
	} {
		all = append(all, group...)
	}
	return all
}

// Parse splits rendered Helm output into documents. A document that carries
// no kind is a comment block or an empty render and is skipped; helm emits
// both between templates.
func Parse(rendered string) ([]Document, error) {
	rendered = strings.ReplaceAll(rendered, "\r\n", "\n")
	var documents []Document
	decoder := yaml.NewDecoder(strings.NewReader(rendered))
	for {
		var document Document
		err := decoder.Decode(&document)
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, fmt.Errorf("parse rendered manifest: %w", err)
		}
		if document.Kind == "" {
			continue
		}
		documents = append(documents, document)
	}
	return documents, nil
}

// ImageReferences returns every container image a render declares, once
// each, sorted. The conformance rules read the same containers to judge
// them; the pin survey reads them to ask what they are pinned to, so the
// chart half of its inventory is derived from the charts rather than
// restated in a list that could disagree with them (srd006 R1.1).
func ImageReferences(documents []Document) []string {
	seen := map[string]bool{}
	var images []string
	for _, document := range documents {
		for _, container := range document.containers() {
			if container.Image == "" || seen[container.Image] {
				continue
			}
			seen[container.Image] = true
			images = append(images, container.Image)
		}
	}
	sort.Strings(images)
	return images
}

// Check applies every rendered-manifest rule to one chart-and-overlay render.
// Findings come back sorted, so two runs over the same render report in the
// same order.
func Check(chart, overlay string, documents []Document) []Finding {
	var findings []Finding
	for _, document := range documents {
		findings = append(findings, checkImages(chart, overlay, document)...)
		findings = append(findings, checkPullPolicy(chart, overlay, document)...)
		findings = append(findings, checkServiceType(chart, overlay, document)...)
	}
	findings = append(findings, checkReadiness(chart, overlay, documents)...)
	findings = append(findings, checkOneAgentImage(chart, overlay, documents)...)
	sort.Slice(findings, func(i, j int) bool { return findings[i].Key() < findings[j].Key() })
	return findings
}

// checkOneAgentImage applies R9.1: every agent workload a chart renders runs
// one image, the application agent image. It reads only agent main containers
// (R9.2 classification via Container.IsAgentWorkload), so a classified
// init/tool-donor or non-agent container may differ without a finding. When
// the agent containers disagree, the reference is the image the most of them
// share — ties broken lexicographically so the report is deterministic — and
// every agent container that differs from it is a finding.
func checkOneAgentImage(chart, overlay string, documents []Document) []Finding {
	type agentContainer struct {
		resource string
		image    string
	}
	var agents []agentContainer
	counts := map[string]int{}
	for _, document := range documents {
		for _, container := range document.containers() {
			if container.Image == "" || !container.IsAgentWorkload() {
				continue
			}
			resource := document.Resource() + " container/" + container.Name
			agents = append(agents, agentContainer{resource, container.Image})
			counts[container.Image]++
		}
	}
	if len(counts) < 2 {
		return nil
	}
	reference := ""
	for image, count := range counts {
		if reference == "" || count > counts[reference] ||
			(count == counts[reference] && image < reference) {
			reference = image
		}
	}
	var findings []Finding
	for _, agent := range agents {
		if agent.image == reference {
			continue
		}
		findings = append(findings, Finding{chart, overlay, "R9.1", agent.resource, agent.image,
			"agent workload image differs from the application agent image " + reference})
	}
	return findings
}

// IsRepositoryImage reports whether an image reference names an image this
// checkout builds (srd005 R2.2). An Artifact Registry copy is still this
// checkout's build: gcp:pushAgentCore pushes commit-tagged images under
// <region>-docker.pkg.dev/<project>/<repository>/, and the registry prefix
// is per-deployment configuration, so the classification keys on the pushed
// image's own name rather than the registry host (eng08).
func IsRepositoryImage(image string) bool {
	lowered := strings.ToLower(image)
	for _, prefix := range RepositoryImagePrefixes {
		if strings.HasPrefix(lowered, prefix) {
			return true
		}
	}
	if strings.Contains(lowered, "-docker.pkg.dev/") {
		for _, name := range repositoryImageNames {
			if strings.HasSuffix(repositoryOf(lowered), "/"+name) {
				return true
			}
		}
	}
	return false
}

// repositoryImageNames are the image names this checkout pushes to a cloud
// registry. A mirrored third-party image (the CLI donor) is not among them,
// so it keeps its digest obligation.
var repositoryImageNames = []string{"agent-core", "agent-core-toolchain"}

// repositoryOf strips the tag and digest from a lowered reference.
func repositoryOf(lowered string) string {
	repository, _, _ := splitImage(lowered)
	return repository
}

// splitImage separates an image reference into repository, tag, and digest.
// A registry host carrying a port contains a colon before the first slash, so
// the tag is looked for after the last slash only.
func splitImage(image string) (repository, tag, digest string) {
	remainder := image
	if at := strings.Index(remainder, "@"); at >= 0 {
		digest = remainder[at+1:]
		remainder = remainder[:at]
	}
	lastSlash := strings.LastIndex(remainder, "/")
	if colon := strings.LastIndex(remainder, ":"); colon > lastSlash {
		tag = remainder[colon+1:]
		remainder = remainder[:colon]
	}
	return remainder, tag, digest
}

// checkImages applies R1.1, R1.2, R2.1, and R2.2 to every container image.
func checkImages(chart, overlay string, document Document) []Finding {
	var findings []Finding
	for _, container := range document.containers() {
		if container.Image == "" {
			continue
		}
		resource := document.Resource() + " container/" + container.Name
		_, tag, digest := splitImage(container.Image)
		switch {
		case tag == "" && digest == "":
			findings = append(findings, Finding{chart, overlay, "R1.1", resource, container.Image,
				"image carries no tag"})
		case tag == "latest":
			findings = append(findings, Finding{chart, overlay, "R1.1", resource, container.Image,
				"image tag floats"})
		}
		if !IsRepositoryImage(container.Image) && digest == "" {
			findings = append(findings, Finding{chart, overlay, "R2.1", resource, container.Image,
				"third-party image carries no digest"})
		}
	}
	return findings
}

// checkPullPolicy applies R3.1. A container leaving the policy unset takes
// Kubernetes' default, which is Always for a floating tag and IfNotPresent
// otherwise; the chart states it rather than relying on that inference.
func checkPullPolicy(chart, overlay string, document Document) []Finding {
	var findings []Finding
	for _, container := range document.containers() {
		if container.Image == "" {
			continue
		}
		resource := document.Resource() + " container/" + container.Name
		switch container.PullPolicy {
		case "IfNotPresent", "Never":
		case "":
			findings = append(findings, Finding{chart, overlay, "R3.1", resource, "",
				"container sets no imagePullPolicy"})
		default:
			findings = append(findings, Finding{chart, overlay, "R3.1", resource, container.PullPolicy,
				"imagePullPolicy is not IfNotPresent or Never"})
		}
	}
	return findings
}

// checkServiceType applies R4.1.
func checkServiceType(chart, overlay string, document Document) []Finding {
	if document.Kind != "Service" || document.Spec.Type != "LoadBalancer" {
		return nil
	}
	return []Finding{{chart, overlay, "R4.1", document.Resource(), document.Spec.Type,
		"kind provides no load balancer, so the Service would pend"}}
}

// checkReadiness applies R5.1: a Service routes to pods, and a pod with no
// readiness probe is routed to before it can serve. A Service whose selector
// matches no rendered workload is left alone; it selects something this
// render does not carry, and R5.1 is not the rule that catches that.
func checkReadiness(chart, overlay string, documents []Document) []Finding {
	var findings []Finding
	for _, service := range documents {
		if service.Kind != "Service" || len(service.Spec.Selector) == 0 {
			continue
		}
		for _, workload := range documents {
			labels := workload.podLabels()
			if len(labels) == 0 || !selectorMatches(service.Spec.Selector, labels) {
				continue
			}
			serving := servingContainers(service, workload)
			for _, container := range serving {
				if container.ReadinessProbe != nil {
					continue
				}
				findings = append(findings, Finding{chart, overlay, "R5.1",
					workload.Resource() + " container/" + container.Name, service.Resource(),
					"container serves a Service port and declares no readiness probe"})
			}
		}
	}
	return findings
}

// servingContainers returns the containers that answer the Service's ports.
// When no container names or numbers a target port, every container in the
// workload is treated as serving: the Service reaches the pod either way, and
// naming one container would be a guess.
func servingContainers(service, workload Document) []Container {
	containers := workload.containers()
	var serving []Container
	for _, port := range service.Spec.Ports {
		for _, container := range containers {
			if containerServesPort(container, port.TargetPort, port.Port) {
				serving = append(serving, container)
			}
		}
	}
	if len(serving) == 0 {
		return containers
	}
	return serving
}

func containerServesPort(container Container, targetPort any, servicePort int) bool {
	for _, port := range container.Ports {
		switch target := targetPort.(type) {
		case string:
			if port.Name == target {
				return true
			}
		case int:
			if port.ContainerPort == target {
				return true
			}
		case nil:
			if port.ContainerPort == servicePort {
				return true
			}
		}
	}
	return false
}

func selectorMatches(selector map[string]any, labels map[string]string) bool {
	for key, want := range selector {
		got, ok := labels[key]
		if !ok || got != fmt.Sprint(want) {
			return false
		}
	}
	return true
}
