// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/pinsurvey"
)

// srd006 R1.3: the checked-in list must keep describing what is there. A pin
// moved or renamed without updating the list leaves a report citing a line
// that no longer holds it, which is worse than no report.
func TestDeclaredPinsDescribeTheirLocations(t *testing.T) {
	root, err := findRepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	pins, err := pinsurvey.DeclaredPins()
	if err != nil {
		t.Fatal(err)
	}
	if len(pins) == 0 {
		t.Fatal("the declared pin list is empty")
	}
	files, err := pinsurvey.DeclaredPinFiles()
	if err != nil {
		t.Fatal(err)
	}
	for _, pin := range pins {
		file := files[pin.Location]
		if file == "" {
			t.Errorf("%s names no file", pin.Location)
			continue
		}
		content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
		if err != nil {
			t.Errorf("%s: %v", pin.Location, err)
			continue
		}
		text := string(content)
		if pin.Digest != "" && !strings.Contains(text, pin.Digest) {
			t.Errorf("%s: %s no longer holds digest %s", pin.Location, file, pin.Digest)
		}
		if !strings.Contains(text, pin.Tag) {
			t.Errorf("%s: %s no longer holds tag %s", pin.Location, file, pin.Tag)
		}
	}
}

// Every pin a render exposes must be absent from the declared list: the two
// halves partition the inventory, and an image in both would be surveyed and
// reported twice (srd006 R1.1, R1.2).
func TestDeclaredPinsDoNotRestateChartImages(t *testing.T) {
	pins, err := pinsurvey.DeclaredPins()
	if err != nil {
		t.Fatal(err)
	}
	chartOnly := map[string]bool{
		"docker.io/ollama/ollama": true, "docker.io/dolthub/dolt-sql-server": true,
		"docker.io/chromadb/chroma": true, "docker.io/rancher/kubectl": true,
		"docker.io/library/busybox":                      true,
		"docker.io/otel/opentelemetry-collector-contrib": true,
		"docker.io/library/golang":                       true,
		"docker.io/golangci/golangci-lint":               true,
	}
	for _, pin := range pins {
		if chartOnly[pin.Image] {
			t.Errorf("%s restates %s, which the charts already expose", pin.Location, pin.Image)
		}
	}
}

// srd006 R4.1: the target reports and writes nothing.
func TestBumpLeavesTheWorkingTreeUnchanged(t *testing.T) {
	root, err := findRepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	before, err := gitStatus(root)
	if err != nil {
		t.Skipf("git status unavailable: %v", err)
	}
	pins, err := collectPinsForTest(t, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(pins) == 0 {
		t.Fatal("no pins collected")
	}
	after, err := gitStatus(root)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Errorf("collecting pins changed the working tree:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// collectPinsForTest builds the inventory without reaching any registry, so
// the no-write claim is proven without a network (srd006 R4.2).
func collectPinsForTest(t *testing.T, root string) ([]pinsurvey.Pin, error) {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		declared, err := pinsurvey.DeclaredPins()
		return declared, err
	}
	return collectPins(root)
}

// The inventory covers both halves and skips the images this checkout builds.
func TestCollectedPinsCoverBothHalves(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH: the derived half needs a render")
	}
	root, err := findRepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	pins, err := collectPins(root)
	if err != nil {
		t.Fatal(err)
	}
	var sawDeclared, sawChart bool
	donors := map[string]string{
		"golang":                 "sha256:8ac98ca534ac3f51e1f420a1dd2c15e74c75cfa0f23f3ad27eb5d7236c349a0c",
		"golangci/golangci-lint": "sha256:91b27804074a0bacea298707f016911e60cf0cdbc6c7bf5ccacb5f0606d18d60",
	}
	seenDonors := map[string]bool{}
	for _, pin := range pins {
		if strings.HasPrefix(pin.Location, "magefiles/kindrig/") {
			sawDeclared = true
		}
		if strings.HasPrefix(pin.Location, "applications/") {
			sawChart = true
		}
		if strings.Contains(strings.ToLower(pin.Image), "declarative-agents") ||
			strings.HasPrefix(strings.ToLower(pin.Image), "kindrig/") {
			t.Errorf("%s surveys an image this checkout produces: %s", pin.Location, pin.Image)
		}
		if digest, ok := donors[pin.Image]; ok && strings.Contains(pin.Location, "[defaults]") {
			seenDonors[pin.Image] = true
			if pin.Digest != digest {
				t.Errorf("%s donor digest = %q, want %q (%s)", pin.Image, pin.Digest, digest, pin.Location)
			}
		}
	}
	if !sawDeclared {
		t.Error("no declared pin in the inventory")
	}
	if !sawChart {
		t.Error("no chart pin in the inventory")
	}
	for donor := range donors {
		if !seenDonors[donor] {
			t.Errorf("chart pin inventory missing executor donor %s", donor)
		}
	}
}

// srd006 R4.2: the audit stays hermetic, so it must not invoke this target.
func TestAuditDoesNotInvokeBump(t *testing.T) {
	content, err := os.ReadFile("audit.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "Bump()") ||
		strings.Contains(string(content), "TestDeclaredPins") {
		t.Error("audit.go reaches the bump target, which needs the network")
	}
}

func gitStatus(root string) (string, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = root
	out, err := cmd.Output()
	return string(out), err
}
