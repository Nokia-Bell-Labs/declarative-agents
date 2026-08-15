// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package telemetry implements srd008-telemetry.
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// ExporterConfig controls which exporters NewRoot sets up.
// At least one exporter endpoint or FilePath must be non-empty.
type ExporterConfig struct {
	FilePath           string
	OTLPEndpoint       string
	MetricOTLPEndpoint string
}

type providerOptions struct {
	resource *resource.Resource
	spans    []sdktrace.SpanExporter
	metrics  []sdkmetric.Exporter
	file     *os.File
}

type exporterFactories struct {
	createTemp func(string, string) (*os.File, error)
	fileTrace  func(io.Writer) (sdktrace.SpanExporter, error)
	fileMetric func(io.Writer) (sdkmetric.Exporter, error)
	otlpTrace  func(string) (sdktrace.SpanExporter, error)
	otlpMetric func(string) (sdkmetric.Exporter, error)
}

type cleanupAction struct {
	name string
	run  func(context.Context) error
}

type setupCleanup []cleanupAction

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

// IsZero returns true if the Trace was not initialized by NewRoot.
func (t Trace) IsZero() bool { return t.tracer == nil }

// Event records a span event on the current span.
func (t Trace) Event(name string, attrs ...attribute.KeyValue) {
	trace.SpanFromContext(t.ctx).AddEvent(name, trace.WithAttributes(attrs...))
}

// SetAttributes sets attributes on the current span.
func (t Trace) SetAttributes(attrs ...attribute.KeyValue) {
	trace.SpanFromContext(t.ctx).SetAttributes(attrs...)
}

// RecordError records err on the current span and sets the span status
// to error.
func (t Trace) RecordError(err error) {
	span := trace.SpanFromContext(t.ctx)
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

// Context returns the underlying context.Context.
func (t Trace) Context() context.Context { return t.ctx }

// Meter returns the OpenTelemetry Meter.
func (t Trace) Meter() metric.Meter { return t.meter }

// NewTraceFromProvider wraps an existing TracerProvider into a Trace. Useful
// in tests where the provider is set up externally (e.g. with an in-memory
// exporter). The returned Trace has no meter; calling Meter() returns a
// noop meter.
func NewTraceFromProvider(tp trace.TracerProvider, serviceName string, ctx context.Context) Trace {
	tracer := tp.Tracer(serviceName)
	return Trace{tracer: tracer, ctx: ctx}
}

// NewRoot creates providers, starts a root span, and returns a Trace plus a
// shutdown function that flushes exporters. The caller defers shutdown.
//
// serviceName identifies the agent in OTel resource attributes, tracer,
// meter, and temp file prefix (e.g. "executor", "planner").
//
// buildProviders runs before the root span exists and cannot emit OTel
// events; failures at that stage are returned as errors and logged to
// stderr via log.Printf (pre-root boundary).
func NewRoot(serviceName, name string, cfg ExporterConfig, parentCtx context.Context) (Trace, func(), error) {
	if cfg.FilePath == "" && cfg.OTLPEndpoint == "" && cfg.MetricOTLPEndpoint == "" {
		return Trace{}, nil, fmt.Errorf("ExporterConfig: at least one exporter required")
	}

	res, err := newServiceResource(parentCtx, serviceName)
	if err != nil {
		return Trace{}, nil, err
	}

	// Pre-root boundary: buildProviders failures are log-only because
	// no span exists yet to record events on.
	tp, mp, file, err := buildProviders(cfg, res, serviceName)
	if err != nil {
		return Trace{}, nil, fmt.Errorf("telemetry setup: %w", err)
	}

	logExporterConfig(cfg)

	// Set the process providers and W3C propagator globally so spans started via
	// otel.Tracer(...) (the machine_request server span) export, request-scoped
	// runs can wrap the provider (NewTraceFromProvider), and cross-agent
	// traceparent propagation uses the standard context propagator (srd016).
	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	tracer := tp.Tracer(serviceName)
	meter := mp.Meter(serviceName)
	ctx, span := tracer.Start(parentCtx, name)

	span.SetAttributes(
		attribute.Bool("exporter.file_enabled", cfg.FilePath != ""),
		attribute.Bool("exporter.otlp_enabled", cfg.OTLPEndpoint != ""),
		attribute.Bool("exporter.metric_otlp_enabled", metricOTLPEndpoint(cfg) != ""),
	)

	shutdown := buildShutdown(tp, mp, file, cfg.FilePath, span)
	return Trace{tracer: tracer, ctx: ctx, meter: meter}, shutdown, nil
}

// newServiceResource builds the OTel resource for serviceName, merging
// env-derived attributes (OTEL_RESOURCE_ATTRIBUTES) with the explicit
// service name; the explicit name wins on conflict.
func newServiceResource(parentCtx context.Context, serviceName string) (*resource.Resource, error) {
	envResource, err := resource.New(parentCtx, resource.WithFromEnv())
	if err != nil {
		return nil, fmt.Errorf("telemetry resource: %w", err)
	}
	explicitResource := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceNameKey.String(serviceName),
	)
	res, err := resource.Merge(envResource, explicitResource)
	if err != nil {
		return nil, fmt.Errorf("telemetry resource merge: %w", err)
	}
	return res, nil
}

func logExporterConfig(cfg ExporterConfig) {
	if cfg.FilePath != "" {
		log.Printf("telemetry: file exporter -> %s", cfg.FilePath)
	}
	if cfg.OTLPEndpoint != "" {
		log.Printf("telemetry: OTLP trace exporter -> %s", cfg.OTLPEndpoint)
	}
	if endpoint := metricOTLPEndpoint(cfg); endpoint != "" {
		log.Printf("telemetry: OTLP metric exporter -> %s", endpoint)
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
				if err := file.Close(); err != nil {
					log.Printf("telemetry: close %s: %v", tmpName, err)
				}
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
	serviceName string,
) (*sdktrace.TracerProvider, *sdkmetric.MeterProvider, *os.File, error) {
	return buildProvidersWithFactories(cfg, res, serviceName, defaultExporterFactories())
}

func buildProvidersWithFactories(
	cfg ExporterConfig,
	res *resource.Resource,
	serviceName string,
	factories exporterFactories,
) (*sdktrace.TracerProvider, *sdkmetric.MeterProvider, *os.File, error) {
	options := newProviderOptions(res)
	var cleanup setupCleanup
	if err := options.addFileExporters(cfg.FilePath, serviceName, factories, &cleanup); err != nil {
		return nil, nil, nil, cleanup.rollback(err)
	}
	if err := options.addOTLPTraceExporter(cfg.OTLPEndpoint, factories, &cleanup); err != nil {
		return nil, nil, nil, cleanup.rollback(err)
	}
	if err := options.addOTLPMetricExporter(metricOTLPEndpoint(cfg), factories, &cleanup); err != nil {
		return nil, nil, nil, cleanup.rollback(err)
	}
	tp, mp := options.providers()
	cleanup = nil
	return tp, mp, options.file, nil
}

func newProviderOptions(res *resource.Resource) *providerOptions {
	return &providerOptions{resource: res}
}

func (o *providerOptions) providers() (*sdktrace.TracerProvider, *sdkmetric.MeterProvider) {
	traceOptions := []sdktrace.TracerProviderOption{sdktrace.WithResource(o.resource)}
	for _, exporter := range o.spans {
		traceOptions = append(traceOptions, sdktrace.WithBatcher(exporter))
	}
	metricOptions := []sdkmetric.Option{sdkmetric.WithResource(o.resource)}
	for _, exporter := range o.metrics {
		reader := sdkmetric.NewPeriodicReader(exporter)
		metricOptions = append(metricOptions, sdkmetric.WithReader(reader))
	}
	return sdktrace.NewTracerProvider(traceOptions...), sdkmetric.NewMeterProvider(metricOptions...)
}

func (o *providerOptions) addFileExporters(
	path, serviceName string,
	factories exporterFactories,
	cleanup *setupCleanup,
) error {
	if path == "" {
		return nil
	}
	file, traceExp, metricExp, err := fileExporters(path, serviceName, factories, cleanup)
	if err != nil {
		return err
	}
	o.file = file
	o.spans = append(o.spans, traceExp)
	o.metrics = append(o.metrics, metricExp)
	return nil
}

func (o *providerOptions) addOTLPTraceExporter(
	endpoint string,
	factories exporterFactories,
	cleanup *setupCleanup,
) error {
	if endpoint == "" {
		return nil
	}
	exporter, err := factories.otlpTrace(endpoint)
	if err != nil {
		return err
	}
	cleanup.add("OTLP trace exporter", exporter.Shutdown)
	o.spans = append(o.spans, exporter)
	return nil
}

func (o *providerOptions) addOTLPMetricExporter(
	endpoint string,
	factories exporterFactories,
	cleanup *setupCleanup,
) error {
	if endpoint == "" {
		return nil
	}
	exporter, err := factories.otlpMetric(endpoint)
	if err != nil {
		return err
	}
	cleanup.add("OTLP metric exporter", exporter.Shutdown)
	o.metrics = append(o.metrics, exporter)
	return nil
}

// fileExporters writes to a temp file in the same directory; buildShutdown
// renames it to the final path for atomic delivery (srd007 R6.2).
// Pre-root boundary: failures here are returned as errors, not traced.
func fileExporters(
	path, serviceName string,
	factories exporterFactories,
	cleanup *setupCleanup,
) (
	*os.File, sdktrace.SpanExporter, sdkmetric.Exporter, error,
) {
	dir := filepath.Dir(path)
	f, err := factories.createTemp(dir, fmt.Sprintf(".%s-trace-*.tmp", serviceName))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create trace temp file in %s: %w", dir, err)
	}
	cleanup.add("temporary trace file", func(context.Context) error {
		return closeAndRemove(f)
	})
	traceExp, err := factories.fileTrace(f)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("trace exporter: %w", err)
	}
	cleanup.add("file trace exporter", traceExp.Shutdown)
	metricExp, err := factories.fileMetric(f)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("metric exporter: %w", err)
	}
	cleanup.add("file metric exporter", metricExp.Shutdown)
	return f, traceExp, metricExp, nil
}

func defaultExporterFactories() exporterFactories {
	return exporterFactories{
		createTemp: os.CreateTemp,
		fileTrace: func(w io.Writer) (sdktrace.SpanExporter, error) {
			return stdouttrace.New(stdouttrace.WithWriter(w))
		},
		fileMetric: func(w io.Writer) (sdkmetric.Exporter, error) {
			return stdoutmetric.New(stdoutmetric.WithWriter(w))
		},
		otlpTrace:  otlpTraceExporter,
		otlpMetric: otlpMetricExporter,
	}
}

func (c *setupCleanup) add(name string, run func(context.Context) error) {
	*c = append(*c, cleanupAction{name: name, run: run})
}

func (c *setupCleanup) rollback(setupErr error) error {
	errs := []error{setupErr}
	for i := len(*c) - 1; i >= 0; i-- {
		action := (*c)[i]
		if err := action.run(context.Background()); err != nil {
			errs = append(errs, fmt.Errorf("cleanup %s: %w", action.name, err))
		}
	}
	*c = nil
	return errors.Join(errs...)
}

func closeAndRemove(file *os.File) error {
	closeErr := file.Close()
	removeErr := os.Remove(file.Name())
	if os.IsNotExist(removeErr) {
		removeErr = nil
	}
	return errors.Join(closeErr, removeErr)
}

func metricOTLPEndpoint(cfg ExporterConfig) string {
	if cfg.MetricOTLPEndpoint != "" {
		return cfg.MetricOTLPEndpoint
	}
	return cfg.OTLPEndpoint
}

func otlpTraceExporter(endpoint string) (sdktrace.SpanExporter, error) {
	ctx := context.Background()
	traceExp, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("OTLP trace exporter: %w", err)
	}
	return traceExp, nil
}

func otlpMetricExporter(endpoint string) (sdkmetric.Exporter, error) {
	ctx := context.Background()
	metricExp, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(endpoint),
		otlpmetricgrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("OTLP metric exporter: %w", err)
	}
	return metricExp, nil
}
