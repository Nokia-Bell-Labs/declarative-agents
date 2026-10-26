// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"bytes"
	"os/exec"
	"testing"
)

// goModStoredInPlaintext reports whether a go.mod blob as git stores it is
// plaintext. Go's version detection reads go.mod from the git objects at the
// tagged commit, and a git-crypt blob makes every build report (devel)
// (eng03).
func goModStoredInPlaintext(blob []byte) bool {
	return bytes.HasPrefix(blob, []byte("module "))
}

func TestGoModIsStoredInPlaintext(t *testing.T) {
	blob, err := exec.Command("git", "cat-file", "-p", ":go.mod").Output()
	if err != nil {
		t.Skipf("no git index here: %v", err)
	}
	if !goModStoredInPlaintext(blob) {
		t.Fatal("go.mod is stored encrypted; .gitattributes must keep go.mod !filter !diff so builds at a tag carry its version")
	}
}

func TestGoModGateRecognisesCiphertext(t *testing.T) {
	if goModStoredInPlaintext([]byte("\x00GITCRYPT\x00\xa1\xb2")) {
		t.Fatal("a git-crypt blob passed as plaintext")
	}
	if !goModStoredInPlaintext([]byte("module github.com/example/app\n")) {
		t.Fatal("a plaintext go.mod failed")
	}
}
