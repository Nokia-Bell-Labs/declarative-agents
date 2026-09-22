// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package otlp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/objectstore"
	colmetricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"gocloud.dev/blob"
	"google.golang.org/protobuf/encoding/protojson"
)

// Storage status values a query reports so a caller never mistakes a partial
// or outage-truncated read for the whole history (srd042 R6; srd008 R7.2).
const (
	storageComplete    = "complete"
	storagePartial     = "partial"
	storageUnavailable = "unavailable"

	defaultQueryMaxObjects = 500
	defaultQueryMaxBytes   = int64(64 * 1024 * 1024)
	defaultQueryTimeoutMS  = 10000
)

// QueryStorageConfig is the declared, read-only storage backend a query word
// reads from. The backend, bucket URL, endpoint, and prefix are configuration
// only; no request field can redirect a query to another bucket (srd042 R1).
type QueryStorageConfig struct {
	Backend    string
	Connection objectstore.ConnectionConfig
	Prefix     string
	WALPath    string
	MaxObjects int
	MaxBytes   int64
	TimeoutMS  int
	Opener     *objectstore.Opener
}

func (q QueryStorageConfig) isObject() bool { return q.Backend == BackendObject }

func (q QueryStorageConfig) deadline() time.Duration {
	if q.TimeoutMS <= 0 {
		return time.Duration(defaultQueryTimeoutMS) * time.Millisecond
	}
	return time.Duration(q.TimeoutMS) * time.Millisecond
}

// loadTraceSpans returns the spans a trace query aggregates. The filesystem
// backend reads the NDJSON spool unchanged; the object backend reads the
// application bucket, merges pending WAL records, and reports storage status.
func (q QueryStorageConfig) loadTraceSpans(fsPath string) ([]spoolSpan, int, string, error) {
	if !q.isObject() {
		spans, skipped, err := readSpoolFiles(fsPath)
		if err != nil {
			return nil, 0, storageUnavailable, err
		}
		return spans, skipped, storageComplete, nil
	}
	payloads, status := q.gather("trace")
	var spans []spoolSpan
	skipped := 0
	for _, payload := range payloads {
		decoded, err := envelopeToTraceSpans(payload)
		if err != nil {
			skipped++
			continue
		}
		spans = append(spans, decoded...)
	}
	return spans, skipped, status, nil
}

// loadMetricRecords is the metric-signal parallel of loadTraceSpans.
func (q QueryStorageConfig) loadMetricRecords(fsPath string) ([]metricRecord, int, string, error) {
	if !q.isObject() {
		records, skipped, err := readMetricSpoolFiles(fsPath)
		if err != nil {
			return nil, 0, storageUnavailable, err
		}
		return records, skipped, storageComplete, nil
	}
	payloads, status := q.gather("metric")
	var records []metricRecord
	skipped := 0
	for _, payload := range payloads {
		decoded, err := envelopeToMetricRecords(payload)
		if err != nil {
			skipped++
			continue
		}
		records = append(records, decoded...)
	}
	return records, skipped, status, nil
}

// gather reads committed objects for the signal within budget and merges
// pending WAL staged envelopes, deduped by stable batch identity, returning the
// merged envelope bytes and the honest storage status (srd042 R3, R4, R6).
func (q QueryStorageConfig) gather(signal string) ([][]byte, string) {
	ctx, cancel := context.WithTimeout(context.Background(), q.deadline())
	defer cancel()
	objects, seen, truncated, listErr := q.readObjects(ctx, signal)
	staged := q.stagedPayloads(seen)
	status := storageComplete
	switch {
	case listErr != nil:
		status = storageUnavailable
	case truncated:
		status = storagePartial
	}
	return append(objects, staged...), status
}

type keyedSize struct {
	key  string
	size int64
}

// readObjects lists the signal's object keys, reads newest-first within the
// object-count and byte budget, and reports whether the listing was truncated.
// A bucket that cannot be opened, listed, or read is an outage, not a fault:
// the caller labels the response unavailable rather than complete.
func (q QueryStorageConfig) readObjects(ctx context.Context, signal string) ([][]byte, map[string]bool, bool, error) {
	seen := map[string]bool{}
	bucket, done, err := q.Opener.Open(ctx, q.Connection)
	if err != nil {
		return nil, seen, false, err
	}
	defer done()
	prefix := q.signalPrefix(signal)
	listed, listErr := listKeys(ctx, bucket, prefix)
	if listErr != nil {
		return nil, seen, false, listErr
	}
	sort.Slice(listed, func(i, j int) bool { return listed[i].key > listed[j].key })
	maxObjects := q.MaxObjects
	if maxObjects <= 0 {
		maxObjects = defaultQueryMaxObjects
	}
	maxBytes := q.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultQueryMaxBytes
	}
	var payloads [][]byte
	var bytes int64
	for index, item := range listed {
		if len(payloads) >= maxObjects || bytes+item.size > maxBytes {
			return payloads, seen, index < len(listed), nil
		}
		data, readErr := bucket.ReadAll(ctx, item.key)
		if readErr != nil {
			return payloads, seen, false, readErr
		}
		payloads = append(payloads, data)
		seen[item.key] = true
		bytes += item.size
	}
	return payloads, seen, false, nil
}

func listKeys(ctx context.Context, bucket *blob.Bucket, prefix string) ([]keyedSize, error) {
	iterator := bucket.List(&blob.ListOptions{Prefix: prefix})
	var listed []keyedSize
	for {
		object, err := iterator.Next(ctx)
		if err == io.EOF {
			return listed, nil
		}
		if err != nil {
			return nil, err
		}
		if object.IsDir {
			continue
		}
		listed = append(listed, keyedSize{key: object.Key, size: object.Size})
	}
}

// stagedPayloads reads the pending WAL's staged envelopes whose object key has
// not already been read from the bucket, so a batch committed but not yet
// compacted is not counted twice (srd042 R4).
func (q QueryStorageConfig) stagedPayloads(seen map[string]bool) [][]byte {
	if q.WALPath == "" {
		return nil
	}
	pending, err := newWAL(q.WALPath, 0, 0).pending()
	if err != nil {
		return nil
	}
	var payloads [][]byte
	for _, record := range pending {
		if seen[record.Key] || record.StagePath == "" {
			continue
		}
		data, readErr := os.ReadFile(record.StagePath)
		if readErr != nil {
			continue
		}
		payloads = append(payloads, data)
	}
	return payloads
}

func (q QueryStorageConfig) signalPrefix(signal string) string {
	prefix := trimSlashes(q.Prefix)
	if prefix == "" {
		return signal + "/"
	}
	return prefix + "/" + signal + "/"
}

func envelopeToTraceSpans(data []byte) ([]spoolSpan, error) {
	envelope, err := decodeEnvelope(data)
	if err != nil {
		return nil, err
	}
	var request coltracepb.ExportTraceServiceRequest
	if err := protojson.Unmarshal(envelope.Payload, &request); err != nil {
		return nil, fmt.Errorf("decode trace payload %s: %w", envelope.BatchID, err)
	}
	lines, err := encodeStdoutTrace(&request)
	if err != nil {
		return nil, err
	}
	return decodeSpoolSpanLines(lines), nil
}

func envelopeToMetricRecords(data []byte) ([]metricRecord, error) {
	envelope, err := decodeEnvelope(data)
	if err != nil {
		return nil, err
	}
	var request colmetricpb.ExportMetricsServiceRequest
	if err := protojson.Unmarshal(envelope.Payload, &request); err != nil {
		return nil, fmt.Errorf("decode metric payload %s: %w", envelope.BatchID, err)
	}
	lines, err := encodeMetricRecords(&request)
	if err != nil {
		return nil, err
	}
	return decodeMetricRecordLines(lines), nil
}

func decodeSpoolSpanLines(lines []byte) []spoolSpan {
	var spans []spoolSpan
	for _, line := range strings.Split(string(lines), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var span spoolSpan
		if json.Unmarshal([]byte(line), &span) != nil {
			continue
		}
		spans = append(spans, span)
	}
	return spans
}

func decodeMetricRecordLines(lines []byte) []metricRecord {
	var records []metricRecord
	for _, line := range strings.Split(string(lines), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record metricRecord
		if json.Unmarshal([]byte(line), &record) != nil {
			continue
		}
		records = append(records, record)
	}
	return records
}
