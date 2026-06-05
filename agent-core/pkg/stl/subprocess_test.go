// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"fmt"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
)

func TestSubprocessResult_Success(t *testing.T) {
	res := SubprocessResult("test-tool", []byte("ok\n"), nil)
	assert.Equal(t, core.ToolDone, res.Signal)
	assert.Equal(t, "ok", res.Output)
	assert.Equal(t, "test-tool", res.CommandName)
}

func TestSubprocessResult_ExitError(t *testing.T) {
	cmd := exec.Command("false")
	err := cmd.Run()
	res := SubprocessResult("build", []byte("error output\n"), err)
	assert.Equal(t, core.ToolFailed, res.Signal)
	assert.Equal(t, "error output", res.Output)
}

func TestSubprocessResult_InfraError(t *testing.T) {
	res := SubprocessResult("build", nil, fmt.Errorf("binary not found"))
	assert.Equal(t, core.CommandError, res.Signal)
	assert.Error(t, res.Err)
}

func TestCapOutput_Short(t *testing.T) {
	assert.Equal(t, "a\nb\nc", CapOutput("a\nb\nc", 10))
}

func TestCapOutput_Truncated(t *testing.T) {
	result := CapOutput("a\nb\nc\nd\ne", 3)
	assert.Contains(t, result, "a\nb\nc")
	assert.Contains(t, result, "2 lines omitted")
}

func TestExtractStringParam(t *testing.T) {
	json := `{"parameters":{"path":"hello.go","query":"func"}}`
	assert.Equal(t, "hello.go", ExtractStringParam(json, "path"))
	assert.Equal(t, "func", ExtractStringParam(json, "query"))
	assert.Equal(t, "", ExtractStringParam(json, "missing"))
}

func TestExtractIntParam(t *testing.T) {
	json := `{"parameters":{"start_line":5,"max_depth":3}}`
	assert.Equal(t, 5, ExtractIntParam(json, "start_line"))
	assert.Equal(t, 3, ExtractIntParam(json, "max_depth"))
	assert.Equal(t, 0, ExtractIntParam(json, "missing"))
}

func TestExtractStringParam_InvalidJSON(t *testing.T) {
	assert.Equal(t, "", ExtractStringParam("not json", "key"))
}

func TestFailedParamCmd(t *testing.T) {
	cmd := &FailedParamCmd{ToolName: "read", Missing: "path"}
	assert.Equal(t, "read", cmd.Name())
	res := cmd.Execute()
	assert.Equal(t, core.ToolFailed, res.Signal)
	assert.Contains(t, res.Output, "path")
}
