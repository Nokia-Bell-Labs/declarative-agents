// Copyright (c) 2026 Nokia. All rights reserved.

package exec

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

func TestExecBuilderResolvesLateBoundRootAtExecution(t *testing.T) {
	root := "/first"
	builder := &ExecBuilder{
		Def:      catalog.ToolDef{Name: "dynamic-root", Binary: "true"},
		RootFunc: func() string { return root },
	}
	cmd := builder.Build(core.Result{}).(*ExecCmd)

	root = "/second"

	require.Equal(t, "/second", cmd.execDir())
}
