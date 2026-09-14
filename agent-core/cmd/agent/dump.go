// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"
	"io"

	internalload "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/load"
)

func dumpConfig(closure *internalload.Closure, writer io.Writer) error {
	return internalload.DumpConfig(closure, writer)
}

func dumpConfiguredProfile(writer io.Writer) error {
	if flagProfile == "" {
		return fmt.Errorf("--profile is required")
	}
	closure, err := internalload.LoadClosure(flagProfile, internalload.Options{})
	if err != nil {
		return err
	}
	return dumpConfig(closure, writer)
}
