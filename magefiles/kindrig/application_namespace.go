// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package kindrig

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const applicationNamespaceManager = "apprig"

// ApplicationNamespaceRequest binds namespace mechanics to one explicit
// cluster credential. KubeconfigPath selects cloud; otherwise Cluster names a
// kind cluster. No operation reads the ambient kubectl context.
type ApplicationNamespaceRequest struct {
	Cluster        string
	KubeconfigPath string
	Namespace      string
}

type namespaceDocument struct {
	Metadata struct {
		Labels map[string]string `json:"labels"`
	} `json:"metadata"`
}

// EnsureApplicationNamespace creates and labels apprig's stable application
// namespace, or reuses one already carrying the ownership label. It refuses to
// adopt a foreign namespace. The bool reports whether this invocation created
// it, allowing preparation/install compensation to remove only its own work.
func EnsureApplicationNamespace(request ApplicationNamespaceRequest) (bool, error) {
	run, cleanup, err := applicationNamespaceRunner(request)
	if err != nil {
		return false, err
	}
	defer cleanup()
	return ensureApplicationNamespace(run, request.Namespace)
}

func ensureApplicationNamespace(run CommandRunner, namespace string) (bool, error) {
	document, exists, err := readApplicationNamespace(run, namespace)
	if err != nil {
		return false, err
	}
	if exists {
		if got := document.Metadata.Labels["app.kubernetes.io/managed-by"]; got != applicationNamespaceManager {
			return false, fmt.Errorf("application namespace %s is managed by %q, not apprig; refusing adoption",
				namespace, got)
		}
		return false, nil
	}
	if output, err := run("kubectl", "create", "namespace", namespace); err != nil {
		return false, fmt.Errorf("create application namespace %s: %w: %s",
			namespace, err, strings.TrimSpace(string(output)))
	}
	if output, err := run("kubectl", "label", "namespace", namespace,
		"app.kubernetes.io/managed-by="+applicationNamespaceManager, "--overwrite"); err != nil {
		_, deleteErr := run("kubectl", "delete", "namespace", namespace,
			"--ignore-not-found=true", "--wait=true", "--timeout="+scenarioNamespaceDeleteTimeout)
		return false, errors.Join(
			fmt.Errorf("label application namespace %s: %w: %s",
				namespace, err, strings.TrimSpace(string(output))),
			deleteErr,
		)
	}
	return true, nil
}

// DeleteApplicationNamespace removes only an apprig-owned namespace. The
// caller invokes it after kindrig.Undeploy has reached Removed or Absent, so
// namespace cleanup cannot race or replace the canonical undeploy transaction.
func DeleteApplicationNamespace(request ApplicationNamespaceRequest) error {
	run, cleanup, err := applicationNamespaceRunner(request)
	if err != nil {
		return err
	}
	defer cleanup()
	return deleteApplicationNamespace(run, request.Namespace)
}

func deleteApplicationNamespace(run CommandRunner, namespace string) error {
	document, exists, err := readApplicationNamespace(run, namespace)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if got := document.Metadata.Labels["app.kubernetes.io/managed-by"]; got != applicationNamespaceManager {
		return fmt.Errorf("application namespace %s is managed by %q, not apprig; refusing deletion",
			namespace, got)
	}
	if output, err := run("kubectl", "delete", "namespace", namespace,
		"--wait=false"); err != nil {
		return fmt.Errorf("delete application namespace %s: %w: %s",
			namespace, err, strings.TrimSpace(string(output)))
	}
	if output, err := run("kubectl", "wait", "--for=delete", "namespace/"+namespace,
		"--timeout="+scenarioNamespaceDeleteTimeout); err != nil {
		return fmt.Errorf("wait for application namespace %s deletion: %w: %s",
			namespace, err, strings.TrimSpace(string(output)))
	}
	return VerifyDataPlane(run)
}

func readApplicationNamespace(run CommandRunner, namespace string) (namespaceDocument, bool, error) {
	output, err := run("kubectl", "get", "namespace", namespace, "-o", "json", "--ignore-not-found=true")
	if err != nil {
		return namespaceDocument{}, false, fmt.Errorf("read application namespace %s: %w: %s",
			namespace, err, strings.TrimSpace(string(output)))
	}
	if len(strings.TrimSpace(string(output))) == 0 {
		return namespaceDocument{}, false, nil
	}
	var document namespaceDocument
	if err := json.Unmarshal(output, &document); err != nil {
		return namespaceDocument{}, false, fmt.Errorf("decode application namespace %s: %w", namespace, err)
	}
	return document, true, nil
}

func applicationNamespaceRunner(request ApplicationNamespaceRequest) (CommandRunner, func(), error) {
	if !scenarioLabel.MatchString(request.Namespace) {
		return nil, nil, fmt.Errorf("application namespace %q is not a DNS-1123 label", request.Namespace)
	}
	if strings.TrimSpace(request.KubeconfigPath) != "" {
		commands, err := CommandsForKubeconfig(request.KubeconfigPath)
		if err != nil {
			return nil, nil, err
		}
		return commands.Run, func() {}, nil
	}
	if strings.TrimSpace(request.Cluster) == "" {
		return nil, nil, errors.New("application namespace: cluster or kubeconfig is required")
	}
	commands, cleanup, err := ClusterCommands(CaptureRun, request.Cluster)
	if err != nil {
		return nil, nil, err
	}
	return commands.Run, cleanup, nil
}
