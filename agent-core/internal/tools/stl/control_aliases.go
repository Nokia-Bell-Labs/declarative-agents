// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import toolcontrol "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/control"

const rereadNudge = toolcontrol.RereadNudge

type (
	NudgeRereadBuilder = toolcontrol.NudgeRereadBuilder
	DoneBuilder        = toolcontrol.DoneBuilder
	SelfInvokeBuilder  = toolcontrol.SelfInvokeBuilder
)

var SelfInvokeToolSpec = toolcontrol.SelfInvokeToolSpec
