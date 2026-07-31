// Copyright (c) 2026 Nokia. All rights reserved.

package kindrig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentCoreImageBuildArgsTagCurrentContext(t *testing.T) {
	args := strings.Join(AgentCoreImageBuildArgs(DefaultAgentCoreImage), " ")
	if args != "build -t "+DefaultAgentCoreImage+" ." {
		t.Fatalf("build args = %q", args)
	}
}

func TestCopyTreeContentsCopiesRegularFilesOnly(t *testing.T) {
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "builtin", "otlp"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "builtin", "otlp", "all.yaml"), []byte("tools: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "tools")
	if err := copyTreeContents(src, dst); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dst, "builtin", "otlp", "all.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "tools: []\n" {
		t.Fatalf("copied content = %q", data)
	}
}
