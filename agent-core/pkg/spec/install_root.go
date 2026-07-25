// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/corepath"

// SetAgentCoreInstallRoot maps /opt/agent-core references in profiles to this
// directory (for example a development checkout). Leave unset when the
// runtime already provides those absolute paths.
func SetAgentCoreInstallRoot(root string) {
	corepath.SetInstallRoot(root)
}

// AgentCoreInstallRoot returns the root configured with SetAgentCoreInstallRoot.
func AgentCoreInstallRoot() string {
	return corepath.InstallRoot()
}

// MapInstalledCorePath maps a profile path under CoreInstall into
// AgentCoreInstallRoot when that root is set.
func MapInstalledCorePath(p string) string {
	return corepath.Map(p)
}
