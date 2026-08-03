// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// These bind the applier's declared helm flags to the helm the applier image
// ships (srd006 R5.3), and pin the design decision the live tier exposed: the
// apply must not wait.
//
// The validate-without-applying flag is major-version-specific. helm 3 spells it
// --dry-run; helm 4 deprecates that and spells it --dry-run=client. Neither works
// on the other major, so helm_dry_run's declaration and the pinned HELM_VERSION
// are one decision recorded in two files, and nothing else holds them together.
// helm rejects an unknown flag outright, so a half-migrated declaration fails
// every apply on the cluster while the fake-CLI tracer stays green.
//
// helm_upgrade carries no waiting or self-rollback flag at all. --atomic forces
// --wait, so a waited upgrade blocks on a never-ready role until helm's own
// timeout, fails at the apply step, and self-rolls-back into Failed -- which
// makes the machine's explicit verify -> RollingBack -> RolledBack path
// unreachable (apply-machine.yaml). The apply therefore returns immediately and
// the kubectl rollout status verify words catch a stall and compensate it with
// helm_rollback. This guard fails if --atomic, --rollback-on-failure, or --wait
// reappears on helm_upgrade, because the fake CLIs accept any flags at all and
// would not notice the regression.

// helmVersionPattern matches the pinned version in the shared applier Dockerfile.
var helmVersionPattern = regexp.MustCompile(`(?m)^ARG HELM_VERSION=v(\d+)\.`)

// helmDryRunByMajor is what each helm major calls validate-without-applying, the
// one version-specific behavior the applier still depends on.
var helmDryRunByMajor = map[int]string{
	3: "--dry-run",
	4: "--dry-run=client",
}

// helmUpgradeForbiddenFlags are the waiting and self-rollback flags helm_upgrade
// must never carry, in either major's spelling. --wait and --atomic are helm 3's;
// --rollback-on-failure is helm 4's rename of --atomic. Any of them makes the
// apply block and self-roll-back, closing the explicit rollback path.
var helmUpgradeForbiddenFlags = []string{"--atomic", "--wait", "--rollback-on-failure"}

// pinnedHelmMajor reads the helm major the applier image ships.
func pinnedHelmMajor(t *testing.T) int {
	t.Helper()
	appRoot := filepath.Dir(findChartDir(t))
	// The applier image is the shared agent-core/applier.Dockerfile (GH-1368),
	// two levels up from the application root.
	dockerfile := filepath.Join(appRoot, "..", "..", "agent-core", "applier.Dockerfile")
	data, err := os.ReadFile(dockerfile)
	if err != nil {
		t.Fatalf("read agent-core/applier.Dockerfile: %v", err)
	}
	match := helmVersionPattern.FindSubmatch(data)
	if match == nil {
		t.Fatal("agent-core/applier.Dockerfile pins no ARG HELM_VERSION=vN.…; the flag guard cannot tell which helm ships")
	}
	major, err := strconv.Atoi(string(match[1]))
	if err != nil {
		t.Fatalf("parse helm major from %q: %v", match[1], err)
	}
	return major
}

// TestApplierHelmFlagsMatchTheShippedHelm proves the declared flags are the ones
// the pinned helm actually takes: helm_dry_run carries the right dry-run spelling
// and not the other major's, and helm_upgrade carries no waiting or self-rollback
// flag in any spelling.
func TestApplierHelmFlagsMatchTheShippedHelm(t *testing.T) {
	major := pinnedHelmMajor(t)
	wantDryRun, known := helmDryRunByMajor[major]
	if !known {
		t.Fatalf("agent-core/applier.Dockerfile pins helm %d, whose dry-run spelling this guard does not know; "+
			"decide what it calls validate-without-applying, and add it to helmDryRunByMajor", major)
	}

	var decls execDeclarations
	readIntakeYAML(t, filepath.Join(agentDir(t, "applier"), "exec-declarations.yaml"), &decls)
	args := map[string][]string{}
	for _, tool := range decls.Tools {
		args[tool.Name] = tool.Args
	}

	upgrade, ok := args["helm_upgrade"]
	if !ok {
		t.Fatal("the applier declares no helm_upgrade word")
	}
	// The apply must return immediately. A waited upgrade blocks on a never-ready
	// role, fails at the apply step, and self-rolls-back into Failed, leaving the
	// verify -> RollingBack -> RolledBack path unreachable (apply-machine.yaml).
	for _, flag := range helmUpgradeForbiddenFlags {
		if containsString(upgrade, flag) {
			t.Errorf("helm_upgrade passes %s, which makes the apply wait and self-roll-back; "+
				"the apply must return immediately so the kubectl rollout status verify catches a stall "+
				"and helm_rollback compensates it", flag)
		}
	}

	dryRun, ok := args["helm_dry_run"]
	if !ok {
		t.Fatal("the applier declares no helm_dry_run word")
	}
	if !containsString(dryRun, wantDryRun) {
		t.Errorf("helm_dry_run does not pass %s, which helm %d calls the validate-without-applying flag",
			wantDryRun, major)
	}
	// The other major's dry-run spelling must not appear: helm rejects an unknown
	// flag outright, so a half-migrated declaration fails every apply on the
	// cluster while the fake-CLI tracer stays green.
	for otherMajor, otherDryRun := range helmDryRunByMajor {
		if otherMajor == major || otherDryRun == wantDryRun {
			continue
		}
		if containsString(dryRun, otherDryRun) {
			t.Errorf("helm_dry_run passes %s, which is helm %d's spelling, but the image ships helm %d",
				otherDryRun, otherMajor, major)
		}
	}
}

// TestApplierHelmFlagGuardCoversTheDeclaredWords proves the guard reads the words
// it claims to. A guard that silently matched nothing would pass forever.
func TestApplierHelmFlagGuardCoversTheDeclaredWords(t *testing.T) {
	var decls execDeclarations
	readIntakeYAML(t, filepath.Join(agentDir(t, "applier"), "exec-declarations.yaml"), &decls)
	var helmWords int
	for _, tool := range decls.Tools {
		if tool.Binary == "helm" {
			helmWords++
		}
	}
	if helmWords < 3 {
		t.Errorf("the applier declares %d helm words; the guard expects at least the dry-run, the upgrade, "+
			"and the rollback, so a word may have been dropped or renamed", helmWords)
	}
	// helm_rollback runs no version-specific flag today; if it grows one, this
	// guard has to learn about it rather than let it drift.
	for _, tool := range decls.Tools {
		if tool.Name != "helm_rollback" {
			continue
		}
		for _, arg := range tool.Args {
			if strings.HasPrefix(arg, "--") {
				t.Errorf("helm_rollback now passes %s; add it to the version guard if its spelling is major-specific", arg)
			}
		}
	}
}
