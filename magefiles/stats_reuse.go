// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import "github.com/magefile/mage/mg"

// STATS groups repository-wide extensions under the existing stats target.
type STATS mg.Namespace

// Reuse aggregates deterministic reuse metrics from every stats participant.
func (STATS) Reuse() error { return writeReuseStats() }
