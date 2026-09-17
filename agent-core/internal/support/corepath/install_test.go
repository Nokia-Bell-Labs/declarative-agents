// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

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

func TestImportTargetResolvesRelativeAndLibraryRootedPaths(t *testing.T) {
	importer := filepath.Join(string(filepath.Separator), "profiles", "agents", "units", "types.yaml")
	root := t.TempDir()

	SetInstallRoot("")
	t.Cleanup(func() { SetInstallRoot("") })
	got, err := ImportTarget(importer, "../shared/words.yaml")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(string(filepath.Separator), "profiles", "agents", "shared", "words.yaml"), got)

	got, err = ImportTarget(importer, InstallPrefix+"/tools/units/types-core.yaml")
	require.NoError(t, err)
	require.Equal(t, InstallPrefix+"/tools/units/types-core.yaml", filepath.ToSlash(got),
		"without an install root the image path is used as written")

	SetInstallRoot(root)
	got, err = ImportTarget(importer, InstallPrefix+"/tools/units/types-core.yaml")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "tools", "units", "types-core.yaml"), got)

	_, err = ImportTarget(importer, "/home/user/types.yaml")
	require.ErrorIs(t, err, ErrOutsideLibraryRoot)
	_, err = ImportTarget(importer, InstallPrefix+"-backup/types.yaml")
	require.ErrorAs(t, err, &UndeclaredRootError{}, "a sibling prefix is a library form naming an undeclared root")
}

func TestDeclaredLibraryRootsResolveScopedAndReadOnly(t *testing.T) {
	SetInstallRoot("")
	t.Cleanup(func() { SetInstallRoot(""); SetLibraryRoots(nil); SetLibraryOverrides(nil) })
	shared := filepath.Join(t.TempDir(), "lib")
	previous := SetLibraryRoots(map[string]string{"shared": shared})
	require.Nil(t, previous)
	importer := filepath.Join(string(filepath.Separator), "profiles", "agents", "one", "declarations.yaml")

	got, err := ImportTarget(importer, "/opt/shared/units/words.yaml")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(shared, "units", "words.yaml"), got)

	_, err = ImportTarget(importer, "/opt/other/units/words.yaml")
	var undeclared UndeclaredRootError
	require.ErrorAs(t, err, &undeclared)
	require.Equal(t, "other", undeclared.Name)

	inLibrary := filepath.Join(shared, "units", "words.yaml")
	got, err = ImportTarget(inLibrary, "../types.yaml")
	require.NoError(t, err, "a relative import that stays inside the root is fine")
	require.Equal(t, filepath.Join(shared, "types.yaml"), got)
	_, err = ImportTarget(inLibrary, "../../escape.yaml")
	require.ErrorIs(t, err, ErrEscapesLibraryRoot)
	root, ok := LibraryRootOf(inLibrary)
	require.True(t, ok)
	require.Equal(t, "shared", root)

	require.Equal(t, map[string]string{"shared": shared}, SetLibraryRoots(nil), "the previous set comes back for restoring")
	require.Empty(t, LibraryRoots())
	_, err = ImportTarget(importer, "/opt/shared/units/words.yaml")
	require.ErrorAs(t, err, &undeclared, "a root does not outlive the closure that declared it")

	require.Error(t, ValidateLibraryRootName(AgentCoreRoot))
	require.Error(t, ValidateLibraryRootName("Shared_Lib"))
	require.NoError(t, ValidateLibraryRootName("mesh-lib"))
}
