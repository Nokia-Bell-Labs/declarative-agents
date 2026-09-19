// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package llm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/corepath"
)

// TestMain maps the agent-core install root onto this checkout, so a config
// naming a legacy provider resolves to the shipped library's dialect here as
// it does under the runtime image's /opt/agent-core.
func TestMain(m *testing.M) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		panic(err)
	}
	corepath.SetInstallRoot(root)
	os.Exit(m.Run())
}
