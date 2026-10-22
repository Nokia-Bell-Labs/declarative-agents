// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package profileservers

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

const collectorProfile = "collector/profile.yaml"

// coreRoot is the agent-core checkout, which /opt/agent-core imports map to.
var coreRoot = filepath.Join("..", "..")

func opts(env map[string]string) Options { return Options{Env: env, CoreRoot: coreRoot} }

// collectorTree is the catalog collector's profile directory and the units
// directory it imports, as the in-memory tree an operator would hold from a
// ConfigMap (srd009 R5.1).
func collectorTree(t *testing.T) fstest.MapFS {
	t.Helper()
	root := filepath.Join("..", "..", "..", "applications", "catalog", "agents")
	tree := fstest.MapFS{}
	for _, dir := range []string{"collector", "units"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			// A UI's installed dependencies are not part of the profile, and
			// npm links local packages there as symlinks to directories.
			if entry.IsDir() && entry.Name() == "node_modules" {
				return filepath.SkipDir
			}
			if !entry.Type().IsRegular() {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			tree[filepath.ToSlash(rel)] = &fstest.MapFile{Data: data}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return tree
}

// editREST rewrites the collector's rest.yaml in the tree.
func editREST(t *testing.T, tree fstest.MapFS, old, replacement string) {
	t.Helper()
	file := tree["collector/rest.yaml"]
	text := string(file.Data)
	if !strings.Contains(text, old) {
		t.Fatalf("collector rest.yaml lacks %q", old)
	}
	tree["collector/rest.yaml"] = &fstest.MapFile{Data: []byte(strings.ReplaceAll(text, old, replacement))}
}

func serverNamed(t *testing.T, inspection Inspection, name string) Server {
	t.Helper()
	for _, server := range inspection.Servers {
		if server.Name == name {
			return server
		}
	}
	t.Fatalf("no server %s in %+v", name, inspection.Servers)
	return Server{}
}

// TestInspectReadsTheCollectorServers covers srd009 R1.2: the three servers,
// the control and monitor servers, and the variables their addresses read,
// with the declaration defaults in force when env sets nothing.
func TestInspectReadsTheCollectorServers(t *testing.T) {
	inspection, err := Inspect(collectorTree(t), collectorProfile, opts(nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Servers) != 3 {
		t.Fatalf("servers = %+v, want three", inspection.Servers)
	}
	if inspection.ControlServer != "collector_control" || inspection.MonitorServer != "collector_monitor" {
		t.Errorf("control %q monitor %q", inspection.ControlServer, inspection.MonitorServer)
	}
	control := serverNamed(t, inspection, "collector_control")
	if control.Host != "127.0.0.1" || control.Port != 18191 || !control.Health || control.Monitor {
		t.Errorf("control = %+v", control)
	}
	if control.BindVar != "COLLECTOR_BIND_HOST" || control.PortVar != "COLLECTOR_CONTROL_PORT" {
		t.Errorf("control variables = %s %s", control.BindVar, control.PortVar)
	}
	monitor := serverNamed(t, inspection, "collector_monitor")
	if monitor.Port != 18192 || !monitor.Monitor || monitor.Health {
		t.Errorf("monitor = %+v", monitor)
	}
	if query := serverNamed(t, inspection, "collector_query"); query.Port != 18193 || query.Health || query.Monitor {
		t.Errorf("query = %+v", query)
	}
}

// TestInspectExpandsTheGivenEnvironment covers srd009 R1.2: the Agent's
// environment wins over the declaration defaults, and only it - the process
// environment is never consulted.
func TestInspectExpandsTheGivenEnvironment(t *testing.T) {
	t.Setenv("COLLECTOR_QUERY_PORT", "1")
	env := map[string]string{"COLLECTOR_CONTROL_PORT": "19000", "COLLECTOR_BIND_HOST": "0.0.0.0"}
	inspection, err := Inspect(collectorTree(t), collectorProfile, opts(env))
	if err != nil {
		t.Fatal(err)
	}
	if control := serverNamed(t, inspection, "collector_control"); control.Port != 19000 || control.Host != "0.0.0.0" {
		t.Errorf("control = %+v, want 0.0.0.0:19000", control)
	}
	if query := serverNamed(t, inspection, "collector_query"); query.Port != 18193 {
		t.Errorf("query port = %d, want the declaration default, not the process environment", query.Port)
	}
}

// TestInspectRejectsAPortWithoutDefaultOrValue covers srd009 R1.2: an
// address whose port reads a variable with no default is an error that names
// the variable, so an Agent author knows what to set.
func TestInspectRejectsAPortWithoutDefaultOrValue(t *testing.T) {
	tree := collectorTree(t)
	editREST(t, tree, "address: ${COLLECTOR_BIND_HOST:-127.0.0.1}:${COLLECTOR_QUERY_PORT:-18193}",
		"address: ${COLLECTOR_BIND_HOST:-127.0.0.1}:${COLLECTOR_QUERY_PORT}")
	_, err := Inspect(tree, collectorProfile, opts(nil))
	if err == nil || !strings.Contains(err.Error(), "COLLECTOR_QUERY_PORT") {
		t.Fatalf("err = %v, want one naming COLLECTOR_QUERY_PORT", err)
	}
	if _, err := Inspect(tree, collectorProfile, opts(map[string]string{"COLLECTOR_QUERY_PORT": "18193"})); err != nil {
		t.Errorf("with the variable set: %v", err)
	}
}

// TestInspectRejectsAmbiguousOrMissingHealth covers srd009 R1.2: exactly one
// server is the readiness signal.
func TestInspectRejectsAmbiguousOrMissingHealth(t *testing.T) {
	two := collectorTree(t)
	editREST(t, two, "        machine_spec:  {method: GET, path: /monitor/machine,",
		"        health_again: {method: GET, path: /api/lifecycle/health, binding: health}\n        machine_spec:  {method: GET, path: /monitor/machine,")
	if _, err := Inspect(two, collectorProfile, opts(nil)); err == nil || !strings.Contains(err.Error(), "both answer") {
		t.Errorf("two health servers: err = %v", err)
	}
	none := collectorTree(t)
	editREST(t, none, "path: /api/lifecycle/health", "path: /api/lifecycle/healthz")
	if _, err := Inspect(none, collectorProfile, opts(nil)); err == nil || !strings.Contains(err.Error(), "no server answers") {
		t.Errorf("no health server: err = %v", err)
	}
}

// TestValidateAppliesTheStartupBoundary covers srd009 R2.4: the collector
// passes what `agent --validate-config` runs, and a profile whose machine
// file is missing fails to load.
func TestValidateAppliesTheStartupBoundary(t *testing.T) {
	if err := Validate(collectorTree(t), collectorProfile, opts(nil)); err != nil {
		t.Fatalf("collector: %v", err)
	}
	broken := collectorTree(t)
	delete(broken, "collector/machine.yaml")
	err := Validate(broken, collectorProfile, opts(nil))
	if err == nil || !strings.Contains(err.Error(), "machine") {
		t.Fatalf("missing machine: err = %v", err)
	}
}

// TestInspectLeavesNoFilesBehind: the temporary tree is removed after each
// call, so a reconciling operator does not fill its filesystem.
func TestInspectLeavesNoFilesBehind(t *testing.T) {
	before, _ := filepath.Glob(filepath.Join(os.TempDir(), "profileservers-*"))
	if _, err := Inspect(collectorTree(t), collectorProfile, opts(nil)); err != nil {
		t.Fatal(err)
	}
	after, _ := filepath.Glob(filepath.Join(os.TempDir(), "profileservers-*"))
	if len(after) > len(before) {
		t.Errorf("temporary trees grew from %d to %d", len(before), len(after))
	}
}
