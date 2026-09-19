// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package bundles holds the UI bundles compiled into agent-core. A REST
// static_assets binding selects one by name instead of a filesystem root, so an
// agent serves a platform UI with no UI files of its own (applications srd004
// R9, srd029 R5.9).
package bundles

import (
	"embed"
	"io/fs"
	"sort"
)

// The observer bundle is the fleet observer SPA built from
// applications/ui-kit/observer by mage uikit:observer.
//
//go:embed observer
var observer embed.FS

var registry = map[string]fs.FS{
	"observer": mustSub(observer, "observer"),
}

func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}

// Lookup returns the named bundle's file tree.
func Lookup(name string) (fs.FS, bool) {
	fsys, ok := registry[name]
	return fsys, ok
}

// Names lists the compiled bundles in sorted order.
func Names() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
