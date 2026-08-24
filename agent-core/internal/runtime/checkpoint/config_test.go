// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package checkpoint

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func TestRegisterFlagsDefaultsAndHelp(t *testing.T) {
	t.Parallel()
	var cfg Config
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg.RegisterFlags(fs)

	for _, tc := range []struct {
		name  string
		def   string
		usage string
	}{
		{name: "dolt-dsn", def: "", usage: "MySQL-wire DSN to a dolt sql-server for the persistent checkpoint backend (default: no persistence)"},
		{name: "resume-checkpoint", def: "", usage: "checkpoint ID to resume from"},
		{name: "resume-signal", def: "", usage: "resume signal override (default: required machine resume_signal)"},
	} {
		flag := fs.Lookup(tc.name)
		require.NotNil(t, flag, tc.name)
		require.Equal(t, tc.def, flag.DefValue, tc.name)
		require.Equal(t, tc.usage, flag.Usage, tc.name)
	}
}

func TestRegisterFlagsDoesNotTouchCommandLine(t *testing.T) {
	t.Parallel()
	require.Nil(t, pflag.CommandLine.Lookup("dolt-dsn"))
	var cfg Config
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg.RegisterFlags(fs)
	require.Nil(t, pflag.CommandLine.Lookup("dolt-dsn"))
	require.NotNil(t, fs.Lookup("dolt-dsn"))
}

func TestOpenDefaultsToNoop(t *testing.T) {
	t.Parallel()
	opened, err := Config{}.Open(core.MachineSpec{}, "run-test")
	require.NoError(t, err)
	require.IsType(t, core.NoopCheckpoint{}, opened.Checkpoint)
	require.Nil(t, opened.CloseFunc)
}

func TestOpenWithDoltDSNOpensDoltBackend(t *testing.T) {
	t.Parallel()
	RegisterDriver()
	_, err := Config{DoltDSN: "not-a-valid-dsn"}.Open(core.MachineSpec{}, "run-test")
	require.ErrorIs(t, err, core.ErrDolt)
}

func TestOpenUsesOpenDoltSeam(t *testing.T) {
	original := OpenDolt
	t.Cleanup(func() { OpenDolt = original })
	var gotID string
	OpenDolt = func(_, id string, _ func(core.State) bool) (Closeable, error) {
		gotID = id
		return noopCloseable{}, nil
	}
	opened, err := Config{DoltDSN: "test-dsn"}.Open(core.MachineSpec{}, "run-shared")
	require.NoError(t, err)
	require.Equal(t, "run-shared", gotID)
	require.Equal(t, "loop checkpoint", opened.Label)
	require.NoError(t, opened.Close())
}

func TestResumeID(t *testing.T) {
	t.Parallel()
	id, err := Config{}.ResumeID()
	require.NoError(t, err)
	require.Empty(t, id)

	id, err = Config{ResumeCheckpoint: "run-1"}.ResumeID()
	require.NoError(t, err)
	require.Equal(t, "run-1", id)

	_, err = Config{ResumeCheckpoint: "latest"}.ResumeID()
	require.ErrorContains(t, err, "--resume-checkpoint")
	require.ErrorContains(t, err, "provide an explicit run id")
}

type noopCloseable struct{ core.NoopCheckpoint }

func (noopCloseable) Close() error { return nil }
