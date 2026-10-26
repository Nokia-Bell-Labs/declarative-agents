// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package release

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Git runs a git command in the repository and returns its trimmed output.
type Git func(args ...string) (string, error)

// ExecGit is Git in dir.
func ExecGit(dir string) Git {
	return func(args ...string) (string, error) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		var stderr strings.Builder
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
		}
		return strings.TrimSpace(string(out)), nil
	}
}

// DatePrefix is the tag scheme the parent ecosystem shares: v0.YYYYMMDD.N.
const DatePrefix = "v0."

// NextDateTag is the tag for date one past the highest revision any of the
// listed tags (one per line, local and remote) holds for that date.
func NextDateTag(date time.Time, tags ...string) string {
	day := date.Format("20060102")
	re := regexp.MustCompile(`^` + regexp.QuoteMeta(DatePrefix+day) + `\.(\d+)$`)
	next := 0
	for _, list := range tags {
		for _, line := range strings.Split(list, "\n") {
			if m := re.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
				if n, err := strconv.Atoi(m[1]); err == nil && n >= next {
					next = n + 1
				}
			}
		}
	}
	return fmt.Sprintf("%s%s.%d", DatePrefix, day, next)
}

// RemoteTags lists origin's tag names matching pattern.
func RemoteTags(git Git, pattern string) (string, error) {
	out, err := git("ls-remote", "--tags", "--refs", "origin", pattern)
	if err != nil {
		return "", err
	}
	var names []string
	for _, line := range strings.Split(out, "\n") {
		if _, ref, ok := strings.Cut(line, "\t"); ok {
			names = append(names, strings.TrimPrefix(ref, "refs/tags/"))
		}
	}
	return strings.Join(names, "\n"), nil
}

// Release is one repository's release through its gates file (eng03).
type Release struct {
	Root     string
	Branch   string // the branch a release runs on; empty is main
	Git      Git
	Exec     Exec
	Out      io.Writer
	Now      func() time.Time
	Confirm  func(question string) (bool, error) // nil reads a y/N answer from stdin
	LockName string                              // under the git dir; empty is scrum-master-release.lock
	// Preflight checks what publishing the tag needs before the gates run;
	// nil checks nothing.
	Preflight func() error
	// Publish publishes a tag once it is on origin; nil publishes nothing.
	Publish func(tag string) error
}

// Tag runs the gates and, when every gate passed, creates the next
// v0.YYYYMMDD.N tag at HEAD and pushes it as the gates file says.
func (r Release) Tag() error { return r.run(false) }

// DryRun runs the same checks and gates and creates nothing.
func (r Release) DryRun() error { return r.run(true) }

func (r Release) run(dryRun bool) error {
	gates, err := LoadGates(filepath.Join(r.Root, DefaultGatesFile))
	if err != nil {
		return err
	}
	unlock, err := r.lock()
	if err != nil {
		return err
	}
	defer unlock()

	commit, err := r.preflight()
	if err != nil {
		return err
	}
	if r.Preflight != nil {
		if err := r.Preflight(); err != nil {
			return fmt.Errorf("preflight: %w", err)
		}
	}
	if tagged, err := r.Git("tag", "--points-at", commit, "--list", DatePrefix+"*"); err != nil {
		return err
	} else if tagged != "" {
		fmt.Fprintf(r.Out, "release: HEAD is already %s; nothing to release\n", strings.Fields(tagged)[0])
		return nil
	}
	results, err := RunStages(r.Root, gates, r.Exec, r.Out)
	if err != nil {
		return err
	}
	if after, err := r.preflight(); err != nil {
		return fmt.Errorf("after the gates: %w", err)
	} else if after != commit {
		return fmt.Errorf("HEAD moved from %s to %s while the gates ran", commit, after)
	}
	if dryRun {
		fmt.Fprintf(r.Out, "release: dry run passed at %s; no tag created\n", commit)
		return nil
	}

	now := time.Now
	if r.Now != nil {
		now = r.Now
	}
	pattern := DatePrefix + now().Format("20060102") + ".*"
	local, err := r.Git("tag", "--list", pattern)
	if err != nil {
		return err
	}
	remote, err := RemoteTags(r.Git, pattern)
	if err != nil {
		return err
	}
	tag := NextDateTag(now(), local, remote)
	message, err := r.message(tag, commit, results)
	if err != nil {
		return err
	}
	if _, err := r.Git("tag", "-a", tag, commit, "-m", message); err != nil {
		return err
	}
	fmt.Fprintf(r.Out, "release: created %s\n\n%s\n", tag, message)
	pushed, err := r.push(gates.Push, tag)
	if err != nil || r.Publish == nil {
		return err
	}
	if !pushed {
		fmt.Fprintf(r.Out, "release: after pushing %s, run mage publish\n", tag)
		return nil
	}
	return r.Publish(tag)
}

// preflight returns HEAD when the release branch is checked out and the
// worktree is clean.
func (r Release) preflight() (string, error) {
	want := r.Branch
	if want == "" {
		want = "main"
	}
	branch, err := r.Git("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	if branch != want {
		return "", fmt.Errorf("a release runs on %s, not %s", want, branch)
	}
	if status, err := r.Git("status", "--porcelain"); err != nil {
		return "", err
	} else if status != "" {
		return "", errors.New("a release needs a clean worktree")
	}
	return r.Git("rev-parse", "HEAD")
}

// message is the annotation: the gates that passed and the changes since
// the previous release.
func (r Release) message(tag, commit string, results []GateResult) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "Release %s\n\nGates passed:\n", tag)
	for _, res := range results {
		fmt.Fprintf(&b, "  %s / %s (%s)\n", res.Stage, res.Gate, res.Elapsed)
	}
	logRange := commit
	if prev, err := r.Git("describe", "--tags", "--abbrev=0", "--match", DatePrefix+"*", commit); err == nil && prev != "" {
		logRange = prev + ".." + commit
		fmt.Fprintf(&b, "\nChanges since %s:\n", prev)
	} else {
		b.WriteString("\nChanges:\n")
	}
	log, err := r.Git("log", "--oneline", "--no-merges", logRange)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(log, "\n") {
		if line != "" {
			fmt.Fprintf(&b, "  %s\n", line)
		}
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// push pushes tag to origin as mode says and reports whether it did.
func (r Release) push(mode, tag string) (bool, error) {
	push := mode == PushAuto
	if !push {
		confirm := r.Confirm
		if confirm == nil {
			confirm = stdinConfirm
		}
		var err error
		if push, err = confirm(fmt.Sprintf("push %s to origin? [y/N] ", tag)); err != nil {
			return false, err
		}
	}
	if !push {
		fmt.Fprintf(r.Out, "release: %s stays local; push it with git push origin %s\n", tag, tag)
		return false, nil
	}
	if _, err := r.Git("push", "origin", "refs/tags/"+tag); err != nil {
		return false, err
	}
	fmt.Fprintf(r.Out, "release: pushed %s\n", tag)
	return true, nil
}

func stdinConfirm(question string) (bool, error) {
	fmt.Print(question)
	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && answer == "" {
		return false, nil
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes", nil
}

// lock holds a directory under the git dir for the length of a release, so
// two releases of one clone cannot overlap.
func (r Release) lock() (func(), error) {
	name := r.LockName
	if name == "" {
		name = "scrum-master-release.lock"
	}
	path, err := r.Git("rev-parse", "--git-path", name)
	if err != nil {
		return nil, err
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(r.Root, path)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		if os.IsExist(err) {
			owner, _ := os.ReadFile(filepath.Join(path, "owner"))
			return nil, fmt.Errorf("another release holds %s (%s); remove it only when no release is running",
				path, strings.TrimSpace(string(owner)))
		}
		return nil, err
	}
	owner := fmt.Sprintf("pid=%d started=%s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339))
	if err := os.WriteFile(filepath.Join(path, "owner"), []byte(owner), 0o600); err != nil {
		os.RemoveAll(path)
		return nil, err
	}
	return func() { os.RemoveAll(path) }, nil
}
