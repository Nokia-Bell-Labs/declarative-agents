// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package scaffold

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/backlog"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/beads"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/hierarchy"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/install"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/legacyenv"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/routing"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/runtime"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/spectool"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/version"
)

// DoctorOptions are what scrum-master doctor is given.
type DoctorOptions struct {
	Dir     string
	Version string // the running scrum-master's version, which installed skills are checked against
	DryRun  bool
}

// DoctorResult is what scrum-master doctor reports: the repository's level, what
// happened to each file scrum-master scaffolds there, the installed skills that
// differ from this scrum-master, the repair of the documents' format, and the
// release whose profiles the repository vendored (GH-34).
type DoctorResult struct {
	Dir    string            `json:"dir"`
	Level  string            `json:"level"`
	Files  []Result          `json:"files"`
	Skills []install.Finding `json:"skills,omitempty"`
	Repair RepairResult      `json:"repair"`
	// Vendored is the release .scrum-master holds, from its manifest; empty
	// when the repository vendored none.
	Vendored string `json:"vendored,omitempty"`
	// Pinned is the parent release docs/external-pins.yaml pins; PinDrift
	// says it differs from the vendored release, whose format the
	// documents should follow (GH-91).
	Pinned   string `json:"pinned,omitempty"`
	PinDrift string `json:"pin_drift,omitempty"`
	// Credentials is, per shipped profile, every environment variable its
	// declarations read and whether each is set here, so a run's model and
	// tracker credentials are checked before a run (srd043 R4.7, GH-135).
	Credentials []ProfileEnv `json:"credentials,omitempty"`
	// LegacyEnv is the TPM_ variables set here, each read under its
	// SCRUM_MASTER_ name until the setup renames it (GH-38).
	LegacyEnv []string `json:"legacy_env,omitempty"`
	// OrphanedClones is each cached tracker clone that follows no origin,
	// for the person to remove once nothing in it is needed (GH-116); it
	// is drift, so the check profile fails on it (GH-141).
	OrphanedClones []string `json:"orphaned_tracker_clones,omitempty"`
	// Trackers is the program's tracking systems, per control/trackers.md,
	// and each problem reaching them (srd042).
	Trackers *routing.Report `json:"trackers,omitempty"`
	// Bootstrap is the bootstrapping order's check in a subsystem
	// repository that holds hierarchy documents (design.md D15, srd045
	// R6.4); nil where it does not apply.
	Bootstrap *Bootstrap `json:"bootstrap,omitempty"`
	DryRun    bool       `json:"dry_run"`
}

// Bootstrap is the bootstrapping order's check, which the corpus gate
// runs too (GH-90).
type Bootstrap = backlog.Bootstrap

// Drift is what a run of doctor would change, by what it would do: each
// file it would create or rewrite, each document it would repair, and each
// installed skill that differs from this release, and each orphaned
// tracker clone for the person to remove (GH-141). A repository with none
// is up to date (GH-64).
func (r DoctorResult) Drift() []string {
	var out []string
	for _, f := range r.Files {
		switch f.Action {
		case "created", "appended", "refreshed", "extended", "repinned":
			out = append(out, f.Action+" "+f.Path)
		}
	}
	for _, p := range r.Repair.Repaired {
		out = append(out, "repaired "+p)
	}
	for _, s := range r.Skills {
		out = append(out, "skill "+s.Problem+" "+s.Path)
	}
	if r.PinDrift != "" {
		out = append(out, r.PinDrift)
	}
	for _, c := range r.OrphanedClones {
		out = append(out, "orphaned tracker clone "+c)
	}
	return out
}

// bootstrapOrder is the gate's bootstrapping-order check.
func bootstrapOrder(dir string, documents int) *Bootstrap {
	return backlog.BootstrapOrder(dir, documents)
}

// RepairResult is what the repair did to the corpus documents: those
// rewritten in canonical form (or, on a dry run, those that would be), the
// findings the formatter cannot repair with their fix hints, and why the
// repair was skipped, if it was (srd043 R4.4, R4.5).
type RepairResult struct {
	Repaired []string `json:"repaired,omitempty"`
	Findings []string `json:"findings,omitempty"`
	Skipped  string   `json:"skipped,omitempty"`
	// Untracked are documents under docs/ git does not track: the spec
	// tool reads only tracked ones, so they were not checked.
	Untracked []string `json:"untracked,omitempty"`
}

// ProfileEnv is one shipped profile's environment: the variables its
// declarations read, each with whether it is set here.
type ProfileEnv struct {
	Profile string   `json:"profile"`
	Vars    []EnvVar `json:"vars"`
}

// EnvVar is one environment variable a profile names.
type EnvVar struct {
	Name string `json:"name"`
	Set  bool   `json:"set"`
}

// profileCredentials reads every shipped profile's environment variables and
// checks each against this process's environment.
func profileCredentials() ([]ProfileEnv, error) {
	env, err := install.ProfileEnv(scrummaster.Plugin, scrummaster.AgentsDir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(env))
	for name := range env {
		names = append(names, name)
	}
	slices.Sort(names)
	var out []ProfileEnv
	for _, name := range names {
		// A profile that reads no variable lists none: [], not null (GH-2792).
		p := ProfileEnv{Profile: name, Vars: []EnvVar{}}
		for _, v := range env[name] {
			_, set := os.LookupEnv(v) //nolint:forbidigo // Reads the documented SCRUM_MASTER_* environment interface.
			p.Vars = append(p.Vars, EnvVar{Name: v, Set: set})
		}
		out = append(out, p)
	}
	return out, nil
}

// specTool runs the spec tool at a repository's pin, lookGo finds the Go
// toolchain the tool is built with, and built reports a tool already built
// at a release; tests replace them.
var (
	specTool = spectool.Run
	lookGo   = func() error { _, err := exec.LookPath("go"); return err }
	built    = runtime.Built
)

// notCanonical ends the line the tool's check prints for a document fmt
// would rewrite.
const notCanonical = ": not-canonical: run mage fmt"

// repair rewrites the corpus documents at dir in the canonical form of the
// parent release the repository pins, with that release's own formatter,
// then reports what the formatter cannot repair. A dry run only checks.
func repair(dir string, dryRun bool) RepairResult {
	var r RepairResult
	if out, err := exec.Command("git", "-C", dir, "ls-files", "--others", "--exclude-standard", "--", "docs").Output(); err == nil {
		for _, p := range strings.Fields(string(out)) {
			if strings.HasSuffix(p, ".md") {
				r.Untracked = append(r.Untracked, p)
			}
		}
	}
	// In the agent the check runs in process and needs no Go (GH-33).
	if spectool.InProcess == nil && lookGo() != nil {
		tag, err := spectool.Pinned(dir)
		if err != nil || !built(tag) {
			r.Skipped = "no Go toolchain on PATH and no spec tool built at the pinned release; run the check with the agent, which carries it, or install Go"
			return r
		}
	}
	skip := func(err error) {
		r.Skipped = err.Error()
		if strings.Contains(r.Skipped, "no corpus document matches") {
			r.Skipped = "git tracks no corpus document here; commit the documents, then run scrum-master doctor again"
		}
	}
	if !dryRun {
		lines, err := specTool(dir, "fmt", "all")
		if err != nil && len(lines) == 0 {
			skip(err)
			return r
		}
		for _, l := range lines {
			if p, ok := strings.CutPrefix(l, "formatted "); ok {
				r.Repaired = append(r.Repaired, p)
			}
		}
	}
	lines, err := specTool(dir, "check", "all")
	if err != nil && len(lines) == 0 {
		skip(err)
		return r
	}
	for _, l := range lines {
		if p, ok := strings.CutSuffix(l, notCanonical); ok && dryRun {
			r.Repaired = append(r.Repaired, p)
			continue
		}
		r.Findings = append(r.Findings, strings.TrimSuffix(l, ": run mage fmt"))
	}
	return r
}

// systemSRDPath and subsystemSRDPath are the SRDs init and onboard write,
// by which doctor knows a repository's level.
const (
	systemSRDPath    = "docs/specs/software-requirements/srd001-system.md"
	subsystemSRDPath = "docs/specs/software-requirements/srd001-subsystem.md"
)

// Level reads a program repository's level from the files scrum-master wrote there:
// system when the policy or the system SRD is present, subsystem when the
// subsystem SRD is (srd043 R4.1).
func Level(dir string) (string, error) {
	exists := func(p string) bool {
		_, err := os.Stat(filepath.Join(dir, filepath.FromSlash(p)))
		return err == nil
	}
	switch {
	case exists(program.PolicyFile) || exists(systemSRDPath):
		return "system", nil
	case exists(subsystemSRDPath):
		return "subsystem", nil
	}
	return "", fmt.Errorf("%s holds neither %s, %s, nor %s, so it is no program repository scrum-master laid down; run scrum-master init or scrum-master onboard there",
		dir, program.PolicyFile, systemSRDPath, subsystemSRDPath)
}

// extendLabels adds to a kept control/labels.md the axes and values scrum-master's
// defaults list that it lacks, and reports the file extended with them; a
// file that does not parse is kept and its reason reported (srd043 R4.2).
// The tracker's labels come from scrum-master issue labels, which the reason names.
func extendLabels(dir string, files []Result, dryRun bool) error {
	i := slices.IndexFunc(files, func(f Result) bool { return f.Path == program.LabelsFile })
	if i < 0 || files[i].Action != "kept" {
		return nil
	}
	path := filepath.Join(dir, filepath.FromSlash(program.LabelsFile))
	doc, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	out, added, err := ExtendLabels(doc, program.DefaultLabels())
	if err != nil {
		files[i].Reason = "does not parse, so doctor cannot add the labels scrum-master's defaults list: " + err.Error()
		return nil
	}
	if len(added) == 0 {
		return nil
	}
	files[i].Action, files[i].Labels = "extended", added
	files[i].Reason = "adds " + strings.Join(added, ", ") + "; run scrum-master issue labels --control <system repository> to create them in the tracker"
	if dryRun {
		return nil
	}
	return os.WriteFile(path, out, 0o644)
}

// skillProblems are the install findings doctor reports (srd043 R4.3).
var skillProblems = map[string]bool{"missing": true, "outdated": true, "edited": true, "stale": true}

// Doctor brings a repository an older release laid down up to date: it writes
// the files this scrum-master scaffolds at the repository's level that are missing,
// refreshes the orientation files' managed blocks, keeps every other file,
// repairs the documents to the pinned format, and reports the installed
// skills scrum-master install would change and the runtime at the release scrum-master pins
// (srd043 R4).
// It files nothing and touches no tracker.
func Doctor(o DoctorOptions) (DoctorResult, error) {
	r := DoctorResult{Dir: o.Dir, DryRun: o.DryRun}
	if err := requireRepository(o.Dir); err != nil {
		return r, err
	}
	level, err := Level(o.Dir)
	if err != nil {
		return r, err
	}
	r.Level = level
	if r.Trackers, err = routing.Check(o.Dir); err != nil {
		return r, err
	}
	if level == "subsystem" {
		docs, err := hierarchy.Load(o.Dir)
		if err != nil {
			return r, err
		}
		if len(docs) > 0 {
			r.Bootstrap = bootstrapOrder(o.Dir, len(docs))
		}
	}
	files, err := corpusFiles(level, o.Dir)
	if err != nil {
		return r, err
	}
	if level == "system" {
		for n, b := range controlFiles() {
			files[n] = b
		}
	}
	if r.Files, err = write(o.Dir, files, false, o.DryRun); err != nil {
		return r, err
	}
	for i, f := range r.Files {
		if f.Action == "kept" && f.Reason == keptEdited {
			r.Files[i].Reason = "edited; doctor keeps it"
		}
	}
	if level == "system" {
		if err := extendLabels(o.Dir, r.Files, o.DryRun); err != nil {
			return r, err
		}
		if err := upgradeSystem(o.Dir, r.Files, o.DryRun); err != nil {
			return r, err
		}
		moved, err := moveRoster(o.Dir, o.DryRun)
		if err != nil {
			return r, err
		}
		if moved.Path != "" {
			r.Files = append(r.Files, moved)
		}
	}
	r.Repair = repair(o.Dir, o.DryRun)
	builtAgainst := ""
	if b, err := os.ReadFile(filepath.Join(o.Dir, install.ManifestFile)); err == nil {
		var m install.Manifest
		if json.Unmarshal(b, &m) == nil {
			r.Vendored, builtAgainst = m.Release, m.Runtime
		}
	}
	r.Pinned, _ = spectool.Pinned(o.Dir)
	// A corpus pins the runtime its scrum-master was built against, which
	// is the vendored release only when both ship from one repository
	// (GH-2755, GH-2771).
	accepted := []string{r.Vendored, builtAgainst, runtime.DefaultPin()}
	if r.Vendored != "" && r.Pinned != "" && !slices.Contains(accepted, r.Pinned) && version.IsRelease(r.Vendored) && version.IsRelease(r.Pinned) {
		r.PinDrift = fmt.Sprintf("pin %s in %s, but %s is vendored; run the upgrade profile to move the pin and repair the documents to its format", r.Pinned, spectool.PinsFile, r.Vendored)
	}
	if r.Credentials, err = profileCredentials(); err != nil {
		return r, err
	}
	r.LegacyEnv = legacyenv.Set()
	r.OrphanedClones = beads.OrphanedClones()
	if agents := install.InstalledAgents(o.Dir); len(agents) > 0 {
		skills, err := install.EmbeddedSkills()
		if err != nil {
			return r, err
		}
		findings, err := install.Check(o.Dir, agents, skills, o.Version)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return r, err
		}
		for _, f := range findings {
			if skillProblems[f.Problem] {
				r.Skills = append(r.Skills, f)
			}
		}
	}
	return r, nil
}
