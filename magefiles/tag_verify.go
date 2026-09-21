// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/magefile/mage/mg"
)

// annotationSubstanceBar is the minimum content, in characters of subject and
// body after trimming, below which an annotation is refused (srd007 R2.2).
// It is the one place the bar is stated; refusals quote it.
const annotationSubstanceBar = 30

// TAG groups the release-tag verbs. Uppercase like CLEAN and STATS: the
// lowercase name would collide with the Tag target, and mage lowercases the
// namespace for the command line either way.
type TAG mg.Namespace

// Verify refuses a release tag whose annotation is missing, degenerate, or
// under the substance bar, or that is not reachable from main. It reports one
// named reason per failure and mutates nothing (srd007-release-annotation,
// ENG01 "Release annotation"). The release runbook and the release skill run
// it after annotating and before any push.
func (TAG) Verify(tag string) error {
	if err := verifyTagAnnotation(".", tag); err != nil {
		return err
	}
	fmt.Printf("tag:verify %s: annotated, substantive, and on main\n", tag)
	return nil
}

// verifyTagAnnotation applies the four rules against one repository, so the
// tests drive scratch repositories rather than this one's refs.
func verifyTagAnnotation(dir, tag string) error {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return errors.New("tag:verify needs a tag name")
	}
	objectType, err := gitIn(dir, "for-each-ref", "refs/tags/"+tag, "--format=%(objecttype)")
	if err != nil {
		return fmt.Errorf("tag:verify %s: %w", tag, err)
	}
	// R1.1: the tag exists and is an annotated tag object, not a bare ref to
	// a commit. mage tag creates the lightweight ref; annotation is the step
	// this check exists to see.
	switch strings.TrimSpace(objectType) {
	case "":
		return fmt.Errorf("tag:verify %s: refused, the tag does not exist (R1.1)", tag)
	case "tag":
	default:
		return fmt.Errorf(
			"tag:verify %s: refused, the tag is lightweight and carries no annotation (R1.1)", tag)
	}

	subject, err := gitIn(dir, "tag", "-l", "--format=%(contents:subject)", tag)
	if err != nil {
		return fmt.Errorf("tag:verify %s: read subject: %w", tag, err)
	}
	body, err := gitIn(dir, "tag", "-l", "--format=%(contents:body)", tag)
	if err != nil {
		return fmt.Errorf("tag:verify %s: read body: %w", tag, err)
	}
	subject = strings.TrimSpace(subject)
	body = strings.TrimSpace(body)

	// R2.1: an annotation that names only the tag says nothing.
	if subject == "" {
		return fmt.Errorf("tag:verify %s: refused, the annotation subject is empty (R2.1)", tag)
	}
	if subject == tag {
		return fmt.Errorf(
			"tag:verify %s: refused, the annotation subject is the tag name itself (R2.1)", tag)
	}
	// R2.2: below the substance bar the annotation is a gesture, not a record.
	if content := len(subject) + len(body); content < annotationSubstanceBar {
		return fmt.Errorf(
			"tag:verify %s: refused, the annotation carries %d characters against a bar of %d (R2.2)",
			tag, content, annotationSubstanceBar)
	}

	// R3.1: the release line is main. An absent main is its own failure — a
	// pass by default would wave through a tag nobody can place.
	if _, err := gitIn(dir, "rev-parse", "--verify", "refs/heads/"+baseBranch); err != nil {
		return fmt.Errorf("tag:verify %s: refused, no local %s to check reachability against (R3.1)",
			tag, baseBranch)
	}
	if _, err := gitIn(dir, "merge-base", "--is-ancestor", tag+"^{commit}", baseBranch); err != nil {
		return fmt.Errorf("tag:verify %s: refused, the tag is not reachable from %s (R3.1)",
			tag, baseBranch)
	}
	return nil
}

// gitIn runs one git command in a directory and returns its output. A failing
// command surfaces stderr, which for verification is the diagnosis.
func gitIn(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(out))
		if trimmed != "" {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), trimmed)
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}
