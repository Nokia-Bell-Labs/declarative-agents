// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"
	"os"
	"os/exec"
)

// Presentation serves chatbot-mesh.slide with the module-pinned Go present tool.
func Presentation() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	cmd := presentationCommand(root)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("serve chatbot-mesh presentation: %w", err)
	}
	return nil
}

// Present is a short alias for Presentation.
func Present() error {
	return Presentation()
}

func presentationCommand(application string) *exec.Cmd {
	cmd := exec.Command("go", "tool", "present", "-play=false", "chatbot-mesh.slide")
	cmd.Dir = application
	return cmd
}
