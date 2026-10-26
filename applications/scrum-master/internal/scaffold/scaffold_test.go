// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package scaffold

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/beads"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/routing"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker/fake"
)

func identity(t *testing.T) {
	t.Helper()
	for _, k := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(k, "operator")
	}
	for _, k := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(k, "operator@example.invalid")
	}
}

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// repo is an empty git repository.
func repo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run(t, dir, "init", "-q", "-b", "main")
	return dir
}

// skeleton is every file init writes, less the skills.
var skeleton = []string{
	"control/labels.md", "control/policy.md", "control/states.md",
	"docs/ARCHITECTURE.md", "docs/SPECIFICATIONS.md", "docs/VISION.md", "docs/constitutions/design.md",
	"docs/corpus-charter.yaml", "docs/external-pins.yaml", "docs/road-map.md",
	"docs/specs/software-requirements/srd001-system.md",
	"AGENTS.md", "README.md", "docs/specs/epics/README.md", "docs/specs/initiatives/README.md", "docs/specs/themes/README.md",
	"docs/engineering/README.md",
}

// orientationMarkers are the begin and end lines every orientation file
// carries around scrum-master's text (srd043 R3.5).
var orientationMarkers = []string{"<!-- managed:begin", BlockEnd}

func TestInitWritesOrientation(t *testing.T) {
	dir := repo(t)
	if _, err := Init(InitOptions{Dir: dir}); err != nil {
		t.Fatal(err)
	}
	for f, want := range map[string]string{
		"README.md":                        "# Program system repository",
		"docs/specs/themes/README.md":      "`thNNN-short-name.md`",
		"docs/specs/initiatives/README.md": "`inNNN-short-name.md`",
		"docs/specs/epics/README.md":       "`epNNN-short-name.md`",
		"docs/engineering/README.md":       "`engNN-short-name.md`",
	} {
		b, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range append([]string{want}, orientationMarkers...) {
			if !strings.Contains(string(b), m) {
				t.Errorf("%s lacks %q", f, m)
			}
		}
		if strings.Contains(string(b), "{{") {
			t.Errorf("%s has an unfilled placeholder", f)
		}
	}
	for _, f := range []string{"docs/specs/features/README.md", "docs/specs/stories/README.md"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			t.Errorf("a system repository got %s", f)
		}
	}
}

func actions(rs []Result) map[string]string {
	m := map[string]string{}
	for _, r := range rs {
		m[r.Path] = r.Action
	}
	return m
}

func TestInitWritesSkeleton(t *testing.T) {
	dir := repo(t)
	r, err := Init(InitOptions{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	got := actions(r.Files)
	for _, f := range skeleton {
		if got[f] != "created" {
			t.Errorf("%s: %q, want created", f, got[f])
		}
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("%s not written: %v", f, err)
		}
	}
	if len(got) != len(skeleton) {
		t.Errorf("init wrote %d files, want %d: %v", len(got), len(skeleton), got)
	}
}

func TestInitWritesControlFiles(t *testing.T) {
	dir := repo(t)
	if _, err := Init(InitOptions{Dir: dir}); err != nil {
		t.Fatal(err)
	}
	f := fake.New()
	for _, n := range []string{program.LabelsFile, program.PolicyFile} {
		b, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			t.Fatal(err)
		}
		f.AddFile("g/system", "main", n, b)
	}
	labels, err := program.LoadLabels(f, "g/system", "main")
	if err != nil || !reflect.DeepEqual(labels, program.DefaultLabels()) {
		t.Fatalf("control/labels.md reads back as %v, %v; want the defaults", labels, err)
	}
	// The fresh program's subsystem SRDs declare no component yet, and the
	// system's corpus release needs a peer read.
	want := InitPolicy()
	want.ComponentsRead = true
	// The written policy declares the default admission rules by name, so
	// removing one from the file disables it (GH-130).
	want.AdmissionRules = program.RuleNames()
	if g := want.Gate("corpus"); len(g) != 2 || g[0].Command != ProgramAudit || g[1].Kind != "peer-read" || want.AuditCommand != ProgramAudit {
		t.Fatalf("init's corpus gate = %+v, audit %q; want scrum-master backlog check and a peer read", g, want.AuditCommand)
	}
	policy, err := program.LoadPolicy(f, "g/system", "main")
	if err != nil || !reflect.DeepEqual(policy, want) {
		t.Fatalf("control/policy.md reads back as %+v, %v; want %+v", policy, err, want)
	}
}

func TestInitInstallsSkills(t *testing.T) {
	dir := repo(t)
	r, err := Init(InitOptions{Dir: dir, Agents: []string{"claude"}, Version: "test"})
	if err != nil || len(r.Skills) == 0 {
		t.Fatalf("init with --agent claude: %d skill results, %v", len(r.Skills), err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "skills", "implement", "SKILL.md")); err != nil {
		t.Fatalf("implement not installed for claude: %v", err)
	}
}

// bare is a bare repository, empty or holding one commit on main.
func bare(t *testing.T, withCommit bool) string {
	t.Helper()
	identity(t)
	remote := filepath.Join(t.TempDir(), "tracker.git")
	run(t, filepath.Dir(remote), "init", "-q", "--bare", "-b", "main", remote)
	if withCommit {
		seed := filepath.Join(t.TempDir(), "seed")
		run(t, filepath.Dir(seed), "clone", "-q", remote, seed)
		run(t, seed, "commit", "-q", "--allow-empty", "-m", "first")
		run(t, seed, "push", "-q", "origin", "HEAD:main")
	}
	return remote
}

func TestInitSeedsEmptyTracker(t *testing.T) {
	remote, dir := bare(t, false), repo(t)
	r, err := Init(InitOptions{Dir: dir, Tracker: remote, Group: "g", User: "operator"})
	if err != nil || r.Tracker == nil || !r.Tracker.Seeded {
		t.Fatalf("init --tracker on an empty tracker: %+v, %v", r.Tracker, err)
	}
	clone := filepath.Join(t.TempDir(), "check")
	run(t, filepath.Dir(clone), "clone", "-q", remote, clone)
	labels, err := beads.NewTracker(clone, "operator", dir).GroupLabels("g")
	if err != nil || len(labels) != len(program.DefaultLabels()) {
		t.Fatalf("tracker labels: %d, %v; want %d", len(labels), err, len(program.DefaultLabels()))
	}
	if b, err := os.ReadFile(filepath.Join(clone, beads.ConfigFile)); err != nil || string(b) != "issue-prefix: g\nlayout: files\n" || r.Tracker.Prefix != "g" {
		t.Fatalf("the first commit's %s = %q, %v; result prefix %q; want issue-prefix g and the files layout", beads.ConfigFile, b, err, r.Tracker.Prefix)
	}
}

func TestInitSetsAnExplicitPrefix(t *testing.T) {
	remote, dir := bare(t, false), repo(t)
	r, err := Init(InitOptions{Dir: dir, Tracker: remote, Group: "acme/program", Prefix: "aeop", User: "operator"})
	if err != nil || r.Tracker == nil || r.Tracker.Prefix != "aeop" {
		t.Fatalf("init --prefix aeop: %+v, %v", r.Tracker, err)
	}
	if out := humanInit(r); !strings.Contains(out, "issue prefix aeop") {
		t.Fatalf("init does not name the prefix:\n%s", out)
	}
}

func TestInitLeavesTrackerWithCommits(t *testing.T) {
	remote, dir := bare(t, true), repo(t)
	before := run(t, ".", "--git-dir", remote, "rev-parse", "main")
	r, err := Init(InitOptions{Dir: dir, Tracker: remote, Group: "g", User: "operator"})
	if err != nil || r.Tracker == nil || r.Tracker.Seeded {
		t.Fatalf("init --tracker on a tracker with commits: %+v, %v", r.Tracker, err)
	}
	if after := run(t, ".", "--git-dir", remote, "rev-parse", "main"); after != before {
		t.Fatalf("the tracker moved from %s to %s", before, after)
	}
}

// TestInitSeedsATrackerBranch holds srd043 R1.5 for a program of one
// repository: --tracker-branch makes the branch as an orphan holding only
// the config file in a repository that has commits, leaves main as it is,
// writes the branch to control/trackers.md, and leaves the branch alone on
// a second run.
func TestInitSeedsATrackerBranch(t *testing.T) {
	remote, dir := bare(t, true), repo(t)
	const branch = "scrum-master/tracker"
	main := run(t, ".", "--git-dir", remote, "rev-parse", "main")
	r, err := Init(InitOptions{Dir: dir, Tracker: remote, TrackerBranch: branch, Group: "g", User: "operator"})
	if err != nil || r.Tracker == nil || !r.Tracker.Seeded || r.Tracker.Branch != branch {
		t.Fatalf("init --tracker-branch: %+v, %v", r.Tracker, err)
	}
	if after := run(t, ".", "--git-dir", remote, "rev-parse", "main"); after != main {
		t.Fatalf("main moved from %s to %s", main, after)
	}
	files := strings.Fields(run(t, ".", "--git-dir", remote, "ls-tree", "-r", "--name-only", branch))
	if !slices.Contains(files, beads.ConfigFile) || slices.ContainsFunc(files, func(f string) bool { return !strings.HasPrefix(f, ".beads/") }) {
		t.Fatalf("the tracker branch holds %q; want %s and nothing outside .beads/", files, beads.ConfigFile)
	}
	if out, err := exec.Command("git", "--git-dir", remote, "merge-base", "main", branch).CombinedOutput(); err == nil {
		t.Fatalf("the tracker branch shares history with main at %s", out)
	}
	clone := filepath.Join(t.TempDir(), "check")
	run(t, filepath.Dir(clone), "clone", "-q", "--single-branch", "--branch", branch, remote, clone)
	tr := beads.NewTracker(clone, "operator", dir)
	tr.Store.Branch = branch
	if labels, err := tr.GroupLabels("g"); err != nil || len(labels) != len(program.DefaultLabels()) {
		t.Fatalf("tracker branch labels: %d, %v; want %d", len(labels), err, len(program.DefaultLabels()))
	}
	doc, err := os.ReadFile(filepath.Join(dir, routing.File))
	if err != nil {
		t.Fatal(err)
	}
	if c, err := routing.Parse(doc); err != nil || c.Default.TrackerRepository != remote || c.Default.TrackerBranch != branch {
		t.Fatalf("%s = %+v, %v; want the tracker repository on %s", routing.File, c.Default, err, branch)
	}
	head := run(t, ".", "--git-dir", remote, "rev-parse", branch)
	again, err := Init(InitOptions{Dir: dir, Tracker: remote, TrackerBranch: branch, Group: "g", User: "operator"})
	if err != nil || again.Tracker == nil || again.Tracker.Seeded {
		t.Fatalf("a second init --tracker-branch: %+v, %v", again.Tracker, err)
	}
	if after := run(t, ".", "--git-dir", remote, "rev-parse", branch); after != head {
		t.Fatalf("the tracker branch moved from %s to %s", head, after)
	}
}

func TestInitReportsNoTracker(t *testing.T) {
	t.Setenv("SCRUM_MASTER_TRACKER", "")
	r, err := Init(InitOptions{Dir: repo(t)})
	if err != nil || r.Tracker == nil || r.Tracker.Seeded || r.Tracker.Reason != NoTracker {
		t.Fatalf("init without a tracker: %+v, %v", r.Tracker, err)
	}
	if out := humanInit(r); !strings.Contains(out, "tracker: "+NoTracker) {
		t.Fatalf("init without a tracker does not say so:\n%s", out)
	}
	t.Setenv("SCRUM_MASTER_TRACKER", "https://example.com/tracker.git")
	if r, err := Init(InitOptions{Dir: repo(t)}); err != nil || r.Tracker != nil {
		t.Fatalf("init with SCRUM_MASTER_TRACKER set reported a tracker: %+v, %v", r.Tracker, err)
	}
}

func TestScaffoldRerunChangesNothing(t *testing.T) {
	dir := repo(t)
	if _, err := Init(InitOptions{Dir: dir}); err != nil {
		t.Fatal(err)
	}
	r, err := Init(InitOptions{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range r.Files {
		if f.Action != "kept" {
			t.Errorf("second run: %s %s, want kept", f.Path, f.Action)
		}
	}
}

func TestScaffoldKeepsEditedFile(t *testing.T) {
	dir := repo(t)
	if _, err := Init(InitOptions{Dir: dir}); err != nil {
		t.Fatal(err)
	}
	vision := filepath.Join(dir, "docs", "VISION.md")
	os.WriteFile(vision, []byte("the owner's vision\n"), 0o644)
	r, err := Init(InitOptions{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(vision)
	if actions(r.Files)["docs/VISION.md"] != "kept" || string(b) != "the owner's vision\n" {
		t.Fatalf("edited VISION: %s, content %q", actions(r.Files)["docs/VISION.md"], b)
	}
}

func TestScaffoldForceReplaces(t *testing.T) {
	dir := repo(t)
	if _, err := Init(InitOptions{Dir: dir}); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "docs", "VISION.md"), []byte("edited\n"), 0o644)
	r, err := Init(InitOptions{Dir: dir, Force: true})
	if err != nil || actions(r.Files)["docs/VISION.md"] != "overwritten" {
		t.Fatalf("--force: %v, %v", actions(r.Files)["docs/VISION.md"], err)
	}
}

func TestScaffoldRefusesNonRepository(t *testing.T) {
	dir := t.TempDir()
	if _, err := Init(InitOptions{Dir: dir}); err == nil || !strings.Contains(err.Error(), "not a git repository") {
		t.Fatalf("init outside a repository: %v", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("init wrote into a non-repository: %v", entries)
	}
}

func TestScaffoldDryRunWritesNothing(t *testing.T) {
	dir := repo(t)
	r, err := Init(InitOptions{Dir: dir, Agents: []string{"claude"}, DryRun: true})
	if err != nil || len(r.Files) != len(skeleton) {
		t.Fatalf("dry run: %d files reported, %v", len(r.Files), err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != ".git" {
			t.Fatalf("dry run wrote %s", e.Name())
		}
	}
}

func TestScaffoldWritesParentPin(t *testing.T) {
	dir := repo(t)
	if _, err := Init(InitOptions{Dir: dir}); err != nil {
		t.Fatal(err)
	}
	release, err := ParentRelease()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "docs", "external-pins.yaml"))
	if !strings.Contains(string(b), parentRepository+": "+release+"\n") {
		t.Fatalf("pins:\n%s\nwant %s at %s", b, parentRepository, release)
	}
}

// The design constitution is the process's own, copied as it is; the
// templates are what a program fills in, so they name nobody (design.md D6).
func TestTemplatesNameNoProject(t *testing.T) {
	forbidden := []string{"scrum-master", "nokia", "github", "gitlab", "anthropic", "homeostat"}
	check := func(name string, b []byte) {
		// The policy's corpus gate is the words' own command, the one name
		// a program runs them by (GH-32).
		lower := strings.ToLower(strings.ReplaceAll(string(b), ProgramAudit, ""))
		for _, w := range forbidden {
			if strings.Contains(lower, w) {
				t.Errorf("%s names %q", name, w)
			}
		}
	}
	fs.WalkDir(templates, "templates", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			b, _ := templates.ReadFile(p)
			check(p, b)
		}
		return err
	})
	for n, b := range controlFiles() {
		check(n, b)
	}
	charter, err := charterFile()
	if err != nil {
		t.Fatal(err)
	}
	check("docs/corpus-charter.yaml", charter)
}

// TestInitDeclaresTheSystemComponent declares the system repository a
// component of the program in the system SRD, from its origin, so issues
// filed there derive a component; without an origin the field is empty.
func TestInitDeclaresTheSystemComponent(t *testing.T) {
	dir := repo(t)
	run(t, dir, "remote", "add", "origin", "https://git.example.invalid/g/system.git")
	if _, err := Init(InitOptions{Dir: dir}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, systemSRDPath))
	if err != nil {
		t.Fatal(err)
	}
	f := fake.New()
	f.AddFile("g/system", "main", systemSRDPath, b)
	subs, err := program.LoadSubsystems(f, "g/system", "main")
	if err != nil {
		t.Fatal(err)
	}
	c, sub, ok := subs.ComponentFor("g/system")
	if !ok || c.Name != "system" || c.Path != "/" || sub.Owner != "The technical program manager" {
		t.Fatalf("system component = %+v %+v %v:\n%s", c, sub, ok, b)
	}
	l, err := program.Derive(f, "g/system", "g/system", "main", []string{"docs/VISION.md"}, program.Labelling{})
	if err != nil || l.Component != "system" {
		t.Fatalf("an issue in the system repository derives %+v, %v", l, err)
	}

	bare := repo(t)
	if _, err := Init(InitOptions{Dir: bare}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(bare, systemSRDPath)); !strings.Contains(string(b), "<!-- field: implementation_targets | form: \"empty list\" -->\n## Implementation targets\n\nNone\n") {
		t.Fatalf("without an origin:\n%s", b)
	}
}

// init records the origin as configured, not as url.insteadOf expands it,
// so the URL in the system SRD is the one every clone shares (D5, GH-93).
func TestImplementationTargetsKeepTheConfiguredOrigin(t *testing.T) {
	dir := repo(t)
	run(t, dir, "remote", "add", "origin", "git@github.com:acme/program.git")
	run(t, dir, "config", "url.https://github.com/.insteadOf", "git@github.com:")
	if got := implementationTargets(dir); !strings.Contains(got, "| git@github.com:acme/program.git |") || strings.Contains(got, "https://github.com/acme") {
		t.Fatalf("implementation targets:\n%s", got)
	}
}

// init records the program group in control/trackers.md beside the
// tracker, and says when it cannot (GH-103).
func TestInitRecordsTheProgramGroup(t *testing.T) {
	remote, dir := bare(t, false), repo(t)
	r, err := Init(InitOptions{Dir: dir, Tracker: remote, Group: "aeop", User: "operator"})
	if err != nil || r.Group != "aeop" || !strings.Contains(humanInit(r), "program group: aeop, recorded") {
		t.Fatalf("init --group: %+v, %v\n%s", r.Group, err, humanInit(r))
	}
	if g := routing.RecordedGroup(dir); g != "aeop" {
		t.Fatalf("control/trackers.md records %q", g)
	}
	t.Setenv("SCRUM_MASTER_TRACKER", "")
	r, err = Init(InitOptions{Dir: repo(t), Group: "aeop"})
	if err != nil || r.Group != "" || !strings.Contains(r.GroupReason, "no tracking system is known") {
		t.Fatalf("init --group with no tracker: %+v, %v", r, err)
	}
}
