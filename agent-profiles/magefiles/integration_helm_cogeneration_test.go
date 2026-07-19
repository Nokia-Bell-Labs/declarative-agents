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

// TestChatbotFanOutCoGeneratedForNRags locks GH-372: the chatbot request-machine
// fan-out chain and the request-fanout rag_queryN words are rendered from the
// ragUnits list, so a values change scales the fan-out breadth with the topology.
// A three-RAG render must carry the full Retrieving0->1->2->Composing chain, one
// rag_queryN word per unit, and a matching compose input per source.
func TestChatbotFanOutCoGeneratedForNRags(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chart := findChartDir(t)
	args := []string{"template", "t", chart}
	for i, name := range []string{"alpha", "bravo", "charlie"} {
		args = append(args,
			"--set", fmt.Sprintf("ragUnits[%d].name=%s", i, name),
			"--set", fmt.Sprintf("ragUnits[%d].collection=c%d", i, i),
			"--set", fmt.Sprintf("ragUnits[%d].embeddingModel=m", i),
		)
	}
	out, err := exec.Command("helm", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	render := string(out)
	machine := configMapKeyBlock(render, "agents__chatbot__request-machine.yaml")
	fanout := configMapKeyBlock(render, "agents__chatbot__request-fanout.yaml")
	if machine == "" || fanout == "" {
		t.Fatal("co-generated request-machine.yaml or request-fanout.yaml key not found")
	}

	// The chain: each RAG state routes on to the next; the last routes to Composing.
	wantTransitions := []string{
		"state: Embedding,       signal: QueryEmbedded,  next: Retrieving0,   action: rag_query0",
		"state: Retrieving0,     signal: QueryRejected,  next: Retrieving1, action: rag_query1",
		"state: Retrieving1,     signal: CommandError,   next: Retrieving2, action: rag_query2",
		"state: Retrieving2,     signal: QueryResponded, next: Composing, action: compose_prompt",
	}
	for _, tr := range wantTransitions {
		if !strings.Contains(machine, tr) {
			t.Errorf("co-generated machine missing transition: %s", tr)
		}
	}
	if strings.Contains(machine, "Retrieving3") {
		t.Error("co-generated machine has a Retrieving3 state for a three-RAG values set")
	}
	// One rag_queryN word and one compose input per unit.
	for i := 0; i < 3; i++ {
		if !strings.Contains(fanout, fmt.Sprintf("name: rag_query%d", i)) {
			t.Errorf("co-generated fanout missing rag_query%d", i)
		}
		if !strings.Contains(fanout, fmt.Sprintf("chunks%d: $from(rag_query%d).mapped.documents", i, i)) {
			t.Errorf("co-generated compose missing chunks%d input", i)
		}
		if !strings.Contains(fanout, fmt.Sprintf("[rag%d]", i)) {
			t.Errorf("co-generated compose template missing [rag%d] header", i)
		}
	}
	if strings.Contains(fanout, "name: rag_query3") {
		t.Error("co-generated fanout has rag_query3 for a three-RAG values set")
	}
	// The runtime {{ chunksN }} template body must survive Helm rendering literally.
	if !strings.Contains(fanout, "{{ chunks2 }}") {
		t.Error("co-generated compose template did not preserve the literal {{ chunks2 }} body")
	}
}

// TestChatbotRestCoGeneratedFromRagUnits locks srd015 R2.1: the chatbot rest.yaml
// RAG client entries are template output derived from the ragUnits list, so a
// drifted or hand-edited client entry is impossible by construction. It renders
// the chart with a three-RAG values set and asserts the co-generated rest.yaml
// carries exactly those RAG clients pointed at their Service DNS names, with no
// stale entry from the packaged profile.
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

	// One client per unit, named by index, pointed at the unit's Service DNS.
	for i, u := range units {
		client := fmt.Sprintf("rag%d:", i)
		url := fmt.Sprintf("http://t-chatbot-mesh-%s:18085", u.name)
		if !strings.Contains(rest, client) {
			t.Errorf("co-generated rest.yaml missing client %q", client)
		}
		if !strings.Contains(rest, url) {
			t.Errorf("co-generated rest.yaml missing base_url %q", url)
		}
		upstream := fmt.Sprintf("%s: http://t-chatbot-mesh-%s:18087", u.name, u.name)
		if !strings.Contains(rest, upstream) {
			t.Errorf("co-generated rest.yaml missing monitor_proxy upstream %q", upstream)
		}
	}
	// A fourth client index must not appear for a three-unit render, and the
	// packaged rag1@loopback entry must not survive the override.
	if strings.Contains(rest, "rag3:") {
		t.Error("co-generated rest.yaml has a rag3 client for a three-unit values set")
	}
	if strings.Contains(rest, "http://127.0.0.1:18095") {
		t.Error("packaged loopback RAG client leaked into the co-generated rest.yaml")
	}
	// Servers must bind 0.0.0.0 so Services route to the pod.
	if !strings.Contains(rest, "address: 0.0.0.0:18080") {
		t.Error("co-generated chat server does not bind 0.0.0.0")
	}
}

// TestChatbotUXMonitoredAgentsCoGenerated locks the monitored-agents surface to
// the same ragUnits list (srd015 R2).
func TestChatbotUXMonitoredAgentsCoGenerated(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chart := findChartDir(t)
	out, err := exec.Command("helm", "template", "t", chart,
		"--set", "ragUnits[0].name=only", "--set", "ragUnits[0].collection=c", "--set", "ragUnits[0].embeddingModel=m",
	).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	ux := configMapKeyBlock(string(out), "agents__chatbot__ui__ux.yaml")
	if ux == "" {
		t.Fatal("co-generated ux.yaml key not found")
	}
	if !strings.Contains(ux, "name: only") {
		t.Error("ux monitored_agents missing the sole rag unit")
	}
	if strings.Contains(ux, "name: rag1") {
		t.Error("packaged rag1 monitored-agent leaked into the co-generated ux.yaml")
	}
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
		candidate := filepath.Join(dir, "deploy", "chatbot-mesh", "Chart.yaml")
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
