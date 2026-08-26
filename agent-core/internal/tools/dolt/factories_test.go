// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package dolt

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

func TestRegisterFactoriesProbesExpectedInits(t *testing.T) {
	t.Parallel()

	want := []string{InitProvision, InitQuery, InitWrite}
	br := toolregistry.NewBuiltinRegistry()
	RegisterFactories(br, FactoryDeps{})
	require.ElementsMatch(t, want, br.Names())

	require.ElementsMatch(t, want, catalogInits(t, "dolt", toolregistry.StandardFactoryDeps{
		RegisterDolt: func(br *toolregistry.BuiltinRegistry) { RegisterFactories(br, FactoryDeps{}) },
	}))
}

func catalogInits(t *testing.T, family string, deps toolregistry.StandardFactoryDeps) []string {
	t.Helper()
	for _, entry := range toolregistry.StandardFactoryCatalog(deps) {
		if entry.Name == family {
			return entry.Inits
		}
	}
	t.Fatalf("standard catalog missing family %q", family)
	return nil
}

func TestRegisterFactoriesDefersIdentityErrorUntilBuild(t *testing.T) {
	t.Parallel()

	identityErr := errors.New("resolve active Dolt checkpoint identity: parse Dolt connection")
	br := toolregistry.NewBuiltinRegistry()
	require.NotPanics(t, func() {
		RegisterFactories(br, FactoryDeps{CheckpointIdentityErr: identityErr})
	})
	require.ElementsMatch(t, []string{InitProvision, InitQuery, InitWrite}, br.Names())

	factory, ok := br.Resolve(InitQuery)
	require.True(t, ok)
	_, err := factory(catalog.ToolDef{Name: "lookup_records"}, nil)
	require.ErrorIs(t, err, identityErr)
}

func TestStaticConnectionsResolvesNamedDSN(t *testing.T) {
	t.Parallel()
	c := StaticConnections{"DOLT_WORD_DSN": "word:secret@tcp(localhost:3306)/ignored"}
	got, err := c.ResolveConnection(context.Background(), "DOLT_WORD_DSN", nil)
	require.NoError(t, err)
	require.Equal(t, "word:secret@tcp(localhost:3306)/ignored", got)

	_, err = c.ResolveConnection(context.Background(), "MISSING", nil)
	require.ErrorContains(t, err, `configured Dolt connection reference "MISSING" is unavailable`)
}

func TestRegisterFlagsDefaultsAndHelp(t *testing.T) {
	t.Parallel()
	var cfg Config
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg.RegisterFlags(fs)
	flag := fs.Lookup("dolt-connection")
	require.NotNil(t, flag)
	require.Equal(t, "[]", flag.DefValue)
	require.Equal(t, "named Dolt word DSN (NAME=DSN); independent of --dolt-dsn", flag.Usage)
	require.NoError(t, fs.Parse([]string{
		"--dolt-connection", "DOLT_WORD_DSN=word:secret@tcp(localhost:3306)/db",
	}))
	require.Equal(t, "word:secret@tcp(localhost:3306)/db", cfg.Connections["DOLT_WORD_DSN"])
}

func TestRegisterFlagsDoesNotTouchCommandLine(t *testing.T) {
	t.Parallel()
	require.Nil(t, pflag.CommandLine.Lookup("dolt-connection"))
	var cfg Config
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg.RegisterFlags(fs)
	require.Nil(t, pflag.CommandLine.Lookup("dolt-connection"))
	require.NotNil(t, fs.Lookup("dolt-connection"))
}
