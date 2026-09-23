// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package objectstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func TestDeletePrefixUsesOnlyDeclaredAuthority(t *testing.T) {
	for _, scheme := range []string{"mem", "file"} {
		t.Run(scheme, func(t *testing.T) {
			opener := NewOpener()
			connection := connectionFor(t, scheme)
			for _, key := range []string{"app/traces/a.json", "app/metrics/b.json", "peer/traces/a.json"} {
				executeWrite(t, opener, connection, key, key)
			}
			audit := filepath.Join(t.TempDir(), "purge.ndjson")
			builder := &DeletePrefixBuilder{Opener: opener, Config: DeletePrefixConfig{
				Connection: connection, Prefix: "app/", MaxObjects: 10,
				Application: "app", AuditPath: audit,
			}}
			// A malicious request prefix is ignored: the builder has no request
			// authority and deletes only its rendered app/ prefix.
			result := builder.Build(paramsResult(map[string]string{"prefix": "peer/"})).Execute()
			if result.Signal != core.ToolDone {
				t.Fatalf("delete prefix: %v %s", result.Signal, result.Output)
			}
			var outcome deletePrefixOutcome
			if err := json.Unmarshal([]byte(result.Output), &outcome); err != nil {
				t.Fatal(err)
			}
			if outcome.Status != "purged" || outcome.Matched != 2 ||
				outcome.Deleted != 2 || outcome.Remaining != 0 {
				t.Fatalf("outcome = %+v", outcome)
			}
			if read := executeRead(t, opener, connection, "peer/traces/a.json"); read.Signal != core.ToolDone {
				t.Fatal("declared app prefix deletion crossed into peer prefix")
			}
			if read := executeRead(t, opener, connection, "app/traces/a.json"); read.Signal == core.ToolDone {
				t.Fatal("target object survived successful purge")
			}
			if data, err := os.ReadFile(audit); err != nil || len(data) == 0 {
				t.Fatalf("audit = %q, %v", data, err)
			}
		})
	}
}

func TestDeletePrefixRefusesOverBudgetWithoutMutation(t *testing.T) {
	opener := NewOpener()
	connection := connectionFor(t, "mem")
	executeWrite(t, opener, connection, "app/a", "a")
	executeWrite(t, opener, connection, "app/b", "b")
	builder := &DeletePrefixBuilder{Opener: opener, Config: DeletePrefixConfig{
		Connection: connection, Prefix: "app/", MaxObjects: 1,
		Application: "app", AuditPath: filepath.Join(t.TempDir(), "audit.ndjson"),
	}}
	result := builder.Build(core.Result{}).Execute()
	var outcome deletePrefixOutcome
	if result.Signal != core.ToolDone || json.Unmarshal([]byte(result.Output), &outcome) != nil {
		t.Fatalf("result = %+v", result)
	}
	if outcome.Status != "refused" || outcome.Deleted != 0 || outcome.Remaining != 2 {
		t.Fatalf("outcome = %+v", outcome)
	}
	for _, key := range []string{"app/a", "app/b"} {
		if read := executeRead(t, opener, connection, key); read.Signal != core.ToolDone {
			t.Fatalf("refused purge changed %s", key)
		}
	}
}
