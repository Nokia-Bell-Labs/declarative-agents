// Copyright (c) 2026 Nokia. All rights reserved.

// Package ragmerge provides the rag_merge builtin: a word that combines several
// RAG query results addressed by command-state $from(label) selectors into one
// distance-ordered, source-tagged chunk list, so a later compose word renders a
// grounding prompt over several RAG sources (srd014 R3 multi-RAG fan-out). A
// source that failed (absent or carrying no documents) is recorded as degraded
// and skipped; a source whose reported embedding model does not match the query
// embedding's model is excluded and recorded.
package ragmerge

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

const defaultMergeSignal = core.Signal("Merged")

// Source names one prior RAG query word and the tag its chunks carry.
type Source struct {
	Label string
	Tag   string
}

// Builder constructs rag_merge commands.
type Builder struct {
	ToolName               string
	Sources                []Source
	ExpectedEmbeddingModel string
	MaxChunks              int
	Signal                 core.Signal
}

// Build returns a rag_merge command. The engine injects the command-state view
// before dispatch (core.CommandStateAware).
func (b Builder) Build(_ core.Result) core.Command {
	return &ragMergeCmd{
		name:          b.ToolName,
		sources:       b.Sources,
		expectedModel: b.ExpectedEmbeddingModel,
		maxChunks:     b.MaxChunks,
		signal:        b.Signal,
	}
}

type ragMergeCmd struct {
	name          string
	sources       []Source
	expectedModel string
	maxChunks     int
	signal        core.Signal
	view          core.CommandStateView
}

func (c *ragMergeCmd) Name() string { return c.name }

// SetCommandState receives the read-only command-state view so each source's
// $from(label) result resolves against prior steps.
func (c *ragMergeCmd) SetCommandState(view core.CommandStateView) { c.view = view }

var _ core.CommandStateAware = (*ragMergeCmd)(nil)

func (c *ragMergeCmd) Undo(_ core.Result) core.Result { return core.NoopUndo(c.Name()) }

type chunk struct {
	text     string
	distance float64
	tag      string
	order    int // stable tiebreaker preserving source and retrieval order
}

// Execute resolves every configured source from command state, excludes the
// embedding-model mismatches, merges the remaining chunks ordered by ascending
// distance with a per-source tag, caps the total, and reports the degraded and
// excluded sources.
func (c *ragMergeCmd) Execute() core.Result {
	var chunks []chunk
	var degraded, excluded []string
	order := 0
	for _, source := range c.sources {
		docs, dists, model, ok := c.resolveSource(source.Label)
		if !ok {
			degraded = append(degraded, source.Label)
			continue
		}
		if c.expectedModel != "" && model != c.expectedModel {
			excluded = append(excluded, source.Label)
			continue
		}
		for i, doc := range docs {
			chunks = append(chunks, chunk{
				text:     doc,
				distance: distanceAt(dists, i),
				tag:      source.Tag,
				order:    order,
			})
			order++
		}
	}

	sort.SliceStable(chunks, func(i, j int) bool {
		if chunks[i].distance == chunks[j].distance {
			return chunks[i].order < chunks[j].order
		}
		return chunks[i].distance < chunks[j].distance
	})
	if c.maxChunks > 0 && len(chunks) > c.maxChunks {
		chunks = chunks[:c.maxChunks]
	}

	documents := make([]string, 0, len(chunks))
	for _, ch := range chunks {
		documents = append(documents, fmt.Sprintf("[%s] %s", ch.tag, ch.text))
	}

	signal := c.signal
	if signal == "" {
		signal = defaultMergeSignal
	}
	payload := map[string]interface{}{
		"mapped": map[string]interface{}{
			"documents": documents,
			"degraded":  nonNil(degraded),
			"excluded":  nonNil(excluded),
		},
	}
	out, err := json.Marshal(payload)
	if err != nil {
		return core.Result{Signal: core.CommandError, Err: err, Output: err.Error(), CommandName: c.Name()}
	}
	return core.Result{Signal: signal, CommandName: c.Name(), Output: string(out)}
}

// resolveSource reads a source's documents, distances, and embedding model from
// command state. It returns ok=false when the label is absent or carries no
// documents, so a degraded RAG (whose query failed and the machine routed past
// it) is skipped rather than fatal.
func (c *ragMergeCmd) resolveSource(label string) (docs []string, dists []float64, model string, ok bool) {
	rawDocs, err := core.ResolveFromSelector(c.view, "$from("+label+").mapped.documents")
	if err != nil {
		return nil, nil, "", false
	}
	docs = flattenStrings(rawDocs)
	if len(docs) == 0 {
		return nil, nil, "", false
	}
	if rawDists, err := core.ResolveFromSelector(c.view, "$from("+label+").mapped.distances"); err == nil {
		dists = flattenFloats(rawDists)
	}
	if rawModel, err := core.ResolveFromSelector(c.view, "$from("+label+").mapped.embedding_model"); err == nil {
		if s, isStr := rawModel.(string); isStr {
			model = s
		}
	}
	return docs, dists, model, true
}

// flattenStrings accepts the Chroma per-query shape [["a","b"]] as well as a flat
// ["a","b"] list and returns the string chunks.
func flattenStrings(v interface{}) []string {
	items, ok := v.([]interface{})
	if !ok {
		return nil
	}
	if len(items) == 1 {
		if inner, nested := items[0].([]interface{}); nested {
			items = inner
		}
	}
	var out []string
	for _, item := range items {
		if s, isStr := item.(string); isStr {
			out = append(out, s)
		}
	}
	return out
}

// flattenFloats mirrors flattenStrings for the parallel distances list.
func flattenFloats(v interface{}) []float64 {
	items, ok := v.([]interface{})
	if !ok {
		return nil
	}
	if len(items) == 1 {
		if inner, nested := items[0].([]interface{}); nested {
			items = inner
		}
	}
	var out []float64
	for _, item := range items {
		if f, isNum := item.(float64); isNum {
			out = append(out, f)
		}
	}
	return out
}

// distanceAt returns the distance for index i, or +Inf when it is missing so an
// unscored chunk sorts after scored ones rather than ahead of them.
func distanceAt(dists []float64, i int) float64 {
	if i < len(dists) {
		return dists[i]
	}
	return math.Inf(1)
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
