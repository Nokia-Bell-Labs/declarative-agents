// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package otlp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// EnvelopeSchemaVersion is the version tag every persisted object carries, so a
// reader can refuse an envelope it does not understand rather than guess
// (srd042 durable-object persistence; srd008 R6.3).
const EnvelopeSchemaVersion = "otlp-object/v1"

// EnvelopeMeta is the identity a collector stamps on one batch before it is
// persisted. Every field is declared configuration or receiver-observed intake
// metadata; none is derived from the receiver's process-local sequence
// (srd008 R6.3).
type EnvelopeMeta struct {
	Signal            string
	Application       string
	Namespace         string
	Run               string
	CollectorInstance string
	PayloadFormat     string
	ReceivedAt        time.Time
}

// ObjectEnvelope is the one versioned immutable object written per batch. Its
// key and checksum are fixed before the first upload attempt and never change,
// so a retry or restart reuses the same identity (srd008 R6.2, R6.3, R6.4).
type ObjectEnvelope struct {
	SchemaVersion     string          `json:"schema_version"`
	Signal            string          `json:"signal"`
	Application       string          `json:"application"`
	Namespace         string          `json:"namespace"`
	Run               string          `json:"run"`
	CollectorInstance string          `json:"collector_instance"`
	BatchID           string          `json:"batch_id"`
	ReceivedAt        time.Time       `json:"received_at"`
	PayloadFormat     string          `json:"payload_format"`
	PayloadChecksum   string          `json:"payload_checksum"`
	Payload           json.RawMessage `json:"payload"`
}

// newEnvelope builds the immutable envelope for one batch. The batch identity
// is the payload checksum, so identical bytes resolve to one object (an
// idempotent re-put) and different bytes resolve to different objects, with no
// dependence on any per-process counter (srd008 R4, R6.2).
func newEnvelope(meta EnvelopeMeta, payload []byte) ObjectEnvelope {
	checksum := payloadChecksum(payload)
	return ObjectEnvelope{
		SchemaVersion:     EnvelopeSchemaVersion,
		Signal:            meta.Signal,
		Application:       meta.Application,
		Namespace:         meta.Namespace,
		Run:               meta.Run,
		CollectorInstance: meta.CollectorInstance,
		BatchID:           checksum,
		ReceivedAt:        meta.ReceivedAt.UTC(),
		PayloadFormat:     meta.PayloadFormat,
		PayloadChecksum:   checksum,
		Payload:           append(json.RawMessage(nil), payload...),
	}
}

// objectKey is the immutable key the envelope is stored under. The signal and a
// date prefix keep traces and metrics in separate, browsable object families
// (srd008 R5, R6.1); the batch id makes the key stable and collision-free.
func (e ObjectEnvelope) objectKey(prefix string) string {
	key := fmt.Sprintf("%s/%s/%s.json", e.Signal, e.ReceivedAt.UTC().Format("2006-01-02"), e.BatchID)
	if prefix == "" {
		return key
	}
	return fmt.Sprintf("%s/%s", trimSlashes(prefix), key)
}

func (e ObjectEnvelope) encode() ([]byte, error) {
	data, err := json.Marshal(e)
	if err != nil {
		return nil, fmt.Errorf("encode object envelope %s: %w", e.BatchID, err)
	}
	return data, nil
}

func decodeEnvelope(data []byte) (ObjectEnvelope, error) {
	var envelope ObjectEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return ObjectEnvelope{}, fmt.Errorf("decode object envelope: %w", err)
	}
	return envelope, nil
}

func payloadChecksum(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func trimSlashes(prefix string) string {
	for len(prefix) > 0 && prefix[len(prefix)-1] == '/' {
		prefix = prefix[:len(prefix)-1]
	}
	for len(prefix) > 0 && prefix[0] == '/' {
		prefix = prefix[1:]
	}
	return prefix
}
