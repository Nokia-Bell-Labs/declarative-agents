// Copyright (c) 2026 Nokia. All rights reserved.

package otlp

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

const (
	// InitSpoolSpans identifies the deterministic trace spool factory.
	InitSpoolSpans     = "spool_spans"
	defaultBatchSource = "$.batch"
)

// SpoolConfig configures deterministic trace evidence.
type SpoolConfig struct {
	Path        string
	BatchSource string
	MaxBytes    int64
	MaxFiles    int
}

// SpoolBuilder constructs trace spool commands.
type SpoolBuilder struct {
	ToolName string
	Config   SpoolConfig
}

// Build captures the previous result for current-value selectors.
func (b SpoolBuilder) Build(previous core.Result) core.Command {
	return &spoolCommand{toolName: b.ToolName, config: b.Config, previous: previous}
}

type spoolCommand struct {
	toolName string
	config   SpoolConfig
	previous core.Result
	view     core.CommandStateView
}

func (c *spoolCommand) Name() string { return c.toolName }

func (c *spoolCommand) SetCommandState(view core.CommandStateView) { c.view = view }

func (c *spoolCommand) Execute() core.Result {
	request, err := resolveBatch(c.config.BatchSource, c.previous.Output, c.view)
	if err != nil {
		return receiverError(c.Name(), fmt.Errorf("%s: %w", c.Name(), err))
	}
	lines, err := encodeStdoutTrace(request)
	if err != nil {
		return receiverError(c.Name(), fmt.Errorf("%s: %w", c.Name(), err))
	}
	written, err := appendSpool(c.config, lines)
	if err != nil {
		return receiverError(c.Name(), fmt.Errorf("%s: %w", c.Name(), err))
	}
	payload, err := protojson.Marshal(request)
	if err != nil {
		return receiverError(c.Name(), err)
	}
	output := struct {
		Path      string          `json:"path"`
		SpanCount int             `json:"span_count"`
		Bytes     int             `json:"bytes_written"`
		Batch     json.RawMessage `json:"batch"`
	}{
		Path: c.config.Path, SpanCount: requestSpanCount(request), Bytes: written, Batch: payload,
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return receiverError(c.Name(), err)
	}
	return core.Result{Signal: core.Signal("SpansSpooled"), CommandName: c.Name(), Output: string(encoded)}
}

func (c *spoolCommand) Undo(_ core.Result) core.Result {
	return core.NoopUndo(c.Name())
}

var _ core.CommandStateAware = (*spoolCommand)(nil)

func resolveBatch(
	selector string,
	previous string,
	view core.CommandStateView,
) (*coltracepb.ExportTraceServiceRequest, error) {
	payload, err := resolveSelectorPayload(selector, previous, view)
	if err != nil {
		return nil, err
	}
	var request coltracepb.ExportTraceServiceRequest
	if err := protojson.Unmarshal(payload, &request); err != nil {
		return nil, fmt.Errorf("decode selected OTLP batch: %w", err)
	}
	return &request, nil
}

// resolveSelectorPayload resolves a batch_source selector against the current
// command state or the previous output and returns the selected value as JSON.
// Both the trace and metric spool words decode this payload into their own
// OTLP request type.
func resolveSelectorPayload(
	selector string,
	previous string,
	view core.CommandStateView,
) ([]byte, error) {
	if selector == "" {
		selector = defaultBatchSource
	}
	parsed, ok := core.ParseSelector(selector)
	if !ok {
		return nil, fmt.Errorf("batch_source %q is not a valid selector", selector)
	}
	var value any
	if parsed.Label != "" {
		resolved, err := core.ResolveFromSelector(view, selector)
		if err != nil {
			return nil, err
		}
		value = resolved
	} else {
		var source map[string]any
		if err := json.Unmarshal([]byte(previous), &source); err != nil {
			return nil, fmt.Errorf("previous output is not a JSON object")
		}
		resolved, found := parsed.Resolve(source)
		if !found {
			return nil, fmt.Errorf("batch_source %q did not resolve against previous output", selector)
		}
		value = resolved
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode selected batch: %w", err)
	}
	return payload, nil
}

var spoolLocks sync.Map

func appendSpool(config SpoolConfig, lines []byte) (int, error) {
	if config.Path == "" {
		return 0, fmt.Errorf("spool path is required")
	}
	lockValue, _ := spoolLocks.LoadOrStore(config.Path, &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	if err := os.MkdirAll(filepath.Dir(config.Path), 0o755); err != nil {
		return 0, fmt.Errorf("create spool directory: %w", err)
	}
	if err := rotateBeforeAppend(config, int64(len(lines))); err != nil {
		return 0, err
	}
	file, err := os.OpenFile(config.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return 0, fmt.Errorf("open spool %s: %w", config.Path, err)
	}
	written, writeErr := file.Write(lines)
	syncErr := file.Sync()
	closeErr := file.Close()
	switch {
	case writeErr != nil:
		return written, fmt.Errorf("append spool %s: %w", config.Path, writeErr)
	case written != len(lines):
		return written, fmt.Errorf("append spool %s: short write %d of %d", config.Path, written, len(lines))
	case syncErr != nil:
		return written, fmt.Errorf("sync spool %s: %w", config.Path, syncErr)
	case closeErr != nil:
		return written, fmt.Errorf("close spool %s: %w", config.Path, closeErr)
	default:
		return written, nil
	}
}

func rotateBeforeAppend(config SpoolConfig, incoming int64) error {
	if config.MaxBytes <= 0 {
		return nil
	}
	info, err := os.Stat(config.Path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat spool %s: %w", config.Path, err)
	}
	if info.Size() == 0 || info.Size()+incoming <= config.MaxBytes {
		return nil
	}
	maxFiles := config.MaxFiles
	if maxFiles <= 0 {
		maxFiles = 1
	}
	if maxFiles == 1 {
		if err := os.Remove(config.Path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("rotate spool %s: %w", config.Path, err)
		}
		return nil
	}
	for index := maxFiles - 1; index >= 1; index-- {
		source := rotatedPath(config.Path, index-1)
		if index == 1 {
			source = config.Path
		}
		target := rotatedPath(config.Path, index)
		if index == maxFiles-1 {
			_ = os.Remove(target)
		}
		if err := os.Rename(source, target); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("rotate spool %s to %s: %w", source, target, err)
		}
	}
	return nil
}

func rotatedPath(path string, generation int) string {
	return fmt.Sprintf("%s.%d", path, generation)
}

func encodeStdoutTrace(request *coltracepb.ExportTraceServiceRequest) ([]byte, error) {
	var output strings.Builder
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	for _, resourceSpans := range request.GetResourceSpans() {
		resource := stdoutAttributes(resourceSpans.GetResource().GetAttributes())
		for _, scopeSpans := range resourceSpans.GetScopeSpans() {
			scope := map[string]any{
				"Name":       scopeSpans.GetScope().GetName(),
				"Version":    scopeSpans.GetScope().GetVersion(),
				"SchemaURL":  scopeSpans.GetSchemaUrl(),
				"Attributes": stdoutAttributes(scopeSpans.GetScope().GetAttributes()),
			}
			for _, span := range scopeSpans.GetSpans() {
				if err := encoder.Encode(stdoutSpan(span, resource, scope)); err != nil {
					return nil, fmt.Errorf("encode span %q: %w", span.GetName(), err)
				}
			}
		}
	}
	return []byte(output.String()), nil
}

func stdoutSpan(span *tracepb.Span, resource []map[string]any, scope map[string]any) map[string]any {
	return map[string]any{
		"Name": span.GetName(),
		"SpanContext": stdoutSpanContext(
			span.GetTraceId(), span.GetSpanId(), span.GetFlags(), span.GetTraceState(),
		),
		"Parent":                 stdoutSpanContext(span.GetTraceId(), span.GetParentSpanId(), span.GetFlags(), ""),
		"SpanKind":               int(span.GetKind()),
		"StartTime":              unixNanoTime(span.GetStartTimeUnixNano()),
		"EndTime":                unixNanoTime(span.GetEndTimeUnixNano()),
		"Attributes":             stdoutAttributes(span.GetAttributes()),
		"Events":                 stdoutSpanEvents(span.GetEvents()),
		"Links":                  stdoutSpanLinks(span.GetLinks()),
		"Status":                 stdoutSpanStatus(span.GetStatus()),
		"DroppedAttributes":      span.GetDroppedAttributesCount(),
		"DroppedEvents":          span.GetDroppedEventsCount(),
		"DroppedLinks":           span.GetDroppedLinksCount(),
		"ChildSpanCount":         0,
		"Resource":               resource,
		"InstrumentationScope":   scope,
		"InstrumentationLibrary": scope,
	}
}

func stdoutSpanEvents(spanEvents []*tracepb.Span_Event) []map[string]any {
	events := make([]map[string]any, 0, len(spanEvents))
	for _, event := range spanEvents {
		events = append(events, map[string]any{
			"Name": event.GetName(), "Attributes": stdoutAttributes(event.GetAttributes()),
			"DroppedAttributeCount": event.GetDroppedAttributesCount(),
			"Time":                  unixNanoTime(event.GetTimeUnixNano()),
		})
	}
	return events
}

func stdoutSpanLinks(spanLinks []*tracepb.Span_Link) []map[string]any {
	links := make([]map[string]any, 0, len(spanLinks))
	for _, link := range spanLinks {
		links = append(links, map[string]any{
			"SpanContext": stdoutSpanContext(
				link.GetTraceId(), link.GetSpanId(), link.GetFlags(), link.GetTraceState(),
			),
			"Attributes":            stdoutAttributes(link.GetAttributes()),
			"DroppedAttributeCount": link.GetDroppedAttributesCount(),
		})
	}
	return links
}

func stdoutSpanStatus(status *tracepb.Status) map[string]any {
	return map[string]any{
		"Code":        int(status.GetCode()),
		"Description": status.GetMessage(),
	}
}

func stdoutSpanContext(traceID, spanID []byte, flags uint32, traceState string) map[string]any {
	return map[string]any{
		"TraceID": hex.EncodeToString(traceID), "SpanID": hex.EncodeToString(spanID),
		"TraceFlags": fmt.Sprintf("%02x", flags&0xff), "TraceState": traceState, "Remote": false,
	}
}

func stdoutAttributes(attributes []*commonpb.KeyValue) []map[string]any {
	sorted := append([]*commonpb.KeyValue(nil), attributes...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].GetKey() < sorted[j].GetKey()
	})
	output := make([]map[string]any, 0, len(sorted))
	for _, attribute := range sorted {
		valueType, value := stdoutAttributeValue(attribute.GetValue())
		output = append(output, map[string]any{
			"Key":   attribute.GetKey(),
			"Value": map[string]any{"Type": valueType, "Value": value},
		})
	}
	return output
}

func stdoutAttributeValue(value *commonpb.AnyValue) (string, any) {
	if value == nil {
		return "INVALID", nil
	}
	switch typed := value.Value.(type) {
	case *commonpb.AnyValue_StringValue:
		return "STRING", typed.StringValue
	case *commonpb.AnyValue_BoolValue:
		return "BOOL", typed.BoolValue
	case *commonpb.AnyValue_IntValue:
		return "INT64", typed.IntValue
	case *commonpb.AnyValue_DoubleValue:
		return "FLOAT64", typed.DoubleValue
	case *commonpb.AnyValue_BytesValue:
		return "STRING", hex.EncodeToString(typed.BytesValue)
	case *commonpb.AnyValue_ArrayValue:
		return stdoutArrayValue(typed.ArrayValue.GetValues())
	default:
		return "INVALID", anyValue(value)
	}
}

func stdoutArrayValue(values []*commonpb.AnyValue) (string, any) {
	if len(values) == 0 {
		return "STRINGSLICE", []string{}
	}
	valueType, _ := stdoutAttributeValue(values[0])
	sliceType := valueType + "SLICE"
	output := make([]any, 0, len(values))
	for _, value := range values {
		currentType, current := stdoutAttributeValue(value)
		if currentType != valueType || currentType == "INVALID" {
			return "INVALID", anyValue(&commonpb.AnyValue{
				Value: &commonpb.AnyValue_ArrayValue{
					ArrayValue: &commonpb.ArrayValue{Values: values},
				},
			})
		}
		output = append(output, current)
	}
	return sliceType, output
}

func unixNanoTime(value uint64) time.Time {
	return time.Unix(0, int64(value)).UTC()
}
