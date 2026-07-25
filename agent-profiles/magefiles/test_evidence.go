// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

// validateTestEvidence runs the declarative jurist audit profile, which owns
// inventory, claim resolution, test execution, reduction, and reporting.
func validateTestEvidence(run profileSmokeRunner, binary, root, coreRoot string) error {
	profile := filepath.Join(root, "agents", "jurist", "audit-profile.yaml")
	out, err := run(binary,
		"--profile", profile,
		"--directory", root,
		"--core-root", coreRoot,
	)
	if err == nil {
		fmt.Printf("validated formal go_test evidence under %s through jurist audit profile\n", root)
		return nil
	}
	detail := strings.TrimSpace(string(out))
	if detail == "" {
		detail = err.Error()
	}
	return fmt.Errorf("formal go_test evidence audit failed:\n%s", detail)
}
