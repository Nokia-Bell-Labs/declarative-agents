// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The kit owns panel paths, the monitor and trace clients, the fleet poll, the
// status bar, and the machine layout (srd004). An application UI that
// redefines one of them has forked the kit, so this sweep keeps the monorepo
// free of the copies GH-2160 removed (GH-2275).

// uiKitOwnedHelper matches a local definition of a kit path helper.
var uiKitOwnedHelper = regexp.MustCompile(`\b(?:function|const|let|var)\s+(splitPanelPath|panelHref)\b`)

// uiKitOwnedFiles are file names that only the kit may hold. A stem entry
// matches any TypeScript or JavaScript extension.
var (
	uiKitOwnedStems = map[string]bool{
		"monitorApi":      true,
		"traceApi":        true,
		"useFleet":        true,
		"machineLayout":   true,
		"machineCollapse": true,
	}
	uiKitOwnedNames = map[string]bool{
		"StatusBar.tsx": true,
		"route.tsx":     true,
	}
)

// uiRawFetch and uiRawEventSource match I/O that bypasses the kit client
// (srd004 R6.1). A method call such as client.fetch( is not a raw fetch.
var (
	uiRawFetch       = regexp.MustCompile(`(?:^|[^\w.$])fetch\s*\(|\b(?:window|globalThis|self)\.fetch\s*\(`)
	uiRawEventSource = regexp.MustCompile(`\bnew\s+EventSource\s*\(`)
)

// uiKitModuleImport matches an import of the kit's JavaScript entry or a
// sub-path of it; stylesheet imports (tokens.css, styles.css) are excluded by
// the caller, so a tokens-only consumer is not a kit-client UI.
var uiKitModuleImport = regexp.MustCompile(`["'](` + regexp.QuoteMeta(uiKitPackageName) + `(?:/[\w./-]+)?)["']`)

// uiTraceProxyRoute is the retired per-application trace proxy (srd004 R2.3).
const uiTraceProxyRoute = "/trace-proxy"

func isUIScript(name string) bool {
	switch filepath.Ext(name) {
	case ".ts", ".tsx", ".js", ".jsx", ".mjs":
		return true
	}
	return false
}

func isRestDeclaration(name string) bool {
	return name == "rest.yaml" || strings.HasSuffix(name, "-rest.yaml")
}

// sweepUIDuplication walks the applications tree rooted at applications and
// returns one "path: reason" finding per offence, paths relative to the tree
// and slash-separated. It skips node_modules, dist, testdata, and the kit.
func sweepUIDuplication(applications string) ([]string, error) {
	kitDir := filepath.Join(applications, filepath.FromSlash(strings.TrimPrefix(uiKitDir, "applications/")))
	var findings []string
	var packages []string
	report := func(path, reason string) {
		rel, err := filepath.Rel(applications, path)
		if err != nil {
			rel = path
		}
		findings = append(findings, filepath.ToSlash(rel)+": "+reason)
	}

	err := filepath.WalkDir(applications, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if path == kitDir || name == "node_modules" || name == "dist" || name == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if name == "package.json" {
			packages = append(packages, filepath.Dir(path))
		}
		stem := strings.TrimSuffix(name, filepath.Ext(name))
		if uiKitOwnedNames[name] || (isUIScript(name) && uiKitOwnedStems[stem]) {
			report(path, "file name "+name+" belongs to the ui-kit; import it from "+uiKitPackageName)
		}
		if !isUIScript(name) && !isRestDeclaration(name) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		source := string(data)
		if isUIScript(name) {
			for _, m := range uiKitOwnedHelper.FindAllStringSubmatch(source, -1) {
				report(path, "defines "+m[1]+"; import it from "+uiKitPackageName)
			}
		}
		if strings.Contains(source, uiTraceProxyRoute) {
			report(path, "names "+uiTraceProxyRoute+"; read traces through the monitor proxy (srd004 R2.3)")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	for _, dir := range packages {
		pkgFindings, err := sweepKitClientPackage(dir)
		if err != nil {
			return nil, err
		}
		for _, f := range pkgFindings {
			report(f[0], f[1])
		}
	}
	sort.Strings(findings)
	return findings, nil
}

// sweepKitClientPackage reports raw fetch and EventSource calls in the src/
// tree of a UI package that depends on the kit and imports its modules. Such
// a package reads and writes through the kit client, including its domain
// calls (srd004 R6.1).
func sweepKitClientPackage(dir string) ([][2]string, error) {
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, err
	}
	if _, ok := pkg.Dependencies[uiKitPackageName]; !ok {
		if _, ok := pkg.DevDependencies[uiKitPackageName]; !ok {
			return nil, nil
		}
	}
	src := filepath.Join(dir, "src")
	if !isDir(src) {
		return nil, nil
	}
	sources := map[string]string{}
	importsKit := false
	err = filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "node_modules" || d.Name() == "dist" || d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !isUIScript(d.Name()) {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sources[path] = string(body)
		for _, m := range uiKitModuleImport.FindAllStringSubmatch(string(body), -1) {
			if !strings.HasSuffix(m[1], ".css") {
				importsKit = true
			}
		}
		return nil
	})
	if err != nil || !importsKit {
		return nil, err
	}
	var findings [][2]string
	for path, source := range sources {
		if uiRawFetch.MatchString(source) {
			findings = append(findings, [2]string{path, "raw fetch( in a kit UI; use the kit client"})
		}
		if uiRawEventSource.MatchString(source) {
			findings = append(findings, [2]string{path, "raw new EventSource( in a kit UI; use the kit client"})
		}
	}
	return findings, nil
}

// TestNoUIKitDuplicationInApplications fails when a monorepo UI redefines what
// the kit owns (GH-2275, srd004 R2.3, R6.1).
func TestNoUIKitDuplicationInApplications(t *testing.T) {
	t.Parallel()
	applications, err := filepath.Abs(filepath.Join("..", "applications"))
	if err != nil {
		t.Fatal(err)
	}
	findings, err := sweepUIDuplication(applications)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range findings {
		t.Errorf("applications/%s", f)
	}
}

// TestSweepUIDuplicationReportsEachShape plants every offending shape, and a
// look-alike that is not an offence, so the sweep cannot go blind.
func TestSweepUIDuplicationReportsEachShape(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "applications")
	kitDep := `{"dependencies":{"` + uiKitPackageName + `":"file:../../ui-kit"}}`
	files := map[string]string{
		// Kit-owned helper definitions.
		"app/ui/src/paths.ts":  "export function panelHref(id: string) { return id }\n",
		"app/ui/src/split.tsx": "const splitPanelPath = (p: string) => p.split('/')\n",
		// Kit-owned file names.
		"app/ui/src/monitorApi.ts":            "export {}\n",
		"app/ui/src/traceApi.ts":              "export {}\n",
		"app/ui/src/StatusBar.tsx":            "export {}\n",
		"app/ui/src/useFleet.ts":              "export {}\n",
		"app/ui/src/machineLayout.ts":         "export {}\n",
		"app/ui/src/machineCollapse.tsx":      "export {}\n",
		"app/ui/src/route.tsx":                "export {}\n",
		"app/ui/package.json":                 kitDep,
		"app/ui/src/main.tsx":                 "import { AppShell } from \"" + uiKitPackageName + "\"\nimport \"" + uiKitPackageName + "/styles.css\"\n",
		"app/ui/src/raw.ts":                   "export const r = () => fetch('/x')\n",
		"app/ui/src/window.ts":                "export const w = () => window.fetch('/x')\n",
		"app/ui/src/stream.ts":                "export const s = () => new EventSource('/s')\n",
		"app/ui/src/api.ts":                   "export const a = (c: { getJSON(p: string): unknown }) => c.getJSON('/monitor/state')\n",
		"app/rest.yaml":                       "path: /trace-proxy/{path...}\n",
		"app/observer-rest.yaml":              "path: /trace-proxy/x\n",
		"app/ui/vite.config.ts":               "export default { server: { proxy: { '/trace-proxy': 'x' } } }\n",
		"tokens/ui/package.json":              kitDep,
		"tokens/ui/src/App.css":               "@import \"" + uiKitPackageName + "/tokens.css\";\n",
		"tokens/ui/src/apiClient.ts":          "export const r = () => fetch('/docs')\n",
		"plain/ui/package.json":               `{"dependencies":{"react":"^19"}}`,
		"plain/ui/src/api.ts":                 "export const r = () => fetch('/x')\n",
		"app/ui/node_modules/x/StatusBar.tsx": "function panelHref() {}\n",
		"app/ui/dist/assets/monitorApi.ts":    "new EventSource('/trace-proxy')\n",
		"app/testdata/useFleet.ts":            "export {}\n",
		"ui-kit/src/shell/paths.ts":           "export function splitPanelPath() {}\nexport function panelHref() {}\n",
		"ui-kit/src/api/monitorApi.ts":        "export {}\n",
		"ui-kit/src/client/client.ts":         "fetch('/x'); new EventSource('/y')\n",
		"other/StatusBar.test.tsx":            "export {}\n",
	}
	for rel, content := range files {
		writeUIFile(t, filepath.Join(root, filepath.FromSlash(rel)), content)
	}

	got, err := sweepUIDuplication(root)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range got {
		paths = append(paths, f[:strings.Index(f, ": ")])
	}
	want := []string{
		"app/observer-rest.yaml",
		"app/rest.yaml",
		"app/ui/src/StatusBar.tsx",
		"app/ui/src/machineCollapse.tsx",
		"app/ui/src/machineLayout.ts",
		"app/ui/src/monitorApi.ts",
		"app/ui/src/paths.ts",
		"app/ui/src/raw.ts",
		"app/ui/src/route.tsx",
		"app/ui/src/split.tsx",
		"app/ui/src/stream.ts",
		"app/ui/src/traceApi.ts",
		"app/ui/src/useFleet.ts",
		"app/ui/src/window.ts",
		"app/ui/vite.config.ts",
	}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("sweep findings:\n%s\nwant offences in:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
