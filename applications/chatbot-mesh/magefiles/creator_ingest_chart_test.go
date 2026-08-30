// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func renderChart(t *testing.T, sets ...string) (string, bool) {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	args := []string{"template", "t", findChartDir(t)}
	for _, set := range sets {
		args = append(args, "--set", set)
	}
	out, err := exec.Command("helm", args...).CombinedOutput()
	return string(out), err == nil
}

// creatorContainerEnv returns the creator container's env block from a render,
// so an assertion about the creator cannot be satisfied by another pod's
// variable of the same name.
func creatorContainerEnv(t *testing.T, rendered string) string {
	t.Helper()
	const marker = "name: creator"
	start := strings.Index(rendered, marker)
	if start < 0 {
		t.Fatalf("the render declares no creator container")
	}
	section := rendered[start:]
	if end := strings.Index(section, "volumeMounts"); end > 0 {
		section = section[:end]
	}
	return section
}

// The child runs as a subprocess of the creator and inherits its environment.
// Without a Chroma authority it falls back to the loopback address a developer
// host answers, which in a pod resolves to nothing.
func TestCreatorCarriesTheIngestChromaAuthority(t *testing.T) {
	out, ok := renderChart(t, "controlPlane.enabled=true")
	if !ok {
		t.Fatalf("control-plane render failed:\n%s", out)
	}
	env := creatorContainerEnv(t, out)
	for _, want := range []string{
		`{name: CHROMA_URL, value: "http://t-chatbot-mesh-rag0-chroma:8000"}`,
		`{name: CHROMA_HOST, value: "t-chatbot-mesh-rag0-chroma"}`,
		`{name: CHROMA_PORT, value: "8000"}`,
	} {
		if !strings.Contains(env, want) {
			t.Errorf("the creator container omits %s", want)
		}
	}
}

// The Chroma is a choice, not a lookup: a mesh has one per RAG unit. Naming a
// unit moves the ingest to it, and nothing about the ordering of the list
// decides where documents land.
func TestCreatorIngestChromaFollowsTheNamedRagUnit(t *testing.T) {
	out, ok := renderChart(t, "controlPlane.enabled=true",
		"controlPlane.creator.ingestRagUnit=rag1")
	if !ok {
		t.Fatalf("render failed:\n%s", out)
	}
	env := creatorContainerEnv(t, out)
	if !strings.Contains(env, "t-chatbot-mesh-rag1-chroma") {
		t.Error("naming rag1 did not move the ingest Chroma to that unit")
	}
	if strings.Contains(env, "t-chatbot-mesh-rag0-chroma") {
		t.Error("the creator still points at rag0's Chroma after naming rag1")
	}
}

// The collection is the one the targeted unit's rag-server serves, so documents
// the ingest writes are documents a turn can retrieve.
func TestCreatorIngestCollectionIsTheServedCollection(t *testing.T) {
	out, ok := renderChart(t, "controlPlane.enabled=true",
		"controlPlane.creator.ingestRagUnit=rag1")
	if !ok {
		t.Fatalf("render failed:\n%s", out)
	}
	env := creatorContainerEnv(t, out)
	if !strings.Contains(env, `{name: CORPUS_INGEST_COLLECTION, value: "corpus2"}`) {
		t.Error("the ingest collection is not the collection rag1 serves")
	}
}

// srd002 R3.3 compares each rag-server's reported identity against the query
// vector's, so the family the corpus is written in, the family the unit
// reports, and the family the query embeds at have to be one string. The model
// therefore follows the ingest unit's declared embeddingModel, and the
// rest_ref and operation stay unset so the child keeps its declared Ollama
// defaults; naming them would be configuration with nothing to configure.
func TestCreatorIngestModelFollowsTheIngestUnit(t *testing.T) {
	out, ok := renderChart(t, "controlPlane.enabled=true")
	if !ok {
		t.Fatalf("render failed:\n%s", out)
	}
	env := creatorContainerEnv(t, out)
	if !strings.Contains(env, `{name: CORPUS_EMBEDDING_MODEL, value: "qwen3-embedding:8b"}`) {
		t.Error("the creator does not name the identity its ingest unit reports")
	}
	for _, absent := range []string{
		"CORPUS_INGEST_EMBEDDING_REST_REF", "CORPUS_INGEST_EMBEDDING_OPERATION",
	} {
		if strings.Contains(env, absent) {
			t.Errorf("the creator carries %q with nothing to configure", absent)
		}
	}
}

// An operator pointing the ingest at an in-cluster Ollama gets the allowlist
// host derived from the URL, because the two are one configuration.
func TestCreatorIngestOllamaURLCarriesItsHost(t *testing.T) {
	out, ok := renderChart(t, "controlPlane.enabled=true",
		"controlPlane.creator.ingestOllamaURL=http://t-chatbot-mesh-ollama:11434")
	if !ok {
		t.Fatalf("render failed:\n%s", out)
	}
	env := creatorContainerEnv(t, out)
	for _, want := range []string{
		`{name: CORPUS_INGEST_OLLAMA_URL, value: "http://t-chatbot-mesh-ollama:11434"}`,
		`{name: CORPUS_INGEST_OLLAMA_HOST, value: "t-chatbot-mesh-ollama"}`,
	} {
		if !strings.Contains(env, want) {
			t.Errorf("the creator container omits %s", want)
		}
	}
}

// The child's clients must be repointable at all. Loopback literals are correct
// on a developer host and unreachable in a pod, and an allowlist still pinned to
// 127.0.0.1 would refuse a repointed base URL rather than reach it.
func TestCorpusIngestClientsTakeTheirAuthorityFromTheEnvironment(t *testing.T) {
	data, err := os.ReadFile("../agents/corpus-ingest/corpus-rest.yaml")
	if err != nil {
		t.Fatal(err)
	}
	declared := string(data)
	for _, want := range []string{
		"${CHROMA_URL:-http://127.0.0.1:8000}",
		"${CORPUS_INGEST_OLLAMA_URL:-http://127.0.0.1:11434}",
		"${CHROMA_HOST:-127.0.0.1}",
		"${CORPUS_INGEST_OLLAMA_HOST:-127.0.0.1}",
		"${CHROMA_PORT:-8000}",
		"${CORPUS_INGEST_OLLAMA_PORT:-11434}",
	} {
		if !strings.Contains(declared, want) {
			t.Errorf("corpus-rest.yaml does not reference %s", want)
		}
	}
	// The defaults are what a developer host answers, so a host-side ingest that
	// exported none of these behaves exactly as it did.
	if strings.Contains(declared, "base_url: http://127.0.0.1:") {
		t.Error("a client still hardcodes a loopback base_url; it cannot be repointed in a pod")
	}
}
