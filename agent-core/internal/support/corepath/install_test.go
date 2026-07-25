// Copyright (c) 2026 Nokia. All rights reserved.

package corepath

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMap(t *testing.T) {
	root := t.TempDir()
	SetInstallRoot(root)
	t.Cleanup(func() { SetInstallRoot("") })

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "prefix", path: InstallPrefix, want: root},
		{name: "asset", path: InstallPrefix + "/tools/builtin/llm", want: filepath.Join(root, "tools", "builtin", "llm")},
		{name: "cleaned asset", path: InstallPrefix + "/tools/../tools/exec", want: filepath.Join(root, "tools", "exec")},
		{name: "similar prefix rejected", path: InstallPrefix + "-backup/tools", want: ""},
		{name: "relative rejected", path: "tools/builtin", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, Map(tt.path))
		})
	}
}

func TestMapWithoutInstallRoot(t *testing.T) {
	SetInstallRoot("")
	t.Cleanup(func() { SetInstallRoot("") })

	require.Empty(t, Map(InstallPrefix+"/tools"))
}
