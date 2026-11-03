// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package routing

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

type forgeStub struct {
	groups map[string][]string
	files  map[string]bool
}

func (f forgeStub) GroupProjects(group string) ([]string, error) {
	p, ok := f.groups[group]
	if !ok {
		return nil, errors.New("forbidden")
	}
	return p, nil
}

func (f forgeStub) HostTree(string, string) ([]string, error) { return nil, nil }

func (f forgeStub) HostFile(project, _ string) ([]byte, string, error) {
	if f.files[project] {
		return []byte("x"), "c", nil
	}
	return nil, "", tracker.ErrNotFound
}

func TestSystemsIn(t *testing.T) {
	f := forgeStub{groups: map[string][]string{
		"acme/prog/pods/harness": {"acme/prog/pods/harness/opt"},
		"acme/prog":              {"acme/prog/program", "acme/prog/pods/harness/opt"},
		"acme":                   {"acme/other", "acme/prog/program"},
	}, files: map[string]bool{"acme/prog/program": true, "acme/other": true}}
	got, err := systemsIn(f, "acme/prog/pods/harness/opt")
	if err != nil || !reflect.DeepEqual(got, []string{"acme/prog/program"}) {
		t.Fatalf("nearest group = %v, %v; acme/other is further away", got, err)
	}
	f.groups["acme/prog"] = []string{"acme/prog/a", "acme/prog/b"}
	f.files = map[string]bool{"acme/prog/a": true, "acme/prog/b": true}
	if got, err = systemsIn(f, "acme/prog/x"); err != nil || len(got) != 2 {
		t.Fatalf("two in one group = %v, %v", got, err)
	}
	if got, err = systemsIn(forgeStub{groups: map[string][]string{}}, "acme/prog/x"); err != nil || got != nil {
		t.Fatalf("groups that cannot be read = %v, %v", got, err)
	}
	many := make([]string, maxGroupProjects+1)
	if _, err = systemsIn(forgeStub{groups: map[string][]string{"acme": many}}, "acme/x"); err == nil || !strings.Contains(err.Error(), "--system") {
		t.Fatalf("a very large group = %v", err)
	}
}
