// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package corpus

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// The keys of a document's people block, in its frontmatter: who wrote it and
// who signs off on it.
const (
	FrontAuthors  = "authors"
	FrontReviewer = "reviewer"
)

// Front is a document's frontmatter alone, for what needs nothing of its body.
func Front(data []byte) (map[string]any, error) {
	front, _, err := splitFrontmatter(data)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if err := yaml.Unmarshal([]byte(front), &out); err != nil {
		return nil, fmt.Errorf("frontmatter: %w", err)
	}
	return out, nil
}

// FrontNames is the people a frontmatter key names, as a YAML list or a
// comma-separated string.
func FrontNames(front map[string]any, key string) []string {
	var out []string
	add := func(s string) {
		for _, n := range strings.Split(s, ",") {
			if n = strings.TrimSpace(n); n != "" {
				out = append(out, n)
			}
		}
	}
	switch v := front[key].(type) {
	case string:
		add(v)
	case []any:
		for _, e := range v {
			if s, ok := e.(string); ok {
				add(s)
			}
		}
	}
	return out
}

// SetFront is the document with key's frontmatter line reading value: rewritten
// in place, else added last, and dropped for an empty value. Everything else
// stays as written.
func SetFront(data []byte, key, value string) ([]byte, error) {
	if !bytes.HasPrefix(data, []byte("---\n")) {
		return nil, errors.New("no frontmatter: a corpus document opens with ---")
	}
	end := bytes.Index(data[4:], []byte("\n---"))
	if end < 0 {
		return nil, errors.New("frontmatter is not closed by ---")
	}
	end += 4
	var kept []string
	line := key + ": " + value
	found := false
	for _, l := range strings.Split(string(data[4:end]), "\n") {
		switch {
		case !strings.HasPrefix(l, key+":"):
			kept = append(kept, l)
		case value != "" && !found:
			kept, found = append(kept, line), true
		}
	}
	if value != "" && !found {
		kept = append(kept, line)
	}
	return append([]byte("---\n"+strings.Join(kept, "\n")), data[end:]...), nil
}
