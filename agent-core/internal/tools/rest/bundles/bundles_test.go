// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package bundles

import (
	"io/fs"
	"testing"
)

func TestObserverBundleHasAnIndex(t *testing.T) {
	fsys, ok := Lookup("observer")
	if !ok {
		t.Fatal("observer bundle is not registered")
	}
	if _, err := fs.Stat(fsys, "index.html"); err != nil {
		t.Fatalf("observer bundle has no index.html: %v", err)
	}
	// A built bundle, not a placeholder: mage uikit:observer emits hashed
	// assets beside the index.
	assets, err := fs.Glob(fsys, "assets/index-*.js")
	if err != nil || len(assets) == 0 {
		t.Fatalf("observer bundle has no built script asset (run mage uikit:observer): %v", err)
	}
	if _, ok := Lookup("absent"); ok {
		t.Fatal("Lookup found an unregistered bundle")
	}
	if names := Names(); len(names) != 1 || names[0] != "observer" {
		t.Fatalf("Names() = %v, want [observer]", names)
	}
}
