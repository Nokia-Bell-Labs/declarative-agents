// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package scaffold

import (
	"bytes"
	"slices"
	"strings"
)

// BlockBegin and BlockEnd bound the managed text in an orientation file. A
// later run replaces what lies between them and keeps every line outside
// them (srd043 R3.5). They name no project, as the templates do (GH-36).
const (
	BlockBegin = "<!-- managed:begin: the init and onboard profiles write the text down to managed:end and refresh it on a rerun; write your own notes outside it -->"
	BlockEnd   = "<!-- managed:end -->"
)

// blockBeginPrefixes match a begin line whatever note it carries, so a
// reworded note still opens the block; the tpm marker an earlier release
// wrote still opens one, and a rerun replaces it (GH-36).
var blockBeginPrefixes = []string{"<!-- managed:begin", "<!-- tpm:begin"}

// blockEnds close a block: the current marker and the earlier one.
var blockEnds = []string{BlockEnd, "<!-- tpm:end -->"}

// splitManaged cuts content around its managed block: the lines before the
// begin line, the block from the begin line through the end line, and the
// lines after it. ok is false when content holds no whole block.
func splitManaged(content []byte) (before, block, after []byte, ok bool) {
	lines := bytes.SplitAfter(content, []byte("\n"))
	begin, end := -1, -1
	for i, l := range lines {
		t := strings.TrimSpace(string(l))
		switch {
		case begin < 0 && slices.ContainsFunc(blockBeginPrefixes, func(p string) bool { return strings.HasPrefix(t, p) }):
			begin = i
		case begin >= 0 && slices.Contains(blockEnds, t):
			end = i
		}
		if end >= 0 {
			break
		}
	}
	if begin < 0 || end < 0 {
		return nil, nil, nil, false
	}
	return bytes.Join(lines[:begin], nil), bytes.Join(lines[begin:end+1], nil), bytes.Join(lines[end+1:], nil), true
}

// refreshManaged is cur with fresh's managed block in it: replacing cur's
// block, how "refreshed", or appended after cur's text and one blank line
// when cur has none, how "appended" (srd043 R3.5). ok is false when fresh
// has no whole block, so the file is not an orientation file.
func refreshManaged(cur, fresh []byte) (merged []byte, how string, ok bool) {
	_, block, _, ok := splitManaged(fresh)
	if !ok {
		return nil, "", false
	}
	before, _, after, ok := splitManaged(cur)
	if !ok {
		out := append([]byte{}, cur...)
		if len(out) > 0 {
			if !bytes.HasSuffix(out, []byte("\n")) {
				out = append(out, '\n')
			}
			if !bytes.HasSuffix(out, []byte("\n\n")) {
				out = append(out, '\n')
			}
		}
		return append(out, block...), "appended", true
	}
	return append(append(append([]byte{}, before...), block...), after...), "refreshed", true
}
