// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package site

import (
	"os"
	"os/exec"
	"strings"
)

// DecryptedEnv, set to 1, lets the corpus word copy a corpus the system
// repository encrypts at rest into a reports repository that does not
// (GH-115).
const DecryptedEnv = "SCRUM_MASTER_SITE_DECRYPTED"

// encrypted reports whether git encrypts p in the checkout at dir: its
// filter attribute is git-crypt. The path need not exist.
func encrypted(dir, p string) bool {
	out, err := exec.Command("git", "-C", dir, "check-attr", "filter", "--", p).Output()
	return err == nil && strings.HasSuffix(strings.TrimSpace(string(out)), ": git-crypt")
}

// withheld is why the corpus word copies none of the system's documents:
// the system repository encrypts them at rest and the reports repository
// does not encrypt the site, so their text would land there in the clear.
// Empty when either encrypts as the other does, or DecryptedEnv opts in.
func withheld(system, reports string) string {
	if os.Getenv(DecryptedEnv) == "1" { //nolint:forbidigo // Reads the documented SCRUM_MASTER_* environment interface.
		return ""
	}
	if !encrypted(system, "docs/VISION.md") && !encrypted(system, "docs/ARCHITECTURE.md") {
		return ""
	}
	if encrypted(reports, Index) {
		return ""
	}
	return "the system repository encrypts its documents and this reports repository does not encrypt site/, so the corpus pages would hold their text in the clear; encrypt site/ here, or set " + DecryptedEnv + "=1 to copy them anyway"
}
