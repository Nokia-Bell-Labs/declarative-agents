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

// TestChatbotFanOutCoGeneratedForNRags locks the chatbot request-machine fan-out
// chain and the request-fanout rag_queryN words to the ragUnits list, so a values
// change scales the fan-out breadth with the topology. A three-RAG render must
// carry the full Retrieving/Checking/Keeping chain through to Composing, one
// rag_queryN, compare_modelN, and keep_chunksN word per unit, and a matching
// compose input per source.
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

	// The chain: an answered RAG is checked against the query embedding model
	// before its chunks are kept, and every other outcome routes straight on to
	// the next RAG; the last routes to Composing (srd002 R3.3, GH-765).
	wantTransitions := []string{
		"state: Embedding,       signal: QueryEmbedded,  next: DeclaringModel, action: declare_query_model",
		"state: DeclaringModel,  signal: QueryModelDeclared, next: Retrieving0, action: rag_query0",
		"state: Retrieving0,     signal: QueryResponded, next: Checking0,  action: compare_model0",
		"state: Retrieving0,     signal: QueryRejected,  next: Marking0,   action: mark_rejected0",
		"state: Checking0,       signal: ModelMatched,   next: Keeping0,   action: keep_chunks0",
		"state: Checking0,       signal: ModelDiffered,  next: Marking0,   action: mark_excluded_model0",
		"state: Retrieving0,     signal: CommandError,   next: Marking0,   action: mark_degraded0",
		"state: Keeping0,        signal: ChunksKept0, next: Retrieving1, action: rag_query1",
		"state: Marking0,        signal: SourceMarked0, next: Retrieving1, action: rag_query1",
		"state: Retrieving1,     signal: CommandError,   next: Marking1,   action: mark_degraded1",
		"state: Marking1,        signal: SourceMarked1, next: Retrieving2, action: rag_query2",
		"state: Retrieving2,     signal: QueryResponded, next: Checking2,  action: compare_model2",
		"state: Checking2,       signal: ModelDiffered,  next: Marking2,   action: mark_excluded_model2",
		"state: Keeping2,        signal: ChunksKept2, next: Composing, action: compose_prompt",
		"state: Marking2,        signal: SourceMarked2, next: Composing, action: compose_prompt",
		"state: Answering,       signal: LLMResponded,   next: Reporting,     action: compose_response",
		"state: Reporting,       signal: ResponseComposed, next: LLMResponded",
	}
	for _, tr := range wantTransitions {
		if !strings.Contains(machine, tr) {
			t.Errorf("co-generated machine missing transition: %s", tr)
		}
	}
	for _, state := range []string{"Retrieving3", "Checking3", "Keeping3", "Marking3"} {
		if strings.Contains(machine, state) {
			t.Errorf("co-generated machine has a %s state for a three-RAG values set", state)
		}
	}
	// One rag_queryN, compare_modelN, and keep_chunksN word per unit, and a
	// compose input reading that unit through its keep label: composing from
	// $from(rag_queryN) directly would keep an excluded source's chunks, since
	// they stay addressable in command state after the exclusion.
	for i := 0; i < 3; i++ {
		for _, word := range []string{"rag_query", "compare_model", "keep_chunks", "mark_excluded_model", "mark_rejected", "mark_degraded"} {
			if !strings.Contains(fanout, fmt.Sprintf("name: %s%d", word, i)) {
				t.Errorf("co-generated fanout missing %s%d", word, i)
			}
		}
		if !strings.Contains(fanout, fmt.Sprintf("chunks%d: $from(keep_chunks%d).documents", i, i)) {
			t.Errorf("co-generated compose missing chunks%d input", i)
		}
		name := []string{"alpha", "bravo", "charlie"}[i]
		if !strings.Contains(fanout, "rest_ref: "+name) {
			t.Errorf("co-generated query %d does not select topology client %q", i, name)
		}
		if !strings.Contains(fanout, "operation: "+name+"_query") {
			t.Errorf("co-generated query %d does not select topology operation %q", i, name+"_query")
		}
		if !strings.Contains(fanout, "["+name+"]") {
			t.Errorf("co-generated compose template missing [%s] header", name)
		}
	}
	for _, word := range []string{"rag_query3", "compare_model3", "keep_chunks3", "mark_degraded3"} {
		if strings.Contains(fanout, "name: "+word) {
			t.Errorf("co-generated fanout has %s for a three-RAG values set", word)
		}
	}
	// The runtime {{ chunksN }} template body must survive Helm rendering literally.
	if !strings.Contains(fanout, "{{ chunks2 }}") {
		t.Error("co-generated compose template did not preserve the literal {{ chunks2 }} body")
	}
}

// TestChatbotRestCoGeneratedFromRagUnits locks srd003 R2.1: the chatbot rest.yaml
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

	// One client per unit, preserving its topology identity and pointing at its
	// Service DNS. Positional aliases would silently rename later sources after
	// an operator removes or replaces a unit.
	for _, u := range units {
		client := u.name + ":"
		operation := u.name + "_query:"
		url := fmt.Sprintf("http://t-chatbot-mesh-%s:18085", u.name)
		if !strings.Contains(rest, client) {
			t.Errorf("co-generated rest.yaml missing client %q", client)
		}
		if !strings.Contains(rest, operation) {
			t.Errorf("co-generated rest.yaml missing operation %q", operation)
		}
		if !strings.Contains(rest, url) {
			t.Errorf("co-generated rest.yaml missing base_url %q", url)
		}
		upstream := fmt.Sprintf("%s: http://t-chatbot-mesh-%s:18087", u.name, u.name)
		if !strings.Contains(rest, upstream) {
			t.Errorf("co-generated rest.yaml missing monitor_proxy upstream %q", upstream)
		}
	}
	// No positional client alias may appear, and the packaged rag1@loopback entry
	// must not survive the override.
	for _, alias := range []string{"rag0:", "rag1:", "rag2:", "rag3:"} {
		if strings.Contains(rest, "\n    "+alias) {
			t.Errorf("co-generated rest.yaml has positional client alias %q", alias)
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

// TestChatbotUXMonitoredAgentsCoGenerated locks the monitored-agents surface to
// the same ragUnits list (srd003 R2).
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
	ux := configMapKeyBlock(string(out), "ux__ux.yaml")
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
