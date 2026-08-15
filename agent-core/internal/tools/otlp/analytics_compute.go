// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package otlp

import "sort"

// defaultExemplarCap bounds exemplar_trace_ids when a tool declares no
// exemplar_cap. Kept small: the Explore page links a handful of representative
// traces per selection, not the whole matched set.
const defaultExemplarCap = 20

// exemplarTraceIDs projects a span set onto a capped, deterministic,
// deduplicated list of trace IDs. Ordering is lexicographic on the hex trace
// ID so the same selection always yields the same exemplars regardless of
// spool read order; a non-positive limit selects the default cap.
func exemplarTraceIDs(spans []enrichedSpan, limit int) []string {
	if limit <= 0 {
		limit = defaultExemplarCap
	}
	seen := make(map[string]struct{}, len(spans))
	ids := make([]string, 0, len(spans))
	for _, s := range spans {
		if s.traceID == "" {
			continue
		}
		if _, ok := seen[s.traceID]; ok {
			continue
		}
		seen[s.traceID] = struct{}{}
		ids = append(ids, s.traceID)
	}
	sort.Strings(ids)
	if len(ids) > limit {
		ids = ids[:limit]
	}
	return ids
}

func filterEnriched(spans []enrichedSpan, f spanFilter) []enrichedSpan {
	out := make([]enrichedSpan, 0, len(spans))
	for _, s := range spans {
		if f.matches(s) {
			out = append(out, s)
		}
	}
	return out
}

func buildHeatmap(spans []enrichedSpan, cfg SpanHeatmapConfig) heatmapPayload {
	timeBuckets := cfg.TimeBuckets
	if timeBuckets <= 0 {
		timeBuckets = defaultTimeBuckets
	}
	edges := cfg.DurationEdgesMs
	if len(edges) == 0 {
		edges = defaultDurationEdgesMs
	}
	tMin, tMax := timeRange(spans, cfg.Filter)
	timeEdges := make([]int64, timeBuckets+1)
	span := tMax - tMin
	for i := 0; i <= timeBuckets; i++ {
		timeEdges[i] = tMin + span*int64(i)/int64(timeBuckets)
	}
	cells := make([][]int, timeBuckets)
	for i := range cells {
		cells[i] = make([]int, len(edges))
	}
	for _, s := range spans {
		ti := timeBucketIndex(s.startMs, tMin, tMax, timeBuckets)
		di := durationBucketIndex(s.durMs, edges)
		cells[ti][di]++
	}
	return heatmapPayload{
		TimeBucketBoundaries:     timeEdges,
		DurationBucketBoundaries: append([]int64(nil), edges...),
		Cells:                    cells,
	}
}

// timeRange returns the inclusive-exclusive span the heatmap X axis covers.
// The filter's explicit range wins; otherwise the matched spans' min/max start.
func timeRange(spans []enrichedSpan, f spanFilter) (int64, int64) {
	if f.StartMs != 0 && f.EndMs != 0 {
		return f.StartMs, f.EndMs
	}
	if len(spans) == 0 {
		return 0, 0
	}
	minMs, maxMs := spans[0].startMs, spans[0].startMs
	for _, s := range spans {
		if s.startMs < minMs {
			minMs = s.startMs
		}
		if s.startMs > maxMs {
			maxMs = s.startMs
		}
	}
	if f.StartMs != 0 {
		minMs = f.StartMs
	}
	if f.EndMs != 0 {
		maxMs = f.EndMs
	}
	// A single instant still needs a non-zero width so every span lands in bucket 0.
	if maxMs <= minMs {
		maxMs = minMs + 1
	}
	return minMs, maxMs
}

func timeBucketIndex(startMs, tMin, tMax int64, buckets int) int {
	if tMax <= tMin {
		return 0
	}
	idx := int((startMs - tMin) * int64(buckets) / (tMax - tMin))
	if idx < 0 {
		return 0
	}
	if idx >= buckets {
		return buckets - 1
	}
	return idx
}

// durationBucketIndex returns the half-open bucket for a duration: bucket i
// holds [edges[i], edges[i+1]); the last bucket is the overflow at or above the
// final edge.
func durationBucketIndex(durMs int64, edges []int64) int {
	for i := 0; i < len(edges)-1; i++ {
		if durMs >= edges[i] && durMs < edges[i+1] {
			return i
		}
	}
	return len(edges) - 1
}

func buildGroupBy(spans []enrichedSpan, cfg SpanGroupByConfig) (groups []groupCount, groupBy string, droppedGroups, droppedSpans int) {
	groupBy = cfg.GroupBy
	if groupBy == "" {
		return nil, "", 0, 0
	}
	counts := map[string]int{}
	for _, s := range spans {
		if v, ok := s.groupValue(groupBy); ok {
			counts[v]++
		}
	}
	all := make([]groupCount, 0, len(counts))
	for v, n := range counts {
		all = append(all, groupCount{Value: v, Count: n})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Count != all[j].Count {
			return all[i].Count > all[j].Count
		}
		return all[i].Value < all[j].Value
	})
	topN := effectiveTopN(cfg.TopN, cfg.MaxTopN)
	if len(all) <= topN {
		return all, groupBy, 0, 0
	}
	for _, g := range all[topN:] {
		droppedSpans += g.Count
	}
	return all[:topN], groupBy, len(all) - topN, droppedSpans
}

func effectiveTopN(topN, maxTopN int) int {
	if maxTopN <= 0 {
		maxTopN = defaultMaxTopN
	}
	if topN <= 0 {
		topN = defaultTopN
	}
	if topN > maxTopN {
		topN = maxTopN
	}
	return topN
}

type attrPair struct{ key, value string }

func attrPairCounts(spans []enrichedSpan) map[attrPair]int {
	counts := map[attrPair]int{}
	for _, s := range spans {
		for k, v := range s.attrs {
			counts[attrPair{k, v}]++
		}
	}
	return counts
}

// rankDivergence ranks attribute key/value pairs by how much more concentrated
// they are inside the selection than outside it. The score is the inside
// proportion minus the outside proportion; an empty selection yields an empty
// ranking.
func rankDivergence(inside, outside []enrichedSpan, topN int) ([]divergenceEntry, int) {
	if len(inside) == 0 {
		return nil, 0
	}
	insideCounts := attrPairCounts(inside)
	outsideCounts := attrPairCounts(outside)
	entries := make([]divergenceEntry, 0, len(insideCounts))
	for pair, in := range insideCounts {
		inProp := float64(in) / float64(len(inside))
		outProp := 0.0
		if len(outside) > 0 {
			outProp = float64(outsideCounts[pair]) / float64(len(outside))
		}
		entries = append(entries, divergenceEntry{
			Key: pair.key, Value: pair.value,
			InsideCount: in, OutsideCount: outsideCounts[pair],
			InsideProportion: inProp, OutsideProportion: outProp,
			Score: inProp - outProp,
		})
	}
	sort.Slice(entries, func(i, j int) bool { return lessDivergence(entries[i], entries[j]) })
	if len(entries) <= topN {
		return entries, 0
	}
	return entries[:topN], len(entries) - topN
}

func lessDivergence(a, b divergenceEntry) bool {
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	if a.Key != b.Key {
		return a.Key < b.Key
	}
	return a.Value < b.Value
}
