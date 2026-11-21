// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Clean removes rendered chapter output from every version directory.
func Clean() error {
	var errs []error
	for _, version := range specVersions {
		errs = append(errs, cleanRendered(version.renderOutputDir(".")))
	}
	return errors.Join(errs...)
}

func cleanRendered(outputDir string) error {
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("list %s: %w", outputDir, err)
	}
	var errs []error
	for _, entry := range entries {
		path := filepath.Join(outputDir, entry.Name())
		fmt.Printf("rm %s\n", path)
		if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("remove %s: %w", path, err))
		}
	}
	return errors.Join(errs...)
}
