// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/version"
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

// runningAs stamps the running binary as a release for one test.
func runningAs(t *testing.T, release string) {
	t.Helper()
	saved := version.Release
	version.Release = release
	t.Cleanup(func() { version.Release = saved })
}

// vendoredAt is a repository vendoring a release, and the source it came from.
func vendoredAt(t *testing.T, release string) (dir string, src VendorSource) {
	t.Helper()
	root := t.TempDir()
	dir = t.TempDir()
	writeTree(t, root, map[string]string{"agents/pop/profile.yaml": "name: " + release + "\n", "tools/done.yaml": "done\n"})
	src = VendorSource{Release: release, Agents: filepath.Join(root, "agents"), CoreTools: filepath.Join(root, "tools")}
	if _, err := Vendor(dir, src, false, false); err != nil {
		t.Fatal(err)
	}
	return dir, src
}

// A second machine that pulled the upgrade before it ran install.sh cannot
// take the repository back: the vendoring refuses, naming both releases and
// the fix, and nothing changes, a forced run and a dry run included (GH-2899).
func TestVendorRefusesAnOlderRelease(t *testing.T) {
	dir, src := vendoredAt(t, "v0.20261103.4")
	before, err := os.ReadFile(filepath.Join(dir, ManifestFile))
	if err != nil {
		t.Fatal(err)
	}
	src.Release = "v0.20261103.2"
	writeTree(t, filepath.Dir(src.Agents), map[string]string{"agents/pop/profile.yaml": "name: older\n"})
	for _, dryRun := range []bool{false, true} {
		_, err := Vendor(dir, src, true, dryRun)
		if err == nil {
			t.Fatalf("dryRun=%v: an older release was vendored", dryRun)
		}
		for _, want := range []string{"vendors v0.20261103.4", "v0.20261103.2", "install.sh v0.20261103.4", "nothing was written"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("the refusal %q does not name %q", err, want)
			}
		}
	}
	after, _ := os.ReadFile(filepath.Join(dir, ManifestFile))
	if string(after) != string(before) {
		t.Errorf("the manifest changed:\n%s", after)
	}
	profile, _ := os.ReadFile(filepath.Join(dir, AgentsVendored, "pop", "profile.yaml"))
	if !strings.Contains(string(profile), "v0.20261103.4") {
		t.Errorf("the vendored profile changed: %s", profile)
	}
}

// An agent older than the release it would vendor refuses, as when PATH finds
// an older agent than install.sh installed; the release's own agent vendors,
// the repository moving forward (GH-2899).
func TestVendorRefusesAnAgentOlderThanTheRelease(t *testing.T) {
	dir, src := vendoredAt(t, "v0.20261103.2")
	src.Release = "v0.20261103.4"
	runningAs(t, "v0.20261103.3")
	_, err := Vendor(dir, src, false, false)
	if err == nil || !strings.Contains(err.Error(), "this agent is v0.20261103.3") || !strings.Contains(err.Error(), "PATH") {
		t.Fatalf("Vendor = %v, want a refusal naming the agent and PATH", err)
	}
	if got, _ := VendoredRelease(dir); got != "v0.20261103.2" {
		t.Fatalf("the refusal moved the repository to %q", got)
	}
	runningAs(t, "v0.20261103.4")
	if _, err := Vendor(dir, src, false, false); err != nil {
		t.Fatalf("the release's own agent: %v", err)
	}
	if got, _ := VendoredRelease(dir); got != "v0.20261103.4" {
		t.Fatalf("vendored = %q", got)
	}
}

// A development build or a source tree is no release, so neither is refused,
// whichever way the releases around it point (GH-2899).
func TestVendorRefusesNothingThatIsNoRelease(t *testing.T) {
	runningAs(t, "")
	dir, src := vendoredAt(t, "v0.20261103.4")
	src.Release = "source:/some/tree"
	if _, err := Vendor(dir, src, false, false); err != nil {
		t.Fatalf("a source tree: %v", err)
	}
	dir, src = vendoredAt(t, "source:/some/tree")
	src.Release = "v0.20261103.2"
	if _, err := Vendor(dir, src, false, false); err != nil {
		t.Fatalf("a release over a source tree: %v", err)
	}
	if got := StaleAgent(dir); got != "" {
		t.Errorf("a development build is stale: %q", got)
	}
}

// init and onboard read the same refusal before their first write (GH-2899).
func TestCheckSourceRefusesAnOlderInstall(t *testing.T) {
	dir, _ := vendoredAt(t, "v0.20261103.4")
	home := t.TempDir()
	writeTree(t, home, map[string]string{"release": "v0.20261103.2\n"})
	t.Setenv(SourceEnv, "")
	t.Setenv(HomeEnv, home)
	err := CheckSource(dir)
	if err == nil || !strings.Contains(err.Error(), "install.sh v0.20261103.4") || !strings.Contains(err.Error(), "nothing was written") {
		t.Fatalf("CheckSource = %v", err)
	}
	writeTree(t, home, map[string]string{"release": "v0.20261103.5\n"})
	if err := CheckSource(dir); err != nil {
		t.Fatalf("a newer install: %v", err)
	}
}

// The running agent reads as stale against the release its repository
// vendors, from any folder in it, and never past the repository's top
// (GH-2899).
func TestStaleAgent(t *testing.T) {
	dir, _ := vendoredAt(t, "v0.20261103.4")
	runningAs(t, "v0.20261103.2")
	got := StaleAgent(dir)
	for _, want := range []string{"v0.20261103.2", "v0.20261103.4", "install.sh v0.20261103.4"} {
		if !strings.Contains(got, want) {
			t.Errorf("StaleAgent = %q, want it to name %q", got, want)
		}
	}
	deep := filepath.Join(dir, "docs", "specs")
	os.MkdirAll(deep, 0o755)
	if StaleAgentAt(deep) != got {
		t.Errorf("from a subfolder: %q", StaleAgentAt(deep))
	}
	inner := filepath.Join(dir, "vendor", "other")
	os.MkdirAll(filepath.Join(inner, ".git"), 0o755)
	if s := StaleAgentAt(inner); s != "" {
		t.Errorf("read past another repository's top: %q", s)
	}
	runningAs(t, "v0.20261103.4")
	if s := StaleAgent(dir); s != "" {
		t.Errorf("the release's own agent reads as stale: %q", s)
	}
	runningAs(t, "v0.20261103.5")
	if s := StaleAgent(dir); s != "" {
		t.Errorf("a newer agent reads as stale: %q", s)
	}
}
