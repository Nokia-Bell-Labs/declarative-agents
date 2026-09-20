// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The corpus audit is the gate GH-2310 found missing: the checks existed and
// nothing ran them over this module's documents. These tests pin the wiring —
// which profile is run, with which roots — and the failure contract. The
// evidence that the checks themselves fire is the profile run in mage audit,
// and the charter names which ones gate.

func TestValidateSpecificationCorpusRunsTheCorpusAuditProfile(t *testing.T) {
	var gotBinary string
	var gotArgs []string
	run := func(binary string, args ...string) ([]byte, error) {
		gotBinary, gotArgs = binary, args
		return []byte("All consistency checks passed.\n"), nil
	}

	if err := validateSpecificationCorpus(run, "/tmp/agent", "/modules/catalog", "/modules/agent-core"); err != nil {
		t.Fatalf("a passing audit returned %v", err)
	}
	if gotBinary != "/tmp/agent" {
		t.Errorf("binary = %q", gotBinary)
	}
	want := []string{
		"--profile", filepath.Join("/modules/catalog", filepath.FromSlash(corpusAuditProfileRel)),
		"--directory", "/modules/catalog",
		"--core-root", "/modules/agent-core",
	}
	if strings.Join(gotArgs, " ") != strings.Join(want, " ") {
		t.Errorf("args = %v, want %v", gotArgs, want)
	}
}

// The agent exits non-zero when the machine reaches Failed, which for this
// profile means the charter reported an error-level finding.
func TestValidateSpecificationCorpusFailsWhenTheAuditFails(t *testing.T) {
	run := func(string, ...string) ([]byte, error) {
		return []byte("[error] catalog-corpus-charter/broken-touchpoint (spec_corpus):\n" +
				"  - use case rel17.0-uc001-rig-doctor touchpoint references non-existent SRD srd999-does-not-exist\n"),
			errors.New("exit status 2")
	}

	err := validateSpecificationCorpus(run, "/tmp/agent", "/modules/catalog", "/modules/agent-core")
	if err == nil {
		t.Fatal("a failed audit returned nil")
	}
	if !strings.Contains(err.Error(), "/modules/catalog") {
		t.Errorf("error %q does not name the module it audited", err)
	}
}

// The charter is what decides which checks gate, so a change to the gate is a
// change to a checked-in document rather than to Go. This pins the two the
// epic's acceptance criteria name, so removing either is a deliberate edit
// here as well.
func TestCatalogCharterGatesTheTraceabilityChecks(t *testing.T) {
	var charter struct {
		Checks []struct {
			Kind   string   `yaml:"kind"`
			Checks []string `yaml:"checks"`
		} `yaml:"checks"`
	}
	path := filepath.Join("..", "docs", "corpus-charter.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := yaml.Unmarshal(data, &charter); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	selected := map[string]bool{}
	for _, check := range charter.Checks {
		if check.Kind != "spec_corpus" {
			continue
		}
		for _, id := range check.Checks {
			selected[id] = true
		}
	}
	for _, want := range []string{
		"broken-touchpoint", "broken-citation",
		"index-broken-path", "index-missing-use-case", "index-missing-test-suite",
		"roadmap-missing-use-case",
	} {
		if !selected[want] {
			t.Errorf("the catalog charter does not gate on %q", want)
		}
	}
}
