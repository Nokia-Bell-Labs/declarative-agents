// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
	"github.com/magefile/mage/mg"
)

// Platform groups the persistent developer da-platform targets (GH-2215).
type Platform mg.Namespace

// Up creates, or reuses and re-checks, the developer da-platform and leaves it
// running, so application integration targets reuse it instead of starting their
// own. A reused platform has unreferenced node images pruned before conformance.
// A release run (mage tag, mage tagDryRun) deletes and recreates da-platform;
// run platform:up again after one.
func (Platform) Up() error {
	_, err := kindrig.UpPlatform(kindrig.PlatformOptions{})
	return err
}

// Down stops the developer da-platform compute and no other cluster. It refuses
// while a managed application namespace remains and preserves the retained
// local object store; platform:reset deletes that store.
func (Platform) Down() error {
	return kindrig.DownPlatform(kindrig.DefaultRun, kindrig.PlatformCommandBinding)
}

// Reset is the explicit destructive operation that deletes the retained local
// object store. It refuses while da-platform or a managed application is
// active; there is no force bypass.
func (Platform) Reset() error {
	return kindrig.ResetPlatform(kindrig.DefaultRun, kindrig.PlatformCommandBinding)
}
