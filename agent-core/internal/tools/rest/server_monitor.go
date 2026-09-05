// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"net/http"

	obsmonitor "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	restdef "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/definition"
	restmonitor "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/monitor"
)

const (
	monitorViewMachine          = restmonitor.ViewMachine
	monitorViewDeclaredMachines = restmonitor.ViewDeclaredMachines
	monitorViewDeclaredTools    = restmonitor.ViewDeclaredTools
	monitorViewState            = restmonitor.ViewState
	monitorViewTools            = restmonitor.ViewTools
	monitorViewMetrics          = restmonitor.ViewMetrics
	monitorViewEvents           = restmonitor.ViewEvents
	monitorViewCommandState     = restmonitor.ViewCommandState
)

func (r *serverRuntime) writeReadState(w http.ResponseWriter, name string, endpoint restdef.Endpoint) {
	restmonitor.WriteReadState(r.monitorSurface(), w, name, endpoint.MonitorView, endpoint.Labels)
}

func (r *serverRuntime) writeStaticMetadata(w http.ResponseWriter, endpoint restdef.Endpoint) {
	restmonitor.WriteStaticMetadata(r.monitorSurface(), w, endpoint.MonitorView)
}

func (r *serverRuntime) streamMonitorEvents(w http.ResponseWriter) {
	restmonitor.StreamEvents(r.monitorSurface(), w)
}

func (r *serverRuntime) monitorSurface() restmonitor.Surface {
	return restmonitor.Surface{
		WriteJSON:      writeJSON,
		StateOutput:    r.stateOutput,
		MetadataOutput: r.metadataOutput,
		Snapshot:       r.monitorSnapshot,
		Machine: func() *core.MachineSpec {
			return r.def.Monitor.Machine
		},
		DeclaredMachines: func() []core.MachineSpec {
			if len(r.def.Monitor.DeclaredMachines) == 0 && r.def.Monitor.Machine != nil {
				return []core.MachineSpec{*r.def.Monitor.Machine}
			}
			return r.def.Monitor.DeclaredMachines
		},
		Tools: func() []catalog.ToolDef {
			return r.def.Monitor.Tools
		},
		DeclaredTools: func() []map[string]interface{} {
			return r.def.Monitor.DeclaredTools
		},
		CommandState: func() core.CommandStateSource {
			return r.def.Monitor.CommandState
		},
		MaxResponseBytes: func() int {
			return r.def.Limits.MaxResponseBytes
		},
		Endpoints: r.monitorRoutes,
	}
}

func (r *serverRuntime) monitorSnapshot() obsmonitor.Snapshot {
	if r.def.Monitor.Store == nil {
		return obsmonitor.Snapshot{}
	}
	return r.def.Monitor.Store.Snapshot()
}

func (r *serverRuntime) monitorRoutes() []restmonitor.Route {
	out := make([]restmonitor.Route, 0, len(r.def.Server.Endpoints))
	for name, endpoint := range r.def.Server.Endpoints {
		out = append(out, restmonitor.Route{
			Name: name, Method: endpoint.Method, Path: endpoint.Path,
			Binding: endpoint.Binding, MonitorView: endpoint.MonitorView,
			BodySchema: endpoint.Request.BodySchema,
		})
	}
	return out
}
