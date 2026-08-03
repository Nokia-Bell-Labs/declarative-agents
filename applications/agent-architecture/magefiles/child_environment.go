// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"strings"
)

// childPathWithPrefix preserves the inherited PATH while placing integration
// fakes first. Child-process pass-through is a sanctioned declared-tool contract.
func childPathWithPrefix(environ []string, directory string) string {
	const key = "PATH="
	for _, entry := range environ {
		if strings.HasPrefix(entry, key) {
			return key + directory + string(os.PathListSeparator) + strings.TrimPrefix(entry, key)
		}
	}
	return key + directory
}
