// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package legacyenv reads the environment variables the words and the
// profiles read under the names they had before GH-38: TPM_<NAME> for
// what is now SCRUM_MASTER_<NAME>. A setup or CI job that still sets the
// old names keeps working while it moves, and doctor names each one it
// still sets.
package legacyenv

import (
	"os"
	"slices"
	"strings"
)

// Prefix is every variable's prefix; Legacy is the one it replaced.
const (
	Prefix = "SCRUM_MASTER_"
	Legacy = "TPM_"
)

// Adopt sets SCRUM_MASTER_<NAME> to TPM_<NAME>'s value for every TPM_
// variable whose new name is unset, so the words' reads and the profiles'
// ${SCRUM_MASTER_<NAME>} references see it. A new name that is set wins.
// It runs before an agent build loads a profile, from the words' package
// initialization.
func Adopt() {
	for _, name := range Set() {
		renamed := Renamed(name)
		if _, set := os.LookupEnv(renamed); !set { //nolint:forbidigo // This package bridges the legacy environment names (GH-38).
			os.Setenv(renamed, os.Getenv(name)) //nolint:forbidigo // This package bridges the legacy environment names (GH-38).
		}
	}
}

// Set is the TPM_ variables set in this process's environment, sorted.
func Set() []string {
	var names []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, Legacy) && len(name) > len(Legacy) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

// Renamed is a TPM_ variable's name since GH-38.
func Renamed(name string) string { return Prefix + strings.TrimPrefix(name, Legacy) }
