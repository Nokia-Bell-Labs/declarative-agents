// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The entry shapes are taken from the corpus, not invented, and each one that
// is skipped is skipped for a stated reason.
func TestReferencePathReadsTheCorpusShapes(t *testing.T) {
	t.Parallel()
	for name, testCase := range map[string]struct {
		entry string
		want  string
	}{
		"path with a note": {
			"magefiles/kindrig/diagnose.go (shared on-demand diagnosis)",
			"magefiles/kindrig/diagnose.go",
		},
		"path with a requirement id and a note": {
			"applications/catalog/docs/specs/software-requirements/srd020-collector.yaml R7.4 (static_assets root)",
			"applications/catalog/docs/specs/software-requirements/srd020-collector.yaml",
		},
		"bare path": {
			"docs/ARCHITECTURE.yaml",
			"docs/ARCHITECTURE.yaml",
		},
		"title and url": {
			"kind Quick Start, https://kind.sigs.k8s.io/docs/user/quick-start/",
			"",
		},
		"bare url": {
			"https://kind.sigs.k8s.io/docs/user/ingress/",
			"",
		},
		"issue with a note": {
			"GH-1316 (chatbot-mesh ux/ to agents/chatbot/ui migration)",
			"",
		},
		// The trap. The entry cites an SRD by id and the file on disk is
		// srd013-standard-tool-library.yaml, so reading it as a path reports a
		// miss that is really a citation style.
		"srd id with a requirement id": {
			"agent-core/docs/specs/software-requirements/srd013 R5.6/R5.7 (mounted-declaration parameterization)",
			"",
		},
		"prose with no path": {
			"The applier's pinned CLI donor",
			"",
		},
		"empty": {"", ""},
	} {
		if got := referencePath(testCase.entry); got != testCase.want {
			t.Errorf("%s: referencePath(%q) = %q, want %q", name, testCase.entry, got, testCase.want)
		}
	}
}

// Two conventions are in use: a repository-level document writes repo-relative
// paths, and a module's own document writes them relative to its module.
func TestDocumentReferencesResolveAgainstEitherConvention(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	write(t, root, "agent-core/docs/VISION.yaml", "id: vision\n")
	write(t, root, "magefiles/kindrig/diagnose.go", "package kindrig\n")

	write(t, root, "agent-core/docs/constitutions/design.yaml",
		"id: design\nreferences:\n- docs/VISION.yaml\n")
	if failures := documentReferenceFailures(root, "agent-core/docs/constitutions/design.yaml"); len(failures) != 0 {
		t.Errorf("module-relative reference reported %v, want it resolved against agent-core/", failures)
	}

	write(t, root, "docs/engineering/eng01-rig.yaml",
		"id: eng01\nreferences:\n- magefiles/kindrig/diagnose.go (the rig)\n")
	if failures := documentReferenceFailures(root, "docs/engineering/eng01-rig.yaml"); len(failures) != 0 {
		t.Errorf("repo-relative reference reported %v, want it resolved against the root", failures)
	}
}

func TestDocumentReferencesReportEveryDeadPath(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	write(t, root, "docs/engineering/eng09-example.yaml", strings.Join([]string{
		"id: eng09",
		"references:",
		"- magefiles/kindrig/gone.go (a file that moved)",
		"- docs/also-gone.yaml",
		"- https://example.invalid/page",
		"- GH-1234 (an issue)",
	}, "\n")+"\n")

	failures := documentReferenceFailures(root, "docs/engineering/eng09-example.yaml")
	if len(failures) != 2 {
		t.Fatalf("failures = %v, want exactly the two dead paths", failures)
	}
	for _, want := range []string{"gone.go", "also-gone.yaml"} {
		found := false
		for _, failure := range failures {
			if strings.Contains(failure, want) {
				found = true
			}
		}
		if !found {
			t.Errorf("failures = %v, want one naming %q", failures, want)
		}
	}
}

// A document with no references list, or one this shape cannot hold, is the
// placement check's finding to make rather than this one's.
func TestDocumentReferencesStaySilentWithoutAList(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	write(t, root, "docs/engineering/eng10-bare.yaml", "id: eng10\ntitle: No references\n")
	if failures := documentReferenceFailures(root, "docs/engineering/eng10-bare.yaml"); len(failures) != 0 {
		t.Errorf("document without references reported %v, want silence", failures)
	}
	write(t, root, "docs/engineering/eng11-odd.yaml", "id: eng11\nreferences:\n  nested: value\n")
	if failures := documentReferenceFailures(root, "docs/engineering/eng11-odd.yaml"); len(failures) != 0 {
		t.Errorf("unreadable references list reported %v, want the placement check to own it", failures)
	}
}

func write(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
