// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package agentbuild

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildTagSelectsProduction(t *testing.T) {
	t.Parallel()
	if BuildTag != "production" {
		t.Fatalf("BuildTag = %q, want production", BuildTag)
	}
}

func TestBuildReturnsErrorForMissingTarget(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var log bytes.Buffer
	out, err := Build(dir, filepath.Join(dir, "agent-bin"), &log)
	if err == nil {
		t.Fatal("expected build error for a root without ./cmd/agent")
	}
	if out != "" {
		t.Fatalf("output path = %q, want empty on failure", out)
	}
	if !strings.Contains(err.Error(), dir) {
		t.Fatalf("error %q does not mention coreRoot %q", err, dir)
	}
}
