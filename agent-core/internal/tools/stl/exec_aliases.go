// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"time"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	toolexec "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/exec"
)

// DefaultOutputLineCap is the default maximum number of output lines before truncation.
const DefaultOutputLineCap = toolexec.DefaultOutputLineCap
const defaultWaitDelay = 3 * time.Second

type (
	ExecBuilder    = toolexec.ExecBuilder
	ExecCmd        = toolexec.ExecCmd
	FailedParamCmd = toolexec.FailedParamCmd
	RunResult      = toolexec.RunResult
)

var (
	SubprocessResult   = toolexec.SubprocessResult
	CapOutput          = toolexec.CapOutput
	ExtractStringParam = toolexec.ExtractStringParam
	ExtractIntParam    = toolexec.ExtractIntParam
	ProcGroupCmd       = toolexec.ProcGroupCmd
	RunProcGroup       = toolexec.RunProcGroup
	ParseTestMetrics   = toolexec.ParseTestMetrics
	ParseBuildMetrics  = toolexec.ParseBuildMetrics
)

// RegisterToolDefs registers exec tool definitions with the given registry.
func RegisterToolDefs(reg *core.Registry, root string, defs []ToolDef) {
	toolexec.RegisterToolDefs(reg, root, defs)
}
