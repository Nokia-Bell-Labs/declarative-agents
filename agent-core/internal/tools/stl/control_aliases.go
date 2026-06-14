// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import toolcontrol "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/control"

const rereadNudge = toolcontrol.RereadNudge

type (
	NudgeRereadBuilder = toolcontrol.NudgeRereadBuilder
	DoneBuilder        = toolcontrol.DoneBuilder
	SelfInvokeBuilder  = toolcontrol.SelfInvokeBuilder
)

var SelfInvokeToolSpec = toolcontrol.SelfInvokeToolSpec
