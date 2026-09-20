// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Clean removes rendered chapter output.
func Clean() error {
	entries, err := os.ReadDir(renderOutputDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("list %s: %w", renderOutputDir, err)
	}
	var errs []error
	for _, entry := range entries {
		path := filepath.Join(renderOutputDir, entry.Name())
		fmt.Printf("rm %s\n", path)
		if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("remove %s: %w", path, err))
		}
	}
	return errors.Join(errs...)
}
