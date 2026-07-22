// Copyright (c) 2026 Nokia. All rights reserved.

package docsapi

import (
	"encoding/json"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/lifecycle"
	"github.com/stretchr/testify/require"
	"net"
	"strings"
	"testing"
	"time"
)

func newLaunchDocumentationBuilder(t *testing.T, host *DocumentationHostLifecycle) launchDocumentationBuilder {
	t.Helper()
	root := t.TempDir()
	writeDocFixture(t, root, "VISION.yaml", "title: Vision\n")
	return launchDocumentationBuilder{
		config: ToolConfig{Addr: "127.0.0.1:0", DocsDir: root},
		host:   host,
	}
}

func curatorExitLoopParams(t *testing.T, host *DocumentationHostLifecycle, launchedAddr *string) core.LoopParams {
	t.Helper()
	machine, err := core.LoadMachineSpec(curatorProfileAssetPath(t, "machine.yaml"))
	require.NoError(t, err)
	reg := core.NewRegistry()
	reg.Register(core.ToolSpec{Name: "launch_documentation"}, newLaunchDocumentationBuilder(t, host))
	reg.Register(core.ToolSpec{Name: "stop_documentation"}, stopDocumentationBuilder{host: host})
	registerStaticDocsSignal(reg, "launch_docs_control", "ServerLaunched", "{}")
	registerStaticDocsSignal(reg, "launch_monitor_rest", "ServerLaunched", "{}")
	registerStaticDocsSignal(reg, "stop_monitor_rest", "ServerStopped", "{}")
	registerStaticDocsSignal(reg, "await_docs_control", "ExitRequested", `{"payload":{"reason":"operator requested shutdown","status":"success"}}`)
	reg.Register(core.ToolSpec{Name: "exit_agent"}, lifecycle.ExitBuilder{
		Config: lifecycle.ExitConfig{Status: "success"}, Shutdown: func() {},
	})
	return core.LoopParams{MachineSpec: &machine, Registry: reg, Trace: tracing.NoopTracer{}, Hooks: core.LoopHooks{
		OnResult: captureLaunchAddr(t, launchedAddr),
	}}
}

func captureLaunchAddr(t *testing.T, launchedAddr *string) func(core.RunResult, core.Result) core.RunResult {
	t.Helper()
	return func(rr core.RunResult, res core.Result) core.RunResult {
		if res.CommandName == "launch_documentation" && res.Signal == core.Signal("ServerLaunched") {
			*launchedAddr = requireResultAddr(t, res)
		}
		return rr
	}
}

func registerStaticDocsSignal(reg *core.Registry, name string, signal core.Signal, output string) {
	reg.Register(core.ToolSpec{Name: name}, staticDocsSignalBuilder{name: name, signal: signal, output: output})
}

func requireResultAddr(t *testing.T, result core.Result) string {
	t.Helper()
	var output map[string]string
	require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
	require.NotEmpty(t, output["addr"])
	return output["addr"]
}

func requireAddressReleased(t *testing.T, addr string) {
	t.Helper()
	require.Eventually(t, func() bool {
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			return false
		}
		_ = listener.Close()
		return true
	}, time.Second, 10*time.Millisecond)
}

func requireDocsHostStoppedEvent(t *testing.T, result core.RunResult) {
	t.Helper()
	require.NotEmpty(t, result.Events)
	last := result.Events[len(result.Events)-1]
	require.Equal(t, "stop_documentation", last.CommandName)
	require.Equal(t, core.Signal("ServerStopped"), last.Signal)
}

func responseTrace(t *testing.T, data []byte) map[string]interface{} {
	t.Helper()
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &body))
	trace, _ := body["trace"].(map[string]interface{})
	require.NotNil(t, trace)
	return trace
}

func proxyBackendAddr(t *testing.T, proxy *LazyMachineRequestProxy) string {
	t.Helper()
	proxy.mu.Lock()
	defer proxy.mu.Unlock()
	require.NotEmpty(t, proxy.baseURL)
	return strings.TrimPrefix(proxy.baseURL, "http://")
}
