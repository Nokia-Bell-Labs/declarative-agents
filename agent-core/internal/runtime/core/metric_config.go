// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"fmt"
	"regexp"
	"strings"
)

// MetricLabels are trusted low-cardinality workflow labels from machine YAML.
type MetricLabels map[string]string

// MetricConfig declares planned tool metric instruments and attributes.
type MetricConfig struct {
	Instruments []MetricInstrument `yaml:"instruments,omitempty"`
	Attributes  []MetricAttribute  `yaml:"attributes,omitempty"`
	Disabled    bool               `yaml:"disabled,omitempty"`
}

// MetricInstrument declares one tool-owned metric stream.
type MetricInstrument struct {
	Name        string    `yaml:"name"`
	Kind        string    `yaml:"kind"`
	Unit        string    `yaml:"unit,omitempty"`
	Description string    `yaml:"description"`
	ValueSource string    `yaml:"value_source"`
	Attributes  []string  `yaml:"attributes,omitempty"`
	Buckets     []float64 `yaml:"buckets,omitempty"`
}

// MetricAttribute declares one low-cardinality metric label.
type MetricAttribute struct {
	Name          string   `yaml:"name"`
	Source        string   `yaml:"source"`
	Cardinality   string   `yaml:"cardinality"`
	AllowedValues []string `yaml:"allowed_values,omitempty"`
	Redaction     string   `yaml:"redaction,omitempty"`
}

var metricNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.]*$`)

var allowedMetricKinds = stringSet("counter", "up_down_counter", "histogram", "gauge")

var allowedAttributeSources = stringSet(
	"config_literal", "tool_name", "tool_family", "state", "signal",
	"status", "error_class", "workflow_label", "configured_operation",
)

var prohibitedMetricSources = stringSet(
	"raw_prompt", "full_model_response", "full_tool_output", "secret",
	"arbitrary_url", "request_id", "timestamp", "stack_trace",
	"command_output", "user_free_text",
)

// ValidateMetricConfig checks ToolDef metrics declarations before recording starts.
func ValidateMetricConfig(owner string, cfg MetricConfig) error {
	var errs []string
	for i, instrument := range cfg.Instruments {
		errs = append(errs, validateMetricInstrument(owner, i, instrument)...)
	}
	for i, attr := range cfg.Attributes {
		errs = append(errs, validateMetricAttribute(owner, i, attr)...)
	}
	if len(errs) > 0 {
		return fmt.Errorf("metric config validation: %s", strings.Join(errs, "; "))
	}
	return nil
}

// ValidateMetricLabels checks trusted workflow labels on machines and transitions.
func ValidateMetricLabels(owner string, labels MetricLabels) error {
	var errs []string
	for name, value := range labels {
		errs = append(errs, validateMetricLabel(owner, name, value)...)
	}
	if len(errs) > 0 {
		return fmt.Errorf("metric label validation: %s", strings.Join(errs, "; "))
	}
	return nil
}

func validateMetricInstrument(owner string, index int, inst MetricInstrument) []string {
	prefix := fmt.Sprintf("%s.metrics.instruments[%d]", owner, index)
	var errs []string
	errs = append(errs, validateMetricName(prefix+".name", inst.Name)...)
	if !allowedMetricKinds[inst.Kind] {
		errs = append(errs, fmt.Sprintf("%s.kind %q is unsupported", prefix, inst.Kind))
	}
	errs = append(errs, validateSource(prefix+".value_source", inst.ValueSource)...)
	if inst.Description == "" {
		errs = append(errs, prefix+".description is required")
	}
	for _, attr := range inst.Attributes {
		errs = append(errs, validateMetricName(prefix+".attributes", attr)...)
	}
	return errs
}

func validateMetricAttribute(owner string, index int, attr MetricAttribute) []string {
	prefix := fmt.Sprintf("%s.metrics.attributes[%d]", owner, index)
	var errs []string
	errs = append(errs, validateMetricName(prefix+".name", attr.Name)...)
	if !allowedAttributeSources[attr.Source] {
		errs = append(errs, fmt.Sprintf("%s.source %q is unsupported", prefix, attr.Source))
	}
	if attr.Cardinality != "low" && attr.Cardinality != "bounded" {
		errs = append(errs, fmt.Sprintf("%s.cardinality %q is not low or bounded", prefix, attr.Cardinality))
	}
	if attr.Cardinality == "bounded" && len(attr.AllowedValues) == 0 {
		errs = append(errs, prefix+".allowed_values is required for bounded cardinality")
	}
	if attr.Redaction != "" && !validRedaction(attr.Redaction) {
		errs = append(errs, fmt.Sprintf("%s.redaction %q is unsupported", prefix, attr.Redaction))
	}
	return errs
}

func validateMetricLabel(owner, name, value string) []string {
	var errs []string
	errs = append(errs, validateMetricName(owner, name)...)
	if value == "" || len(value) > 128 || prohibitedMetricSources[value] {
		errs = append(errs, fmt.Sprintf("%s.%s value %q is not a safe metric label", owner, name, value))
	}
	return errs
}

func validateMetricName(field, name string) []string {
	if name == "" || !metricNamePattern.MatchString(name) {
		return []string{fmt.Sprintf("%s %q is not a valid metric name", field, name)}
	}
	if prohibitedMetricSources[name] {
		return []string{fmt.Sprintf("%s %q is an unsafe metric source", field, name)}
	}
	return nil
}

func validateSource(field, source string) []string {
	if source == "" || !metricNamePattern.MatchString(source) || prohibitedMetricSources[source] {
		return []string{fmt.Sprintf("%s %q is not a safe source selector", field, source)}
	}
	return nil
}

func validRedaction(redaction string) bool {
	switch redaction {
	case "omit", "hash", "classify", "none":
		return true
	default:
		return false
	}
}

func stringSet(values ...string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}
