// Copyright (c) 2026 Nokia. All rights reserved.

package monitor

func cloneSample(sample MetricSample) MetricSample {
	sample.Attributes = cloneStringMap(sample.Attributes)
	return sample
}

func cloneSamples(samples []MetricSample) []MetricSample {
	if len(samples) == 0 {
		return nil
	}
	out := make([]MetricSample, 0, len(samples))
	for _, sample := range samples {
		out = append(out, cloneSample(sample))
	}
	return out
}

func cloneSchema(schema MetricSchema) MetricSchema {
	schema.Attributes = append([]string(nil), schema.Attributes...)
	return schema
}

func cloneSchemas(schemas map[string]MetricSchema) map[string]MetricSchema {
	if len(schemas) == 0 {
		return nil
	}
	out := make(map[string]MetricSchema, len(schemas))
	for name, schema := range schemas {
		out[name] = cloneSchema(schema)
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneToolAggregates(in map[string]ToolAggregate) map[string]ToolAggregate {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]ToolAggregate, len(in))
	for name, agg := range in {
		out[name] = agg
	}
	return out
}

func cloneMetricAggregates(in map[string]MetricAggregate) map[string]MetricAggregate {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]MetricAggregate, len(in))
	for name, agg := range in {
		out[name] = agg
	}
	return out
}
