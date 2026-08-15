// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package spec

import "strings"

// topLevelBranches splits a -run pattern on alternation that is not inside a
// group, so "TestA|TestB" yields two proofs while "Test(A|B)" stays one. Group
// alternation is expanded separately by explicitNameBranches; here it is left
// whole. An unbalanced pattern is returned as a single branch and fails later
// at compile, where the error names the real problem.
func topLevelBranches(pattern string) []string {
	var branches []string
	var current strings.Builder
	depth := 0
	for i := 0; i < len(pattern); i++ {
		switch ch := pattern[i]; ch {
		case '\\':
			current.WriteByte(ch)
			if i+1 < len(pattern) {
				i++
				current.WriteByte(pattern[i])
			}
			continue
		case '(', '[':
			depth++
		case ')', ']':
			depth--
		case '|':
			if depth == 0 {
				branches = append(branches, current.String())
				current.Reset()
				continue
			}
		}
		current.WriteByte(pattern[i])
	}
	branches = append(branches, current.String())
	if depth != 0 {
		return []string{pattern}
	}
	for _, b := range branches {
		if strings.TrimSpace(b) == "" {
			return []string{pattern} // an empty branch matches everything; do not split
		}
	}
	return branches
}

// explicitNameBranches expands a -run pattern into the separate named proofs it
// asserts. It first splits top-level alternation (TestA|TestB), then, for each
// branch, distributes a single parenthesized alternation whose alternatives are
// all plain test-name fragments — so Test(A|B|C)$ becomes TestA$, TestB$, TestC$.
// A group is left whole when any alternative carries regex metacharacters (it is
// then a shared-prefix shorthand rather than a list of explicit names, and
// expanding it would mean reimplementing regex expansion). Distribution is
// semantically exact for explicit names: the group matches iff one alternative
// matches, and requiring every distributed branch to match some test is exactly
// "every named test exists".
func explicitNameBranches(pattern string) []string {
	var out []string
	for _, branch := range topLevelBranches(pattern) {
		out = append(out, distributeExplicitGroup(branch)...)
	}
	return out
}

// distributeExplicitGroup expands the first top-level parenthesized
// explicit-name alternation in branch, recursing so nested or successive groups
// are also expanded. A branch with no such group is returned unchanged.
func distributeExplicitGroup(branch string) []string {
	open, close := firstTopLevelGroup(branch)
	if open < 0 || close < 0 {
		return []string{branch}
	}
	prefix, body, suffix := branch[:open], branch[open+1:close], branch[close+1:]
	alts := topLevelBranches(body)
	// topLevelBranches returns the whole body as one element when it cannot
	// split (e.g. unbalanced): that is not an explicit-name list.
	if len(alts) < 2 {
		return []string{branch}
	}
	for _, alt := range alts {
		if !isPlainTestNameFragment(alt) {
			return []string{branch} // not an explicit-name group; leave whole
		}
	}
	var out []string
	for _, alt := range alts {
		out = append(out, distributeExplicitGroup(prefix+alt+suffix)...)
	}
	return out
}

// firstTopLevelGroup returns the byte offsets of the first '(' at nesting depth
// zero and its matching ')', or (-1, -1) when there is no balanced top-level
// group. Escaped and character-class-contained parentheses are ignored.
func firstTopLevelGroup(pattern string) (openIdx, closeIdx int) {
	open := -1
	depth := 0
	inClass := false
	for i := 0; i < len(pattern); i++ {
		switch ch := pattern[i]; ch {
		case '\\':
			i++ // skip the escaped rune
		case '[':
			inClass = true
		case ']':
			inClass = false
		case '(':
			if inClass {
				continue
			}
			if depth == 0 {
				open = i
			}
			depth++
		case ')':
			if inClass {
				continue
			}
			depth--
			if depth == 0 && open >= 0 {
				return open, i
			}
		}
	}
	return -1, -1
}

// isPlainTestNameFragment reports whether alt is a nonempty run of identifier
// runes, i.e. a literal fragment of a Go test name with no regex operators.
func isPlainTestNameFragment(alt string) bool {
	if alt == "" {
		return false
	}
	for i := 0; i < len(alt); i++ {
		c := alt[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '_':
		default:
			return false
		}
	}
	return true
}
