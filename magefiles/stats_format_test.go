// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// combinedFixture mirrors the shape mage stats collects: a module with nested
// file trees, a module with agents, and the synthetic agents_total.
const combinedFixture = `{
  "agent-core": {
    "go": {
      "src_lines": 38732,
      "test_lines": 33635,
      "total_lines": 72367
    },
    "yaml": {
      "total": {
        "files": 226,
        "lines": 32791
      },
      "docs": {
        "total": {
          "files": 103,
          "lines": 24533
        },
        "categories": {
          "top_level": {
            "files": 4,
            "lines": 2995
          }
        }
      }
    }
  },
  "agent-profiles": {
    "agents": {
      "total": {
        "agents": 1,
        "states": 6,
        "transitions": 9,
        "tools": 3,
        "yaml": {
          "files": 4,
          "lines": 145
        }
      },
      "per_agent": {
        "jurist": {
          "states": 6,
          "transitions": 9,
          "tools": 3,
          "yaml": {
            "files": 4,
            "lines": 145
          }
        }
      }
    }
  }
}`

func formatFixture(t *testing.T, doc string) string {
	t.Helper()
	out, err := formatJSON([]byte(doc))
	if err != nil {
		t.Fatalf("formatJSON returned error: %v", err)
	}
	return string(out)
}

// lineWith returns the first line containing want, failing if absent.
func lineWith(t *testing.T, out, want string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, want) {
			return line
		}
	}
	t.Fatalf("no line containing %q in:\n%s", want, out)
	return ""
}

// TestFormatJSONPreservesValues proves the formatter only changes whitespace:
// the reformatted document parses to exactly the same values as the input.
func TestFormatJSONPreservesValues(t *testing.T) {
	t.Parallel()
	out := formatFixture(t, combinedFixture)

	var before, after any
	if err := json.Unmarshal([]byte(combinedFixture), &before); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	if err := json.Unmarshal([]byte(out), &after); err != nil {
		t.Fatalf("reformatted output is not valid JSON: %v\n%s", err, out)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("formatting changed the document:\nbefore %#v\nafter  %#v", before, after)
	}
}

// TestFormatJSONCollapsesSmallValues proves values that fit on one line are
// printed on one line, which is the whole point: a leaf files/lines pair and a
// whole agent each become a single readable fact.
func TestFormatJSONCollapsesSmallValues(t *testing.T) {
	t.Parallel()
	out := formatFixture(t, combinedFixture)

	if got := lineWith(t, out, `"go"`); !strings.Contains(got, `"total_lines": 72367}`) {
		t.Errorf("go block should be one line, got %q", got)
	}
	if got := lineWith(t, out, `"top_level"`); !strings.Contains(got, `{"files": 4, "lines": 2995}`) {
		t.Errorf("leaf should be one line, got %q", got)
	}
	if got := lineWith(t, out, `"jurist"`); !strings.Contains(got, `"yaml": {"files": 4, "lines": 145}}`) {
		t.Errorf("agent entry should be one line, got %q", got)
	}
	if lines := strings.Count(out, "\n"); lines > 20 {
		t.Errorf("output is %d lines, want the fixture to collapse well under the 34 it started at", lines)
	}
}

// TestFormatJSONExpandsWideValues proves a value too wide for one line still
// expands, so the format degrades gracefully rather than emitting a very long
// line.
func TestFormatJSONExpandsWideValues(t *testing.T) {
	t.Parallel()
	wide := `{"m": {"a_very_long_key_name_one": 111111, "a_very_long_key_name_two": 222222,
		"a_very_long_key_name_three": 333333, "a_very_long_key_name_four": 444444}}`
	out := formatFixture(t, wide)

	if !strings.Contains(out, "{\n") {
		t.Errorf("wide value should expand, got:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "a_very_long_key_name_one") && strings.Contains(line, "a_very_long_key_name_four") {
			t.Errorf("wide value stayed on one line: %q", line)
		}
	}
}

// TestFormatJSONPreservesKeyOrder proves object key order survives, so a
// "total" still precedes the breakdown it summarizes.
func TestFormatJSONPreservesKeyOrder(t *testing.T) {
	t.Parallel()
	out := formatFixture(t, combinedFixture)

	totalAt := strings.Index(out, `"total"`)
	categoriesAt := strings.Index(out, `"categories"`)
	if totalAt < 0 || categoriesAt < 0 {
		t.Fatalf("missing keys in:\n%s", out)
	}
	if totalAt > categoriesAt {
		t.Errorf("total@%d should precede categories@%d:\n%s", totalAt, categoriesAt, out)
	}

	perAgentAt := strings.Index(out, `"per_agent"`)
	agentsTotalAt := strings.Index(out, `"agents": 1`)
	if agentsTotalAt > perAgentAt {
		t.Errorf("agents total should precede per_agent:\n%s", out)
	}
}

// TestFormatJSONKeepsIntegerLiterals proves large counts stay integers rather
// than picking up float notation on the round trip.
func TestFormatJSONKeepsIntegerLiterals(t *testing.T) {
	t.Parallel()
	out := formatFixture(t, `{"go": {"src_lines": 38732, "big": 12345678901, "ratio": 1.5}}`)

	for _, want := range []string{"38732", "12345678901", "1.5"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing literal %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "e+") {
		t.Errorf("output uses float notation:\n%s", out)
	}
}

// TestFormatJSONHandlesArraysAndScalars proves non-object values survive, so
// the formatter stays correct if a module's stats output grows a list.
func TestFormatJSONHandlesArraysAndScalars(t *testing.T) {
	t.Parallel()
	out := formatFixture(t, `{"list": [1, 2, 3], "name": "agent-core", "ok": true, "none": null}`)

	if !strings.Contains(out, `"list": [1, 2, 3]`) {
		t.Errorf("short array should stay on one line:\n%s", out)
	}
	for _, want := range []string{`"name": "agent-core"`, `"ok": true`, `"none": null`} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

// TestFormatJSONBadInput proves malformed collected output surfaces as an
// error rather than a half-written document.
func TestFormatJSONBadInput(t *testing.T) {
	t.Parallel()
	if _, err := formatJSON([]byte(`{"agent-core":`)); err == nil {
		t.Fatal("formatJSON = nil error, want parse failure")
	}
}

// TestDecodeOrderedPreservesKeyOrder pins the ordering guarantee the formatter
// depends on.
func TestDecodeOrderedPreservesKeyOrder(t *testing.T) {
	t.Parallel()
	v, err := decodeOrdered([]byte(`{"z": 1, "a": 2, "m": {"nested": 3}}`))
	if err != nil {
		t.Fatalf("decodeOrdered returned error: %v", err)
	}
	obj, ok := v.(*orderedObject)
	if !ok {
		t.Fatalf("decodeOrdered returned %T, want *orderedObject", v)
	}
	if want := []string{"z", "a", "m"}; !reflect.DeepEqual(obj.keys, want) {
		t.Fatalf("keys = %#v, want %#v", obj.keys, want)
	}
}
