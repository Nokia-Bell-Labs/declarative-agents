// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

// Audit checks profile declarations against the external agent-core checkout.
func Audit() error {
	return auditProfiles(Validate)
}

func auditProfiles(validate func() error) error {
	return validate()
}
