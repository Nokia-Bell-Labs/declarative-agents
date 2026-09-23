// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package imageinventory_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/imageinventory"
	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/pinsurvey"
)

func TestRegisteredFamiliesAreDocumented(t *testing.T) {
	if len(imageinventory.Families) < 8 {
		t.Fatalf("families = %d, want the ENG01 table set", len(imageinventory.Families))
	}
	for _, family := range imageinventory.Families {
		if family.ID == "" || family.Class == "" || family.Owner == "" || family.Retention == "" {
			t.Fatalf("incomplete family %#v", family)
		}
	}
}

func TestClassifyReferenceCoversLocalAndUpstream(t *testing.T) {
	gitRef, err := kindrig.FormatGitLocal(
		kindrig.RuntimeRole, "agent-core", "111111111111", "linux/arm64")
	if err != nil {
		t.Fatal(err)
	}
	family, err := imageinventory.ClassifyReference(gitRef)
	if err != nil || family.ID != "agent-core-runtime-local" {
		t.Fatalf("runtime local = %#v err %v", family, err)
	}
	family, err = imageinventory.ClassifyReference(kindrig.CLIDonorImage)
	if err != nil || family.ID != "rig-upstream-pin" {
		t.Fatalf("cli donor = %#v err %v", family, err)
	}
}

func TestDeclaredPinsClassifyIntoFamilies(t *testing.T) {
	pins, err := pinsurvey.DeclaredPins()
	if err != nil {
		t.Fatal(err)
	}
	for _, pin := range pins {
		family, err := imageinventory.ClassifyPin(pin)
		if err != nil {
			t.Errorf("%s: %v", pin.Location, err)
			continue
		}
		if family.ID == "" {
			t.Errorf("%s: empty family", pin.Location)
		}
	}
}

func TestSweepActivePathsFindsNoProhibitedIdentity(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	findings, err := imageinventory.SweepActivePaths(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) > 0 {
		var report strings.Builder
		for _, finding := range findings {
			report.WriteString(finding.Path)
			report.WriteString(":")
			report.WriteString(strings.TrimSpace(finding.Image))
			report.WriteString(" [")
			report.WriteString(finding.Rule)
			report.WriteString("] ")
			report.WriteString(finding.Detail)
			report.WriteString("\n")
		}
		t.Fatalf("prohibited active image references:\n%s", report.String())
	}
}
