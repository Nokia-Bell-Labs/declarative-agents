// Copyright (c) 2026 Nokia. All rights reserved.

package dolt

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIdentityNormalizationExcludesCredentials(t *testing.T) {
	t.Parallel()

	first, err := IdentityFromDSN(
		"alice:secret@tcp(LOCALHOST:3306)/ignored?tls=true",
		"Domain_DB",
	)
	require.NoError(t, err)
	second, err := IdentityFromDSN(
		"bob:different@tcp(localhost:3306)/another?tls=false",
		"domain_db",
	)
	require.NoError(t, err)

	require.True(t, first.SameServer(second))
	require.True(t, first.SameDatabase(second))
	require.Equal(t, "tcp://localhost:3306", first.Server)
	require.Equal(t, "domain_db", first.Database)
	require.NotContains(t, first.Server+first.Database, "alice")
	require.NotContains(t, first.Server+first.Database, "secret")
	require.NotContains(t, first.Server+first.Database, "tls")
}

func TestIdentityComparisonDistinguishesDatabaseAndServer(t *testing.T) {
	t.Parallel()

	domain, err := IdentityFromDSN("u:p@tcp(localhost:3306)/", "domain")
	require.NoError(t, err)
	checkpoint, err := IdentityFromDSN("u:p@tcp(localhost:3306)/", "checkpoint")
	require.NoError(t, err)
	remote, err := IdentityFromDSN("u:p@tcp(db.example:3306)/", "domain")
	require.NoError(t, err)

	require.True(t, domain.SameServer(checkpoint))
	require.False(t, domain.SameDatabase(checkpoint))
	require.False(t, domain.SameServer(remote))
	require.False(t, domain.SameDatabase(remote))
}

func TestIdentityUsesDSNDatabaseWhenNotOverridden(t *testing.T) {
	t.Parallel()

	identity, err := IdentityFromDSN("u:p@unix(/tmp/../tmp/dolt.sock)/AgentDomain", "")
	require.NoError(t, err)
	require.Equal(t, "unix:///tmp/dolt.sock", identity.Server)
	require.Equal(t, "agentdomain", identity.Database)
}
