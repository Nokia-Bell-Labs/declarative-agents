// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package spectool

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestSpecToolSourceUsesThePinnedRelease(t *testing.T) {
	dir, release := t.TempDir(), t.TempDir()
	const releaseModule = "example.com/parent/agent-core"
	os.WriteFile(filepath.Join(release, "go.mod"), []byte("module "+releaseModule+"\n\ngo 1.26\n"), 0o644)
	os.WriteFile(filepath.Join(release, "go.sum"), []byte("example.com/x v1.0.0 h1:abc=\n"), 0o644)
	if err := writeSource(dir, release); err != nil {
		t.Fatal(err)
	}
	mod, _ := os.ReadFile(filepath.Join(dir, "go.mod"))
	if !strings.Contains(string(mod), "replace "+releaseModule+" => "+release) {
		t.Fatalf("go.mod does not resolve the release to its extracted source:\n%s", mod)
	}
	if sum, _ := os.ReadFile(filepath.Join(dir, "go.sum")); !strings.Contains(string(sum), "example.com/x") {
		t.Fatal("go.sum does not carry the release's checksums")
	}
	main, _ := os.ReadFile(filepath.Join(dir, "main.go"))
	for _, want := range []string{`"` + releaseModule + `/pkg/spec/markdown"`, "ConvertYAML", "FormatLinked", "LinkIndexes", "Diagnose", "not-canonical"} {
		if !strings.Contains(string(main), want) {
			t.Errorf("main.go lacks %s", want)
		}
	}
}

func TestSpecToolIsBuiltOncePerTag(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(Binary(dir), []byte("built"), 0o755)
	ran := 0
	got, err := Ensure(dir, filepath.Join(dir, "src", "agent-core"), func(string, string, ...string) error { ran++; return nil })
	if err != nil || got != Binary(dir) || ran != 0 {
		t.Fatalf("got %s err %v ran %d", got, err, ran)
	}
	if !strings.HasPrefix(filepath.Base(got), "spectool-built-") || filepath.Base(got) == "spectool-built-" {
		t.Errorf("binary %s is not keyed on the tool's source", got)
	}
}

func TestModuleOfReadsTheDeclaredPath(t *testing.T) {
	gomod := filepath.Join(t.TempDir(), "go.mod")
	os.WriteFile(gomod, []byte("// comment\nmodule example.com/parent/agent-core\n\ngo 1.26\n"), 0o644)
	if got, err := moduleOf(gomod); err != nil || got != "example.com/parent/agent-core" {
		t.Fatalf("got %q err %v", got, err)
	}
}

// TestSpecToolReadsObjectivesAsSRDs holds the tool to reading a hierarchy
// or objective document, at any depth under its directory, with the SRD
// declaration under the scope of its kind (design.md hierarchy).
func TestSpecToolReadsObjectivesAsSRDs(t *testing.T) {
	m := regexp.MustCompile(`hierarchyPath = regexp.MustCompile\(("[^"]+")\)`).FindStringSubmatch(Program("main", "m"))
	if m == nil {
		t.Fatal("the program declares no hierarchyPath")
	}
	pattern, err := strconv.Unquote(m[1])
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(pattern)
	for path, want := range map[string]bool{
		"docs/specs/objectives/obj001-reliable-agents.md":       true,
		"docs/specs/objectives/obj001-reliable-agents.yaml":     true,
		"testdata/objectives/malformed/obj002-missing-owner.md": true,
		"docs/specs/software-requirements/srd001-labels.md":     false,
		"docs/specs/objectives/README.md":                       false,
		"testdata/legacy/obj001-reliable-agents.yaml":           false,
		"docs/specs/initiatives/in001-reliable-agents.md":       true,
		"docs/specs/stories/st001-score-trajectories.md":        true,
		"testdata/hierarchy/valid/docs/specs/themes/th001-a.md": true,
		"docs/specs/stories/README.md":                          false,
	} {
		if re.MatchString(path) != want {
			t.Errorf("%s: hierarchy = %v, want %v", path, !want, want)
		}
	}
	for _, want := range []string{`declarations["srd"]`, `"objective"`, `"story"`} {
		if !strings.Contains(Program("main", "m"), want) {
			t.Errorf("the program lacks %s", want)
		}
	}
}

func TestPinnedReadsThePin(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "docs"), 0o755)
	os.WriteFile(filepath.Join(dir, PinsFile), []byte("pins:\n  "+ParentRepository+": v9.9.9\n"), 0o644)
	if tag, err := Pinned(dir); err != nil || tag != "v9.9.9" {
		t.Fatalf("got %q err %v", tag, err)
	}
	os.WriteFile(filepath.Join(dir, PinsFile), []byte("pins: {}\n"), 0o644)
	if _, err := Pinned(dir); err == nil || !strings.Contains(err.Error(), "pins no release") {
		t.Fatalf("an empty pin: %v", err)
	}
}
