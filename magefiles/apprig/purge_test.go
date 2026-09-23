// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package apprig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunApplicationPurgeFixesAuthorityOutsideRequest(t *testing.T) {
	root := t.TempDir()
	script := filepath.Join(root, "purge-agent")
	body := `#!/bin/sh
set -eu
request=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--request" ]; then request="$2"; shift 2; continue; fi
  shift
done
test "$PURGE_APPLICATION" = "fixture"
test "$PURGE_BUCKET_URL" = "gs://fixture-telemetry"
test "$PURGE_ENDPOINT" = "http://fake-gcs/storage/v1/"
test "$PURGE_PREFIX" = "fixture/"
grep -q '"confirmation":"purge:fixture"' "$request"
mkdir -p "$(dirname "$PURGE_AUDIT_PATH")"
printf '%s\n' '{"status":"purged","application":"fixture","bucket_url":"gs://fixture-telemetry","prefix":"fixture/","matched":2,"deleted":2,"remaining":0,"keys_sha256":"abc","at":"2026-09-22T00:00:00Z"}' >> "$PURGE_AUDIT_PATH"
`
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	resolved := Resolved{
		Application: "fixture", BucketURL: "gs://fixture-telemetry",
		ObjectPrefix: "fixture", Workspace: filepath.Join(root, "workspace"),
		EvidenceDir: filepath.Join(root, "build", "kind-evidence"),
	}
	err := RunApplicationPurge(
		resolved, "purge:fixture",
		PurgeBinding{
			Endpoint:       "http://fake-gcs/storage/v1/",
			AuditDirectory: filepath.Join(root, "external-audit"),
		},
		PurgeAgent{Binary: script, Profile: "profile.yaml", CoreRoot: "core"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "external-audit", "fixture.ndjson")); err != nil {
		t.Fatalf("external audit absent: %v", err)
	}
}
