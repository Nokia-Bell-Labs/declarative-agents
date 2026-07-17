// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestKnowledgeManagerConformance launches the documentation-curator profile,
// waits for its documentation host to become healthy, posts the control
// lifecycle exit, and asserts the machine stops its owned listeners and reaches
// the Done terminal state.
//
// This is the heaviest server family: the profile binds a documentation host, a
// control REST server, a machine-request server, and a monitor REST server, so
// the test rewrites all four bound addresses into ephemeral copies of the
// profile's builtin.yaml, rest.yaml, and openapi.yaml (mirroring the proven
// magefiles/integration_docs.go setup) before serving.
//
// Traces srd011-knowledge-manager: R2.2 (documentation-serving and lifecycle
// control tool families), R3.1 (control exit and listener shutdown), and R3.2
// (Done terminal outcome).
func TestKnowledgeManagerConformance(t *testing.T) {
	coreRoot := RequireCoreRoot(t)
	tmp := t.TempDir()
	profileDir := ProfilePath(filepath.Join("agents", "knowledge-manager", "documentation-curator"))

	docsAddr := FreeAddr(t)
	controlAddr := FreeAddr(t)
	requestAddr := FreeAddr(t)
	monitorAddr := FreeAddr(t)

	// builtin.yaml: point the documentation host at a free address and absolute
	// corpus paths so the host serves without depending on the working dir.
	builtinContent := rewriteFile(t, filepath.Join(profileDir, "builtin.yaml"), map[string]string{
		"addr: :18081":         "addr: " + fmt.Sprintf("%q", docsAddr),
		"docs_dir: docs":       "docs_dir: " + fmt.Sprintf("%q", filepath.Join(coreRoot, "docs")),
		"configs_dir: configs": "configs_dir: " + fmt.Sprintf("%q", filepath.Join(coreRoot, "configs")),
		"source_dir: .":        "source_dir: " + fmt.Sprintf("%q", coreRoot),
		"profile_path: agents/knowledge-manager/documentation-curator/profile.yaml": "profile_path: " + fmt.Sprintf("%q", filepath.Join(tmp, "profile.yaml")),
	})
	writeEphemeral(t, tmp, "builtin.yaml", builtinContent)

	// rest.yaml: bind the control, machine-request, monitor, and docs-client
	// addresses to the free ports allocated above.
	restContent := rewriteFile(t, filepath.Join(profileDir, "rest.yaml"), map[string]string{
		"http://127.0.0.1:18081":   "http://" + docsAddr,
		"ports: [18081]":           "ports: [" + PortOf(t, docsAddr) + "]",
		"ports: [18082]":           "ports: [" + PortOf(t, controlAddr) + "]",
		"ports: [18083]":           "ports: [" + PortOf(t, requestAddr) + "]",
		"ports: [18084]":           "ports: [" + PortOf(t, monitorAddr) + "]",
		"address: 127.0.0.1:18082": "address: " + controlAddr,
		"address: 127.0.0.1:18083": "address: " + requestAddr,
		"address: 127.0.0.1:18084": "address: " + monitorAddr,
	})
	restPath := writeEphemeral(t, tmp, "rest.yaml", restContent)

	// openapi.yaml is referenced by rest.yaml relative to its own directory, so
	// it must sit beside the ephemeral rest.yaml with a matching base URL.
	openapiContent := rewriteFile(t, filepath.Join(profileDir, "openapi.yaml"), map[string]string{
		"http://127.0.0.1:18081": "http://" + docsAddr,
	})
	writeEphemeral(t, tmp, "openapi.yaml", openapiContent)

	// request-machine.yaml and ui/ux.yaml are resolved relative to the ephemeral
	// profile/rest, so copy them verbatim into the temp tree.
	writeEphemeral(t, tmp, "request-machine.yaml", rewriteFile(t, filepath.Join(profileDir, "request-machine.yaml"), nil))
	uiDir := filepath.Join(tmp, "ui")
	if err := os.MkdirAll(uiDir, 0o755); err != nil {
		t.Fatalf("create ui dir: %v", err)
	}
	writeEphemeral(t, uiDir, "ux.yaml", rewriteFile(t, filepath.Join(profileDir, "ui", "ux.yaml"), nil))

	profilePath := writeEphemeral(t, tmp, "profile.yaml", fmt.Sprintf(`name: documentation-curator-conformance
machine: %q
tools:
  - %q
tool_declarations:
  - %q
  - %q
  - %q
  - %q
rest_definitions:
  - %q
`, filepath.Join(profileDir, "machine.yaml"),
		filepath.Join(profileDir, "tools.yaml"),
		filepath.Join(tmp, "builtin.yaml"),
		filepath.Join(profileDir, "declarations.yaml"),
		filepath.Join(profileDir, "request-declarations.yaml"),
		filepath.Join(coreRoot, "tools", "builtin", "lifecycle", "exit-agent.yaml"),
		restPath))

	server := Serve(t, ServeConfig{Profile: profilePath, Directory: coreRoot})
	server.WaitHealthy("http://"+docsAddr+"/api/v1/health", 15*time.Second)
	if status := server.Post("http://"+controlAddr+"/api/lifecycle/exit", `{"reason":"conformance","status":"success"}`); status != http.StatusAccepted {
		t.Fatalf("lifecycle exit POST status = %d, want %d", status, http.StatusAccepted)
	}
	result := server.WaitExit(15 * time.Second)

	// srd011 R3.2: clean terminal outcome with no error-status spans.
	result.RequireExit(t, 0)
	result.RootRequired(t)
	result.RequireNoErrorSpans(t)

	// srd011 R2.2/R3.1: documentation host, control launch/await, monitor
	// launch/stop, and exit_agent lifecycle vocabulary is visible.
	result.RequireToolSpans(t,
		"serve_documentation",
		"launch_curator_control",
		"launch_monitor_rest",
		"await_curator_control",
		"exit_agent",
		"stop_monitor_rest",
	)

	// srd011 R3.2: the machine reaches the Done terminal state.
	result.RequireTerminalState(t, "Done")
}
