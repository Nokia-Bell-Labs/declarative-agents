// Copyright (c) 2026 Nokia. All rights reserved.

package main

// Audit checks profile declarations against the external agent-core checkout.
func Audit() error {
	return auditProfiles(Validate)
}

func auditProfiles(validate func() error) error {
	return validate()
}
