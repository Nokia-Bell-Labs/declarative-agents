// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package apprig

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// PurgeAgent locates the approved model-free catalog profile and runtime.
type PurgeAgent struct {
	Binary   string
	Profile  string
	CoreRoot string
	Cleanup  func()
}

// PurgeBinding carries provider bootstrap that is not application identity.
// Empty Endpoint selects ambient cloud identity; kind supplies fake GCS.
type PurgeBinding struct {
	Endpoint       string
	AuditDirectory string
}

// RunApplicationPurge renders fixed storage coordinates into the profile
// environment, seeds only the exact confirmation token, and runs the approved
// model-free machine. Request data cannot select bucket, endpoint, prefix, or
// application identity.
func RunApplicationPurge(
	resolved Resolved,
	confirmation string,
	binding PurgeBinding,
	agent PurgeAgent,
) error {
	if agent.Binary == "" || agent.Profile == "" || agent.CoreRoot == "" {
		return fmt.Errorf("app:purge: approved purge agent is incomplete")
	}
	if agent.Cleanup != nil {
		defer agent.Cleanup()
	}
	workspace := filepath.Join(resolved.Workspace, "purge")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return fmt.Errorf("app:purge: create workspace: %w", err)
	}
	request, err := writePurgeRequest(workspace, confirmation)
	if err != nil {
		return err
	}
	auditDirectory := binding.AuditDirectory
	if auditDirectory == "" {
		auditDirectory = filepath.Join(filepath.Dir(resolved.EvidenceDir), "purge")
	}
	auditPath := filepath.Join(auditDirectory, resolved.Application+".ndjson")
	command := exec.Command(agent.Binary,
		"--profile", agent.Profile,
		"--directory", workspace,
		"--request", request,
		"--core-root", agent.CoreRoot,
	)
	command.Env = append(os.Environ(),
		"PURGE_APPLICATION="+resolved.Application,
		"PURGE_BUCKET_URL="+resolved.BucketURL,
		"PURGE_ENDPOINT="+binding.Endpoint,
		"PURGE_PREFIX="+strings.Trim(resolved.ObjectPrefix, "/")+"/",
		"PURGE_AUDIT_PATH="+auditPath,
	)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("app:purge %s failed: %w", resolved.Application, err)
	}
	if err := verifyPurgeAudit(auditPath, resolved); err != nil {
		return err
	}
	fmt.Printf("app:purge: verified %s; audit %s\n", resolved.Application, auditPath)
	return nil
}

func writePurgeRequest(workspace, confirmation string) (string, error) {
	encoded, err := json.Marshal(map[string]any{
		"parameters": map[string]string{"confirmation": confirmation},
	})
	if err != nil {
		return "", err
	}
	path := filepath.Join(workspace, "request.json")
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		return "", fmt.Errorf("app:purge: write request: %w", err)
	}
	return path, nil
}

func verifyPurgeAudit(path string, resolved Resolved) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("app:purge: read external audit: %w", err)
	}
	defer file.Close()
	var last string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) != "" {
			last = scanner.Text()
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	var record struct {
		Status      string `json:"status"`
		Application string `json:"application"`
		BucketURL   string `json:"bucket_url"`
		Remaining   int    `json:"remaining"`
	}
	if last == "" || json.Unmarshal([]byte(last), &record) != nil ||
		record.Status != "purged" || record.Application != resolved.Application ||
		record.BucketURL != resolved.BucketURL || record.Remaining != 0 {
		return fmt.Errorf("app:purge: external audit does not prove verified deletion")
	}
	return nil
}
