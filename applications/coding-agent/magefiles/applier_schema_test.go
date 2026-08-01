// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// These cover the coding chart schema the applier's validate step rests on
// (srd006 R2.1, AC2).
//
// The applier rejects a non-conforming values patch by rendering the chart with a
// helm dry-run, which validates against values.schema.json. That rejection is the
// applier's entire input-validation story, and integration:applier cannot prove
// it: its fake helm returns whatever exit code the scenario sets, so a schema that
// accepted every document would pass that tracer unchanged. Only real helm against
// the real coding chart can say whether the schema rejects anything.
//
// Substitution, stated because it matters: the applier runs `helm upgrade
// --dry-run`, which needs a reachable cluster. `helm template` validates values
// against the same values.schema.json without one, so that is what these run. The
// command form the applier itself issues is proven on a live cluster by
// integration:applierLive.

// applierValuesFixture returns a values fixture path from the application's testdata.
func applierValuesFixture(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("..", "testdata", "integration", "applier-values", name)
}

// renderWithValues renders the source chart with one values file, returning
// helm's combined output and whether it succeeded. The coding chart validates
// values against values.schema.json before templating, so a schema rejection
// surfaces on the source chart without staged profiles.
func renderWithValues(t *testing.T, values string) (string, bool) {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	out, err := exec.Command("helm", "template", "t", findChartDir(t), "-f", values).CombinedOutput()
	return string(out), err == nil
}

// TestChartSchemaAcceptsAConformingPatch proves the schema does not reject the
// values shape an operator actually decides. A schema that rejected everything
// would pass the rejection test below while breaking every apply.
func TestChartSchemaAcceptsAConformingPatch(t *testing.T) {
	out, ok := renderWithValues(t, applierValuesFixture(t, "conforming.yaml"))
	if !ok {
		t.Fatalf("the conforming patch did not render:\n%s", out)
	}
	// The patch's own value must appear, or the render succeeded while ignoring
	// the file the test thinks it exercised.
	if !strings.Contains(out, "321Mi") {
		t.Errorf("the render does not carry 321Mi; the values file was not applied:\n%s", out)
	}
}

// TestChartSchemaRejectsANonConformingPatch is the assertion integration:applier
// cannot make. It requires the rejection to name the constraint that caused it:
// helm exiting non-zero proves nothing on its own, since a chart that failed to
// render for an unrelated reason exits non-zero too.
func TestChartSchemaRejectsANonConformingPatch(t *testing.T) {
	out, ok := renderWithValues(t, applierValuesFixture(t, "non-conforming.yaml"))
	if ok {
		t.Fatalf("the non-conforming patch rendered clean; values.schema.json is not enforcing the role replica bound")
	}
	for _, want := range []string{
		"schema",                  // the failure came from schema validation
		"/roles/planner/replicas", // at the field the fixture violates
		"maximum",                 // the constraint it violated
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the rejection does not mention %q, so it may not be the schema rejecting:\n%s", want, out)
		}
	}
}

// TestChartSchemaConstrainsRoleReplicas pins the constraint the fixture rests on.
// Loosening the replica bound in values.schema.json must break something, and
// without this it would break only the fixture's rejection -- which reads as a
// test problem rather than as the guard being removed.
func TestChartSchemaConstrainsRoleReplicas(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(findChartDir(t), "values.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	schema := string(data)
	if !strings.Contains(schema, `"maximum": 1`) {
		t.Error("values.schema.json no longer bounds a role to one replica; " +
			"a patch scaling a role past the single shared-workspace writer would validate")
	}
	if !strings.Contains(schema, `"const": 18200`) {
		t.Error("values.schema.json no longer pins the planner request port; " +
			"a drifted port would diverge from the applier exec words and the rest.yaml listener")
	}
}
