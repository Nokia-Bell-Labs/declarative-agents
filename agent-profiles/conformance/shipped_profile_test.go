// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCopyShippedProfile proves the harness helper copies a shipped profile's
// whole directory tree into a temp dir, patches only the named fields, and
// leaves everything else byte-identical to the shipped source. It is pure file
// I/O and does not build the agent binary, so it runs in the model-free fast
// default without AGENT_CORE_ROOT.
func TestCopyShippedProfile(t *testing.T) {
	const rel = "agents/rest/profile.yaml"
	srcDir := filepath.Dir(ProfilePath(rel))

	// The rest profile is a good tree exemplar: it has a hard-coded listen
	// address in rest.yaml and an openapi/ subdirectory, so the copy must patch
	// the named field and recurse into subdirs.
	copied := CopyShippedProfile(t, rel, map[string]string{
		"127.0.0.1:0": "127.0.0.1:12345",
	})

	if base := filepath.Base(copied); base != "profile.yaml" {
		t.Fatalf("copied profile base = %q, want profile.yaml", base)
	}
	dstDir := filepath.Dir(copied)

	// The patched field is applied in the file that carries it.
	restCopy := readFile(t, filepath.Join(dstDir, "rest.yaml"))
	if !strings.Contains(restCopy, "127.0.0.1:12345") {
		t.Errorf("patched rest.yaml missing replacement; got:\n%s", restCopy)
	}
	if strings.Contains(restCopy, "127.0.0.1:0") {
		t.Errorf("patched rest.yaml still contains the original address; got:\n%s", restCopy)
	}

	// The recursive copy reaches subdirectory assets.
	if _, err := os.Stat(filepath.Join(dstDir, "openapi", "payments.yaml")); err != nil {
		t.Errorf("openapi/payments.yaml not copied recursively: %v", err)
	}

	// Files that carry no patched field are byte-identical to the shipped source.
	for _, name := range []string{"machine.yaml", "tools.yaml", "profile.yaml"} {
		if got, want := readFile(t, filepath.Join(dstDir, name)), readFile(t, filepath.Join(srcDir, name)); got != want {
			t.Errorf("%s changed by copy; the helper must patch only named fields", name)
		}
	}
}

// TestCopyShippedProfileNoPatches copies a shipped profile with no patches and
// asserts every file is byte-identical to the shipped source.
func TestCopyShippedProfileNoPatches(t *testing.T) {
	const rel = "agents/monitor/profile.yaml"
	srcDir := filepath.Dir(ProfilePath(rel))

	copied := CopyShippedProfile(t, rel, nil)
	dstDir := filepath.Dir(copied)

	for _, name := range []string{"profile.yaml", "machine.yaml", "tools.yaml", "declarations.yaml", "rest.yaml"} {
		if got, want := readFile(t, filepath.Join(dstDir, name)), readFile(t, filepath.Join(srcDir, name)); got != want {
			t.Errorf("%s changed by an unpatched copy", name)
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
