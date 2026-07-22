// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadTestGraph(t *testing.T) *Graph {
	t.Helper()
	c, err := LoadCorpus(filepath.Join("testdata", "valid"))
	require.NoError(t, err)
	g, err := BuildGraph(c)
	require.NoError(t, err)
	return g
}

func TestBuildGraph_NodeCounts(t *testing.T) {
	g := loadTestGraph(t)

	assert.Equal(t, 2, len(g.NodesByKind(KindRelease)), "releases")
	assert.Equal(t, 3, len(g.NodesByKind(KindSRD)), "SRDs")
	assert.Equal(t, 4, len(g.NodesByKind(KindReqGroup)), "req-groups (R1+R2 for auth, R1 for api, R1 for storage)")
	assert.Equal(t, 8, len(g.NodesByKind(KindReqItem)), "req-items total")
	assert.Equal(t, 4, len(g.NodesByKind(KindAC)), "ACs total")
	assert.Equal(t, 1, len(g.NodesByKind(KindUseCase)), "use-cases")
	assert.Equal(t, 1, len(g.NodesByKind(KindTestSuite)), "test-suites")
	assert.Equal(t, 2, len(g.NodesByKind(KindTestCase)), "test-cases")
}

func TestBuildGraph_TotalNodeCount(t *testing.T) {
	g := loadTestGraph(t)
	// 2 releases + 3 SRDs + 4 groups + 8 items + 4 ACs + 1 UC + 1 TS + 2 TC = 25
	// + 1 machine + 4 states + 3 signals + 3 transitions = 36
	// + 1 tool-decl = 37
	assert.Equal(t, 37, g.NodeCount())
}

func TestBuildGraph_ReleaseOrdering(t *testing.T) {
	g := loadTestGraph(t)

	orderEdges := g.EdgesByRel(RelOrders)
	require.Len(t, orderEdges, 1)
	assert.Equal(t, "release:00.0", orderEdges[0].Source)
	assert.Equal(t, "release:00.1", orderEdges[0].Target)
}

func TestBuildGraph_SRDAssignment(t *testing.T) {
	g := loadTestGraph(t)

	authNode, ok := g.Node("srd001-auth")
	require.True(t, ok)
	assert.Equal(t, "00.0", authNode.Release)

	storageNode, ok := g.Node("srd003-storage")
	require.True(t, ok)
	assert.Equal(t, "00.1", storageNode.Release)
}

func TestBuildGraph_ContainsEdges(t *testing.T) {
	g := loadTestGraph(t)

	srdContains := g.OutgoingByRel("srd001-auth", RelContains)
	assert.Contains(t, srdContains, "srd001-auth:R1")
	assert.Contains(t, srdContains, "srd001-auth:R2")
	assert.Contains(t, srdContains, "srd001-auth:AC1")
	assert.Contains(t, srdContains, "srd001-auth:AC2")

	groupContains := g.OutgoingByRel("srd001-auth:R1", RelContains)
	assert.Contains(t, groupContains, "srd001-auth:R1.1")
	assert.Contains(t, groupContains, "srd001-auth:R1.2")
	assert.Contains(t, groupContains, "srd001-auth:R1.3")
}

func TestBuildGraph_IntraSRDSucceeds(t *testing.T) {
	g := loadTestGraph(t)

	succ := g.OutgoingByRel("srd001-auth:R1.1", RelSucceeds)
	assert.Contains(t, succ, "srd001-auth:R1.2")

	succ = g.OutgoingByRel("srd001-auth:R1.3", RelSucceeds)
	assert.Contains(t, succ, "srd001-auth:R2.1", "cross-group ordering")
}

func TestBuildGraph_InterSRDDependsOn(t *testing.T) {
	g := loadTestGraph(t)

	deps := g.OutgoingByRel("srd002-api", RelDependsOn)
	assert.Contains(t, deps, "srd001-auth")

	deps = g.OutgoingByRel("srd003-storage", RelDependsOn)
	assert.Contains(t, deps, "srd001-auth")
}

func TestBuildGraph_ACTraces(t *testing.T) {
	g := loadTestGraph(t)

	traces := g.OutgoingByRel("srd001-auth:AC1", RelTraces)
	assert.Len(t, traces, 3)
	assert.Contains(t, traces, "srd001-auth:R1.1")
	assert.Contains(t, traces, "srd001-auth:R1.2")
	assert.Contains(t, traces, "srd001-auth:R1.3")
}

func TestBuildGraph_UseCaseTouches(t *testing.T) {
	g := loadTestGraph(t)

	touches := g.OutgoingByRel("rel00.0-uc001-login", RelTouches)
	assert.Contains(t, touches, "srd001-auth")
	assert.Contains(t, touches, "srd002-api")
}

func TestBuildGraph_UseCaseCitesGroups(t *testing.T) {
	g := loadTestGraph(t)

	cites := g.OutgoingByRel("rel00.0-uc001-login", RelCites)
	assert.Contains(t, cites, "srd001-auth:R1")
	assert.Contains(t, cites, "srd001-auth:R2")
	assert.Contains(t, cites, "srd002-api:R1")
}

func TestBuildGraph_UseCaseCitesACs(t *testing.T) {
	g := loadTestGraph(t)

	cites := g.OutgoingByRel("rel00.0-uc001-login", RelCites)
	assert.Contains(t, cites, "srd001-auth:AC1")
	assert.Contains(t, cites, "srd001-auth:AC2")
}

func TestBuildGraph_TestSuiteCovers(t *testing.T) {
	g := loadTestGraph(t)

	covers := g.OutgoingByRel("test-rel00.0", RelCovers)
	assert.Contains(t, covers, "rel00.0-uc001-login")
}

func TestBuildGraph_TestCaseCovers(t *testing.T) {
	g := loadTestGraph(t)

	covers := g.OutgoingByRel("test-rel00.0:TestLogin_ValidCredentials", RelCovers)
	assert.Contains(t, covers, "srd001-auth:AC1")

	covers = g.OutgoingByRel("test-rel00.0:TestLogin_InvalidCredentials", RelCovers)
	assert.Contains(t, covers, "srd001-auth:AC2")
}

func TestBuildGraph_AssignEdges(t *testing.T) {
	g := loadTestGraph(t)

	assigns := g.OutgoingByRel("release:00.0", RelAssigns)
	assert.Contains(t, assigns, "srd001-auth")
	assert.Contains(t, assigns, "srd002-api")

	assigns = g.OutgoingByRel("release:00.1", RelAssigns)
	assert.Contains(t, assigns, "srd003-storage")
}

func TestBuildGraph_NodeLookup(t *testing.T) {
	g := loadTestGraph(t)

	n, ok := g.Node("srd001-auth:R1.2")
	require.True(t, ok)
	assert.Equal(t, KindReqItem, n.Kind)
	assert.Equal(t, "srd001-auth", n.SRDID)
	assert.Equal(t, "R1", n.Group)
	assert.Equal(t, 2, n.Weight)

	_, ok = g.Node("nonexistent")
	assert.False(t, ok)
}

func TestBuildGraph_IncomingByRel(t *testing.T) {
	g := loadTestGraph(t)

	incoming := g.IncomingByRel("srd001-auth", RelDependsOn)
	assert.Contains(t, incoming, "srd002-api")
	assert.Contains(t, incoming, "srd003-storage")
}

func TestBuildGraph_EdgesReturnsSorted(t *testing.T) {
	g := loadTestGraph(t)
	edges := g.Edges()
	for i := 1; i < len(edges); i++ {
		prev := edges[i-1]
		cur := edges[i]
		if prev.Source == cur.Source {
			assert.LessOrEqual(t, prev.Target, cur.Target)
		} else {
			assert.Less(t, prev.Source, cur.Source)
		}
	}
}

func TestParseTouchpoint(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		input     string
		wantSRD   string
		wantGroup []string
	}{
		{name: "multiple requirement groups", input: "srd005-cli R1, R2, R3 -- description", wantSRD: "srd005-cli", wantGroup: []string{"R1", "R2", "R3"}},
		{name: "tagged reference", input: "T1: srd005-cli R1, R2 -- description", wantSRD: "srd005-cli", wantGroup: []string{"R1", "R2"}},
		{name: "multi-digit tag", input: "T12: srd026-lifecycle-tools R1 -- lifecycle tools", wantSRD: "srd026-lifecycle-tools", wantGroup: []string{"R1"}},
		{name: "missing tag colon", input: "T1 srd005-cli R1 -- malformed tag"},
		{name: "nonnumeric tag", input: "TA: srd005-cli R1 -- malformed tag"},
		{name: "unknown SRD remains parseable", input: "T1: srd999-missing R1 -- unknown SRD stays parseable", wantSRD: "srd999-missing", wantGroup: []string{"R1"}},
		{name: "untagged reference", input: "srd001-auth R1 -- desc", wantSRD: "srd001-auth", wantGroup: []string{"R1"}},
		{name: "unrelated prose", input: "agent-core telemetry -- something"},
		{name: "empty", input: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srd, groups := parseTouchpoint(tt.input)
			assert.Equal(t, tt.wantSRD, srd)
			assert.Equal(t, tt.wantGroup, groups)
		})
	}
}

func TestParseACTrace(t *testing.T) {
	srd, ac := parseACTrace("srd001-auth AC1")
	assert.Equal(t, "srd001-auth", srd)
	assert.Equal(t, "AC1", ac)

	srd, ac = parseACTrace("bad")
	assert.Empty(t, srd)
	assert.Empty(t, ac)
}
