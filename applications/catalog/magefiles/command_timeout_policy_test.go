// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/profileaudit"
)

func TestCatalogProfileTimeoutEnvelopes(t *testing.T) {
	root := repoRootFromTest(t)
	coreRoot, err := resolveAgentCoreRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	profiles, err := discoverAuditProfiles(root)
	if err != nil {
		t.Fatal(err)
	}
	coverage := map[string]bool{}
	for _, profile := range profiles {
		switch name := filepath.Base(profile); {
		case name == "profile.yaml":
			coverage["canonical"] = true
		case strings.HasPrefix(name, "profile-"):
			coverage["variant"] = true
		case strings.HasSuffix(name, "-profile.yaml"):
			coverage["compatibility alias"] = true
		}
		if err := profileaudit.ValidateWithOptions(
			profile, profileaudit.Options{CoreRoot: coreRoot},
		); err != nil {
			t.Errorf("%s: %v", profile, err)
		}
	}
	for _, class := range []string{"canonical", "variant", "compatibility alias"} {
		if !coverage[class] {
			t.Errorf("catalog audit inventory contains no %s profile", class)
		}
	}
	t.Logf("validated %d catalog profiles and fixtures", len(profiles))
}
