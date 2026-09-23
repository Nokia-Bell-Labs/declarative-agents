// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package otlp

import (
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/objectstore"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

// decodeQueryStorage resolves a query word's read-only storage backend. An
// object backend that names no bucket URL fails the load with the fault named;
// bucket, endpoint, and prefix are configuration only (srd042 R11.1).
func decodeQueryStorage(
	toolName string, raw QueryStorageToolConfig, vars map[string]string, opener *objectstore.Opener,
) (QueryStorageConfig, error) {
	switch raw.Backend {
	case "", BackendFilesystem:
		return QueryStorageConfig{Backend: BackendFilesystem}, nil
	case BackendObject:
		if raw.BucketURL == "" {
			return QueryStorageConfig{}, fmt.Errorf("tool %q object storage requires bucket_url", toolName)
		}
		walPath := raw.WALPath
		if walPath != "" {
			walPath = resolvePath(walPath, vars)
		}
		return QueryStorageConfig{
			Backend:    BackendObject,
			Connection: objectstore.ConnectionConfig{BucketURL: raw.BucketURL, Endpoint: raw.Endpoint},
			Prefix:     raw.Prefix, WALPath: walPath,
			MaxObjects: raw.MaxObjects, MaxBytes: raw.MaxBytes, TimeoutMS: raw.TimeoutMS,
			Opener: opener,
		}, nil
	default:
		return QueryStorageConfig{}, fmt.Errorf(
			"tool %q has unknown storage backend %q (supported: filesystem, object)", toolName, raw.Backend)
	}
}

func queryListFactory(opener *objectstore.Opener) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		var raw QueryListToolConfig
		if err := catalog.DecodeToolConfig(def, &raw); err != nil {
			return nil, err
		}
		if raw.Path == "" {
			return nil, fmt.Errorf("tool %q config requires path", def.Name)
		}
		storage, err := decodeQueryStorage(def.Name, raw.Storage, vars, opener)
		if err != nil {
			return nil, err
		}
		return ListTracesBuilder{
			ToolName: def.Name,
			Config: QueryListConfig{
				Path: resolvePath(raw.Path, vars), PageSize: pageSizeOrDefault(raw.PageSize),
				MaxPageSize: maxPageOrDefault(raw.MaxPageSize), Offset: raw.Offset, Storage: storage,
			},
		}, nil
	}
}

func queryGetFactory(opener *objectstore.Opener) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		var raw QueryGetToolConfig
		if err := catalog.DecodeToolConfig(def, &raw); err != nil {
			return nil, err
		}
		if raw.Path == "" {
			return nil, fmt.Errorf("tool %q config requires path", def.Name)
		}
		storage, err := decodeQueryStorage(def.Name, raw.Storage, vars, opener)
		if err != nil {
			return nil, err
		}
		return GetTraceBuilder{
			ToolName: def.Name,
			Config:   QueryGetConfig{Path: resolvePath(raw.Path, vars), TraceID: raw.TraceID, Storage: storage},
		}, nil
	}
}

func queryListMetricsFactory(opener *objectstore.Opener) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		var raw QueryListMetricsToolConfig
		if err := catalog.DecodeToolConfig(def, &raw); err != nil {
			return nil, err
		}
		if raw.Path == "" {
			return nil, fmt.Errorf("tool %q config requires path", def.Name)
		}
		storage, err := decodeQueryStorage(def.Name, raw.Storage, vars, opener)
		if err != nil {
			return nil, err
		}
		return ListMetricsBuilder{
			ToolName: def.Name,
			Config: QueryListMetricsConfig{
				Path: resolvePath(raw.Path, vars), PageSize: pageSizeOrDefault(raw.PageSize),
				MaxPageSize: maxPageOrDefault(raw.MaxPageSize), Offset: raw.Offset, Storage: storage,
			},
		}, nil
	}
}

func queryGetMetricFactory(opener *objectstore.Opener) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		var raw QueryGetMetricToolConfig
		if err := catalog.DecodeToolConfig(def, &raw); err != nil {
			return nil, err
		}
		if raw.Path == "" {
			return nil, fmt.Errorf("tool %q config requires path", def.Name)
		}
		storage, err := decodeQueryStorage(def.Name, raw.Storage, vars, opener)
		if err != nil {
			return nil, err
		}
		return GetMetricBuilder{
			ToolName: def.Name,
			Config: QueryGetMetricConfig{
				Path: resolvePath(raw.Path, vars), MetricName: raw.MetricName,
				PageSize: raw.PageSize, MaxPageSize: raw.MaxPageSize, Offset: raw.Offset, Storage: storage,
			},
		}, nil
	}
}

func pageSizeOrDefault(v int) int {
	if v <= 0 {
		return defaultPageSize
	}
	return v
}

func maxPageOrDefault(v int) int {
	if v <= 0 {
		return defaultMaxPageSize
	}
	return v
}
