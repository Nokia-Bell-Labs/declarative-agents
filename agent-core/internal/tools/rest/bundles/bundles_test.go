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
	if _, ok := Lookup("absent"); ok {
		t.Fatal("Lookup found an unregistered bundle")
	}
	if names := Names(); len(names) != 1 || names[0] != "observer" {
		t.Fatalf("Names() = %v, want [observer]", names)
	}
}
