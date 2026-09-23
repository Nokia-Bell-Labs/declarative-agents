// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package otlp

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/objectstore"
)

// persistTrace writes one immutable trace envelope to an object store and
// returns the object key, so a query test can assert it is read back.
func persistTrace(t *testing.T, store *objectSpoolStore, service string, spans int) string {
	t.Helper()
	outcome, err := store.persist(context.Background(), spoolBatch{
		signal: "trace", payloadFormat: "otlp-protojson-trace",
		payload: tracePayload(t, service, spans), received: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("persist trace: %v", err)
	}
	return outcome.objectKey
}

func listTraces(t *testing.T, cfg QueryListConfig) map[string]any {
	t.Helper()
	result := (ListTracesBuilder{ToolName: "query_list_traces", Config: cfg}).
		Build(core.Result{}).Execute()
	if result.Signal != core.Signal("TracesListed") {
		t.Fatalf("list traces: %v %s", result.Signal, result.Output)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(result.Output), &out); err != nil {
		t.Fatalf("decode list output: %v", err)
	}
	return out
}

// srd042 R4, AC1: a query over an application bucket returns telemetry a prior
// collector lifetime wrote, and object storage reports complete.
func TestObjectQueryReadsBucketWrittenByPredecessor(t *testing.T) {
	connection := objectstore.ConnectionConfig{BucketURL: "file://" + t.TempDir()}
	writer := newObjectStore(t, connection, filepath.Join(t.TempDir(), "w.wal"))
	persistTrace(t, writer, "chatbot", 2)
	persistTrace(t, writer, "handler", 1)

	// A fresh reader (new opener, no WAL) reads only the bucket.
	out := listTraces(t, QueryListConfig{
		Storage: QueryStorageConfig{
			Backend: BackendObject, Connection: connection, Opener: objectstore.NewOpener(),
		},
	})
	if status, _ := out["storage_status"].(string); status != storageComplete {
		t.Fatalf("storage_status = %v, want complete", out["storage_status"])
	}
	// The shared helper stamps one trace id, so both persisted batches
	// aggregate into one trace; reading both objects yields its 3 spans.
	traces, _ := out["traces"].([]any)
	if len(traces) != 1 {
		t.Fatalf("traces = %d, want 1 aggregated trace read from the bucket", len(traces))
	}
	first, _ := traces[0].(map[string]any)
	if count, _ := first["span_count"].(float64); int(count) != 3 {
		t.Fatalf("span_count = %v, want 3 spans merged from both objects", first["span_count"])
	}
}

// srd042 R1, AC2: no query request field can select another bucket. The
// builder's request seed carries a bucket_url that must be ignored.
func TestObjectQueryIgnoresRequestBucket(t *testing.T) {
	own := objectstore.ConnectionConfig{BucketURL: "file://" + t.TempDir()}
	other := objectstore.ConnectionConfig{BucketURL: "file://" + t.TempDir()}
	persistTrace(t, newObjectStore(t, own, filepath.Join(t.TempDir(), "own.wal")), "mine", 1)
	persistTrace(t, newObjectStore(t, other, filepath.Join(t.TempDir(), "other.wal")), "theirs", 3)

	cfg := QueryListConfig{Storage: QueryStorageConfig{
		Backend: BackendObject, Connection: own, Opener: objectstore.NewOpener(),
	}}
	seed := core.Result{Output: `{"parameters":{"bucket_url":"` + other.BucketURL + `","page_size":50}}`}
	result := (ListTracesBuilder{ToolName: "query_list_traces", Config: cfg}).Build(seed).Execute()
	var out map[string]any
	_ = json.Unmarshal([]byte(result.Output), &out)
	if total, _ := out["total"].(float64); int(total) != 1 {
		t.Fatalf("total = %v, want only the configured bucket's 1 trace", out["total"])
	}
}

// srd042 R4: a batch still pending in the WAL merges with committed objects
// without a duplicate, because the object key matches the staged record.
func TestObjectQueryMergesPendingWALWithoutDuplicate(t *testing.T) {
	connection := objectstore.ConnectionConfig{BucketURL: "file://" + t.TempDir()}
	wal := filepath.Join(t.TempDir(), "c.wal")
	store := newObjectStore(t, connection, wal)
	key := persistTrace(t, store, "chatbot", 1)

	// Re-record the same committed batch as pending in the WAL (a commit that
	// did not compact): the query must not double-count it.
	envelope := newEnvelope(EnvelopeMeta{Signal: "trace", ReceivedAt: time.Now().UTC()}, tracePayload(t, "chatbot", 1))
	data, _ := envelope.encode()
	stage, _ := store.stage(envelope.BatchID, data)
	if err := store.wal.appendPending(walRecord{Key: key, BatchID: envelope.BatchID, Checksum: envelope.PayloadChecksum, StagePath: stage}); err != nil {
		t.Fatalf("append pending: %v", err)
	}
	out := listTraces(t, QueryListConfig{Storage: QueryStorageConfig{
		Backend: BackendObject, Connection: connection, WALPath: wal, Opener: store.opener,
	}})
	if total, _ := out["total"].(float64); int(total) != 1 {
		t.Fatalf("total = %v, want 1 (committed and pending are the same batch)", out["total"])
	}
}

// srd042 R3, AC4: an object budget smaller than the object count truncates the
// read and reports partial rather than complete.
func TestObjectQueryBudgetReportsPartial(t *testing.T) {
	connection := objectstore.ConnectionConfig{BucketURL: "file://" + t.TempDir()}
	store := newObjectStore(t, connection, filepath.Join(t.TempDir(), "c.wal"))
	persistTrace(t, store, "a", 1)
	persistTrace(t, store, "b", 2)
	persistTrace(t, store, "c", 3)

	out := listTraces(t, QueryListConfig{PageSize: 50, Storage: QueryStorageConfig{
		Backend: BackendObject, Connection: connection, Opener: store.opener, MaxObjects: 2,
	}})
	if status, _ := out["storage_status"].(string); status != storagePartial {
		t.Fatalf("storage_status = %v, want partial under an object budget", out["storage_status"])
	}
}

// srd042 R6, AC5: a bucket that cannot be opened is unavailable, and WAL-only
// data is never labeled complete.
func TestObjectQueryBucketOutageIsUnavailable(t *testing.T) {
	out := listTraces(t, QueryListConfig{Storage: QueryStorageConfig{
		Backend:    BackendObject,
		Connection: objectstore.ConnectionConfig{BucketURL: "file:///proc/nonexistent-agent-core-bucket/x"},
		Opener:     objectstore.NewOpener(), TimeoutMS: 2000,
	}})
	if status, _ := out["storage_status"].(string); status != storageUnavailable {
		t.Fatalf("storage_status = %v, want unavailable on outage", out["storage_status"])
	}
}
