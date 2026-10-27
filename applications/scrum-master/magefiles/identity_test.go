// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/identity"
)

// The files outside Go that name scrum-master's home agree with
// internal/identity, and the tutorials and guides link the README's
// install section rather than repeat the install command (GH-2755).
func TestPackagingNamesTheOneHome(t *testing.T) {
	root, err := repositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	read := func(p string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(root, p))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	if !strings.Contains(read("install.sh"), "base=${SCRUM_MASTER_RELEASE_URL:-"+identity.ReleasesURL+"}") {
		t.Errorf("install.sh's default release page is not %s", identity.ReleasesURL)
	}
	var plugin struct {
		Repository string `json:"repository"`
	}
	if err := json.Unmarshal([]byte(read("plugin/plugin.json")), &plugin); err != nil || plugin.Repository != identity.HomeURL {
		t.Errorf("plugin.json's repository = %q, %v, want %s", plugin.Repository, err, identity.HomeURL)
	}
	if !strings.Contains(read("Dockerfile"), `org.opencontainers.image.source="`+identity.HomeURL+`"`) {
		t.Errorf("the Dockerfile's source label is not %s", identity.HomeURL)
	}
	install := regexp.MustCompile(`https://\S+/install\.sh`)
	urls := install.FindAllString(read("README.md"), -1)
	if len(urls) == 0 {
		t.Error("the README names no install URL")
	}
	for _, u := range urls {
		if u != identity.InstallURL {
			t.Errorf("the README's install URL %s is not %s", u, identity.InstallURL)
		}
	}
	for _, dir := range []string{"docs/tutorials", "docs/guides"} {
		files, _ := filepath.Glob(filepath.Join(root, dir, "*.md"))
		for _, f := range files {
			b, _ := os.ReadFile(f)
			if install.Match(b) {
				t.Errorf("%s repeats the install command; link the README's Install section", f)
			}
		}
	}
}

// No Go string spells either repository's name, or this application's
// module path, outside internal/identity and import paths, so changing
// either is one edit (GH-2755).
func TestNoGoStringSpellsARepositoryOrTheModule(t *testing.T) {
	root, err := repositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	mod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	first, _, _ := strings.Cut(string(mod), "\n")
	module := strings.TrimSpace(strings.TrimPrefix(first, "module"))
	names := []string{module, identity.Home, identity.Runtime}
	err = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == ".regression" || d.Name() == "testdata" || strings.HasPrefix(d.Name(), ".")) && p != root {
			return filepath.SkipDir
		}
		if d.IsDir() || filepath.Ext(p) != ".go" || filepath.Base(filepath.Dir(p)) == "identity" {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), p, nil, 0)
		if err != nil {
			return err
		}
		imports := map[*ast.BasicLit]bool{}
		for _, im := range f.Imports {
			imports[im.Path] = true
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING || imports[lit] {
				return true
			}
			s, _ := strconv.Unquote(lit.Value)
			for _, name := range names {
				if strings.Contains(s, name) {
					rel, _ := filepath.Rel(root, p)
					t.Errorf("%s spells %q; take it from internal/identity or go.mod", rel, name)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
