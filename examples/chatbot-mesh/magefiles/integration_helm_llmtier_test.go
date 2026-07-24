// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestOllamaTierRendersWithDefaults locks srd003 R6: with the defaults
// (ollama.enabled=true) the chart renders the in-cluster LLM tier -- an Ollama
// StatefulSet, its Service, the model-preload Job pulling every declared model,
// and a wait-for-llm-models readiness init container on every agent workload.
func TestOllamaTierRendersWithDefaults(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chart := findChartDir(t)
	out, err := exec.Command("helm", "template", "t", chart).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	render := string(out)
	for _, want := range []string{
		"name: t-chatbot-mesh-ollama",         // StatefulSet + Service
		"name: t-chatbot-mesh-ollama-preload", // preload Job
		"name: wait-for-llm-models",           // agent readiness init
		"ollama pull",                         // the preload pulls models
	} {
		if !strings.Contains(render, want) {
			t.Errorf("default render missing %q (the LLM tier must render with defaults)", want)
		}
	}
	// The embedding base_url points at the in-cluster Ollama Service.
	if !strings.Contains(render, "base_url: http://t-chatbot-mesh-ollama:11434") {
		t.Error("default render does not point the embedding client at the in-cluster Ollama Service")
	}
	// Every declared model reaches the preload (named once in values).
	for _, model := range []string{"qwen3-embedding:8b", "qwen2.5:3b", "ornith:9b"} {
		if !strings.Contains(render, model) {
			t.Errorf("preload/config missing declared model %q", model)
		}
	}
	// The preload runs the ollama image, which ships neither wget nor curl, so its
	// reachability probe must be the ollama CLI, never an HTTP client absent from
	// the image (a wget probe hung the preload forever).
	if !strings.Contains(render, "until ollama list") {
		t.Error("preload reachability probe must use `ollama list` (the ollama image has no wget/curl)")
	}
	if strings.Contains(render, "wget -qO- \"$OLLAMA_HOST") {
		t.Error("preload must not probe reachability with wget: the ollama image ships no wget, so the loop never exits")
	}
	// Every agent workload must carry the positive preload-completion gate: the
	// chatbot and one rag-server per declared unit. Asserted as a set rather than
	// a count, so the gate landing on the wrong workload fails too -- and so the
	// assertion does not go stale when the default topology changes, which is
	// what a hardcoded 2 did against a two-unit default (GH-701).
	assertLLMGatedWorkloads(t, render, expectedGatedWorkloads(t, "t"))
}

// TestKindLLMModelsReachDeployedDeclarations locks the values-to-profile half of
// srd003 R2/R6. The readiness gate is insufficient if the mounted declarations
// still register their local-development model defaults after preload.
func TestKindLLMModelsReachDeployedDeclarations(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chart := findChartDir(t)
	staged, cleanup, err := stageSmokeChart(chart, filepath.Dir(chart))
	if err != nil {
		t.Fatalf("stage chart: %v", err)
	}
	defer cleanup()

	out, err := exec.Command("helm", "template", "t", staged,
		"--values", filepath.Join(staged, "ci", "kind-llm-values.yaml"),
	).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	render := string(out)
	for _, want := range []string{
		`name: CHATBOT_ROUTER_MODEL`,
		`name: CHATBOT_FAST_MODEL`,
		`name: CHATBOT_DEEP_MODEL`,
		`value: "qwen2.5:0.5b"`,
		`model: "${CHATBOT_ROUTER_MODEL:-qwen2.5:3b}"`,
		`model: "${CHATBOT_FAST_MODEL:-qwen2.5:3b}"`,
		`model: "${CHATBOT_DEEP_MODEL:-ornith:9b}"`,
	} {
		if !strings.Contains(render, want) {
			t.Errorf("kind LLM render missing %q", want)
		}
	}
	for _, stale := range []string{`model: "qwen2.5:3b"`, `model: "ornith:9b"`} {
		if strings.Contains(render, stale) {
			t.Errorf("kind LLM render still registers unconfigured model %q", stale)
		}
	}
}

// expectedGatedWorkloads names the agent workloads that must wait on the LLM
// preload under the chart defaults: the chatbot and one rag-server per declared
// ragUnit, each named <release>-chatbot-mesh-<unit> as rag-units.yaml renders it.
func expectedGatedWorkloads(t *testing.T, release string) []string {
	t.Helper()
	var values struct {
		RagUnits []struct {
			Name string `yaml:"name"`
		} `yaml:"ragUnits"`
	}
	data, err := os.ReadFile(filepath.Join(findChartDir(t), "values.yaml"))
	if err != nil {
		t.Fatalf("read values.yaml: %v", err)
	}
	if err := yaml.Unmarshal(data, &values); err != nil {
		t.Fatalf("parse values.yaml: %v", err)
	}
	if len(values.RagUnits) == 0 {
		t.Fatal("values.yaml declares no ragUnits; the RAG topology or the key name changed")
	}
	fullname := release + "-chatbot-mesh"
	want := []string{fullname + "-chatbot"}
	for _, unit := range values.RagUnits {
		want = append(want, fullname+"-"+unit.Name)
	}
	return want
}

// assertLLMGatedWorkloads proves the workloads carrying the wait-for-llm-models
// init container are exactly the expected set, and that each gate is the positive
// model-presence poll rather than a bare sleep or a completion-blind wait.
func assertLLMGatedWorkloads(t *testing.T, render string, want []string) {
	t.Helper()
	type workload struct {
		Kind     string `yaml:"kind"`
		Metadata struct {
			Name string `yaml:"name"`
		} `yaml:"metadata"`
		Spec struct {
			Template struct {
				Spec struct {
					InitContainers []struct {
						Name string   `yaml:"name"`
						Args []string `yaml:"args"`
					} `yaml:"initContainers"`
				} `yaml:"spec"`
			} `yaml:"template"`
		} `yaml:"spec"`
	}

	gated := map[string]bool{}
	decoder := yaml.NewDecoder(strings.NewReader(render))
	for {
		var object workload
		err := decoder.Decode(&object)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("decode rendered manifest: %v", err)
		}
		if object.Kind == "" {
			continue
		}
		for _, init := range object.Spec.Template.Spec.InitContainers {
			if init.Name != "wait-for-llm-models" {
				continue
			}
			gated[object.Metadata.Name] = true
			// The gate is positive: it polls until every declared model is
			// present in /api/tags. A gate that only waited for the Job object
			// would let an agent start before the models it needs exist.
			if !strings.Contains(strings.Join(init.Args, "\n"), `until wget -qO- "$url"`) {
				t.Errorf("%s carries wait-for-llm-models without the model-presence poll", object.Metadata.Name)
			}
		}
	}

	for _, name := range want {
		if !gated[name] {
			t.Errorf("%s does not wait on the LLM preload; it could start before its models exist", name)
		}
		delete(gated, name)
	}
	for name := range gated {
		t.Errorf("%s carries the LLM preload gate but is not an agent workload that needs it", name)
	}
}

// TestOllamaDisabledReproducesExternalEndpoint locks R6.1: with ollama.enabled=false
// and an endpoint override, no Ollama tier renders and the co-generated embedding
// client points at the operator-supplied service (the pre-tier external behavior),
// so the co-generation contract (R2) is unchanged.
func TestOllamaDisabledReproducesExternalEndpoint(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chart := findChartDir(t)
	out, err := exec.Command("helm", "template", "t", chart,
		"--set", "ollama.enabled=false",
		"--set", "llm.externalURL=http://ollama.example:11434",
	).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	render := string(out)
	for _, absent := range []string{
		"app.kubernetes.io/component: ollama",
		"ollama-preload",
		"wait-for-llm-models",
	} {
		if strings.Contains(render, absent) {
			t.Errorf("disabled render still contains %q (no LLM tier must render when disabled)", absent)
		}
	}
	if !strings.Contains(render, "base_url: http://ollama.example:11434") {
		t.Error("disabled render does not point the embedding client at the external endpoint override")
	}
	// External-tier rendering removes chart-owned Ollama resources and their
	// preload gates, but retains every agent workload -- the same set the gate
	// assertion above covers, so neither goes stale against the other.
	for _, workload := range expectedGatedWorkloads(t, "t") {
		if !strings.Contains(render, "name: "+workload) {
			t.Errorf("external-tier render removed agent workload %q", workload)
		}
	}
}

func TestLLMPreloadReadinessTransitionSequence(t *testing.T) {
	var calls []string
	run := func(name string, args ...string) ([]byte, error) {
		call := name + " " + strings.Join(args, " ")
		calls = append(calls, call)
		switch {
		case strings.Contains(call, "get job/llm-chatbot-mesh-ollama-preload"):
			return []byte(`{"spec":{"suspend":true},"status":{"succeeded":0}}`), nil
		case strings.Contains(call, "get deployment"):
			return []byte(`{"items":[
				{"metadata":{"name":"llm-chatbot-mesh-chatbot","labels":{"app.kubernetes.io/component":"chatbot"}},"spec":{"replicas":1},"status":{"readyReplicas":0}},
				{"metadata":{"name":"llm-chatbot-mesh-rag0","labels":{"app.kubernetes.io/component":"rag-server"}},"spec":{"replicas":1},"status":{"readyReplicas":0}}
			]}`), nil
		default:
			return []byte("ok"), nil
		}
	}

	workloads, err := beginLLMPreloadTransition(run)
	if err != nil {
		t.Fatal(err)
	}
	if err := finishLLMPreloadTransition(run, workloads); err != nil {
		t.Fatal(err)
	}
	if got, want := workloads, []string{"llm-chatbot-mesh-chatbot", "llm-chatbot-mesh-rag0"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("workloads = %v, want %v", got, want)
	}
	sequence := strings.Join(calls, "\n")
	for _, ordered := range []string{
		"get job/llm-chatbot-mesh-ollama-preload",
		"get deployment",
		"patch job/llm-chatbot-mesh-ollama-preload",
		"wait --for=condition=complete",
		"rollout status deployment/llm-chatbot-mesh-chatbot",
		"rollout status deployment/llm-chatbot-mesh-rag0",
	} {
		index := strings.Index(sequence, ordered)
		if index < 0 {
			t.Fatalf("command sequence missing %q:\n%s", ordered, sequence)
		}
		sequence = sequence[index+len(ordered):]
	}
}

func TestLLMPreloadReadinessTransitionDiagnostics(t *testing.T) {
	tests := []struct {
		name     string
		job      string
		workload string
		failOn   string
		wantErr  string
	}{
		{
			name: "preload not suspended", job: `{"spec":{"suspend":false},"status":{}}`,
			wantErr: "requires suspended incomplete Job",
		},
		{
			name: "agent already ready", job: `{"spec":{"suspend":true},"status":{}}`,
			workload: `{"items":[
				{"metadata":{"name":"chatbot","labels":{"app.kubernetes.io/component":"chatbot"}},"spec":{"replicas":1},"status":{"readyReplicas":1}},
				{"metadata":{"name":"rag0","labels":{"app.kubernetes.io/component":"rag-server"}},"spec":{"replicas":1},"status":{}}
			]}`,
			wantErr: "became ready before preload",
		},
		{
			name: "preload wait failure", job: `{"spec":{"suspend":true},"status":{}}`,
			workload: unreadyLLMWorkloadsJSON(), failOn: "wait --for=condition=complete",
			wantErr: "preload Job did not complete",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			run := func(name string, args ...string) ([]byte, error) {
				call := name + " " + strings.Join(args, " ")
				if tc.failOn != "" && strings.Contains(call, tc.failOn) {
					return []byte("controlled kubectl diagnostic"), errors.New("controlled failure")
				}
				if strings.Contains(call, "get job/") {
					return []byte(tc.job), nil
				}
				if strings.Contains(call, "get deployment") {
					return []byte(tc.workload), nil
				}
				return []byte("ok"), nil
			}
			_, err := beginLLMPreloadTransition(run)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
			}
			if tc.failOn != "" && !strings.Contains(err.Error(), "controlled kubectl diagnostic") {
				t.Fatalf("error missing live command diagnostic: %v", err)
			}
		})
	}
}

func TestHelmLLMTierInstallExposesTransition(t *testing.T) {
	var command string
	err := helmInstallLLMWithRunner("/chart", func(name string, args ...string) ([]byte, error) {
		command = name + " " + strings.Join(args, " ")
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(command, " --wait") {
		t.Fatalf("helm install hides readiness transition behind --wait: %s", command)
	}
	if !strings.Contains(command, "--set ollama.preload.suspend=true") {
		t.Fatalf("helm install does not suspend preload for observation: %s", command)
	}
}

func unreadyLLMWorkloadsJSON() string {
	return `{"items":[
		{"metadata":{"name":"chatbot","labels":{"app.kubernetes.io/component":"chatbot"}},"spec":{"replicas":1},"status":{}},
		{"metadata":{"name":"rag0","labels":{"app.kubernetes.io/component":"rag-server"}},"spec":{"replicas":1},"status":{}}
	]}`
}
