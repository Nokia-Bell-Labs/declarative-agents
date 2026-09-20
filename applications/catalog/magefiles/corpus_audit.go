// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

// corpusAuditProfileRel is the profile that loads this module's specification
// corpus under the catalog charter and validates it. The sibling
// audit-profile.yaml audits formal go_test evidence; this one audits the
// specification graph and the document indexes (srd014-spec-graph, GH-2310).
const corpusAuditProfileRel = "agents/specification-critic/corpus-audit-profile.yaml"

// validateSpecificationCorpus runs the corpus audit over the catalog's own
// documents. The agent exits non-zero when its machine reaches the Failed
// terminal, which for this profile means the charter's checks reported an
// error-level finding; the findings are already on the caller's stdout, so the
// error names the audit rather than repeating them.
//
// The charter names the checks that gate rather than running every check the
// validator knows: agents/specification-critic/suites/catalog-corpus-charter.yaml
// records which are excluded and why.
func validateSpecificationCorpus(run profileSmokeRunner, binary, root, coreRoot string) error {
	profile := filepath.Join(root, filepath.FromSlash(corpusAuditProfileRel))
	out, err := run(binary,
		"--profile", profile,
		"--directory", root,
		"--core-root", coreRoot,
	)
	detail := strings.TrimSpace(string(out))
	if detail != "" {
		fmt.Println(detail)
	}
	if err != nil {
		return fmt.Errorf("specification corpus audit failed under %s", root)
	}
	return nil
}
