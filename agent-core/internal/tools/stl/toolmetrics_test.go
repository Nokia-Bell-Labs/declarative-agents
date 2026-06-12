// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTestMetrics_AllPass(t *testing.T) {
	output := `=== RUN   TestAdd
--- PASS: TestAdd (0.00s)
=== RUN   TestSub
--- PASS: TestSub (0.00s)
PASS
ok  	example.com/calc	0.003s`

	m := ParseTestMetrics(output)
	require.NotNil(t, m)
	assert.Equal(t, 2, m.Total)
	assert.Equal(t, 2, m.Passed)
	assert.Equal(t, 0, m.Failed)
}

func TestParseTestMetrics_MixedResults(t *testing.T) {
	output := `=== RUN   TestAdd
--- PASS: TestAdd (0.00s)
=== RUN   TestSub
--- FAIL: TestSub (0.00s)
    calc_test.go:15: expected 3, got 4
=== RUN   TestMul
--- PASS: TestMul (0.00s)
=== RUN   TestDiv
--- FAIL: TestDiv (0.00s)
    calc_test.go:25: divide by zero
FAIL
FAIL	example.com/calc	0.004s`

	m := ParseTestMetrics(output)
	require.NotNil(t, m)
	assert.Equal(t, 4, m.Total)
	assert.Equal(t, 2, m.Passed)
	assert.Equal(t, 2, m.Failed)
}

func TestParseTestMetrics_BuildFailed(t *testing.T) {
	output := `# example.com/calc
./calc.go:5:2: undefined: fmt
FAIL	example.com/calc [build failed]`

	m := ParseTestMetrics(output)
	require.NotNil(t, m)
	assert.Equal(t, 0, m.Total)
	assert.Equal(t, 0, m.Passed)
	assert.Equal(t, 0, m.Failed)
	assert.Equal(t, true, m.Details["build_failed"])
}

func TestParseTestMetrics_WithSkips(t *testing.T) {
	output := `=== RUN   TestAdd
--- PASS: TestAdd (0.00s)
=== RUN   TestIntegration
--- SKIP: TestIntegration (0.00s)
    calc_test.go:30: skipping integration test
PASS
ok  	example.com/calc	0.002s`

	m := ParseTestMetrics(output)
	require.NotNil(t, m)
	assert.Equal(t, 2, m.Total)
	assert.Equal(t, 1, m.Passed)
	assert.Equal(t, 0, m.Failed)
	assert.Equal(t, 1, m.Details["skipped"])
}

func TestParseTestMetrics_NoTests(t *testing.T) {
	output := `testing: warning: no tests to run
PASS
ok  	example.com/calc	0.001s`

	m := ParseTestMetrics(output)
	require.NotNil(t, m)
	assert.Equal(t, 0, m.Total)
	assert.Equal(t, 0, m.Passed)
	assert.Equal(t, 0, m.Failed)
}

func TestParseBuildMetrics_Clean(t *testing.T) {
	m := ParseBuildMetrics("")
	require.NotNil(t, m)
	assert.Equal(t, 0, m.Total)
	assert.Equal(t, 0, m.Failed)
}

func TestParseBuildMetrics_SingleFile(t *testing.T) {
	output := `calc.go:5:2: undefined: fmt
calc.go:10:3: too many arguments`

	m := ParseBuildMetrics(output)
	require.NotNil(t, m)
	assert.Equal(t, 1, m.Total)
	assert.Equal(t, 1, m.Failed)
	assert.Equal(t, 0, m.Passed)
	assert.Equal(t, 2, m.Details["error_lines"])
}

func TestParseBuildMetrics_MultipleFiles(t *testing.T) {
	output := `calc.go:5:2: undefined: fmt
calc.go:10:3: too many arguments
helper.go:3:1: expected declaration
main.go:20:5: cannot use x`

	m := ParseBuildMetrics(output)
	require.NotNil(t, m)
	assert.Equal(t, 3, m.Total)
	assert.Equal(t, 3, m.Failed)
	assert.Equal(t, 0, m.Passed)
	assert.Equal(t, 4, m.Details["error_lines"])
}
