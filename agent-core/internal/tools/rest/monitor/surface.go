// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package monitor

import (
	"net/http"

	obsmonitor "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

// Surface is the parent-owned runtime the monitor views read. It is a struct of
// funcs so the leaf does not import rest (a parent->leaf import would otherwise
// cycle) and so the surface stays wider than a 1-3 method interface.
type Surface struct {
	WriteJSON        func(http.ResponseWriter, int, map[string]interface{})
	StateOutput      func() map[string]interface{}
	MetadataOutput   func() map[string]interface{}
	Snapshot         func() obsmonitor.Snapshot
	Machine          func() *core.MachineSpec
	Tools            func() []catalog.ToolDef
	CommandState     func() core.CommandStateSource
	MaxResponseBytes func() int
	Endpoints        func() []Route
}

// Route is the OpenAPI-facing subset of a REST endpoint. Binding values are the
// YAML literals ("read_state", "stream_events", "emit_signal") so this package
// does not import rest.
type Route struct {
	Name        string
	Method      string
	Path        string
	Binding     string
	MonitorView string
	BodySchema  map[string]interface{}
}
