// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

func TestResolveDiagnoseTarget(t *testing.T) {
	cases := []struct {
		scenario string
		want     diagnoseTarget
	}{
		{"helm-smoke", diagnoseTarget{cluster: kindrig.PlatformClusterName, namespace: "da-helm-smoke"}},
		{"demo", diagnoseTarget{cluster: chatbotDemoCluster, namespace: "default"}},
	}
	for _, tc := range cases {
		got, err := resolveDiagnoseTarget(tc.scenario)
		if err != nil || got != tc.want {
			t.Errorf("resolveDiagnoseTarget(%q) = %+v, %v; want %+v", tc.scenario, got, err, tc.want)
		}
	}
	for _, bad := range []string{"", "Helm_Smoke", strings.Repeat("a", 62)} {
		if _, err := resolveDiagnoseTarget(bad); err == nil {
			t.Errorf("resolveDiagnoseTarget(%q) accepted an invalid scenario", bad)
		}
	}
}

func TestDiagnosePaths(t *testing.T) {
	now := time.Date(2026, 9, 19, 16, 30, 5, 0, time.UTC)
	dir := diagnoseEvidenceDirectory("/app", "helm-smoke", now)
	if want := filepath.Join("/app", "build", "kind-evidence", "diagnose-helm-smoke-20260919T163005Z"); dir != want {
		t.Errorf("evidence directory = %s, want %s", dir, want)
	}
	spool := diagnoseTraceSpool("/app")
	if want := filepath.Join("/app", "observability", ".run", "spool", "traces", "collector.ndjson"); spool != want {
		t.Errorf("trace spool = %s, want %s", spool, want)
	}
}
