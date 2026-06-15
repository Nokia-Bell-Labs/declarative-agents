// Copyright (c) 2026 Nokia. All rights reserved.

// Package monitor provides the embedded monitor store and recorder ports.
package monitor

import "time"

// InstrumentKind names the supported metric instrument families.
type InstrumentKind string

const (
	// InstrumentCounter records monotonic counts.
	InstrumentCounter InstrumentKind = "counter"
	// InstrumentUpDownCounter records values that may increase or decrease.
	InstrumentUpDownCounter InstrumentKind = "up_down_counter"
	// InstrumentHistogram records distribution samples.
	InstrumentHistogram InstrumentKind = "histogram"
	// InstrumentGauge records the latest observed value.
	InstrumentGauge InstrumentKind = "gauge"
)

// Limits bounds retained monitor data.
type Limits struct {
	Events      int
	Samples     int
	Diagnostics int
	Errors      int
}

// MetricSchema describes one known monitor metric stream.
type MetricSchema struct {
	Name        string
	Kind        InstrumentKind
	Unit        string
	Description string
	Attributes  []string
}

// MetricSample is a normalized monitor metric sample.
type MetricSample struct {
	Name        string
	Kind        InstrumentKind
	Unit        string
	Description string
	Value       float64
	ToolName    string
	RunID       string
	State       string
	Signal      string
	Status      string
	Attributes  map[string]string
	Timestamp   time.Time
}

// RunEvent is the monitor copy of one runtime dispatch event.
type RunEvent struct {
	Iteration   int
	Timestamp   time.Time
	CommandName string
	Signal      string
	FromState   string
	ToState     string
	Duration    time.Duration
	TokensIn    int
	TokensOut   int
}

// RunSnapshot captures current run state for monitor readers.
type RunSnapshot struct {
	RunID     string
	Status    string
	State     string
	Signal    string
	Iteration int
	UpdatedAt time.Time
}

// Diagnostic records monitor validation, store, or export warnings.
type Diagnostic struct {
	Stage     string
	Message   string
	Metric    string
	ToolName  string
	Timestamp time.Time
}

// RecentError records a bounded monitor-visible error summary.
type RecentError struct {
	Stage       string
	Message     string
	CommandName string
	Timestamp   time.Time
}

// ToolAggregate summarizes accepted samples for one tool.
type ToolAggregate struct {
	ToolName      string
	Dispatches    int
	Successes     int
	Failures      int
	Samples       int
	TotalDuration time.Duration
	LastSignal    string
	LastStatus    string
	UpdatedAt     time.Time
}

// MetricAggregate summarizes one metric stream.
type MetricAggregate struct {
	Name      string
	Kind      InstrumentKind
	Unit      string
	Count     int
	Sum       float64
	Min       float64
	Max       float64
	LastValue float64
	UpdatedAt time.Time
}

// Snapshot is a bounded point-in-time monitor view.
type Snapshot struct {
	Run               RunSnapshot
	Tools             map[string]ToolAggregate
	Metrics           map[string]MetricAggregate
	Schemas           map[string]MetricSchema
	RecentEvents      []RunEvent
	RecentSamples     []MetricSample
	Diagnostics       []Diagnostic
	RecentErrors      []RecentError
	ReadsExecuteTools bool
	ReadsWorkspace    bool
}

// DispatchContext carries runtime-owned identity for standard metrics.
type DispatchContext struct {
	RunID     string
	AgentName string
	State     string
	Iteration int
}
