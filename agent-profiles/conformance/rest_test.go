// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// restOllamaModel is the model the rest ollama-profile variant configures
// (testdata/conformance/rest/ollama-llm.yaml invoke_llm config). The behavioral variant run
// gates on this model being served by Ollama.
const restOllamaModel = "qwen3.6:35b-mlx"

// restWebhookMachine bounds the rest sample profile to its inbound webhook
// server boundary: launch the configured webhook server, await one inbound
// webhook event, then stop to reach Succeeded. It mirrors the launch/await/stop
// tail of the full rest machine (testdata/conformance/rest/machine.yaml) — the wiring
// TestRestShippedProfileWiring asserts the shipped machine actually ships — but
// omits the client read/create/await head. The client head is exercised hermetically at
// the agent-core package level (rest.TestRESTClient_AwaitOperationPolling drives
// create -> await_operation poll -> respond against a mock upstream); the client
// steps cannot run in a profile machine because REST client tools enforce a
// declared-only runtime input contract while the loop threads each step's full
// output verbatim, so a client step following another tool step is rejected and
// a client step as the first action fails on the non-JSON default seed.
const restWebhookMachine = `name: rest-webhook-conformance
initial_state: Idle
budget:
  max_iterations: 8
states:
  - name: Idle
    meaning: Initial state before the webhook server launches.
  - name: LaunchingWebhook
    meaning: The configured webhook server is being launched.
  - name: WaitingWebhook
    meaning: The machine waits for an inbound webhook signal.
  - name: StoppingWebhook
    meaning: The configured webhook server is shutting down.
  - name: Succeeded
    meaning: Terminal. The inbound webhook flow completed.
  - name: Failed
    meaning: Terminal. A REST boundary word failed or timed out.
terminal_states:
  - Succeeded
  - Failed
signals:
  - name: Seed
    trigger: Loop initialization.
  - name: ServerLaunched
    trigger: REST server launch registered configured routes.
  - name: PaymentWebhookReceived
    trigger: REST server await consumed the configured webhook event.
  - name: AwaitTimedOut
    trigger: REST server await timed out.
  - name: ServerStopped
    trigger: REST server stop completed or unblocked an await.
  - name: CommandError
    trigger: Runtime or REST boundary infrastructure failed.
transitions:
  - state: Idle
    signal: Seed
    next: LaunchingWebhook
    action: launch_payment_webhooks
  - state: LaunchingWebhook
    signal: ServerLaunched
    next: WaitingWebhook
    action: await_payment_webhook
  - state: LaunchingWebhook
    signal: CommandError
    next: Failed
  - state: WaitingWebhook
    signal: PaymentWebhookReceived
    next: StoppingWebhook
    action: stop_payment_webhooks
  - state: WaitingWebhook
    signal: AwaitTimedOut
    next: Failed
  - state: WaitingWebhook
    signal: ServerStopped
    next: Failed
  - state: WaitingWebhook
    signal: CommandError
    next: Failed
  - state: StoppingWebhook
    signal: ServerStopped
    next: Succeeded
  - state: StoppingWebhook
    signal: CommandError
    next: Failed
`

const restWebhookTools = `tools:
  - launch_payment_webhooks
  - await_payment_webhook
  - stop_payment_webhooks
`

// TestRestConformance drives the rest sample profile's inbound webhook boundary
// to a Succeeded terminal. It rewrites the profile's REST config to bind the
// payment webhook server to a free loopback port (and points the OpenAPI import
// at its absolute source), launches the bounded server machine, posts the
// configured inbound webhook, and asserts the launch/await/stop server words
// route the enqueued signal to Succeeded with no error-status spans.
//
// Traces srd007-rest: the profile owns inbound REST route authority
// (launch_payment_webhooks), an HTTP handler enqueues a signal the machine
// consumes as a visible await word (await_payment_webhook), and shutdown is an
// explicit machine word (stop_payment_webhooks) reaching Succeeded.
func TestRestConformance(t *testing.T) {
	RequireCoreRoot(t)
	tmp := t.TempDir()
	addr := FreeAddr(t)
	restDir := ProfilePath(filepath.Join("testdata", "conformance", "rest"))

	restContent := rewriteFile(t, filepath.Join(restDir, "rest.yaml"), map[string]string{
		"127.0.0.1:0":                 addr,
		"path: openapi/payments.yaml": "path: " + filepath.Join(restDir, "openapi", "payments.yaml"),
	})
	restPath := writeEphemeral(t, tmp, "rest.yaml", restContent)
	machinePath := writeEphemeral(t, tmp, "machine.yaml", restWebhookMachine)
	toolsPath := writeEphemeral(t, tmp, "tools.yaml", restWebhookTools)
	profilePath := writeEphemeral(t, tmp, "profile.yaml", fmt.Sprintf(`name: rest-webhook-conformance
machine: %q
tools:
  - %q
tool_declarations:
  - %q
rest_definitions:
  - %q
`, machinePath, toolsPath, filepath.Join(restDir, "declarations.yaml"), restPath))

	server := Serve(t, ServeConfig{Profile: profilePath})
	postWebhookUntilAccepted(t, "http://"+addr+"/webhooks/payment", `{"id":"pay_demo","state":"settled"}`, 15*time.Second)
	result := server.WaitExit(15 * time.Second)

	// srd007: clean terminal outcome with a single root and no error spans.
	result.RequireExit(t, 0)
	result.RootRequired(t)
	result.RequireNoErrorSpans(t)

	// srd007: the launch -> await -> stop inbound REST server vocabulary is visible.
	result.RequireToolSpans(t, "launch_payment_webhooks", "await_payment_webhook", "stop_payment_webhooks")

	// srd007: the machine reaches the Succeeded terminal state.
	result.RequireTerminalState(t, "Succeeded")
}

// TestRestShippedProfileWiring asserts, model-free and ungated, that the two
// wrappers an operator ships for the rest family are wired as the behavioral
// runs assume. It proves (a) the shipped sample machine
// (testdata/conformance/rest/machine.yaml) actually contains the launch -> await -> stop
// inbound webhook boundary that TestRestConformance drives via a bounded
// machine — the sample profile's client read/create/await head cannot run
// in-profile (see restWebhookMachine), so the boundary is the runnable slice of
// the shipped grammar — and (b) the shipped ollama-profile.yaml variant
// references its own machine, tools, Ollama LLM declaration, and REST
// definitions. Unlike the behavioral runs it needs no server and no model, so
// it holds in the fast default and where Ollama is absent.
//
// Traces srd007-rest: the shipped machine owns the inbound REST server boundary
// (launch_payment_webhooks -> await_payment_webhook -> stop_payment_webhooks ->
// Succeeded).
func TestRestShippedProfileWiring(t *testing.T) {
	// (a) The shipped sample machine wires the inbound webhook boundary.
	var machine struct {
		InitialState string              `yaml:"initial_state"`
		Transitions  []machineTransition `yaml:"transitions"`
	}
	unmarshalShipped(t, filepath.Join("testdata", "conformance", "rest", "machine.yaml"), &machine)

	requireTransition(t, machine.Transitions, "AwaitingPayment", "RESTResponded", "LaunchingWebhook", "launch_payment_webhooks")
	requireTransition(t, machine.Transitions, "LaunchingWebhook", "ServerLaunched", "WaitingWebhook", "await_payment_webhook")
	requireTransition(t, machine.Transitions, "WaitingWebhook", "PaymentWebhookReceived", "StoppingWebhook", "stop_payment_webhooks")
	requireTransition(t, machine.Transitions, "StoppingWebhook", "ServerStopped", "Succeeded", "")

	// (b) The shipped ollama-profile variant is wired to its own machine, tools,
	// Ollama LLM declaration, and REST definitions.
	var variant struct {
		Machine          string   `yaml:"machine"`
		Tools            []string `yaml:"tools"`
		ToolDeclarations []string `yaml:"tool_declarations"`
		RESTDefinitions  []string `yaml:"rest_definitions"`
	}
	unmarshalShipped(t, filepath.Join("testdata", "conformance", "rest", "ollama-profile.yaml"), &variant)

	if variant.Machine != "ollama-machine.yaml" {
		t.Errorf("shipped rest ollama-profile machine = %q, want ollama-machine.yaml", variant.Machine)
	}
	if !contains(variant.Tools, "ollama-tools.yaml") {
		t.Errorf("shipped rest ollama-profile tools = %v, want to include ollama-tools.yaml", variant.Tools)
	}
	if !contains(variant.ToolDeclarations, "ollama-llm.yaml") {
		t.Errorf("shipped rest ollama-profile tool_declarations = %v, want to include ollama-llm.yaml", variant.ToolDeclarations)
	}
	if !contains(variant.RESTDefinitions, "ollama-rest.yaml") {
		t.Errorf("shipped rest ollama-profile rest_definitions = %v, want to include ollama-rest.yaml", variant.RESTDefinitions)
	}
}

// TestRestOllamaConformance runs the shipped rest ollama-profile.yaml variant
// exactly as an operator ships it — no synthesis and no patching. The model
// boundary (invoke_llm) drives a live REST client word (ollama_list_models)
// whose upstream is the local Ollama itself, so unlike the sample machine's
// machine-sequenced client head this model-selected client call runs in-profile.
//
// It is Ollama-gated: invoke_llm pings Ollama at tool registration and calls the
// model during the run, so with no reachable model the profile cannot start (see
// ollama.go). Because the model chooses when to call the REST word and when to
// answer, the run is asserted to reach one of the profile's shipped terminal
// states via a real model boundary that exercised the REST word.
//
// Traces srd007-rest: OpenAPI imports back a live REST client word the model
// selects, and the REST result feeds the model boundary.
func TestRestOllamaConformance(t *testing.T) {
	RequireCoreRoot(t)
	RequireOllama(t, restOllamaModel)

	result := Run(t, RunConfig{
		Profile:   filepath.Join("testdata", "conformance", "rest", "ollama-profile.yaml"),
		Directory: t.TempDir(),
	})

	// A single root, with a real model boundary (invoke_llm -> chat span) ...
	result.RootRequired(t)
	if chats := result.Spans.NamePrefixed("chat "); len(chats) == 0 {
		t.Fatalf("no chat model-boundary span; span names: %v\noutput:\n%s", result.Spans.Names(), result.Output)
	}
	// ... that selected the live OpenAPI-backed REST word.
	result.RequireToolSpans(t, "ollama_list_models")

	// The run reached a non-failure shipped terminal state; the exit code is not
	// asserted because it varies with the terminal (Succeeded vs BudgetExceeded).
	result.RequireTerminalState(t, "Succeeded", "BudgetExceeded")
}

// postWebhookUntilAccepted posts the inbound webhook body until the server
// accepts it (202) or the timeout elapses, absorbing the bind race between the
// freed port and the launched listener. It stops after the first acceptance so
// the bounded queue receives exactly one event.
func postWebhookUntilAccepted(t *testing.T, url, body string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	last := "no attempt"
	for time.Now().Before(deadline) {
		resp, err := http.Post(url, "application/json", strings.NewReader(body))
		if err != nil {
			last = err.Error()
		} else {
			code := resp.StatusCode
			_ = resp.Body.Close()
			if code == http.StatusAccepted {
				return
			}
			last = fmt.Sprintf("status %d", code)
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("webhook never accepted at %s within %s: %s", url, timeout, last)
}
