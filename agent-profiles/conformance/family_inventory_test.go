// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"io/fs"
	"path/filepath"
	"testing"
)

func TestShippedProfilesMatchFoundationModels(t *testing.T) {
	t.Parallel()
	shipped := map[string]bool{}
	err := filepath.WalkDir(ProfilePath("agents"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() != "profile.yaml" {
			return nil
		}
		rel, err := filepath.Rel(ProfilesRoot(), path)
		if err != nil {
			return err
		}
		shipped[filepath.ToSlash(rel)] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	var taxonomy struct {
		Roles []struct {
			ID          string `yaml:"id"`
			ProfilePath string `yaml:"profile_path"`
		} `yaml:"roles"`
		Fixtures []struct {
			ProfilePath string `yaml:"profile_path"`
		} `yaml:"conformance_fixtures"`
	}
	readRoleYAML(t, "docs/specs/semantic-models/agent-role-taxonomy.yaml", &taxonomy)
	roleIDs := map[string]bool{}
	for _, role := range taxonomy.Roles {
		if !shipped[role.ProfilePath] {
			t.Errorf("taxonomy role %s points to non-shipped profile %s", role.ID, role.ProfilePath)
		}
		delete(shipped, role.ProfilePath)
		roleIDs[role.ID] = true
	}
	if len(shipped) != 0 {
		t.Errorf("shipped profiles missing taxonomy roles: %v", shipped)
	}
	for _, fixture := range taxonomy.Fixtures {
		if filepath.ToSlash(fixture.ProfilePath) == "agents" ||
			filepath.Dir(filepath.ToSlash(fixture.ProfilePath)) == "agents" {
			t.Errorf("conformance fixture classified under agents/: %s", fixture.ProfilePath)
		}
	}

	var model struct {
		RoleGroups []struct {
			Profiles []string `yaml:"profiles"`
		} `yaml:"role_groups"`
		Actors []struct {
			Profile string `yaml:"profile"`
		} `yaml:"actor_boundaries"`
	}
	readRoleYAML(t, "docs/specs/semantic-models/multi-agent-profile-system.yaml", &model)
	grouped := map[string]bool{}
	for _, group := range model.RoleGroups {
		for _, profile := range group.Profiles {
			grouped[profile] = true
		}
	}
	actors := map[string]bool{}
	for _, actor := range model.Actors {
		actors[actor.Profile] = true
	}
	for role := range roleIDs {
		if !grouped[role] {
			t.Errorf("taxonomy role %s has no multi-agent role group", role)
		}
		if !actors[role] {
			t.Errorf("taxonomy role %s has no actor boundary", role)
		}
	}
}

func TestFamilySRDInventoryMatchesSpecificationIndex(t *testing.T) {
	t.Parallel()
	var foundation struct {
		Families []string `yaml:"family_srds"`
		Fixtures []string `yaml:"conformance_fixture_srds"`
	}
	readRoleYAML(t, "docs/specs/software-requirements/srd001-agent-functional-blocks.yaml", &foundation)
	var specifications struct {
		SRDs []struct {
			ID     string `yaml:"id"`
			Status string `yaml:"status"`
		} `yaml:"srd_index"`
	}
	readRoleYAML(t, "docs/SPECIFICATIONS.yaml", &specifications)
	indexed := map[string]string{}
	for _, srd := range specifications.SRDs {
		indexed[srd.ID] = srd.Status
	}
	for _, inventory := range [][]string{foundation.Families, foundation.Fixtures} {
		for _, id := range inventory {
			status, ok := indexed[id]
			if !ok {
				t.Errorf("srd001 inventory references unindexed %s", id)
			} else if status == "relocated" {
				t.Errorf("srd001 current inventory references relocated %s", id)
			}
		}
	}
	wantFamilies := map[string]bool{
		"srd002-executor": true, "srd003-critic": true, "srd004-planner": true,
		"srd005-jurist": true, "srd006-bench": true, "srd008-monitor": true,
		"srd011-knowledge-manager": true, "srd012-chroma-corpus-agents": true,
		"srd018-assembler": true, "srd019-mock": true,
	}
	for _, id := range foundation.Families {
		delete(wantFamilies, id)
	}
	if len(wantFamilies) != 0 {
		t.Errorf("srd001 missing shipped family SRDs: %v", wantFamilies)
	}
}
