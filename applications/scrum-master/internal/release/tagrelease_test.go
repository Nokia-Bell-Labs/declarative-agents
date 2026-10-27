// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package release

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/trackertest"
)

func TestTagRequiresPassingGateAndIncreasingVersion(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, tp trackertest.Program) {
		testTagRequiresPassingGateAndIncreasingVersion(t, tp.Tracker())
	})
}

func testTagRequiresPassingGateAndIncreasingVersion(t *testing.T, f tracker.Tracker) {
	dir, git := repo(t, gatesMarkdown)
	issue, _ := f.CreateIssue("g/po-1", tracker.NewIssue{Title: "Release a"})
	p := policyWith("a", program.Check{Kind: "custom", Environment: "local", Command: "check"})
	failing := false
	gate := func(version string, at time.Time) {
		t.Helper()
		v, _ := ParseVersion(version)
		if _, err := RunGate(f, ReleaseGateInput{Project: "g/po-1", Issue: issue.IID, Artifact: "a", Version: v, Dir: dir,
			Policy: p, Git: git, Out: io.Discard, Now: at,
			Exec: func(string, string, io.Writer) error {
				if failing {
					return io.ErrUnexpectedEOF
				}
				return nil
			}}); err != nil {
			t.Fatal(err)
		}
	}
	tag := func(version string) (TagResult, error) {
		v, _ := ParseVersion(version)
		return ReleaseTag(f, TagInput{Project: "g/po-1", Issue: issue.IID, Artifact: "a", Version: v, Policy: p, Git: git})
	}
	tags := func() string { out, _ := git("tag", "--list"); return out }

	if _, err := tag("0.3.0"); err == nil || !strings.Contains(err.Error(), "no gate has run") {
		t.Fatalf("without a gate: %v", err)
	}
	failing = true
	gate("0.3.0", t0)
	if _, err := tag("0.3.0"); err == nil || !strings.Contains(err.Error(), "did not pass") {
		t.Fatalf("after a failing gate: %v", err)
	}
	if tags() != "" {
		t.Fatalf("tagged without a passing gate: %q", tags())
	}

	failing = false
	gate("0.3.0", t0.Add(time.Hour))
	r, err := tag("0.3.0")
	if err != nil {
		t.Fatal(err)
	}
	if r.Tag != "a/v0.3.0" || !r.Pushed || r.Class != "breaking" {
		t.Fatalf("result %+v", r)
	}
	msg, _ := git("tag", "-l", "--format=%(objecttype) %(contents)", "a/v0.3.0")
	if !strings.HasPrefix(msg, "tag Release a/v0.3.0, breaking") {
		t.Fatalf("tag message %q", msg)
	}
	if remote, _ := RemoteTags(git, "a/*"); !strings.Contains(remote, "a/v0.3.0") {
		t.Fatalf("not pushed: %q", remote)
	}
	notes, _ := f.ListNotes("g/po-1", issue.IID)
	if last := notes[len(notes)-1].Body; !strings.Contains(last, "Tagged a/v0.3.0") || !strings.Contains(last, "breaking") {
		t.Fatalf("release issue note %q", last)
	}

	if _, err := tag("0.2.9"); err == nil || !strings.Contains(err.Error(), "not greater") {
		t.Fatalf("a lower version: %v", err)
	}
	if _, err := git("-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "--allow-empty", "-m", "later"); err != nil {
		t.Fatal(err)
	}
	gate("0.3.1", t0.Add(2*time.Hour))
	if _, err := git("-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "--allow-empty", "-m", "after the gate"); err != nil {
		t.Fatal(err)
	}
	if _, err := tag("0.3.1"); err == nil || !strings.Contains(err.Error(), "not at HEAD") {
		t.Fatalf("HEAD moved after the gate: %v", err)
	}
	if strings.Contains(tags(), "a/v0.3.1") {
		t.Fatal("tagged a revision the gate did not check")
	}
}

// TestTagReadsAPeerReadMadeAfterTheGate tags once someone else approves,
// though the gate ran before the approval, and closes the release issue the
// tag delivers (srd015 R1.2).
func TestTagReadsAPeerReadMadeAfterTheGate(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, tp trackertest.Program) {
		f := tp.Tracker()
		dir, git := repo(t, gatesMarkdown)
		issue, _ := f.CreateIssue("g/po-1", tracker.NewIssue{Title: "Release the corpus"})
		p := policyWith("corpus", program.Check{Kind: "peer-read"})
		v, _ := ParseVersion("0.2.0")
		r, err := RunGate(f, ReleaseGateInput{Project: "g/po-1", Issue: issue.IID, Artifact: "corpus", Version: v, Dir: dir,
			Policy: p, Git: git, Out: io.Discard, Now: t0})
		if err != nil || r.Checks[0].Result != ResultPending {
			t.Fatalf("gate = %+v, %v", r, err)
		}
		tag := func() (TagResult, error) {
			return ReleaseTag(f, TagInput{Project: "g/po-1", Issue: issue.IID, Artifact: "corpus", Version: v, Policy: p, Git: git})
		}
		if _, err := tag(); err == nil || !strings.Contains(err.Error(), "peer-read pending") {
			t.Fatalf("tag before any approval = %v", err)
		}
		if _, err := Approve(tp.As("reviewer"), "g/po-1", issue.IID); err != nil {
			t.Fatal(err)
		}
		res, err := tag()
		if err != nil || !res.Pushed || !res.Closed {
			t.Fatalf("tag after the approval = %+v, %v", res, err)
		}
		if got, _ := f.GetIssue("g/po-1", issue.IID); got.State != "closed" {
			t.Fatalf("release issue %s after its tag", got.State)
		}
	})
}

func TestNextRecommendsFromTheCommits(t *testing.T) {
	first := NextResult{Candidates: []Candidate{{Bump: "patch", Version: "0.0.1"}, {Bump: "minor", Version: "0.1.0"}}}
	first.Recommend(nil, false)
	if first.Recommended != "minor" || first.RecommendedVersion != "0.1.0" {
		t.Errorf("first release: %+v, want minor at 0.1.0 (GH-37)", first)
	}
	for want, messages := range map[string][]string{
		"":      nil,
		"patch": {"Fix the scorer's rounding", "fix: a typo"},
		"minor": {"Fix the scorer", "Add the trajectory store"},
		"major": {"Add v2 scoring\n\nBREAKING CHANGE: scores are now 0-100"},
	} {
		n := NextResult{Latest: "0.1.0", LatestTag: "corpus/v0.1.0"}
		n.Recommend(messages, false)
		if n.Recommended != want || n.Reason == "" {
			t.Errorf("%v: %+v, want %q", messages, n, want)
		}
	}
	// A corpus whose requirements no commit changed republishes: a patch,
	// unless a commit says it breaks (#151).
	for want, messages := range map[string][]string{
		"patch": {"Reword the overview", "Add a use case's diagram"},
		"major": {"Drop R2\n\nBREAKING CHANGE: R2 is gone"},
	} {
		n := NextResult{Latest: "0.7.0", LatestTag: "corpus/v0.7.0"}
		n.Recommend(messages, true)
		if n.Recommended != want || (want == "patch" && !strings.Contains(n.Reason, "change no requirement")) {
			t.Errorf("unchanged requirements %v: %+v, want %q", messages, n, want)
		}
	}
}

// TestRequirementsUnchangedReadsTheSRDs compares the requirements with the
// latest tag, and never calls a checkout without SRDs a republish (#151).
// A plan's road map and the initiatives are requirements too, so plan
// write's merge is not a republish (srd015 R3.1).
func TestRequirementsUnchangedReadsTheSRDs(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	write := func(p, s string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, p), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q", "-b", "main")
	write("README.md", "code\n")
	git("add", "-A")
	git("commit", "-q", "-m", "code")
	git("tag", "grader/v0.1.0")
	write("README.md", "more code\n")
	git("commit", "-q", "-am", "more")
	if same, err := RequirementsUnchanged(dir, "grader/v0.1.0"); err != nil || same {
		t.Fatalf("a checkout with no SRDs = %v, %v; want never a republish", same, err)
	}
	write(SRDDir+"/srd001-x.md", "R1\n")
	git("add", "-A")
	git("commit", "-q", "-m", "srd")
	git("tag", "corpus/v0.7.0")
	write("docs/VISION.md", "words\n")
	git("add", "-A")
	git("commit", "-q", "-m", "vision")
	if same, err := RequirementsUnchanged(dir, "corpus/v0.7.0"); err != nil || !same {
		t.Fatalf("no SRD change since the tag = %v, %v; want a republish", same, err)
	}
	write(SRDDir+"/srd001-x.md", "R1 changed\n")
	git("commit", "-q", "-am", "change R1")
	if same, err := RequirementsUnchanged(dir, "corpus/v0.7.0"); err != nil || same {
		t.Fatalf("an SRD change = %v, %v; want no republish", same, err)
	}
	for i, p := range []string{"docs/road-map.md", "docs/specs/initiatives/in001-scoring.md", "docs/specs/objectives/obj001-agents.md"} {
		tag := fmt.Sprintf("corpus/v0.8.%d", i)
		git("tag", tag)
		write(p, "planned\n")
		git("add", "-A")
		git("commit", "-q", "-m", "plan "+p)
		if same, err := RequirementsUnchanged(dir, tag); err != nil || same {
			t.Fatalf("a change to %s = %v, %v; want no republish", p, same, err)
		}
	}
}

// TestRecommendationSkipsStatusCommits counts only the commits that change
// the artifact: weekly status written in place, the reports and rollups
// under status/ and the roster in control/reports.md, is no release's
// content, merged straight or through a branch (GH-2774).
func TestRecommendationSkipsStatusCommits(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	write := func(p, s string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, p), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q", "-b", "main")
	write(SRDDir+"/srd001-x.md", "R1\n")
	write("control/reports.md", "# Reports\n")
	git("add", "-A")
	git("commit", "-q", "-m", "corpus")
	git("tag", "corpus/v0.56.0")
	write("control/reports.md", "# Reports\n\n- pm\n")
	git("commit", "-q", "-am", "Name the program's reporters")
	git("switch", "-q", "-c", "1-weekly-status")
	write("status/reports/2026-Q4/2026-W44-pm.md", "report\n")
	git("add", "-A")
	git("commit", "-q", "-m", "Weekly status 2026-W44")
	git("switch", "-q", "main")
	git("merge", "-q", "--no-ff", "-m", "Merge 1-weekly-status", "1-weekly-status")
	tags := []string{"corpus/v0.56.0"}
	r := program.Defaults().Releases
	n, err := Recommendation(r, "corpus", dir, tags)
	if err != nil {
		t.Fatal(err)
	}
	if n.Recommended != "" || n.RecommendedVersion != "" || !strings.Contains(n.Reason, "nothing to release") || !strings.Contains(n.Reason, "3 status") {
		t.Fatalf("status commits only = %+v; want nothing to release, saying 3 status commits were not counted", n)
	}
	write(SRDDir+"/srd001-x.md", "R1 changed\n")
	git("commit", "-q", "-am", "Reword R1")
	n, err = Recommendation(r, "corpus", dir, tags)
	if err != nil {
		t.Fatal(err)
	}
	if n.Recommended != "minor" || !strings.HasPrefix(n.Reason, "1 commits since corpus/v0.56.0") {
		t.Fatalf("one requirement change after status commits = %+v; want minor counting that one commit", n)
	}
}
