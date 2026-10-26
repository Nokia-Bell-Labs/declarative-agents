// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package install

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func actions(results []Result, dir string) map[string]string {
	out := map[string]string{}
	for _, r := range results {
		out[rel(dir, r.Path)] = r.Action
	}
	return out
}

// Vendoring copies the profiles and the agent-core tools into VendorDir, a
// rerun changes nothing, an edited file is kept unless forced, a release
// update rewrites what nobody edited, and a file the release dropped is
// removed (GH-29).
func TestVendorCopiesAndReportsDrift(t *testing.T) {
	src, dir := t.TempDir(), t.TempDir()
	s := VendorSource{Release: "v1", Agents: filepath.Join(src, "agents"), CoreTools: filepath.Join(src, "tools")}
	writeTree(t, src, map[string]string{"agents/pop/profile.yaml": "name: pop\n", "agents/pop/old.yaml": "x\n", "tools/builtin/done.yaml": "done\n"})

	r, err := Vendor(dir, s, false, false)
	if err != nil {
		t.Fatal(err)
	}
	a := actions(r, dir)
	if a[".scrum-master/agents/pop/profile.yaml"] != "created" || a[".scrum-master/agent-core/tools/builtin/done.yaml"] != "created" {
		t.Fatalf("first vendoring = %v", a)
	}
	if _, err := os.Stat(filepath.Join(dir, ManifestFile)); err != nil {
		t.Fatalf("no manifest: %v", err)
	}
	if r, _ := Vendor(dir, s, false, false); actions(r, dir)[".scrum-master/agents/pop/profile.yaml"] != "unchanged" {
		t.Fatalf("rerun = %v", actions(r, dir))
	}

	writeTree(t, dir, map[string]string{".scrum-master/agents/pop/profile.yaml": "name: pop # mine\n"})
	writeTree(t, src, map[string]string{"agents/pop/profile.yaml": "name: pop\nmachine: m.yaml\n", "tools/builtin/done.yaml": "done v2\n"})
	os.Remove(filepath.Join(src, "agents/pop/old.yaml"))
	r, _ = Vendor(dir, s, false, false)
	a = actions(r, dir)
	if a[".scrum-master/agents/pop/profile.yaml"] != "kept" || a[".scrum-master/agent-core/tools/builtin/done.yaml"] != "updated" || a[".scrum-master/agents/pop/old.yaml"] != "removed" {
		t.Fatalf("release update = %v", a)
	}
	if r, _ := Vendor(dir, s, false, false); actions(r, dir)[".scrum-master/agents/pop/profile.yaml"] != "kept" {
		t.Fatal("an edited file must stay edited on the next run")
	}
	r, _ = Vendor(dir, s, true, false)
	if actions(r, dir)[".scrum-master/agents/pop/profile.yaml"] != "updated" {
		t.Fatalf("--force = %v", actions(r, dir))
	}
	if b, _ := os.ReadFile(filepath.Join(dir, ".scrum-master/agents/pop/profile.yaml")); string(b) != "name: pop\nmachine: m.yaml\n" {
		t.Fatalf("forced file = %q", b)
	}
}

// A dry run writes nothing.
func TestVendorDryRunWritesNothing(t *testing.T) {
	src, dir := t.TempDir(), t.TempDir()
	writeTree(t, src, map[string]string{"agents/pop/profile.yaml": "name: pop\n", "tools/x.yaml": "x\n"})
	r, err := Vendor(dir, VendorSource{Release: "v1", Agents: filepath.Join(src, "agents"), CoreTools: filepath.Join(src, "tools")}, false, true)
	if err != nil || len(r) != 2 {
		t.Fatalf("results %v %v", r, err)
	}
	if _, err := os.Stat(filepath.Join(dir, VendorDir)); !os.IsNotExist(err) {
		t.Fatal("a dry run wrote " + VendorDir)
	}
}

// The vendored check names a file edited since it was vendored and one
// missing, reads no release, and refuses a repository that vendored
// nothing (GH-35).
func TestCheckVendoredNamesEditedAndMissingFiles(t *testing.T) {
	src, dir := t.TempDir(), t.TempDir()
	if _, err := CheckVendored(dir); err == nil {
		t.Fatal("a repository that vendored nothing must be refused")
	}
	writeTree(t, src, map[string]string{"agents/pop/profile.yaml": "name: pop\n", "agents/pop/machine.yaml": "m\n", "tools/x.yaml": "x\n"})
	if _, err := Vendor(dir, VendorSource{Release: "v1", Agents: filepath.Join(src, "agents"), CoreTools: filepath.Join(src, "tools")}, false, false); err != nil {
		t.Fatal(err)
	}
	if f, err := CheckVendored(dir); err != nil || len(f) != 0 {
		t.Fatalf("a fresh vendoring: %v %v", f, err)
	}
	writeTree(t, dir, map[string]string{".scrum-master/agents/pop/profile.yaml": "name: mine\n"})
	os.Remove(filepath.Join(dir, ".scrum-master/agents/pop/machine.yaml"))
	f, err := CheckVendored(dir)
	got := map[string]string{}
	for _, x := range f {
		got[rel(dir, x.Path)] = x.Problem
	}
	if err != nil || len(f) != 2 || got[".scrum-master/agents/pop/profile.yaml"] != "edited" || got[".scrum-master/agents/pop/machine.yaml"] != "missing" {
		t.Fatalf("findings %v, %v", got, err)
	}
}
