// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package beads

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// record is a line of one of scrum-master's own tracker files beside the issues:
// milestones and labels. Each has a key unique in its file and a time it
// last changed, and merges a record at a time.
type record interface {
	Milestone | Label
	key() string
	changed() string
}

// sameRecord reports whether a and b carry the same content, ignoring when
// each changed.
func sameRecord[T record](a, b T) bool {
	ja, _ := json.Marshal(withoutStamp(a))
	jb, _ := json.Marshal(withoutStamp(b))
	return bytes.Equal(ja, jb)
}

func withoutStamp[T record](r T) T {
	switch v := any(r).(type) {
	case Milestone:
		v.UpdatedAt = ""
		return any(v).(T)
	case Label:
		v.UpdatedAt = ""
		return any(v).(T)
	}
	return r
}

func parseRecords[T record](file string, b []byte) ([]T, error) {
	var out []T
	for n, line := range bytes.Split(b, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var r T
		if err := json.Unmarshal(line, &r); err != nil {
			return nil, fmt.Errorf("%s line %d: %w", file, n+1, err)
		}
		out = append(out, r)
	}
	return out, nil
}

func writeRecords[T record](path string, rs []T) error {
	sorted := slices.Clone(rs)
	slices.SortFunc(sorted, func(a, b T) int { return strings.Compare(a.key(), b.key()) })
	var buf bytes.Buffer
	for _, r := range sorted {
		line, err := json.Marshal(r)
		if err != nil {
			return err
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// mergeRecords merges two clones' records over their merge base, a record
// at a time: the side that changed it wins, and when both did, the later
// writer.
func mergeRecords[T record](base, ours, theirs []T) []T {
	index := func(rs []T) map[string]T {
		m := make(map[string]T, len(rs))
		for _, r := range rs {
			m[r.key()] = r
		}
		return m
	}
	b, t := index(base), index(theirs)
	out := slices.Clone(theirs)
	for _, o := range ours {
		k := o.key()
		tr, inTheirs := t[k]
		br, inBase := b[k]
		switch {
		case !inTheirs:
			out = append(out, o)
		case inBase && sameRecord(o, br):
		case inBase && sameRecord(tr, br), o.changed() > tr.changed():
			out[slices.IndexFunc(out, func(x T) bool { return x.key() == k })] = o
		}
	}
	return out
}

func readRecords[T record](s *Store, file string) ([]T, error) {
	b, err := os.ReadFile(filepath.Join(s.Dir, file))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return parseRecords[T](file, b)
}

func showRecords[T record](s *Store, rev, file string) ([]T, error) {
	out, err := s.git("show", rev+":"+file)
	if err != nil {
		return nil, nil
	}
	return parseRecords[T](file, []byte(out))
}

// mergeRecordFile writes the merge of one record file across base, the
// clone's HEAD, and remote into the working tree, during a sync's merge.
func mergeRecordFile[T record](s *Store, file, base, remote string) error {
	var sides [3][]T
	for n, rev := range []string{base, "HEAD", remote} {
		rs, err := showRecords[T](s, rev, file)
		if err != nil {
			return err
		}
		sides[n] = rs
	}
	merged := mergeRecords(sides[0], sides[1], sides[2])
	if len(merged) == 0 {
		return nil
	}
	return writeRecords(filepath.Join(s.Dir, file), merged)
}
