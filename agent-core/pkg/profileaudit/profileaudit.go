// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package profileaudit inspects the executable timeout closure of one agent
// profile. Callers supply the profile so owner modules keep their inventory.
package profileaudit

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/corepath"
)

var inspectMu sync.Mutex

// Options supplies caller-owned path mappings used while loading a profile.
type Options struct {
	// CoreRoot maps references under /opt/agent-core for this inspection only.
	CoreRoot string
}

// Operation is one finite authority resolved from a reachable selected action.
type Operation struct {
	Profile        string
	Machine        string
	Action         string
	Authority      string
	RawDuration    string
	Duration       time.Duration
	CommandTimeout time.Duration
}

// Diagnostic describes one operation that violates the timeout envelope.
type Diagnostic struct {
	Operation
	Reason string
}

func (d Diagnostic) String() string {
	operation := d.Duration.String()
	if d.Duration <= 0 {
		operation = strconv.Quote(d.RawDuration)
	}
	return fmt.Sprintf(
		"profile %q machine %q action %q authority %q operation %s command_timeout %s: %s",
		d.Profile, d.Machine, d.Action, d.Authority, operation, d.CommandTimeout, d.Reason,
	)
}

// Report contains deterministic operation evidence and policy failures.
type Report struct {
	Operations  []Operation
	Diagnostics []Diagnostic
}

// ValidationError is returned when closure inspection found policy failures.
type ValidationError struct {
	Diagnostics []Diagnostic
}

func (e *ValidationError) Error() string {
	lines := make([]string, 0, len(e.Diagnostics))
	for _, diagnostic := range e.Diagnostics {
		lines = append(lines, diagnostic.String())
	}
	return "profile timeout closure validation: " + strings.Join(lines, "; ")
}

type inspector struct {
	visiting    map[string]bool
	visited     map[string]bool
	operations  []Operation
	diagnostics []Diagnostic
}

// Inspect resolves and checks the executable closure rooted at profilePath.
func Inspect(profilePath string) (Report, error) {
	return InspectWithOptions(profilePath, Options{})
}

// InspectWithOptions resolves and checks a profile with scoped caller options.
// Calls are serialized because the existing loaders consume a shared core-path
// mapper. The previous mapper value is restored before this function returns.
func InspectWithOptions(profilePath string, options Options) (Report, error) {
	inspectMu.Lock()
	defer inspectMu.Unlock()
	previousRoot := corepath.InstallRoot()
	if options.CoreRoot != "" {
		corepath.SetInstallRoot(options.CoreRoot)
		defer corepath.SetInstallRoot(previousRoot)
	}

	i := inspector{visiting: make(map[string]bool), visited: make(map[string]bool)}
	if err := i.inspectProfile(profilePath, ""); err != nil {
		return Report{}, err
	}
	sort.Slice(i.operations, func(a, b int) bool {
		return operationKey(i.operations[a]) < operationKey(i.operations[b])
	})
	sort.Slice(i.diagnostics, func(a, b int) bool {
		return diagnosticKey(i.diagnostics[a]) < diagnosticKey(i.diagnostics[b])
	})
	return Report{Operations: i.operations, Diagnostics: i.diagnostics}, nil
}

// InspectProfile is a descriptive alias for Inspect.
func InspectProfile(profilePath string) (Report, error) { return Inspect(profilePath) }

// Validate enforces the timeout closure policy for profilePath.
func Validate(profilePath string) error {
	return ValidateWithOptions(profilePath, Options{})
}

// ValidateWithOptions enforces the timeout closure with scoped caller options.
func ValidateWithOptions(profilePath string, options Options) error {
	report, err := InspectWithOptions(profilePath, options)
	if err != nil {
		return err
	}
	if len(report.Diagnostics) > 0 {
		return &ValidationError{Diagnostics: report.Diagnostics}
	}
	return nil
}

// ValidateProfile is a descriptive alias for Validate.
func ValidateProfile(profilePath string) error { return Validate(profilePath) }

func operationKey(operation Operation) string {
	return strings.Join([]string{
		operation.Profile, operation.Machine, operation.Action, operation.Authority,
		operation.RawDuration, operation.Duration.String(), operation.CommandTimeout.String(),
	}, "\x00")
}

func diagnosticKey(diagnostic Diagnostic) string {
	return operationKey(diagnostic.Operation) + "\x00" + diagnostic.Reason
}

var _ error = (*ValidationError)(nil)
