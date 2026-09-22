// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package otlp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/objectstore"
	"google.golang.org/protobuf/encoding/protojson"
)

func tracePayload(t *testing.T, service string, spans int) []byte {
	t.Helper()
	payload, err := protojson.Marshal(traceRequest(service, spans))
	if err != nil {
		t.Fatalf("marshal trace request: %v", err)
	}
	return payload
}

func newObjectStore(t *testing.T, connection objectstore.ConnectionConfig, walPath string) *objectSpoolStore {
	t.Helper()
	target, err := newSpoolTarget(SpoolConfig{}, StorageConfig{
		Backend: BackendObject, Connection: connection, WALPath: walPath,
		Application: "chatbot-mesh", Namespace: "da-chatbot-mesh", Run: "run-1",
		CollectorInstance: "collector-0",
	}, objectstore.NewOpener())
	if err != nil {
		t.Fatalf("build object store: %v", err)
	}
	store, ok := target.(*objectSpoolStore)
	if !ok {
		t.Fatalf("target is %T, want *objectSpoolStore", target)
	}
	return store
}

func bucketHas(t *testing.T, opener *objectstore.Opener, connection objectstore.ConnectionConfig, key string) bool {
	t.Helper()
	bucket, done, err := opener.Open(context.Background(), connection)
	if err != nil {
		t.Fatalf("open bucket: %v", err)
	}
	defer done()
	exists, err := bucket.Exists(context.Background(), key)
	if err != nil {
		t.Fatalf("bucket exists %q: %v", key, err)
	}
	return exists
}

// srd008 AC1: object persistence works on mem:// and file:// with no network,
// and the WAL is compacted after a verified commit.
func TestObjectSpoolPersistLocalBackends(t *testing.T) {
	for _, scheme := range []string{"mem", "file"} {
		t.Run(scheme, func(t *testing.T) {
			connection := localConnection(t, scheme)
			wal := filepath.Join(t.TempDir(), "collector.wal")
			store := newObjectStore(t, connection, wal)

			outcome, err := store.persist(context.Background(), spoolBatch{
				signal: "trace", payloadFormat: "otlp-protojson-trace",
				payload: tracePayload(t, "chatbot", 1), received: time.Now().UTC(),
			})
			if err != nil {
				t.Fatalf("persist: %v", err)
			}
			if outcome.walState != walStateCommitted || outcome.objectKey == "" {
				t.Fatalf("outcome = %+v, want committed with an object key", outcome)
			}
			if !bucketHas(t, store.opener, connection, outcome.objectKey) {
				t.Fatalf("object %q absent after persist", outcome.objectKey)
			}
			pending, err := store.wal.pending()
			if err != nil || len(pending) != 0 {
				t.Fatalf("pending = %v (%v), want empty after commit", pending, err)
			}
		})
	}
}

func localConnection(t *testing.T, scheme string) objectstore.ConnectionConfig {
	t.Helper()
	switch scheme {
	case "mem":
		return objectstore.ConnectionConfig{BucketURL: "mem://" + t.Name()}
	case "file":
		return objectstore.ConnectionConfig{BucketURL: "file://" + t.TempDir()}
	default:
		t.Fatalf("no local connection for %q", scheme)
		return objectstore.ConnectionConfig{}
	}
}

// srd008 R7, AC3: an identical re-persist is idempotent (one object, no error),
// and a key that already holds different bytes is refused, never overwritten.
func TestObjectPutIdempotenceAndIntegrity(t *testing.T) {
	connection := objectstore.ConnectionConfig{BucketURL: "mem://" + t.Name()}
	store := newObjectStore(t, connection, filepath.Join(t.TempDir(), "c.wal"))
	payload := tracePayload(t, "chatbot", 1)

	first, err := store.persist(context.Background(), spoolBatch{signal: "trace", payload: payload})
	if err != nil {
		t.Fatalf("first persist: %v", err)
	}
	second, err := store.persist(context.Background(), spoolBatch{signal: "trace", payload: payload})
	if err != nil {
		t.Fatalf("idempotent re-persist: %v", err)
	}
	if first.objectKey != second.objectKey {
		t.Fatalf("re-persist changed the key: %q -> %q", first.objectKey, second.objectKey)
	}

	err = store.putObject(context.Background(), first.objectKey, []byte("different bytes"), "deadbeef")
	if err == nil {
		t.Fatal("a different payload overwrote an existing key")
	}
}

// srd008 R2, R6, AC3, AC5: a batch staged and recorded pending before its
// upload is replayed after a restart under the exact recorded key, producing no
// duplicate, and the WAL is compacted once the object is verified.
func TestObjectSpoolResumesPendingAfterRestart(t *testing.T) {
	// A real, on-disk bucket survives the simulated restart; an in-memory
	// bucket would vanish with the process it models.
	connection := objectstore.ConnectionConfig{BucketURL: "file://" + t.TempDir()}
	wal := filepath.Join(t.TempDir(), "collector.wal")

	crashed := newObjectStore(t, connection, wal)
	envelope := newEnvelope(EnvelopeMeta{
		Signal: "trace", Application: "chatbot-mesh", PayloadFormat: "otlp-protojson-trace",
		ReceivedAt: time.Now().UTC(),
	}, tracePayload(t, "chatbot", 1))
	key := envelope.objectKey("")
	data, err := envelope.encode()
	if err != nil {
		t.Fatalf("encode envelope: %v", err)
	}
	stage, err := crashed.stage(envelope.BatchID, data)
	if err != nil {
		t.Fatalf("stage: %v", err)
	}
	if err := crashed.wal.appendPending(walRecord{
		Key: key, BatchID: envelope.BatchID, Checksum: envelope.PayloadChecksum,
		Bytes: int64(len(data)), StagePath: stage,
	}); err != nil {
		t.Fatalf("append pending: %v", err)
	}
	if bucketHas(t, crashed.opener, connection, key) {
		t.Fatal("object present before the upload; the WAL is not upload-first")
	}

	resumed := newObjectStore(t, connection, wal)
	count, err := resumed.resumePending(context.Background())
	if err != nil || count != 1 {
		t.Fatalf("resume = %d (%v), want 1", count, err)
	}
	if !bucketHas(t, resumed.opener, connection, key) {
		t.Fatalf("object %q absent after resume", key)
	}
	pending, err := resumed.wal.pending()
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending = %v (%v), want empty after resume", pending, err)
	}
	if _, err := os.Stat(stage); !os.IsNotExist(err) {
		t.Fatalf("staged file survived commit: %v", err)
	}
}

// srd008 R4, AC4: two collector lifetimes whose receiver sequence both restart
// at one still yield distinct object identities for distinct batches, because
// identity is content-derived, not sequence-derived.
func TestObjectIdentityIsContentDerived(t *testing.T) {
	connection := objectstore.ConnectionConfig{BucketURL: "mem://" + t.Name()}
	store := newObjectStore(t, connection, filepath.Join(t.TempDir(), "c.wal"))

	first, err := store.persist(context.Background(), spoolBatch{signal: "trace", payload: tracePayload(t, "chatbot", 1)})
	if err != nil {
		t.Fatalf("persist one: %v", err)
	}
	second, err := store.persist(context.Background(), spoolBatch{signal: "trace", payload: tracePayload(t, "handler", 2)})
	if err != nil {
		t.Fatalf("persist two: %v", err)
	}
	if first.objectKey == second.objectKey {
		t.Fatalf("distinct batches shared object key %q", first.objectKey)
	}
}

// srd008 R5: traces and metrics persist as separate object families.
func TestTraceAndMetricObjectsAreSeparate(t *testing.T) {
	connection := objectstore.ConnectionConfig{BucketURL: "mem://" + t.Name()}
	store := newObjectStore(t, connection, filepath.Join(t.TempDir(), "c.wal"))

	trace, err := store.persist(context.Background(), spoolBatch{signal: "trace", payload: tracePayload(t, "chatbot", 1)})
	if err != nil {
		t.Fatalf("persist trace: %v", err)
	}
	metric, err := store.persist(context.Background(), spoolBatch{signal: "metric", payload: []byte(`{"resourceMetrics":[]}`)})
	if err != nil {
		t.Fatalf("persist metric: %v", err)
	}
	if filepath.Dir(trace.objectKey) == filepath.Dir(metric.objectKey) {
		t.Fatalf("trace and metric share a family: %q vs %q", trace.objectKey, metric.objectKey)
	}
}

// srd008 R10: the WAL bound is a named, observable overflow, not a silent drop
// of uncommitted evidence.
func TestWALPendingBoundOverflows(t *testing.T) {
	wal := newWAL(filepath.Join(t.TempDir(), "bounded.wal"), 0, 1)
	if err := wal.appendPending(walRecord{Key: "k1", Bytes: 1}); err != nil {
		t.Fatalf("first pending: %v", err)
	}
	err := wal.appendPending(walRecord{Key: "k2", Bytes: 1})
	if err == nil {
		t.Fatal("second pending record exceeded max_pending without an error")
	}
}
