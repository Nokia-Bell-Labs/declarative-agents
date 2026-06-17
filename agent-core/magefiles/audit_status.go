// Copyright (c) 2026 Nokia. All rights reserved.

package main

import "strings"

func auditRunFailed(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "terminal state: failed") {
			return true
		}
		if strings.Contains(line, "run complete: status=failed") {
			return true
		}
	}
	return false
}
