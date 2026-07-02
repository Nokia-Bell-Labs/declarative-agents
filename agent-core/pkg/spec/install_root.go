// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"path/filepath"
	"strings"
	"sync"
)

var agentCoreInstall struct {
	mu sync.RWMutex
	v  string
}

// SetAgentCoreInstallRoot maps /opt/agent-core references in profiles to this
// directory (for example a development checkout). Leave unset when the
// runtime already provides those absolute paths.
func SetAgentCoreInstallRoot(root string) {
	agentCoreInstall.mu.Lock()
	defer agentCoreInstall.mu.Unlock()
	agentCoreInstall.v = strings.TrimSpace(root)
}

// AgentCoreInstallRoot returns the root configured with SetAgentCoreInstallRoot.
func AgentCoreInstallRoot() string {
	agentCoreInstall.mu.RLock()
	defer agentCoreInstall.mu.RUnlock()
	return agentCoreInstall.v
}

// MapInstalledCorePath maps a profile path under CoreInstall into
// AgentCoreInstallRoot when that root is set.
func MapInstalledCorePath(p string) string {
	root := AgentCoreInstallRoot()
	if root == "" {
		return ""
	}
	clean := filepath.ToSlash(filepath.Clean(p))
	if clean != CoreInstall && !strings.HasPrefix(clean, CoreInstall+"/") {
		return ""
	}
	rel := strings.TrimPrefix(clean, CoreInstall)
	rel = strings.TrimPrefix(rel, "/")
	return filepath.Join(root, filepath.FromSlash(rel))
}
