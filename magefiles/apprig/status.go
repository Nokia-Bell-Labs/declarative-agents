// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package apprig

// Component states a status probe reports. They are deliberately coarse: the
// runner reports what each surface is, separately, and never rolls them into
// one health verdict (#2479 R7).
const (
	StateOK       = "ok"
	StateDegraded = "degraded"
	StateAbsent   = "absent"
	StateUnknown  = "unknown"
)

// ComponentStatus is one surface's read-only state and a human detail.
type ComponentStatus struct {
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
}

// StatusReport is the deterministic, read-only view app:status returns. Every
// surface srd008 separates is reported on its own, so a bucket outage never
// reads as a healthy release and a torn-down query endpoint never reads as
// lost data (#2479 R7, AC6; srd008 R7).
type StatusReport struct {
	Application   string          `json:"application"`
	Namespace     string          `json:"namespace"`
	Release       string          `json:"release"`
	Platform      ComponentStatus `json:"platform"`
	Workloads     ComponentStatus `json:"workloads"`
	Ingress       ComponentStatus `json:"ingress"`
	Collector     ComponentStatus `json:"collector"`
	WAL           ComponentStatus `json:"wal"`
	Bucket        ComponentStatus `json:"bucket"`
	QueryEndpoint ComponentStatus `json:"query_endpoint"`
}

// StatusProbes are the injected read-only probes AggregateStatus calls. Each is
// optional: a nil probe reports StateUnknown, so a caller wires only the
// surfaces it can observe and the report never invents a state. No probe
// mutates the cluster (#2479 R7).
type StatusProbes struct {
	Platform      func() ComponentStatus
	Workloads     func() ComponentStatus
	Ingress       func() ComponentStatus
	Collector     func() ComponentStatus
	WAL           func() ComponentStatus
	Bucket        func() ComponentStatus
	QueryEndpoint func() ComponentStatus
}

// AggregateStatus assembles the report from the resolved identity and the
// injected probes. It is pure: given the same probes it returns the same
// report, and it performs no I/O of its own.
func AggregateStatus(resolved Resolved, probes StatusProbes) StatusReport {
	return StatusReport{
		Application:   resolved.Application,
		Namespace:     resolved.Namespace,
		Release:       resolved.Release,
		Platform:      probeOrUnknown(probes.Platform),
		Workloads:     probeOrUnknown(probes.Workloads),
		Ingress:       probeOrUnknown(probes.Ingress),
		Collector:     probeOrUnknown(probes.Collector),
		WAL:           probeOrUnknown(probes.WAL),
		Bucket:        probeOrUnknown(probes.Bucket),
		QueryEndpoint: probeOrUnknown(probes.QueryEndpoint),
	}
}

func probeOrUnknown(probe func() ComponentStatus) ComponentStatus {
	if probe == nil {
		return ComponentStatus{State: StateUnknown}
	}
	status := probe()
	if status.State == "" {
		status.State = StateUnknown
	}
	return status
}
