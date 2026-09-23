// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package otlp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/objectstore"
	"gocloud.dev/gcerrors"
)

// Storage backends a spool word may target. Filesystem is the default and
// keeps the existing NDJSON spool byte-for-byte, so spool-only mode is
// preserved; object persists one immutable envelope per batch through a
// write-ahead log (srd008 R1, R9; srd042 durable-object persistence).
const (
	BackendFilesystem = "filesystem"
	BackendObject     = "object"
)

// StorageConfig selects and configures a spool word's storage backend. The
// backend, bucket URL, endpoint, and in-bucket prefix are declared
// configuration only; cloud credentials remain ambient workload identity
// (srd008 R8).
type StorageConfig struct {
	Backend           string
	Connection        objectstore.ConnectionConfig
	Prefix            string
	WALPath           string
	StageDir          string
	Application       string
	Namespace         string
	Run               string
	CollectorInstance string
	RetentionClass    string
	RetentionDays     int
	WALMaxBytes       int64
	WALMaxPending     int
}

// ErrObjectIntegrity is the named refusal when a key already holds different
// bytes: object creation is idempotent for identical content and never an
// overwrite (srd008 R7).
var ErrObjectIntegrity = fmt.Errorf("object integrity conflict")

// spoolBatch is what one spool command hands its storage target: both the
// NDJSON evidence the filesystem backend appends and the immutable protojson
// payload the object backend seals into an envelope.
type spoolBatch struct {
	signal        string
	payloadFormat string
	ndjson        []byte
	payload       []byte
	received      time.Time
}

// spoolOutcome is the storage-independent result a spool command reports.
type spoolOutcome struct {
	path      string
	bytes     int
	objectKey string
	checksum  string
	walState  string
}

// spoolTarget is the storage port: one operation, persist one batch's durable
// evidence, with a filesystem and an object-bucket implementation (srd008 R1).
type spoolTarget interface {
	persist(ctx context.Context, batch spoolBatch) (spoolOutcome, error)
}

// newSpoolTarget selects the backend from declared configuration. An unknown
// backend fails at construction with the fault named, not at first use.
func newSpoolTarget(spool SpoolConfig, storage StorageConfig, opener *objectstore.Opener) (spoolTarget, error) {
	switch storage.Backend {
	case "", BackendFilesystem:
		return &fileSpoolStore{config: spool}, nil
	case BackendObject:
		if opener == nil {
			opener = objectstore.NewOpener()
		}
		return &objectSpoolStore{
			opener:     opener,
			connection: storage.Connection,
			prefix:     storage.Prefix,
			stageDir:   storage.StageDir,
			meta: EnvelopeMeta{
				Application: storage.Application, Namespace: storage.Namespace,
				Run: storage.Run, CollectorInstance: storage.CollectorInstance,
				RetentionClass: storage.RetentionClass, RetentionDays: storage.RetentionDays,
			},
			wal: newWAL(storage.WALPath, storage.WALMaxBytes, storage.WALMaxPending),
		}, nil
	default:
		return nil, fmt.Errorf("unknown storage backend %q (supported: filesystem, object)", storage.Backend)
	}
}

// fileSpoolStore is the filesystem backend: the existing bounded NDJSON append,
// unchanged, so a declaration that names no object backend behaves exactly as
// before (srd008 R9).
type fileSpoolStore struct {
	config SpoolConfig
}

func (s *fileSpoolStore) persist(_ context.Context, batch spoolBatch) (spoolOutcome, error) {
	written, err := appendSpool(s.config, batch.ndjson)
	if err != nil {
		return spoolOutcome{}, err
	}
	return spoolOutcome{path: s.config.Path, bytes: written}, nil
}

// objectSpoolStore is the object backend: it stages the envelope bytes locally
// and records the object identity in the WAL before any remote attempt, then
// creates the immutable object idempotently and compacts the WAL only after the
// remote write is verified (srd008 R2, R6, R7, AC5).
type objectSpoolStore struct {
	opener     *objectstore.Opener
	connection objectstore.ConnectionConfig
	prefix     string
	stageDir   string
	meta       EnvelopeMeta
	wal        *wal
}

func (s *objectSpoolStore) persist(ctx context.Context, batch spoolBatch) (spoolOutcome, error) {
	meta := s.meta
	meta.Signal = batch.signal
	meta.PayloadFormat = batch.payloadFormat
	meta.ReceivedAt = batch.received
	envelope := newEnvelope(meta, batch.payload)
	key := envelope.objectKey(s.prefix)
	data, err := envelope.encode()
	if err != nil {
		return spoolOutcome{}, err
	}
	stage, err := s.stage(envelope.BatchID, data)
	if err != nil {
		return spoolOutcome{}, err
	}
	record := walRecord{
		Key: key, BatchID: envelope.BatchID, Checksum: envelope.PayloadChecksum,
		PayloadFormat: envelope.PayloadFormat, Bytes: int64(len(data)), StagePath: stage,
	}
	if err := s.wal.appendPending(record); err != nil {
		return spoolOutcome{}, err
	}
	// Replay the whole journal, including the record just appended, rather
	// than uploading only the current batch. A collector process constructs a
	// fresh target for each spool command, so the first batch after a restart
	// drains evidence left pending by the prior process under its pre-recorded
	// immutable key before reporting the current batch committed (srd008 R6.2;
	// GH-2491 AC3).
	if _, err := s.resumePending(ctx); err != nil {
		return spoolOutcome{}, err
	}
	return spoolOutcome{
		path: key, bytes: len(data), objectKey: key,
		checksum: envelope.PayloadChecksum, walState: walStateCommitted,
	}, nil
}

// putObject creates the object when absent, treats an identical existing object
// as success, and refuses a key that already holds different bytes (srd008 R7).
func (s *objectSpoolStore) putObject(ctx context.Context, key string, data []byte, checksum string) error {
	bucket, done, err := s.opener.Open(ctx, s.connection)
	if err != nil {
		return err
	}
	defer done()
	existing, readErr := bucket.ReadAll(ctx, key)
	switch {
	case gcerrors.Code(readErr) == gcerrors.NotFound:
		if err := bucket.WriteAll(ctx, key, data, nil); err != nil {
			return fmt.Errorf("write object %q: %w", key, err)
		}
		return nil
	case readErr != nil:
		return fmt.Errorf("read existing object %q: %w", key, readErr)
	default:
		if prior, decodeErr := decodeEnvelope(existing); decodeErr == nil && prior.PayloadChecksum == checksum {
			return nil
		}
		return fmt.Errorf("%w: key %q already holds different content", ErrObjectIntegrity, key)
	}
}

func (s *objectSpoolStore) commit(key, checksum, stage string) error {
	if err := s.wal.markCommitted(key, checksum); err != nil {
		return err
	}
	if stage != "" {
		if err := os.Remove(stage); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove staged object %q: %w", stage, err)
		}
	}
	return s.wal.compact()
}

// resumePending replays every pending WAL record after a restart: it re-puts
// the staged bytes under the exact pre-recorded key, which is idempotent, so no
// duplicate logical batch is created (srd008 R4, AC3).
func (s *objectSpoolStore) resumePending(ctx context.Context) (int, error) {
	pending, err := s.wal.pending()
	if err != nil {
		return 0, err
	}
	resumed := 0
	seen := make(map[string]bool, len(pending))
	for _, record := range pending {
		// Repeated delivery of the same content while the bucket is unavailable
		// can append the same content-derived key more than once. Its staged path
		// is also content-derived; commit removes it. Replay that identity once
		// so a duplicate WAL line cannot turn a successful idempotent upload
		// into a missing-stage error.
		if seen[record.Key] {
			continue
		}
		seen[record.Key] = true
		data, readErr := os.ReadFile(record.StagePath)
		if readErr != nil {
			return resumed, fmt.Errorf("read staged object %q: %w", record.StagePath, readErr)
		}
		if err := s.putObject(ctx, record.Key, data, record.Checksum); err != nil {
			return resumed, err
		}
		if err := s.commit(record.Key, record.Checksum, record.StagePath); err != nil {
			return resumed, err
		}
		resumed++
	}
	return resumed, nil
}

func (s *objectSpoolStore) stage(batchID string, data []byte) (string, error) {
	dir := s.stageDir
	if dir == "" {
		dir = filepath.Dir(s.wal.path)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create stage directory: %w", err)
	}
	path := filepath.Join(dir, batchID+".json")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return "", fmt.Errorf("open staged object %q: %w", path, err)
	}
	_, writeErr := file.Write(data)
	syncErr := file.Sync()
	closeErr := file.Close()
	switch {
	case writeErr != nil:
		return "", fmt.Errorf("write staged object %q: %w", path, writeErr)
	case syncErr != nil:
		return "", fmt.Errorf("sync staged object %q: %w", path, syncErr)
	case closeErr != nil:
		return "", fmt.Errorf("close staged object %q: %w", path, closeErr)
	default:
		return path, nil
	}
}
