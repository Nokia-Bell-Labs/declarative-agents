// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package otlp

import (
	"fmt"
	"path/filepath"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

// SpanHeatmapToolConfig carries trusted heatmap shape and spool authority.
type SpanHeatmapToolConfig struct {
	Path            string  `json:"path"`
	TimeBuckets     int     `json:"time_buckets"`
	DurationEdgesMs []int64 `json:"duration_edges_ms"`
	ExemplarCap     int     `json:"exemplar_cap"`
}

// SpanGroupByToolConfig carries trusted grouping caps and spool authority.
type SpanGroupByToolConfig struct {
	Path        string `json:"path"`
	GroupBy     string `json:"group_by"`
	TopN        int    `json:"top_n"`
	MaxTopN     int    `json:"max_top_n"`
	ExemplarCap int    `json:"exemplar_cap"`
}

// SpanBreakdownToolConfig is the declared spool_span_breakdown configuration.
// The baseline and selection filters arrive per-request via the machine seed;
// config carries the spool path and caps.
type SpanBreakdownToolConfig struct {
	Path        string `json:"path"`
	TopN        int    `json:"top_n"`
	MaxTopN     int    `json:"max_top_n"`
	ExemplarCap int    `json:"exemplar_cap"`
}

// resolveSpoolPath resolves a configured spool path against the workspace
// directory when it is relative, matching the srd044 query factories.
func resolveSpoolPath(path string, vars map[string]string) string {
	if !filepath.IsAbs(path) && vars["directory"] != "" {
		return filepath.Join(vars["directory"], path)
	}
	return path
}

func spanHeatmapFactory() toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		var raw SpanHeatmapToolConfig
		if err := catalog.DecodeToolConfig(def, &raw); err != nil {
			return nil, err
		}
		if raw.Path == "" {
			return nil, fmt.Errorf("tool %q config requires path", def.Name)
		}
		for i := 1; i < len(raw.DurationEdgesMs); i++ {
			if raw.DurationEdgesMs[i] <= raw.DurationEdgesMs[i-1] {
				return nil, fmt.Errorf("tool %q config duration_edges_ms must be strictly increasing", def.Name)
			}
		}
		return SpanHeatmapBuilder{
			ToolName: def.Name,
			Config: SpanHeatmapConfig{
				Path: resolveSpoolPath(raw.Path, vars), Filter: spanFilter{Attrs: map[string]string{}},
				TimeBuckets: raw.TimeBuckets, DurationEdgesMs: raw.DurationEdgesMs,
				ExemplarCap: raw.ExemplarCap,
			},
		}, nil
	}
}

func spanGroupByFactory() toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		var raw SpanGroupByToolConfig
		if err := catalog.DecodeToolConfig(def, &raw); err != nil {
			return nil, err
		}
		if raw.Path == "" {
			return nil, fmt.Errorf("tool %q config requires path", def.Name)
		}
		return SpanGroupByBuilder{
			ToolName: def.Name,
			Config: SpanGroupByConfig{
				Path: resolveSpoolPath(raw.Path, vars), Filter: spanFilter{Attrs: map[string]string{}},
				GroupBy: raw.GroupBy, TopN: raw.TopN, MaxTopN: raw.MaxTopN,
				ExemplarCap: raw.ExemplarCap,
			},
		}, nil
	}
}

func spanBreakdownFactory() toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		var raw SpanBreakdownToolConfig
		if err := catalog.DecodeToolConfig(def, &raw); err != nil {
			return nil, err
		}
		if raw.Path == "" {
			return nil, fmt.Errorf("tool %q config requires path", def.Name)
		}
		return SpanBreakdownBuilder{
			ToolName: def.Name,
			Config: SpanBreakdownConfig{
				Path:      resolveSpoolPath(raw.Path, vars),
				Baseline:  spanFilter{Attrs: map[string]string{}},
				Selection: spanFilter{Attrs: map[string]string{}},
				TopN:      raw.TopN, MaxTopN: raw.MaxTopN,
				ExemplarCap: raw.ExemplarCap,
			},
		}, nil
	}
}
