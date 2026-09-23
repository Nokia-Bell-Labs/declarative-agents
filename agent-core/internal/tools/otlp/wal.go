// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package otlp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// WAL states. A record is written pending before the first upload attempt and
// committed once the remote object is verified (srd008 R6.2, R7.2).
const (
	walStatePending   = "pending"
	walStateCommitted = "committed"
)

// walRecord is one line of the write-ahead log: the object identity and upload
// state a restart replays from (srd008 R2, R4).
type walRecord struct {
	Key           string    `json:"key"`
	BatchID       string    `json:"batch_id"`
	Checksum      string    `json:"checksum"`
	PayloadFormat string    `json:"payload_format"`
	Bytes         int64     `json:"bytes"`
	State         string    `json:"state"`
	StagePath     string    `json:"stage_path,omitempty"`
	At            time.Time `json:"at"`
}

// wal is a bounded append-only journal at a stable path. It records object
// identity and upload state before any remote attempt so bucket latency or
// outage leaves replayable evidence, and it bounds its own growth so a stalled
// upstream cannot consume the disk silently (srd008 R2, R6, R10).
type wal struct {
	path       string
	maxBytes   int64
	maxPending int
	mu         sync.Mutex
}

// ErrWALOverflow is the named, observable overflow the bound produces rather
// than silently dropping uncommitted evidence (srd008 R10).
var ErrWALOverflow = fmt.Errorf("write-ahead log overflow")

func newWAL(path string, maxBytes int64, maxPending int) *wal {
	return &wal{path: path, maxBytes: maxBytes, maxPending: maxPending}
}

// appendPending records one batch as pending and syncs the record to disk
// before the caller attempts the remote write (srd008 R6).
func (w *wal) appendPending(record walRecord) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	record.State = walStatePending
	if record.At.IsZero() {
		record.At = time.Now().UTC()
	}
	if err := w.enforceBounds(record); err != nil {
		return err
	}
	return w.appendLocked(record)
}

// markCommitted records that the remote object for key is verified, which lets
// a later compaction drop the pending record (srd008 R7, AC5).
func (w *wal) markCommitted(key, checksum string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.appendLocked(walRecord{
		Key: key, Checksum: checksum, State: walStateCommitted, At: time.Now().UTC(),
	})
}

// pending returns the records whose object has not been committed, in append
// order, so a restart replays exactly the pre-recorded keys (srd008 R4, AC3).
func (w *wal) pending() ([]walRecord, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	records, committed, err := w.readLocked()
	if err != nil {
		return nil, err
	}
	var out []walRecord
	for _, record := range records {
		if record.State != walStatePending {
			continue
		}
		if committed[record.Key] {
			continue
		}
		out = append(out, record)
	}
	return out, nil
}

// compact rewrites the log with only its still-pending records, so a committed
// record is removed only after its remote object was verified, and an
// interrupted compaction leaves the original file untouched (srd008 AC5).
func (w *wal) compact() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	records, committed, err := w.readLocked()
	if err != nil {
		return err
	}
	var keep []walRecord
	for _, record := range records {
		if record.State == walStatePending && !committed[record.Key] {
			keep = append(keep, record)
		}
	}
	return w.rewriteLocked(keep)
}

func (w *wal) enforceBounds(next walRecord) error {
	records, committed, err := w.readLocked()
	if err != nil {
		return err
	}
	pending := 0
	var bytes int64
	for _, record := range records {
		if record.State == walStatePending && !committed[record.Key] {
			pending++
			bytes += record.Bytes
		}
	}
	if w.maxPending > 0 && pending+1 > w.maxPending {
		return fmt.Errorf("%w: %d pending records exceeds max_pending %d", ErrWALOverflow, pending+1, w.maxPending)
	}
	if w.maxBytes > 0 && bytes+next.Bytes > w.maxBytes {
		return fmt.Errorf("%w: %d pending bytes exceeds max_bytes %d", ErrWALOverflow, bytes+next.Bytes, w.maxBytes)
	}
	return nil
}

func (w *wal) appendLocked(record walRecord) error {
	if err := os.MkdirAll(filepath.Dir(w.path), 0o755); err != nil {
		return fmt.Errorf("create wal directory: %w", err)
	}
	line, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode wal record: %w", err)
	}
	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open wal %s: %w", w.path, err)
	}
	_, writeErr := file.Write(append(line, '\n'))
	syncErr := file.Sync()
	closeErr := file.Close()
	switch {
	case writeErr != nil:
		return fmt.Errorf("append wal %s: %w", w.path, writeErr)
	case syncErr != nil:
		return fmt.Errorf("sync wal %s: %w", w.path, syncErr)
	case closeErr != nil:
		return fmt.Errorf("close wal %s: %w", w.path, closeErr)
	default:
		return nil
	}
}

func (w *wal) readLocked() ([]walRecord, map[string]bool, error) {
	file, err := os.Open(w.path)
	if os.IsNotExist(err) {
		return nil, map[string]bool{}, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("open wal %s: %w", w.path, err)
	}
	defer func() { _ = file.Close() }()
	var records []walRecord
	committed := map[string]bool{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var record walRecord
		if err := json.Unmarshal(line, &record); err != nil {
			return nil, nil, fmt.Errorf("decode wal %s: %w", w.path, err)
		}
		records = append(records, record)
		if record.State == walStateCommitted {
			committed[record.Key] = true
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("read wal %s: %w", w.path, err)
	}
	return records, committed, nil
}

func (w *wal) rewriteLocked(records []walRecord) error {
	if len(records) == 0 {
		if err := os.Remove(w.path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove wal %s: %w", w.path, err)
		}
		return nil
	}
	temp := w.path + ".compact"
	file, err := os.OpenFile(temp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open wal temp %s: %w", temp, err)
	}
	for _, record := range records {
		line, marshalErr := json.Marshal(record)
		if marshalErr != nil {
			_ = file.Close()
			return fmt.Errorf("encode wal record: %w", marshalErr)
		}
		if _, writeErr := file.Write(append(line, '\n')); writeErr != nil {
			_ = file.Close()
			return fmt.Errorf("write wal temp %s: %w", temp, writeErr)
		}
	}
	if syncErr := file.Sync(); syncErr != nil {
		_ = file.Close()
		return fmt.Errorf("sync wal temp %s: %w", temp, syncErr)
	}
	if closeErr := file.Close(); closeErr != nil {
		return fmt.Errorf("close wal temp %s: %w", temp, closeErr)
	}
	if err := os.Rename(temp, w.path); err != nil {
		return fmt.Errorf("commit wal compaction %s: %w", w.path, err)
	}
	return nil
}
