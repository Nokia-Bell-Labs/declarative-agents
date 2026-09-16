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

	internalload "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/load"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/corepath"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolrest "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/typesys"
)

var inspectMu sync.Mutex

// Options supplies caller-owned path mappings used while loading a profile.
type Options struct {
	// CoreRoot maps references under /opt/agent-core for this inspection only.
	CoreRoot string
	// OnMachine, when set, receives every machine the profile reaches: its own,
	// and the request machine behind each machine_request endpoint. A caller
	// that checks machine wiring runs it here rather than on the profile's own
	// machine alone, which is all it can see from outside this walk.
	OnMachine func(ReachedMachine) error
}

// ReachedMachine is one machine a profile reaches, with what that machine
// selects. A request machine selects the actions its own transitions name, so
// its tools and its type registry are not the profile's.
type ReachedMachine struct {
	ProfilePath string
	MachinePath string
	Machine     core.MachineSpec
	Selected    []catalog.ToolDef
	Rest        toolrest.Collection
	Types       *typesys.Registry
	// RequestScoped reports a machine an endpoint dispatches rather than the
	// machine a profile runs. The runtime seeds such a machine differently:
	// srd038 R2.10 reserves the synthetic machine_request entry under label
	// seed, and the dispatching endpoint injects the signal it starts on.
	RequestScoped bool
	// InitialSignals are the signals injected into this machine, united over
	// every endpoint that dispatches it.
	InitialSignals []string
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
	onMachine   func(ReachedMachine) error
	reached     []loadedClosure
	injected    map[string]map[string]bool
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

	i := inspector{
		visiting: make(map[string]bool), visited: make(map[string]bool),
		onMachine: options.OnMachine,
	}
	if err := i.inspectProfile(profilePath, "", nil); err != nil {
		return Report{}, err
	}
	if err := i.flushReached(); err != nil {
		return Report{}, err
	}
	return i.report(), nil
}

// InspectClosure checks an already loaded root closure. Child profiles remain
// independently loaded because they are separate declarative programs.
func InspectClosure(closure *internalload.Closure) (Report, error) {
	return InspectClosureWithOptions(closure, Options{})
}

// InspectClosureWithOptions checks a loaded root closure with caller options,
// so a caller can see every machine the walk reaches.
func InspectClosureWithOptions(closure *internalload.Closure, options Options) (Report, error) {
	if closure == nil {
		return Report{}, fmt.Errorf("profile closure is nil")
	}
	inspectMu.Lock()
	defer inspectMu.Unlock()
	i := inspector{
		visiting: make(map[string]bool), visited: make(map[string]bool),
		onMachine: options.OnMachine,
	}
	if err := i.inspectClosure(closure); err != nil {
		return Report{}, err
	}
	if err := i.flushReached(); err != nil {
		return Report{}, err
	}
	return i.report(), nil
}

func (i *inspector) report() Report {
	sort.Slice(i.operations, func(a, b int) bool {
		return operationKey(i.operations[a]) < operationKey(i.operations[b])
	})
	sort.Slice(i.diagnostics, func(a, b int) bool {
		return diagnosticKey(i.diagnostics[a]) < diagnosticKey(i.diagnostics[b])
	})
	return Report{Operations: i.operations, Diagnostics: i.diagnostics}
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

// ValidateClosure enforces the timeout policy without reloading the root
// profile closure.
func ValidateClosure(closure *internalload.Closure) error {
	return ValidateClosureWithOptions(closure, Options{})
}

// ValidateClosureWithOptions enforces the timeout policy with caller options.
func ValidateClosureWithOptions(closure *internalload.Closure, options Options) error {
	report, err := InspectClosureWithOptions(closure, options)
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
