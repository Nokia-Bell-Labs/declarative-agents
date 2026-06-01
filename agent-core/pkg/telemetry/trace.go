// Copyright (c) 2026 Nokia. All rights reserved.

// Package telemetry implements srd008-telemetry.
package telemetry

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// ExporterConfig controls which exporters NewRoot sets up.
// At least one of FilePath or OTLPEndpoint must be non-empty.
type ExporterConfig struct {
	FilePath     string
	OTLPEndpoint string
}

// Trace bundles an OpenTelemetry tracer, a context carrying the active span,
// and a meter. Immutable after construction; Push returns a new Trace.
type Trace struct {
	tracer trace.Tracer
	ctx    context.Context
	meter  metric.Meter
}

// Push starts a child span and returns a new Trace scoped to it plus a done
// function. Callers write: child, done := t.Push("name"); defer done()
func (t Trace) Push(name string, attrs ...attribute.KeyValue) (Trace, func()) {
	ctx, span := t.tracer.Start(t.ctx, name, trace.WithAttributes(attrs...))
	child := Trace{tracer: t.tracer, ctx: ctx, meter: t.meter}
	var once sync.Once
	done := func() { once.Do(func() { span.End() }) }
	return child, done
}

// Event records a span event on the current span.
func (t Trace) Event(name string, attrs ...attribute.KeyValue) {
	trace.SpanFromContext(t.ctx).AddEvent(name, trace.WithAttributes(attrs...))
}

// SetAttributes sets attributes on the current span.
func (t Trace) SetAttributes(attrs ...attribute.KeyValue) {
	trace.SpanFromContext(t.ctx).SetAttributes(attrs...)
}

// Context returns the underlying context.Context.
func (t Trace) Context() context.Context { return t.ctx }

// Meter returns the OpenTelemetry Meter.
func (t Trace) Meter() metric.Meter { return t.meter }

// NewRoot creates providers, starts a root span, and returns a Trace plus a
// shutdown function that flushes exporters. The caller defers shutdown.
//
// buildProviders runs before the root span exists and cannot emit OTel
// events; failures at that stage are returned as errors and logged to
// stderr via log.Printf (pre-root boundary).
func NewRoot(name string, cfg ExporterConfig, parentCtx context.Context) (Trace, func(), error) {
	if cfg.FilePath == "" && cfg.OTLPEndpoint == "" {
		return Trace{}, nil, fmt.Errorf("ExporterConfig: at least one exporter required")
	}

	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceNameKey.String("harness"),
	)

	// Pre-root boundary: buildProviders failures are log-only because
	// no span exists yet to record events on.
	tp, mp, file, err := buildProviders(cfg, res)
	if err != nil {
		return Trace{}, nil, fmt.Errorf("telemetry setup: %w", err)
	}

	logExporterConfig(cfg)

	tracer := tp.Tracer("harness")
	meter := mp.Meter("harness")
	ctx, span := tracer.Start(parentCtx, name)

	span.SetAttributes(
		attribute.Bool("exporter.file_enabled", cfg.FilePath != ""),
		attribute.Bool("exporter.otlp_enabled", cfg.OTLPEndpoint != ""),
	)

	shutdown := buildShutdown(tp, mp, file, cfg.FilePath, span)
	return Trace{tracer: tracer, ctx: ctx, meter: meter}, shutdown, nil
}

func logExporterConfig(cfg ExporterConfig) {
	if cfg.FilePath != "" {
		log.Printf("telemetry: file exporter -> %s", cfg.FilePath)
	}
	if cfg.OTLPEndpoint != "" {
		log.Printf("telemetry: OTLP exporter -> %s", cfg.OTLPEndpoint)
	}
}

func buildShutdown(
	tp *sdktrace.TracerProvider,
	mp *sdkmetric.MeterProvider,
	file *os.File,
	finalPath string,
	rootSpan trace.Span,
) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			// Record shutdown events on the root span before ending it,
			// so they appear in the trace output.
			if err := tp.ForceFlush(context.Background()); err != nil {
				rootSpan.AddEvent("shutdown.trace_flush_error",
					trace.WithAttributes(attribute.String("error", err.Error())),
				)
				log.Printf("telemetry: trace flush error: %v", err)
			}

			rootSpan.End()

			if err := tp.Shutdown(context.Background()); err != nil {
				log.Printf("telemetry: trace shutdown error: %v", err)
			}
			if err := mp.Shutdown(context.Background()); err != nil {
				log.Printf("telemetry: metric shutdown error: %v", err)
			}
			if file != nil {
				tmpName := file.Name()
				file.Close()
				if err := os.Rename(tmpName, finalPath); err != nil {
					log.Printf("telemetry: rename %s -> %s: %v", tmpName, finalPath, err)
				}
			}
			log.Print("telemetry: shutdown complete")
		})
	}
}

// buildProviders creates trace and metric providers. This runs before the
// root span exists (pre-root boundary), so failures are returned as errors
// and logged to stderr. OTel events cannot be emitted here.
func buildProviders(
	cfg ExporterConfig,
	res *resource.Resource,
) (*sdktrace.TracerProvider, *sdkmetric.MeterProvider, *os.File, error) {
	var spanOpts []sdktrace.TracerProviderOption
	var metricOpts []sdkmetric.Option
	var file *os.File

	spanOpts = append(spanOpts, sdktrace.WithResource(res))
	metricOpts = append(metricOpts, sdkmetric.WithResource(res))

	if cfg.FilePath != "" {
		f, traceExp, metricExp, err := fileExporters(cfg.FilePath)
		if err != nil {
			return nil, nil, nil, err
		}
		file = f
		spanOpts = append(spanOpts, sdktrace.WithBatcher(traceExp))
		metricOpts = append(metricOpts, sdkmetric.WithReader(
			sdkmetric.NewPeriodicReader(metricExp),
		))
	}

	if cfg.OTLPEndpoint != "" {
		traceExp, metricExp, err := otlpExporters(cfg.OTLPEndpoint)
		if err != nil {
			return nil, nil, nil, err
		}
		spanOpts = append(spanOpts, sdktrace.WithBatcher(traceExp))
		metricOpts = append(metricOpts, sdkmetric.WithReader(
			sdkmetric.NewPeriodicReader(metricExp),
		))
	}

	tp := sdktrace.NewTracerProvider(spanOpts...)
	mp := sdkmetric.NewMeterProvider(metricOpts...)
	return tp, mp, file, nil
}

// fileExporters writes to a temp file in the same directory; buildShutdown
// renames it to the final path for atomic delivery (srd007 R6.2).
// Pre-root boundary: failures here are returned as errors, not traced.
func fileExporters(path string) (
	*os.File, sdktrace.SpanExporter, sdkmetric.Exporter, error,
) {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".harness-trace-*.tmp")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create trace temp file in %s: %w", dir, err)
	}
	traceExp, err := stdouttrace.New(stdouttrace.WithWriter(f))
	if err != nil {
		f.Close()
		os.Remove(f.Name())
		return nil, nil, nil, fmt.Errorf("trace exporter: %w", err)
	}
	metricExp, err := stdoutmetric.New(stdoutmetric.WithWriter(f))
	if err != nil {
		f.Close()
		os.Remove(f.Name())
		return nil, nil, nil, fmt.Errorf("metric exporter: %w", err)
	}
	return f, traceExp, metricExp, nil
}

func otlpExporters(endpoint string) (sdktrace.SpanExporter, sdkmetric.Exporter, error) {
	ctx := context.Background()
	traceExp, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("OTLP trace exporter: %w", err)
	}
	metricExp, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(endpoint),
		otlpmetricgrpc.WithInsecure(),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("OTLP metric exporter: %w", err)
	}
	return traceExp, metricExp, nil
}
