// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	cpCreatorIngest = "http://127.0.0.1:18110/api/v1/ingest"
	// The family the creator is told it ingests at for this run. The judge reads
	// the same reference the corpus-ingest child embeds at, so setting it here
	// fixes both operands of the comparison the drive exercises.
	cpIngestFamily = "controlplane-tracer-family"
	// The creator's Chroma client declares ports: [8000] (local_chroma_client),
	// so unlike the deployment API this fake cannot take an ephemeral port: it
	// has to stand exactly where Chroma stands.
	cpChromaAddr = "127.0.0.1:8000"
)

// chromaRecorder records which Chroma routes the creator reached. The refusal
// this proves is an absence -- no child ran -- and the count read is what makes
// that absence observable: it sits between the family judge and the child, so a
// run that never counted is a run that never reached the child.
type chromaRecorder struct {
	mu         sync.Mutex
	resolves   int
	counts     int
	lastCreate string
}

func (r *chromaRecorder) recordResolve(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resolves++
	r.lastCreate = name
}

func (r *chromaRecorder) recordCount() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.counts++
}

func (r *chromaRecorder) resolveCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.resolves
}

func (r *chromaRecorder) countReads() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.counts
}

// startFakeChroma serves the two routes the creator's ingest leg drives: the
// get_or_create that resolves a collection to its id, and the document count it
// reads on either side of the child. It echoes the requested collection name
// back, the way Chroma does, so the creator's response reports where documents
// would have gone rather than what it was asked for.
func startFakeChroma(rec *chromaRecorder) (func(), error) {
	listener, err := net.Listen("tcp", cpChromaAddr)
	if err != nil {
		return nil, fmt.Errorf("bind fake Chroma on %s: %w", cpChromaAddr, err)
	}
	const collections = "/api/v2/tenants/default_tenant/databases/default_database/collections"
	mux := http.NewServeMux()
	mux.HandleFunc(collections, func(w http.ResponseWriter, req *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(req.Body).Decode(&body)
		name, _ := body["name"].(string)
		rec.recordResolve(name)
		writeJSON(w, map[string]interface{}{"id": "mock-collection", "name": name})
	})
	mux.HandleFunc(collections+"/", func(w http.ResponseWriter, req *http.Request) {
		if !strings.HasSuffix(req.URL.Path, "/count") {
			http.NotFound(w, req)
			return
		}
		rec.recordCount()
		// Chroma answers a count with a bare integer, which is why the creator
		// carries it as a string and the judge coerces both reads to numbers.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("0"))
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = server.Serve(listener) }()
	return func() { _ = server.Close() }, nil
}

// ingestResponse is the creator's ingest answer, in the fields this drive reads.
type ingestResponse struct {
	Status         string `json:"status"`
	Error          string `json:"error"`
	Collection     string `json:"collection"`
	EmbeddingModel string `json:"embedding_model"`
	Count          string `json:"count"`
}

// driveEmbeddingFamilyRefusal proves the family judge routes both ways
// (srd005 R3.1, GH-205). The judge sits between the collection resolve and the
// pre-run count, so the count read is the observable that separates the two
// branches: a refused intent never reaches it, and an admitted one does.
//
// The admitted branch deliberately stops at that observation rather than
// asserting a 200. Past the judge the leg runs a real corpus-ingest child
// against a profile this tracer does not stage, so the run ends IngestFailed --
// proving the routing, which is this case's claim. A green ingest end to end is
// integration:controlPlaneLive's, and it needs a cluster and a credential.
func driveEmbeddingFamilyRefusal(rec *chromaRecorder) error {
	// The refused drive runs first, while the count read is still provably
	// untouched. Reversing the order would leave the admitted run's count in the
	// recorder and make "no count" unassertable.
	mismatched := fmt.Sprintf(
		`{"directory":"/tmp/controlplane-tracer","collection":"corpus","embedding_model":%q}`,
		cpIngestFamily+"-other")
	data, status, err := requestInference(
		http.MethodPost, cpCreatorIngest, mismatched, "creator ingest with a mismatched family")
	if err != nil {
		return fmt.Errorf("mismatched-family ingest request failed: %w", err)
	}
	if status != http.StatusUnprocessableEntity {
		return fmt.Errorf(
			"mismatched-family ingest status = %d, want 422; the creator did not refuse a family it does not ingest at: %s",
			status, data)
	}
	var refused ingestResponse
	if err := json.Unmarshal(data, &refused); err != nil {
		return fmt.Errorf("decode refusal response: %w: %s", err, data)
	}
	if refused.Error != "embedding_family_mismatch" {
		return fmt.Errorf(
			"refusal error = %q, want embedding_family_mismatch; a shortfall and a mismatched family must not share one code: %s",
			refused.Error, data)
	}
	// The echo is the deployment's family, not the caller's. Asserting only the
	// status would pass on a body that reported the requested family back, which
	// tells an operator nothing about where an ingest would have embedded.
	if refused.EmbeddingModel != cpIngestFamily {
		return fmt.Errorf(
			"refusal reported embedding_model %q, want the deployment's %q; the response does not say what this ingest would have embedded at: %s",
			refused.EmbeddingModel, cpIngestFamily, data)
	}
	// The refusal exists to prevent a write, so proving nothing ran is proving
	// the feature. The count read is the first thing past the judge.
	if got := rec.countReads(); got != 0 {
		return fmt.Errorf(
			"the creator read the collection count %d time(s) on a refused intent; the refusal came after the leg had already started toward the child", got)
	}
	if got := rec.resolveCount(); got < 1 {
		return fmt.Errorf(
			"the creator never resolved a collection (resolve count %d); the refusal did not come from the family judge", got)
	}

	// The admitted branch. Same request but for the family, so a judge that
	// refused everything -- which the case above alone would not catch -- fails
	// here.
	matched := fmt.Sprintf(
		`{"directory":"/tmp/controlplane-tracer","collection":"corpus","embedding_model":%q}`,
		cpIngestFamily)
	data, status, err = requestInference(
		http.MethodPost, cpCreatorIngest, matched, "creator ingest with the deployed family")
	if err != nil {
		return fmt.Errorf("matched-family ingest request failed: %w", err)
	}
	if got := rec.countReads(); got < 1 {
		return fmt.Errorf(
			"the creator never read the collection count on a matching intent (status %d); the judge refused the family it ingests at: %s",
			status, data)
	}
	var admitted ingestResponse
	if err := json.Unmarshal(data, &admitted); err == nil && admitted.Error == "embedding_family_mismatch" {
		return fmt.Errorf("a matching family was answered %s; the judge routes every intent to the refusal", data)
	}
	return nil
}
