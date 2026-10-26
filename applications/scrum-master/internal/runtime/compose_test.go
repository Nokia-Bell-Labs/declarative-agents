// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Compose writes a workspace over both modules at the later go version and
// an overlay adding the registration to cmd/agent, naming the two modules
// by their go.mod (GH-27).
func TestComposeWritesTheWorkspaceAndTheRegistration(t *testing.T) {
	core, app := t.TempDir(), t.TempDir()
	if resolved, err := filepath.EvalSymlinks(core); err == nil {
		core = resolved
	}
	if resolved, err := filepath.EvalSymlinks(app); err == nil {
		app = resolved
	}
	os.WriteFile(filepath.Join(core, "go.mod"), []byte("module example.com/core\n\ngo 1.26.3\n"), 0o644)
	os.WriteFile(filepath.Join(app, "go.mod"), []byte("module example.com/app\n\ngo 1.26\n"), 0o644)
	c, err := Compose(core, app)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Cleanup()
	if c.Dir != core {
		t.Fatalf("dir = %s, want %s", c.Dir, core)
	}
	work := strings.TrimPrefix(c.Env[0], "GOWORK=")
	b, _ := os.ReadFile(work)
	if !strings.HasPrefix(string(b), "go 1.26.3\n") || !strings.Contains(string(b), core) || !strings.Contains(string(b), app) {
		t.Fatalf("go.work = %s", b)
	}
	raw, _ := os.ReadFile(c.Flags[1])
	var overlay struct{ Replace map[string]string }
	if err := json.Unmarshal(raw, &overlay); err != nil {
		t.Fatal(err)
	}
	src, ok := overlay.Replace[filepath.Join(core, "cmd", "agent", "zz_scrum_master_words.go")]
	if !ok {
		t.Fatalf("overlay = %s", raw)
	}
	reg, _ := os.ReadFile(src)
	if !strings.Contains(string(reg), `"example.com/core/pkg/inprocess"`) || !strings.Contains(string(reg), `"example.com/app/words"`) {
		t.Fatalf("registration = %s", reg)
	}
	c.Cleanup()
	if _, err := os.Stat(work); !os.IsNotExist(err) {
		t.Fatalf("cleanup left %s", work)
	}
}

func TestLaterGo(t *testing.T) {
	for _, c := range [][3]string{{"1.26", "1.26.3", "1.26.3"}, {"1.27", "1.26.3", "1.27"}, {"1.26.3", "1.26.3", "1.26.3"}} {
		if got := laterGo(c[0], c[1]); got != c[2] {
			t.Errorf("laterGo(%s, %s) = %s, want %s", c[0], c[1], got, c[2])
		}
	}
}
