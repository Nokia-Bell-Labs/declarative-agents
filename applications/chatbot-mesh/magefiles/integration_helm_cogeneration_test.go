// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestChatbotFanOutCoGeneratedForNRags locks the source-count-independent
// program and the values-driven topology data. Three RAG units change only the
// declare_rag_topology items array; the packaged machine retains one for_each
// and the fan-out declarations retain one rag_query.
func TestChatbotFanOutCoGeneratedForNRags(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chart := findChartDir(t)
	args := []string{"template", "t", chart}
	for i, name := range []string{"alpha", "bravo", "charlie"} {
		args = append(args,
			"--set", fmt.Sprintf("ragUnits[%d].name=%s", i, name),
			"--set", fmt.Sprintf("ragUnits[%d].description=%s corpus", i, name),
			"--set", fmt.Sprintf("ragUnits[%d].collection=c%d", i, i),
			"--set", fmt.Sprintf("ragUnits[%d].embeddingModel=m", i),
		)
	}
	out, err := exec.Command("helm", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	topology := configMapKeyBlock(string(out), "agents__chatbot__request-topology-declarations.yaml")
	if topology == "" {
		t.Fatal("co-generated request-topology-declarations.yaml key not found")
	}
	for _, name := range []string{"alpha", "bravo", "charlie"} {
		if !strings.Contains(topology, `"name": "`+name+`"`) ||
			!strings.Contains(topology, `"description": "`+name+` corpus"`) ||
			!strings.Contains(topology, "t-chatbot-mesh-"+name+":18085") {
			t.Errorf("topology does not declare %s with its description and selected authority", name)
		}
	}
	if strings.Contains(topology, `"catalog":`) &&
		(strings.Contains(configMapCatalog(topology), "http://") || strings.Contains(configMapCatalog(topology), "collection")) {
		t.Error("source classifier catalog exposes trusted target/configuration data")
	}
	if strings.Count(topology, "name: declare_rag_topology") != 1 {
		t.Error("topology must contain exactly one declaration word")
	}

	root := filepath.Join(chart, "..", "agents", "chatbot")
	machine, err := os.ReadFile(filepath.Join(root, "request-machine.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	fanout, err := os.ReadFile(filepath.Join(root, "request-fanout-declarations.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(machine), "for_each:") != 1 ||
		!strings.Contains(string(machine), "items: $from(selected_sources).selected") {
		t.Error("packaged machine must contain one for_each over trusted selected topology entries")
	}
	if strings.Count(string(fanout), "name: rag_query\n") != 1 {
		t.Error("fan-out declarations must contain one rag_query word")
	}
	for _, indexed := range []string{"rag_query0", "rag_query1", "Retrieving0", "Retrieving1", "compare_model0", "keep_chunks0"} {
		if strings.Contains(string(machine), indexed) || strings.Contains(string(fanout), indexed) {
			t.Errorf("source-indexed fan-out name remains: %s", indexed)
		}
	}
}

// TestChatbotRestCoGeneratedFromRagUnits locks one selected-target RAG operation.
// ragUnits still generates the network allowlist and topology authorities while
// the REST operation count remains one.
func TestChatbotRestCoGeneratedFromRagUnits(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chart := findChartDir(t)

	units := []struct{ name, collection string }{
		{"alpha", "ca"}, {"bravo", "cb"}, {"charlie", "cc"},
	}
	args := []string{"template", "t", chart}
	for i, u := range units {
		args = append(args,
			"--set", fmt.Sprintf("ragUnits[%d].name=%s", i, u.name),
			"--set", fmt.Sprintf("ragUnits[%d].description=%s corpus", i, u.name),
			"--set", fmt.Sprintf("ragUnits[%d].collection=%s", i, u.collection),
			"--set", fmt.Sprintf("ragUnits[%d].embeddingModel=m", i),
		)
	}
	out, err := exec.Command("helm", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	rest := configMapKeyBlock(string(out), "agents__chatbot__rest.yaml")
	if rest == "" {
		t.Fatal("co-generated agents__chatbot__rest.yaml key not found in render")
	}

	if strings.Count(rest, "\n    rag:") != 1 ||
		strings.Count(rest, "\n        query:") != 1 {
		t.Error("co-generated rest.yaml must contain one generic RAG client and operation")
	}
	for _, selected := range []string{
		"base_url_source: command_state",
		"base_url_selector: $from(rag_unit).base_url",
	} {
		if !strings.Contains(rest, selected) {
			t.Errorf("generic RAG operation missing %q", selected)
		}
	}
	for _, u := range units {
		host := "t-chatbot-mesh-" + u.name
		if !strings.Contains(rest, "- "+host) {
			t.Errorf("co-generated network allowlist missing host %q", host)
		}
		upstream := fmt.Sprintf("%s: http://t-chatbot-mesh-%s:18087", u.name, u.name)
		if !strings.Contains(rest, upstream) {
			t.Errorf("co-generated rest.yaml missing monitor_proxy upstream %q", upstream)
		}
	}
	// No per-source client or operation may reappear.
	for _, alias := range []string{"alpha:", "bravo:", "charlie:", "alpha_query:", "bravo_query:", "charlie_query:"} {
		if strings.Contains(rest, "\n    "+alias) || strings.Contains(rest, "\n        "+alias) {
			t.Errorf("co-generated rest.yaml has per-source REST entry %q", alias)
		}
	}
	if strings.Contains(rest, "http://127.0.0.1:18095") {
		t.Error("packaged loopback RAG client leaked into the co-generated rest.yaml")
	}
	// Servers must bind 0.0.0.0 so Services route to the pod.
	if !strings.Contains(rest, "address: 0.0.0.0:18080") {
		t.Error("co-generated chat server does not bind 0.0.0.0")
	}
}

// TestChatbotUIMonitoredAgentsCoGenerated locks the monitored-agents surface to
// the same ragUnits list (srd003 R2).
func TestChatbotUIMonitoredAgentsCoGenerated(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chart := findChartDir(t)
	out, err := exec.Command("helm", "template", "t", chart,
		"--set", "ragUnits[0].name=only", "--set", "ragUnits[0].description=only corpus",
		"--set", "ragUnits[0].collection=c", "--set", "ragUnits[0].embeddingModel=m",
	).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	ux := configMapKeyBlock(string(out), "agents__chatbot__ui__ui.yaml")
	if ux == "" {
		t.Fatal("co-generated ui.yaml key not found")
	}
	if !strings.Contains(ux, "name: only") {
		t.Error("ux monitored_agents missing the sole rag unit")
	}
	if strings.Contains(ux, "name: rag1") {
		t.Error("packaged rag1 monitored-agent leaked into the co-generated ui.yaml")
	}
}

func configMapCatalog(topology string) string {
	start := strings.Index(topology, `"catalog":`)
	if start < 0 {
		return ""
	}
	rest := topology[start:]
	end := strings.Index(rest, `"items":`)
	if end < 0 {
		return rest
	}
	return rest[:end]
}

// configMapKeyBlock returns the indented block value of a "  <key>: |-" entry in
// a rendered ConfigMap, dedented, up to the next same-level key.
func configMapKeyBlock(render, key string) string {
	lines := strings.Split(render, "\n")
	var block []string
	inBlock := false
	for _, line := range lines {
		if !inBlock {
			if strings.TrimSpace(line) == key+": |-" {
				inBlock = true
			}
			continue
		}
		if strings.HasPrefix(line, "    ") {
			block = append(block, strings.TrimPrefix(line, "    "))
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		break // dedent to a sibling key ends the block
	}
	return strings.Join(block, "\n")
}

func findChartDir(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		candidate := filepath.Join(dir, "helm", "Chart.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return filepath.Dir(candidate)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Skip("chatbot-mesh chart not found walking up from the test directory")
		}
		dir = parent
	}
}
