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

// restWebhookMachine bounds the rest sample profile to its inbound webhook
// server boundary: launch the configured webhook server, await one inbound
// webhook event, then stop to reach Succeeded. It mirrors the launch/await/stop
// tail of the full rest machine (agents/rest/machine.yaml) but omits the
// client read/create/await head. The client head is exercised hermetically at
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
	restDir := ProfilePath(filepath.Join("agents", "rest"))

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
