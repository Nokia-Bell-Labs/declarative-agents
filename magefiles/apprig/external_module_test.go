// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package apprig

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// GH-2493 AC2: a genuinely separate Go module imports the released package and
// exposes all five thin Mage verbs. Status is executed, not only compiled, so
// manifest loading and the downstream replacement resolve through module
// boundaries.
func TestExternalModuleThinMageTargets(t *testing.T) {
	if _, err := exec.LookPath("mage"); err != nil {
		t.Skip("mage not on PATH")
	}
	fixture := filepath.Join("testdata", "external-module")
	run := func(args ...string) string {
		t.Helper()
		command := exec.Command("mage", args...)
		command.Dir = fixture
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("mage %s: %v\n%s", strings.Join(args, " "), err, output)
		}
		return string(output)
	}
	listed := run("-l")
	for _, target := range []string{"app:up", "app:status", "app:down", "app:diagnose", "app:purge"} {
		if !strings.Contains(listed, target) {
			t.Errorf("external module target list missing %s:\n%s", target, listed)
		}
	}
	status := run("app:status")
	if !strings.Contains(status, `"application":"downstream-fixture"`) ||
		!strings.Contains(status, `"state":"unknown"`) {
		t.Fatalf("external module status did not execute apprig:\n%s", status)
	}
}
