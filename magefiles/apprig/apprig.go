// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package apprig is the reusable, manifest-driven application lifecycle harness
// for the shared da-platform (srd008; GH-2474). It runs any conforming
// application in its own namespace on the platform so downstream repositories
// import one package instead of copying lifecycle code.
//
// This package is a deterministic composition boundary, not a second
// implementation of deployment, teardown, diagnosis, or purge. The release
// transaction runs through the canonical catalog applier via kindrig.Deploy and
// kindrig.Undeploy; diagnosis runs through the canonical rig-doctor via
// kindrig.Diagnose. apprig owns only the deterministic Go around those agents:
// loading and validating the application manifest, resolving coordinates,
// detecting collisions between concurrent applications, and aggregating
// read-only status. It contains no direct Helm lifecycle argv and no second
// diagnosis or purge state machine (srd008 R9; #2479 R1, R13).
//
// # Stable API
//
// The exported surface downstream modules pin is: Manifest and LoadManifest;
// PlatformBinding, Resolved, and Resolve; DetectCollisions and Collision;
// StatusReport, ComponentStatus, StatusProbes, and AggregateStatus; and Runner,
// Preparation, and ErrPurgeUnavailable. These follow the root Mage module's
// version. A field or function is removed only after one release deprecating
// it; new optional fields, hooks, and probes are added without a major bump.
package apprig

// APIVersion is the stable-surface revision downstream modules can assert
// against. It moves with the root Mage module's release, not per commit.
const APIVersion = 1
