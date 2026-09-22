// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package apprig

import (
	"errors"
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

// Preparation is the application-owned artifact result consumed by the shared
// lifecycle. Returning it with an error lets the runner remove only resources
// that invocation created. Cleanup runs on success and failure.
type Preparation struct {
	ChartPath  string
	ValuesPath string
	Overrides  string
	Cleanup    func() error
	// OwnsNamespace reports that Prepare created the resolved namespace before
	// Runner's shared ensure step (needed by applications that provision
	// out-of-release ConfigMaps there). A later deploy failure may then remove
	// it; an existing namespace is never claimed.
	OwnsNamespace bool
}

// Runner binds one manifest to the canonical lifecycle agents. Applications
// supply typed artifact, verification, and status callbacks; they cannot
// replace deploy, undeploy, diagnosis, namespace ordering, or purge authority.
type Runner struct {
	ManifestPath string
	Binding      PlatformBinding
	CatalogRoot  string
	Revision     string
	TraceSpool   string

	Prepare func(Resolved) (Preparation, error)
	Verify  func(Resolved) error
	Probes  func(Resolved) StatusProbes
	// AfterDown performs application-owned cleanup that cannot live in the
	// release (for example a kind hostPath PV). It runs only after canonical
	// undeploy and apprig namespace deletion have both succeeded.
	AfterDown func(Resolved) error

	DeployAgent   func() (kindrig.DeployAgent, error)
	UndeployAgent func() (kindrig.DeployAgent, error)
	DiagnoseAgent func() (kindrig.DiagnoseAgent, error)

	// Purge delegates to the implementation approved by GH-2488. Nil is a
	// fail-closed result, never an imperative deletion fallback.
	Purge func(Resolved) error

	operations lifecycleOperations
}

type lifecycleOperations struct {
	deploy          func(kindrig.DeployRequest) error
	undeploy        func(kindrig.DeployRequest) error
	diagnose        func(kindrig.DiagnoseRequest) error
	ensureNamespace func(kindrig.ApplicationNamespaceRequest) (bool, error)
	deleteNamespace func(kindrig.ApplicationNamespaceRequest) error
}

func defaultLifecycleOperations() lifecycleOperations {
	return lifecycleOperations{
		deploy:          kindrig.Deploy,
		undeploy:        kindrig.Undeploy,
		diagnose:        kindrig.Diagnose,
		ensureNamespace: kindrig.EnsureApplicationNamespace,
		deleteNamespace: kindrig.DeleteApplicationNamespace,
	}
}

// ErrPurgeUnavailable is the explicit result until Runner.Purge is bound to
// the separately approved destructive authority.
var ErrPurgeUnavailable = errors.New("app:purge is unavailable: no approved purge authority is configured")

// Up prepares application-owned artifacts, creates or reuses the stable
// apprig-owned namespace, and delegates the release transaction to the
// canonical deploy machine. A post-deploy verification failure compensates
// through the canonical undeploy machine before namespace cleanup.
func (r Runner) Up() (result error) {
	resolved, err := r.resolve()
	if err != nil {
		return err
	}
	preparation := Preparation{
		ChartPath: resolved.Coordinates.ChartPath, ValuesPath: resolved.Coordinates.ValuesPath,
	}
	if r.Prepare != nil {
		preparation, err = r.Prepare(resolved)
	}
	if preparation.Cleanup != nil {
		defer func() { result = errors.Join(result, preparation.Cleanup()) }()
	}
	if err != nil {
		return fmt.Errorf("prepare %s: %w", resolved.Application, err)
	}
	if preparation.ChartPath == "" {
		preparation.ChartPath = resolved.Coordinates.ChartPath
	}
	if preparation.ValuesPath == "" {
		preparation.ValuesPath = resolved.Coordinates.ValuesPath
	}
	namespace := r.namespaceRequest(resolved)
	created, err := r.ops().ensureNamespace(namespace)
	if err != nil {
		return err
	}
	deployErr := r.deploy(resolved, preparation)
	if deployErr != nil {
		if created || preparation.OwnsNamespace {
			deployErr = errors.Join(deployErr, r.ops().deleteNamespace(namespace))
		}
		return deployErr
	}
	if r.Verify == nil {
		return nil
	}
	if err := r.Verify(resolved); err != nil {
		compensation := r.undeployAndCleanup(resolved)
		return errors.Join(fmt.Errorf("verify %s: %w", resolved.Application, err), compensation)
	}
	return nil
}

// Status returns the read-only, separated health report for one resolved
// application. Missing probes remain unknown rather than being invented.
func (r Runner) Status() (StatusReport, error) {
	resolved, err := r.resolve()
	if err != nil {
		return StatusReport{}, err
	}
	var probes StatusProbes
	if r.Probes != nil {
		probes = r.Probes(resolved)
	}
	return AggregateStatus(resolved, probes), nil
}

// Down reaches Removed or Absent through the canonical undeploy machine before
// deleting the apprig-owned namespace. It never invokes purge. The returned
// status lets a thin target report the retained bucket separately from the now
// absent collector query endpoint.
func (r Runner) Down() (StatusReport, error) {
	resolved, err := r.resolve()
	if err != nil {
		return StatusReport{}, err
	}
	if err := r.undeployAndCleanup(resolved); err != nil {
		return StatusReport{}, err
	}
	var probes StatusProbes
	if r.Probes != nil {
		probes = r.Probes(resolved)
	}
	return AggregateStatus(resolved, probes), nil
}

// Diagnose captures deterministic evidence and delegates interpretation to the
// canonical rig-doctor. A nil agent factory intentionally leaves capture-only
// evidence rather than introducing a second diagnosis engine.
func (r Runner) Diagnose() error {
	resolved, err := r.resolve()
	if err != nil {
		return err
	}
	var agent kindrig.DiagnoseAgent
	if r.DiagnoseAgent != nil {
		agent, err = r.DiagnoseAgent()
		if err != nil {
			return err
		}
	}
	return r.ops().diagnose(kindrig.DiagnoseRequest{
		Scenario: resolved.Application, ApplicationRoot: r.Binding.ApplicationRoot,
		Revision: r.Revision, TraceSpool: r.TraceSpool, Agent: agent,
		Target: &kindrig.DiagnoseTarget{
			Cluster: r.Binding.Cluster, Namespace: resolved.Namespace,
		},
	})
}

// PurgeData delegates resolved application identity to the approved purge
// implementation. No callback means no deletion.
func (r Runner) PurgeData() error {
	resolved, err := r.resolve()
	if err != nil {
		return err
	}
	if r.Purge == nil {
		return ErrPurgeUnavailable
	}
	return r.Purge(resolved)
}

func (r Runner) deploy(resolved Resolved, preparation Preparation) error {
	if r.DeployAgent == nil {
		return errors.New("app:up: no canonical deploy agent configured")
	}
	agent, err := r.DeployAgent()
	if err != nil {
		return err
	}
	coordinates := resolved.Coordinates
	coordinates.ChartPath = preparation.ChartPath
	coordinates.ValuesPath = preparation.ValuesPath
	return r.ops().deploy(kindrig.DeployRequest{
		Cluster: r.Binding.Cluster, KubeconfigPath: r.Binding.KubeconfigPath,
		ApplicationRoot: r.Binding.ApplicationRoot, CatalogRoot: r.CatalogRoot,
		Coordinates: coordinates, Overrides: preparation.Overrides, Agent: agent,
	})
}

func (r Runner) undeployAndCleanup(resolved Resolved) error {
	if r.UndeployAgent == nil {
		return errors.New("app:down: no canonical undeploy agent configured")
	}
	agent, err := r.UndeployAgent()
	if err != nil {
		return err
	}
	request := kindrig.DeployRequest{
		Cluster: r.Binding.Cluster, KubeconfigPath: r.Binding.KubeconfigPath,
		ApplicationRoot: r.Binding.ApplicationRoot, CatalogRoot: r.CatalogRoot,
		Coordinates: kindrig.UndeployCoordinates(
			resolved.Release, resolved.Namespace, resolved.Coordinates.Timeout),
		Agent: agent,
	}
	if err := r.ops().undeploy(request); err != nil {
		return err
	}
	if err := r.ops().deleteNamespace(r.namespaceRequest(resolved)); err != nil {
		return err
	}
	if r.AfterDown != nil {
		return r.AfterDown(resolved)
	}
	return nil
}

func (r Runner) resolve() (Resolved, error) {
	manifest, err := LoadManifest(r.ManifestPath)
	if err != nil {
		return Resolved{}, err
	}
	return Resolve(manifest, r.Binding)
}

func (r Runner) namespaceRequest(resolved Resolved) kindrig.ApplicationNamespaceRequest {
	return kindrig.ApplicationNamespaceRequest{
		Cluster: r.Binding.Cluster, KubeconfigPath: r.Binding.KubeconfigPath,
		Namespace: resolved.Namespace,
	}
}

func (r Runner) ops() lifecycleOperations {
	defaults := defaultLifecycleOperations()
	if r.operations.deploy == nil {
		r.operations.deploy = defaults.deploy
	}
	if r.operations.undeploy == nil {
		r.operations.undeploy = defaults.undeploy
	}
	if r.operations.diagnose == nil {
		r.operations.diagnose = defaults.diagnose
	}
	if r.operations.ensureNamespace == nil {
		r.operations.ensureNamespace = defaults.ensureNamespace
	}
	if r.operations.deleteNamespace == nil {
		r.operations.deleteNamespace = defaults.deleteNamespace
	}
	return r.operations
}
