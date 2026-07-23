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

// These bind the executor's declared helm flags to the helm the executor image
// ships (GH-739).
//
// The flags are major-version-specific. helm 3 takes --atomic and --dry-run;
// helm 4 deprecates both and spells them --rollback-on-failure and
// --dry-run=client. Neither spelling works on the other major, so the
// declarations and the pinned HELM_VERSION are one decision recorded in two
// files, and nothing else holds them together.
//
// The failure this prevents is quiet and expensive. --atomic is what rolls a
// failed upgrade back, and apply-machine.yaml routes Applying + ToolFailed
// straight to Failed with no compensating rollback *because* of it. Bump the
// image to helm 4 and the flag first warns, then eventually goes; the apply
// stops self-rolling-back and that leg leaves the release on a failed revision,
// with every test still green -- integration:executor drives fake CLIs, which
// accept any flags at all.

// helmVersionPattern matches the pinned version in the executor Dockerfile.
var helmVersionPattern = regexp.MustCompile(`(?m)^ARG HELM_VERSION=v(\d+)\.`)

// helmFlagsByMajor is what each helm major calls the two behaviors the executor
// depends on: rolling a failed upgrade back, and validating without applying.
var helmFlagsByMajor = map[int]struct{ rollback, dryRun string }{
	3: {rollback: "--atomic", dryRun: "--dry-run"},
	4: {rollback: "--rollback-on-failure", dryRun: "--dry-run=client"},
}

// pinnedHelmMajor reads the helm major the executor image ships.
func pinnedHelmMajor(t *testing.T) int {
	t.Helper()
	meshRoot := filepath.Dir(findChartDir(t))
	data, err := os.ReadFile(filepath.Join(meshRoot, "executor.Dockerfile"))
	if err != nil {
		t.Fatalf("read executor.Dockerfile: %v", err)
	}
	match := helmVersionPattern.FindSubmatch(data)
	if match == nil {
		t.Fatal("executor.Dockerfile pins no ARG HELM_VERSION=vN.…; the flag guard cannot tell which helm ships")
	}
	major, err := strconv.Atoi(string(match[1]))
	if err != nil {
		t.Fatalf("parse helm major from %q: %v", match[1], err)
	}
	return major
}

// TestExecutorHelmFlagsMatchTheShippedHelm proves the declared flags are the ones
// the pinned helm actually takes, in both directions: the right spelling present
// and the other major's absent.
func TestExecutorHelmFlagsMatchTheShippedHelm(t *testing.T) {
	major := pinnedHelmMajor(t)
	want, known := helmFlagsByMajor[major]
	if !known {
		t.Fatalf("executor.Dockerfile pins helm %d, whose flag spellings this guard does not know; "+
			"decide what it calls the self-rollback and the dry-run, and add it to helmFlagsByMajor", major)
	}

	var decls execDeclarations
	readIntakeYAML(t, filepath.Join(agentDir(t, "executor"), "exec-declarations.yaml"), &decls)
	args := map[string][]string{}
	for _, tool := range decls.Tools {
		args[tool.Name] = tool.Args
	}

	upgrade, ok := args["helm_upgrade"]
	if !ok {
		t.Fatal("the executor declares no helm_upgrade word")
	}
	if !containsString(upgrade, want.rollback) {
		t.Errorf("helm_upgrade does not pass %s, which helm %d calls the self-rollback; "+
			"apply-machine.yaml routes a failed apply straight to Failed because the upgrade rolls itself back, "+
			"so without it a failed apply leaves the release on the failed revision",
			want.rollback, major)
	}
	dryRun, ok := args["helm_dry_run"]
	if !ok {
		t.Fatal("the executor declares no helm_dry_run word")
	}
	if !containsString(dryRun, want.dryRun) {
		t.Errorf("helm_dry_run does not pass %s, which helm %d calls the validate-without-applying flag",
			want.dryRun, major)
	}

	// The other major's spellings must not appear: helm rejects an unknown flag
	// outright, so a half-migrated declaration fails every apply on the cluster
	// while the fake-CLI tracer stays green.
	for otherMajor, other := range helmFlagsByMajor {
		if otherMajor == major {
			continue
		}
		for word, declared := range map[string][]string{"helm_upgrade": upgrade, "helm_dry_run": dryRun} {
			for _, flag := range []string{other.rollback, other.dryRun} {
				if flag == want.rollback || flag == want.dryRun {
					continue // a spelling both majors share is not evidence of drift
				}
				if containsString(declared, flag) {
					t.Errorf("%s passes %s, which is helm %d's spelling, but the image ships helm %d",
						word, flag, otherMajor, major)
				}
			}
		}
	}
}

// TestExecutorHelmFlagGuardCoversTheDeclaredWords proves the guard reads the
// words it claims to. A guard that silently matched nothing would pass forever.
func TestExecutorHelmFlagGuardCoversTheDeclaredWords(t *testing.T) {
	var decls execDeclarations
	readIntakeYAML(t, filepath.Join(agentDir(t, "executor"), "exec-declarations.yaml"), &decls)
	var helmWords int
	for _, tool := range decls.Tools {
		if tool.Binary == "helm" {
			helmWords++
		}
	}
	if helmWords < 3 {
		t.Errorf("the executor declares %d helm words; the guard expects at least the dry-run, the upgrade, "+
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
