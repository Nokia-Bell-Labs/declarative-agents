// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package otlp

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/objectstore"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

// StandardInits lists the receiver lifecycle init names.
var StandardInits = []string{
	InitReceiverLaunch, InitAwaitSpans, InitLoadOTLPBatch, InitSpoolSpans, InitRelaySpans, InitReceiverStop,
	InitSpoolListTraces, InitSpoolGetTrace,
	InitSpoolSpanHeatmap, InitSpoolSpanGroupBy, InitSpoolSpanBreakdown,
	InitAwaitMetrics, InitSpoolMetrics,
	InitSpoolListMetrics, InitSpoolGetMetric,
}

// ReceiverToolConfig is the YAML ToolDef config shared by launch and stop.
type ReceiverToolConfig struct {
	Receiver        string `json:"receiver"`
	Address         string `json:"address"`
	QueueCapacity   int    `json:"queue_capacity"`
	OverflowPolicy  string `json:"overflow_policy"`
	ShutdownTimeout string `json:"shutdown_timeout"`
	DrainPolicy     string `json:"drain_policy"`
}

// AwaitToolConfig is the declared await_spans configuration.
type AwaitToolConfig struct {
	Receiver string `json:"receiver"`
	Timeout  string `json:"timeout"`
}

// SpoolToolConfig is the declared spool_spans configuration.
type SpoolToolConfig struct {
	Path        string            `json:"path"`
	BatchSource string            `json:"batch_source"`
	MaxBytes    int64             `json:"max_bytes"`
	MaxFiles    int               `json:"max_files"`
	Storage     StorageToolConfig `json:"storage"`
}

// StorageToolConfig selects the spool word's storage backend. It defaults to
// the filesystem NDJSON spool; the object backend is declared configuration
// only, and cloud credentials remain ambient workload identity (srd008 R8).
type StorageToolConfig struct {
	Backend           string `json:"backend"`
	BucketURL         string `json:"bucket_url"`
	Endpoint          string `json:"endpoint"`
	Prefix            string `json:"prefix"`
	WALPath           string `json:"wal_path"`
	StageDir          string `json:"stage_dir"`
	Application       string `json:"application"`
	Namespace         string `json:"namespace"`
	Run               string `json:"run"`
	CollectorInstance string `json:"collector_instance"`
	WALMaxBytes       int64  `json:"wal_max_bytes"`
	WALMaxPending     int    `json:"wal_max_pending"`
}

// LoadToolConfig is the declared load_otlp_batch configuration.
type LoadToolConfig struct {
	Path string `json:"path"`
}

// RelayToolConfig is the declared relay_spans configuration.
type RelayToolConfig struct {
	Endpoint        string `json:"endpoint"`
	ReceiverAddress string `json:"receiver_address"`
	BatchSource     string `json:"batch_source"`
	Timeout         string `json:"timeout"`
}

// QueryStorageToolConfig is the declared read-only storage backend a query word
// reads from. It mirrors the spool StorageToolConfig fields a read needs and
// adds per-request budgets; bucket, endpoint, and prefix are configuration only
// (srd042 R1, R3).
type QueryStorageToolConfig struct {
	Backend    string `json:"backend"`
	BucketURL  string `json:"bucket_url"`
	Endpoint   string `json:"endpoint"`
	Prefix     string `json:"prefix"`
	WALPath    string `json:"wal_path"`
	MaxObjects int    `json:"max_objects"`
	MaxBytes   int64  `json:"max_bytes"`
	TimeoutMS  int    `json:"timeout_ms"`
}

// QueryListToolConfig is the declared spool_list_traces configuration.
type QueryListToolConfig struct {
	Path        string                 `json:"path"`
	PageSize    int                    `json:"page_size"`
	MaxPageSize int                    `json:"max_page_size"`
	Offset      int                    `json:"offset"`
	Storage     QueryStorageToolConfig `json:"storage"`
}

// QueryGetToolConfig is the declared spool_get_trace configuration.
type QueryGetToolConfig struct {
	Path    string                 `json:"path"`
	TraceID string                 `json:"trace_id"`
	Storage QueryStorageToolConfig `json:"storage"`
}

// QueryListMetricsToolConfig is the declared spool_list_metrics configuration.
type QueryListMetricsToolConfig struct {
	Path        string                 `json:"path"`
	PageSize    int                    `json:"page_size"`
	MaxPageSize int                    `json:"max_page_size"`
	Offset      int                    `json:"offset"`
	Storage     QueryStorageToolConfig `json:"storage"`
}

// QueryGetMetricToolConfig is the declared spool_get_metric configuration.
type QueryGetMetricToolConfig struct {
	Path        string                 `json:"path"`
	MetricName  string                 `json:"metric_name"`
	PageSize    int                    `json:"page_size"`
	MaxPageSize int                    `json:"max_page_size"`
	Offset      int                    `json:"offset"`
	Storage     QueryStorageToolConfig `json:"storage"`
}

// RegisterFactories registers receiver lifecycle factories over one shared state.
func RegisterFactories(br *toolregistry.BuiltinRegistry, state *State) {
	if state == nil {
		state = NewState()
	}
	opener := objectstore.NewOpener()
	for _, init := range StandardInits {
		switch init {
		case InitAwaitSpans:
			br.Register(init, awaitFactory(state))
		case InitLoadOTLPBatch:
			br.Register(init, loadFactory())
		case InitSpoolSpans:
			br.Register(init, spoolFactory(opener))
		case InitRelaySpans:
			br.Register(init, relayFactory())
		case InitSpoolListTraces:
			br.Register(init, queryListFactory(opener))
		case InitSpoolGetTrace:
			br.Register(init, queryGetFactory(opener))
		case InitSpoolSpanHeatmap:
			br.Register(init, spanHeatmapFactory())
		case InitSpoolSpanGroupBy:
			br.Register(init, spanGroupByFactory())
		case InitSpoolSpanBreakdown:
			br.Register(init, spanBreakdownFactory())
		case InitAwaitMetrics:
			br.Register(init, metricAwaitFactory(state))
		case InitSpoolMetrics:
			br.Register(init, spoolMetricsFactory(opener))
		case InitSpoolListMetrics:
			br.Register(init, queryListMetricsFactory(opener))
		case InitSpoolGetMetric:
			br.Register(init, queryGetMetricFactory(opener))
		default:
			br.Register(init, receiverFactory(init, state))
		}
	}
}

func loadFactory() toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		var raw LoadToolConfig
		if err := catalog.DecodeToolConfig(def, &raw); err != nil {
			return nil, err
		}
		if raw.Path == "" {
			return nil, fmt.Errorf("tool %q config requires path", def.Name)
		}
		path := raw.Path
		if !filepath.IsAbs(path) && vars["directory"] != "" {
			path = filepath.Join(vars["directory"], path)
		}
		return LoadBuilder{ToolName: def.Name, Config: LoadConfig{Path: path}}, nil
	}
}

func relayFactory() toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
		var raw RelayToolConfig
		if err := catalog.DecodeToolConfig(def, &raw); err != nil {
			return nil, err
		}
		timeout := defaultRelayTimeout
		if raw.Timeout != "" {
			parsed, err := time.ParseDuration(raw.Timeout)
			if err != nil {
				return nil, fmt.Errorf("tool %q config has invalid timeout %q", def.Name, raw.Timeout)
			}
			timeout = parsed
		}
		source := raw.BatchSource
		if source == "" {
			source = defaultBatchSource
		}
		config := RelayConfig{
			Endpoint: raw.Endpoint, ReceiverAddress: raw.ReceiverAddress,
			BatchSource: source, Timeout: timeout,
		}
		if err := validateRelayConfig(def.Name, config); err != nil {
			return nil, err
		}
		return RelayBuilder{ToolName: def.Name, Config: config}, nil
	}
}

func receiverFactory(init string, state *State) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
		var raw ReceiverToolConfig
		if err := catalog.DecodeToolConfig(def, &raw); err != nil {
			return nil, err
		}
		cfg, err := decodeReceiverConfig(def.Name, raw)
		if err != nil {
			return nil, err
		}
		if init == InitReceiverLaunch {
			if err := validateReceiverConfig(withReceiverDefaults(cfg)); err != nil {
				return nil, fmt.Errorf("tool %q config: %w", def.Name, err)
			}
		}
		return ReceiverBuilder{ToolName: def.Name, Init: init, Config: cfg, State: state}, nil
	}
}

func awaitFactory(state *State) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
		var raw AwaitToolConfig
		if err := catalog.DecodeToolConfig(def, &raw); err != nil {
			return nil, err
		}
		if raw.Receiver == "" {
			return nil, fmt.Errorf("tool %q config requires receiver", def.Name)
		}
		timeout := defaultBatchAwaitTimeout
		if raw.Timeout != "" {
			parsed, err := time.ParseDuration(raw.Timeout)
			if err != nil || parsed <= 0 {
				return nil, fmt.Errorf("tool %q config has invalid timeout %q", def.Name, raw.Timeout)
			}
			timeout = parsed
		}
		return AwaitBuilder{
			ToolName: def.Name, Config: AwaitConfig{Receiver: raw.Receiver, Timeout: timeout},
			State: state,
		}, nil
	}
}

func spoolFactory(opener *objectstore.Opener) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		raw, source, err := decodeSpoolCommon(def)
		if err != nil {
			return nil, err
		}
		spool := SpoolConfig{
			Path: resolvePath(raw.Path, vars), BatchSource: source,
			MaxBytes: raw.MaxBytes, MaxFiles: raw.MaxFiles,
		}
		storage, err := decodeStorageConfig(def.Name, raw.Storage, vars)
		if err != nil {
			return nil, err
		}
		return SpoolBuilder{ToolName: def.Name, Config: spool, Storage: storage, Opener: opener}, nil
	}
}

// decodeSpoolCommon decodes and validates the fields both spool words share.
func decodeSpoolCommon(def catalog.ToolDef) (SpoolToolConfig, string, error) {
	var raw SpoolToolConfig
	if err := catalog.DecodeToolConfig(def, &raw); err != nil {
		return SpoolToolConfig{}, "", err
	}
	if raw.Path == "" {
		return SpoolToolConfig{}, "", fmt.Errorf("tool %q config requires path", def.Name)
	}
	source := raw.BatchSource
	if source == "" {
		source = defaultBatchSource
	}
	if _, ok := core.ParseSelector(source); !ok {
		return SpoolToolConfig{}, "", fmt.Errorf("tool %q config has invalid batch_source %q", def.Name, source)
	}
	if err := validateSpoolBounds(def.Name, raw); err != nil {
		return SpoolToolConfig{}, "", err
	}
	return raw, source, nil
}

func resolvePath(path string, vars map[string]string) string {
	if !filepath.IsAbs(path) && vars["directory"] != "" {
		return filepath.Join(vars["directory"], path)
	}
	return path
}

// decodeStorageConfig resolves the spool word's storage backend. An object
// backend that names no bucket URL or WAL path fails the load with the fault
// named rather than at first persist (srd008 R8).
func decodeStorageConfig(toolName string, raw StorageToolConfig, vars map[string]string) (StorageConfig, error) {
	switch raw.Backend {
	case "", BackendFilesystem:
		return StorageConfig{Backend: BackendFilesystem}, nil
	case BackendObject:
		if raw.BucketURL == "" {
			return StorageConfig{}, fmt.Errorf("tool %q object storage requires bucket_url", toolName)
		}
		if raw.WALPath == "" {
			return StorageConfig{}, fmt.Errorf("tool %q object storage requires wal_path", toolName)
		}
		stage := raw.StageDir
		if stage != "" {
			stage = resolvePath(stage, vars)
		}
		return StorageConfig{
			Backend:    BackendObject,
			Connection: objectstore.ConnectionConfig{BucketURL: raw.BucketURL, Endpoint: raw.Endpoint},
			Prefix:     raw.Prefix, WALPath: resolvePath(raw.WALPath, vars), StageDir: stage,
			Application: raw.Application, Namespace: raw.Namespace, Run: raw.Run,
			CollectorInstance: raw.CollectorInstance,
			WALMaxBytes:       raw.WALMaxBytes, WALMaxPending: raw.WALMaxPending,
		}, nil
	default:
		return StorageConfig{}, fmt.Errorf(
			"tool %q has unknown storage backend %q (supported: filesystem, object)", toolName, raw.Backend)
	}
}

func metricAwaitFactory(state *State) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
		var raw AwaitToolConfig
		if err := catalog.DecodeToolConfig(def, &raw); err != nil {
			return nil, err
		}
		if raw.Receiver == "" {
			return nil, fmt.Errorf("tool %q config requires receiver", def.Name)
		}
		timeout := defaultBatchAwaitTimeout
		if raw.Timeout != "" {
			parsed, err := time.ParseDuration(raw.Timeout)
			if err != nil || parsed <= 0 {
				return nil, fmt.Errorf("tool %q config has invalid timeout %q", def.Name, raw.Timeout)
			}
			timeout = parsed
		}
		return MetricAwaitBuilder{
			ToolName: def.Name, Config: AwaitConfig{Receiver: raw.Receiver, Timeout: timeout},
			State: state,
		}, nil
	}
}

func spoolMetricsFactory(opener *objectstore.Opener) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		raw, source, err := decodeSpoolCommon(def)
		if err != nil {
			return nil, err
		}
		spool := SpoolConfig{
			Path: resolvePath(raw.Path, vars), BatchSource: source,
			MaxBytes: raw.MaxBytes, MaxFiles: raw.MaxFiles,
		}
		storage, err := decodeStorageConfig(def.Name, raw.Storage, vars)
		if err != nil {
			return nil, err
		}
		return SpoolMetricsBuilder{ToolName: def.Name, Config: spool, Storage: storage, Opener: opener}, nil
	}
}

func validateSpoolBounds(toolName string, config SpoolToolConfig) error {
	if config.MaxBytes < 0 {
		return fmt.Errorf("tool %q config max_bytes must not be negative", toolName)
	}
	if config.MaxFiles < 0 {
		return fmt.Errorf("tool %q config max_files must not be negative", toolName)
	}
	if config.MaxBytes > 0 && config.MaxFiles < 2 {
		return fmt.Errorf("tool %q config max_files must be at least 2 when max_bytes is set", toolName)
	}
	return nil
}

func decodeReceiverConfig(toolName string, raw ReceiverToolConfig) (ReceiverConfig, error) {
	if raw.Receiver == "" {
		return ReceiverConfig{}, fmt.Errorf("tool %q config requires receiver", toolName)
	}
	timeout := defaultShutdownTimeout
	if raw.ShutdownTimeout != "" {
		parsed, err := time.ParseDuration(raw.ShutdownTimeout)
		if err != nil {
			return ReceiverConfig{}, fmt.Errorf(
				"tool %q config has invalid shutdown_timeout %q", toolName, raw.ShutdownTimeout,
			)
		}
		timeout = parsed
	}
	return ReceiverConfig{
		Name: raw.Receiver, Address: raw.Address, QueueCapacity: raw.QueueCapacity,
		OverflowPolicy: OverflowPolicy(raw.OverflowPolicy), ShutdownTimeout: timeout,
		DrainPolicy: DrainPolicy(raw.DrainPolicy),
	}, nil
}
