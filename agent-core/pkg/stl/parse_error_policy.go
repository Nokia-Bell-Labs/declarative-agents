// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
)

// ParseErrorPolicy returns an AfterDispatch hook that counts
// consecutive ParseFailed signals and returns BudgetExhausted when
// the limit is reached. Signals from report_parse_error and
// invoke_llm do not reset the counter (they are part of the
// parse→retry cycle).
func ParseErrorPolicy(maxConsecutive int) func(core.Command, core.Result) core.Signal {
	var consecutive int
	return func(cmd core.Command, res core.Result) core.Signal {
		sig := res.Signal
		if sig == core.ParseFailed {
			consecutive++
		} else if sig != core.ToolDone || (cmd.Name() != "report_parse_error" && cmd.Name() != "invoke_llm") {
			consecutive = 0
		}
		if maxConsecutive > 0 && consecutive >= maxConsecutive {
			return core.BudgetExhausted
		}
		return ""
	}
}
