// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package spec

import (
	"bufio"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const goTestInventoryCheck = "go-test-inventory"

func parseGoPackages(out string) map[string]bool {
	packages := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			packages[line] = true
		}
	}
	return packages
}

// goListEvent is the subset of Go JSON inventory events used for top-level
// tests and package setup or build diagnostics.
type goListEvent struct {
	Action      string
	Package     string
	ImportPath  string
	Test        string
	Output      string
	FailedBuild string
}

func (e goListEvent) packagePath() string {
	if e.Package != "" {
		return e.Package
	}
	return e.ImportPath
}

type goListInventory struct {
	byPackage       map[string]map[string]bool
	allTests        map[string]bool
	diagnostics     map[string][]string
	failedPackages  map[string]bool
	sawPackageEvent bool
}

func newGoListInventory() *goListInventory {
	return &goListInventory{
		byPackage:      map[string]map[string]bool{},
		allTests:       map[string]bool{},
		diagnostics:    map[string][]string{},
		failedPackages: map[string]bool{},
	}
}

// parseGoListTests keeps top-level test names and package build diagnostics from
// a go test -json -list stream. Inventory failures become findings so the
// declarative audit can report their cause instead of misreporting absent tests.
func parseGoListTests(stream *strings.Reader) (
	map[string]map[string]bool, map[string]bool, []Finding, error,
) {
	inventory := newGoListInventory()
	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var ev goListEvent
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			continue
		}
		inventory.consume(ev)
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, nil, fmt.Errorf("scan go test -list output: %w", err)
	}
	if !inventory.sawPackageEvent {
		return nil, nil, nil, fmt.Errorf("go test -list returned no Go JSON package events")
	}
	return inventory.byPackage, inventory.allTests, inventory.failureFindings(), nil
}

func (i *goListInventory) consume(ev goListEvent) {
	pkg := ev.packagePath()
	if pkg == "" {
		return
	}
	i.sawPackageEvent = true
	switch ev.Action {
	case "output", "build-output":
		line := strings.TrimSpace(ev.Output)
		if line == "" {
			return
		}
		if ev.Action == "output" && goTestNameRE.MatchString(line) {
			if i.byPackage[pkg] == nil {
				i.byPackage[pkg] = map[string]bool{}
			}
			i.byPackage[pkg][line] = true
			i.allTests[line] = true
			return
		}
		i.diagnostics[pkg] = append(i.diagnostics[pkg], line)
	case "build-fail":
		i.failedPackages[pkg] = true
		i.appendDiagnostic(pkg, ev.Output)
	case "fail":
		if ev.Test == "" {
			i.failedPackages[pkg] = true
			i.appendDiagnostic(pkg, ev.FailedBuild)
		}
	}
}

func (i *goListInventory) appendDiagnostic(pkg, detail string) {
	if detail = strings.TrimSpace(detail); detail != "" {
		i.diagnostics[pkg] = append(i.diagnostics[pkg], detail)
	}
}

func (i *goListInventory) failureFindings() []Finding {
	failed := make([]string, 0, len(i.failedPackages))
	for pkg := range i.failedPackages {
		failed = append(failed, pkg)
	}
	sort.Strings(failed)
	findings := make([]Finding, 0, len(failed))
	for _, pkg := range failed {
		message := fmt.Sprintf("go test inventory failed for package %s", pkg)
		if detail := strings.Join(i.diagnostics[pkg], " "); detail != "" {
			message += ": " + detail
		}
		findings = append(findings, Finding{
			Check:   goTestInventoryCheck,
			Level:   "error",
			Message: message,
		})
	}
	return findings
}
