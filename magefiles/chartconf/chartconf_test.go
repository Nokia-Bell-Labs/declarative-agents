// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package chartconf

import (
	"strings"
	"testing"
)

// deployment renders one Deployment and its Service, so a rule that needs a
// selector to resolve has one. Fields the caller does not set are left out
// rather than defaulted, because an absent field is itself a case several
// rules judge.
func deployment(image, pullPolicy, probe string) string {
	container := "      - name: agent\n        image: " + image + "\n"
	if pullPolicy != "" {
		container += "        imagePullPolicy: " + pullPolicy + "\n"
	}
	container += "        ports:\n        - name: http\n          containerPort: 8080\n"
	if probe != "" {
		container += "        readinessProbe:\n          httpGet:\n            path: " + probe +
			"\n            port: http\n"
	}
	return `apiVersion: apps/v1
kind: Deployment
metadata:
  name: agent
spec:
  template:
    metadata:
      labels:
        app: agent
    spec:
      containers:
` + container + `---
apiVersion: v1
kind: Service
metadata:
  name: agent
spec:
  selector:
    app: agent
  ports:
  - port: 8080
    targetPort: http
`
}

func check(t *testing.T, rendered string) []Finding {
	t.Helper()
	documents, err := Parse(rendered)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return Check("coding-agent", "kind-values.yaml", documents)
}

func rules(findings []Finding) []string {
	seen := make([]string, 0, len(findings))
	for _, finding := range findings {
		seen = append(seen, finding.Rule)
	}
	return seen
}

func hasRule(findings []Finding, rule string) bool {
	for _, finding := range findings {
		if finding.Rule == rule {
			return true
		}
	}
	return false
}

const pinnedThirdParty = "docker.io/alpine/k8s:1.31.4@sha256:" +
	"9c4976d47656d78cf53a92b0203fc54ac45eae18a2b45001ac221c27da4c8036"

const repositoryImage = "ghcr.io/nokia-bell-labs/declarative-agents/agent-core:0.1.0"

func TestConformingRenderProducesNoFinding(t *testing.T) {
	for name, image := range map[string]string{
		"third-party pinned by digest":               pinnedThirdParty,
		"repository image on a release tag":          repositoryImage,
		"repository image on a commit tag":           "ghcr.io/nokia-bell-labs/declarative-agents/agent-core:a1b2c3d4e5f6",
		"repository image on a fail-closed sentinel": "ghcr.io/nokia-bell-labs/declarative-agents/agent-core:must-be-overridden-with-git-revision",
	} {
		t.Run(name, func(t *testing.T) {
			findings := check(t, deployment(image, "IfNotPresent", "/healthz"))
			if len(findings) != 0 {
				t.Fatalf("expected no finding, got %v", findings)
			}
		})
	}
}

// R1.1: a tag that floats, and a reference carrying no tag at all.
func TestFloatingTagIsReported(t *testing.T) {
	for name, image := range map[string]string{
		"latest": "ollama/ollama:latest",
		"no tag": "ollama/ollama",
	} {
		t.Run(name, func(t *testing.T) {
			findings := check(t, deployment(image, "IfNotPresent", "/healthz"))
			if !hasRule(findings, "R1.1") {
				t.Fatalf("floating tag not reported: %v", rules(findings))
			}
		})
	}
}

// R2.1 and R2.2: the digest rule applies to third-party images and exempts
// images this checkout builds.
func TestDigestRuleAppliesOnlyToThirdPartyImages(t *testing.T) {
	findings := check(t, deployment("chromadb/chroma:1.5.3", "IfNotPresent", "/healthz"))
	if !hasRule(findings, "R2.1") {
		t.Fatalf("third-party image without a digest not reported: %v", rules(findings))
	}
	findings = check(t, deployment(repositoryImage, "IfNotPresent", "/healthz"))
	if hasRule(findings, "R2.1") {
		t.Fatalf("repository image wrongly required to carry a digest: %v", findings)
	}
}

// A registry host carrying a port must not be mistaken for a tag.
func TestRegistryPortIsNotReadAsTag(t *testing.T) {
	repository, tag, digest := splitImage("localhost:5000/agent-core:1.2.3")
	if repository != "localhost:5000/agent-core" || tag != "1.2.3" || digest != "" {
		t.Fatalf("split %q/%q/%q", repository, tag, digest)
	}
	repository, tag, digest = splitImage("localhost:5000/agent-core")
	if repository != "localhost:5000/agent-core" || tag != "" || digest != "" {
		t.Fatalf("split %q/%q/%q", repository, tag, digest)
	}
}

// R3.1: Always is refused and an unset policy is refused, because an unset
// policy resolves differently depending on the tag.
func TestPullPolicyIsReported(t *testing.T) {
	findings := check(t, deployment(pinnedThirdParty, "Always", "/healthz"))
	if !hasRule(findings, "R3.1") {
		t.Fatalf("imagePullPolicy Always not reported: %v", rules(findings))
	}
	findings = check(t, deployment(pinnedThirdParty, "", "/healthz"))
	if !hasRule(findings, "R3.1") {
		t.Fatalf("absent imagePullPolicy not reported: %v", rules(findings))
	}
	findings = check(t, deployment(pinnedThirdParty, "Never", "/healthz"))
	if hasRule(findings, "R3.1") {
		t.Fatalf("Never wrongly reported: %v", findings)
	}
}

// R4.1.
func TestLoadBalancerServiceIsReported(t *testing.T) {
	rendered := `apiVersion: v1
kind: Service
metadata:
  name: gateway
spec:
  type: LoadBalancer
  ports:
  - port: 80
`
	findings := check(t, rendered)
	if !hasRule(findings, "R4.1") {
		t.Fatalf("LoadBalancer Service not reported: %v", rules(findings))
	}
	findings = check(t, strings.Replace(rendered, "  type: LoadBalancer\n", "  type: ClusterIP\n", 1))
	if hasRule(findings, "R4.1") {
		t.Fatalf("ClusterIP wrongly reported: %v", findings)
	}
}

// R5.1: a selected workload without a readiness probe is reported, and a
// workload no Service selects is left alone.
func TestReadinessIsRequiredOnlyWhereAServiceSelects(t *testing.T) {
	findings := check(t, deployment(pinnedThirdParty, "IfNotPresent", ""))
	if !hasRule(findings, "R5.1") {
		t.Fatalf("selected workload without readiness not reported: %v", rules(findings))
	}
	unselected := `apiVersion: batch/v1
kind: Job
metadata:
  name: migrate
spec:
  template:
    metadata:
      labels:
        app: migrate
    spec:
      containers:
      - name: migrate
        image: ` + pinnedThirdParty + `
        imagePullPolicy: IfNotPresent
`
	findings = check(t, unselected)
	if hasRule(findings, "R5.1") {
		t.Fatalf("unselected workload wrongly required to declare readiness: %v", findings)
	}
}

// A Service whose selector matches nothing in this render is not a readiness
// failure; it selects something the render does not carry.
func TestServiceSelectingNothingIsNotAReadinessFailure(t *testing.T) {
	rendered := `apiVersion: v1
kind: Service
metadata:
  name: external
spec:
  selector:
    app: absent
  ports:
  - port: 80
`
	if findings := check(t, rendered); len(findings) != 0 {
		t.Fatalf("expected no finding, got %v", findings)
	}
}

// R8.1: a finding names chart, overlay, resource, value, and rule.
func TestFindingNamesEverythingAReaderNeeds(t *testing.T) {
	findings := check(t, deployment("ollama/ollama:latest", "IfNotPresent", "/healthz"))
	if len(findings) == 0 {
		t.Fatal("expected a finding")
	}
	message := findings[0].String()
	for _, want := range []string{"coding-agent", "kind-values.yaml", "R1.1", "Deployment/agent", "ollama/ollama:latest"} {
		if !strings.Contains(message, want) {
			t.Errorf("message %q does not name %q", message, want)
		}
	}
}

// Findings are ordered, so two runs over one render report identically.
func TestFindingsAreOrdered(t *testing.T) {
	rendered := deployment("ollama/ollama:latest", "Always", "")
	first, second := check(t, rendered), check(t, rendered)
	if len(first) != len(second) {
		t.Fatalf("run lengths differ: %d and %d", len(first), len(second))
	}
	for i := range first {
		if first[i].Key() != second[i].Key() {
			t.Fatalf("order differs at %d: %s and %s", i, first[i].Key(), second[i].Key())
		}
	}
}

// A comment-only or empty document between templates is skipped, not parsed
// into a nameless resource.
func TestParseSkipsDocumentsCarryingNoKind(t *testing.T) {
	documents, err := Parse("# Source: chart/templates/notes.txt\n---\n\n---\n" +
		"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: c\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(documents) != 1 || documents[0].Kind != "ConfigMap" {
		t.Fatalf("expected one ConfigMap, got %d: %v", len(documents), documents)
	}
}

// The pin survey derives its chart inventory from this, so it must return
// every image once and in a stable order (srd006 R1.1).
func TestImageReferencesAreUniqueAndOrdered(t *testing.T) {
	rendered := deployment(pinnedThirdParty, "IfNotPresent", "/healthz") +
		"---\n" + deployment(repositoryImage, "IfNotPresent", "/healthz")
	documents, err := Parse(rendered)
	if err != nil {
		t.Fatal(err)
	}
	images := ImageReferences(documents)
	if len(images) != 2 {
		t.Fatalf("images = %v, want two distinct", images)
	}
	if images[0] > images[1] {
		t.Fatalf("images not sorted: %v", images)
	}
	if ImageReferences(nil) != nil {
		t.Error("an empty render should yield no images")
	}
}

// agentDeployment renders one agent-workload Deployment: a main container
// launched with --profile (the R9.2 agent signal) and an optional init
// container that runs a shell command and is not an agent workload.
func agentDeployment(name, image, initImage string) string {
	doc := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: ` + name + `
spec:
  template:
    metadata:
      labels:
        app.kubernetes.io/component: ` + name + `
    spec:
`
	if initImage != "" {
		doc += `      initContainers:
      - name: ` + name + `-donor
        image: ` + initImage + `
        command: ["sh", "-c", "cp -f /usr/bin/tool /tools/"]
`
	}
	doc += `      containers:
      - name: ` + name + `
        image: ` + image + `
        imagePullPolicy: IfNotPresent
        args: ["--profile", "/profiles/agents/` + name + `/profile.yaml"]
`
	return doc
}

// R9.1: agent workloads that disagree on their image are reported, keyed on
// the majority image; a classified init/tool-donor container that differs is
// not (R9.2).
func TestOneAgentImageRule(t *testing.T) {
	planner := "ghcr.io/nokia-bell-labs/declarative-agents/agent-core:c0ffee0"
	alternate := "ghcr.io/nokia-bell-labs/declarative-agents/alternate-agent:c0ffee0"
	donor := "docker.io/alpine/k8s:1.31.4@sha256:" + strings.Repeat("a", 64)

	uniform := agentDeployment("planner", planner, donor) +
		"---\n" + agentDeployment("executor", planner, "") +
		"---\n" + agentDeployment("collector", planner, "")
	if findings := check(t, uniform); hasRule(findings, "R9.1") {
		t.Fatalf("uniform agent images should not report R9.1: %v", findings)
	}

	diverged := agentDeployment("planner", planner, donor) +
		"---\n" + agentDeployment("executor", planner, "") +
		"---\n" + agentDeployment("collector", alternate, "")
	findings := check(t, diverged)
	r9 := 0
	for _, f := range findings {
		if f.Rule == "R9.1" {
			r9++
			if f.Value != alternate {
				t.Errorf("R9.1 should flag the divergent %q, got %q", alternate, f.Value)
			}
		}
	}
	if r9 != 1 {
		t.Fatalf("expected exactly one R9.1 finding, got %d: %v", r9, findings)
	}
}

// An Artifact Registry copy of this checkout's image is repository-built;
// a mirrored third-party image under the same registry is not (srd005 R2.2,
// eng08).
func TestArtifactRegistryClassification(t *testing.T) {
	cases := map[string]bool{
		"us-central1-docker.pkg.dev/demo/agents/agent-core:a1b2c3": true,
		"us-central1-docker.pkg.dev/demo/agents/alternate:a1b2c3":  false,
		"us-central1-docker.pkg.dev/demo/agents/cli-donor:1.31.4":  false,
		"docker.io/alpine/k8s:1.31.4":                              false,
	}
	for image, want := range cases {
		if got := IsRepositoryImage(image); got != want {
			t.Errorf("IsRepositoryImage(%q) = %v, want %v", image, got, want)
		}
	}
}
