// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package kindrig

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var platformLive = flag.Bool("kindrig.platform-live", false,
	"boot a real da-platform kind cluster (needs docker, kind, kubectl)")

// TestPlatformLifecycleLive is the tier's live evidence (GH-2215). Opt in with
// go test ./kindrig -run TestPlatformLifecycleLive -args -kindrig.platform-live.
// It leaves an interrupted platform behind, proves the next start replaces it
// and passes conformance, then proves an injected conformance failure still
// captures evidence and deletes the cluster.
func TestPlatformLifecycleLive(t *testing.T) {
	if !*platformLive {
		t.Skip("live platform test needs -kindrig.platform-live")
	}
	interrupted, err := StartPlatform(PlatformOptions{})
	if err != nil {
		t.Fatalf("first start: %v", err)
	}
	interrupted.unbind()
	if !Exists(DefaultRun, PlatformClusterName) {
		t.Fatal("interrupted platform is not listed")
	}

	platform, err := StartPlatform(PlatformOptions{})
	if err != nil {
		t.Fatalf("start over leftover: %v", err)
	}
	if !platform.Cluster.Created {
		t.Fatal("start over leftover adopted it instead of recreating it")
	}
	platform.Stop(false)
	if Exists(DefaultRun, PlatformClusterName) {
		t.Fatal("platform survived a successful stop")
	}

	evidence := filepath.Join(t.TempDir(), "evidence")
	injected := errors.New("injected conformance failure")
	_, err = StartPlatform(PlatformOptions{
		EvidenceDirectory: evidence,
		conformance:       func(CommandRunner, string) error { return injected },
	})
	if !errors.Is(err, injected) || !strings.Contains(err.Error(), "platform-conformance") {
		t.Fatalf("injected start error = %v", err)
	}
	if Exists(DefaultRun, PlatformClusterName) {
		t.Fatal("platform survived an injected conformance failure")
	}
	entries, err := os.ReadDir(evidence)
	if err != nil || len(entries) == 0 {
		t.Fatalf("failure evidence missing: %v %v", entries, err)
	}
}
