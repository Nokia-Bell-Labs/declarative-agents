// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package conformance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func purgeRequest(t *testing.T, confirmation string) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"parameters": map[string]string{"confirmation": confirmation},
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "request.json")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestApplicationPurgeRefusesThenDeletesOnlyRenderedPrefix(t *testing.T) {
	bucket := t.TempDir()
	for key, content := range map[string]string{
		"chatbot-mesh/traces/a.json":  "a",
		"chatbot-mesh/metrics/b.json": "b",
		"coding-agent/traces/a.json":  "peer",
	} {
		path := filepath.Join(bucket, filepath.FromSlash(key))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	audit := filepath.Join(t.TempDir(), "purge.ndjson")
	env := []string{
		"PURGE_APPLICATION=chatbot-mesh",
		"PURGE_BUCKET_URL=file://" + bucket,
		"PURGE_PREFIX=chatbot-mesh/",
		"PURGE_AUDIT_PATH=" + audit,
	}
	refused := Run(t, RunConfig{
		Profile:   "agents/application-purge/profile.yaml",
		Directory: t.TempDir(), Request: purgeRequest(t, "purge:coding-agent"), Env: env,
	})
	refused.RequireExit(t, 2)
	if state, _ := refused.TerminalOutcome(t); state != "Refused" {
		t.Fatalf("refused state = %s", state)
	}
	if _, err := os.Stat(filepath.Join(bucket, "chatbot-mesh", "traces", "a.json")); err != nil {
		t.Fatal("mismatched confirmation touched retained data")
	}

	purged := Run(t, RunConfig{
		Profile:   "agents/application-purge/profile.yaml",
		Directory: t.TempDir(), Request: purgeRequest(t, "purge:chatbot-mesh"), Env: env,
	})
	purged.RequireExit(t, 0)
	if state, _ := purged.TerminalOutcome(t); state != "Purged" {
		t.Fatalf("purged state = %s", state)
	}
	for _, key := range []string{"traces/a.json", "metrics/b.json"} {
		if _, err := os.Stat(filepath.Join(bucket, "chatbot-mesh", key)); !os.IsNotExist(err) {
			t.Fatalf("application object %s survived purge: %v", key, err)
		}
	}
	if data, err := os.ReadFile(filepath.Join(bucket, "coding-agent", "traces", "a.json")); err != nil || string(data) != "peer" {
		t.Fatalf("peer object changed: %q, %v", data, err)
	}
	if data, err := os.ReadFile(audit); err != nil || len(data) == 0 {
		t.Fatalf("external purge audit = %q, %v", data, err)
	}
}
