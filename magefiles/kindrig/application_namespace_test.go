// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package kindrig

import (
	"errors"
	"strings"
	"testing"
)

func TestEnsureApplicationNamespaceCreatesAndLabels(t *testing.T) {
	var commands []string
	run := func(name string, args ...string) ([]byte, error) {
		command := strings.Join(append([]string{name}, args...), " ")
		commands = append(commands, command)
		switch {
		case strings.Contains(command, "get namespace da-fixture"):
			return nil, nil
		case strings.Contains(command, "create namespace da-fixture"):
			return []byte("created"), nil
		case strings.Contains(command, "label namespace da-fixture"):
			return []byte("labeled"), nil
		default:
			return nil, errors.New("unexpected command")
		}
	}
	created, err := ensureApplicationNamespace(run, "da-fixture")
	if err != nil || !created {
		t.Fatalf("ensure = created %t, err %v", created, err)
	}
	if got := strings.Join(commands, "\n"); !strings.Contains(got,
		"app.kubernetes.io/managed-by=apprig") {
		t.Fatalf("namespace was not ownership-labeled:\n%s", got)
	}
}

func TestEnsureApplicationNamespaceReusesOnlyApprigOwned(t *testing.T) {
	owned := func(_ string, _ ...string) ([]byte, error) {
		return []byte(`{"metadata":{"labels":{"app.kubernetes.io/managed-by":"apprig"}}}`), nil
	}
	if created, err := ensureApplicationNamespace(owned, "da-fixture"); err != nil || created {
		t.Fatalf("owned reuse = created %t, err %v", created, err)
	}
	foreign := func(_ string, _ ...string) ([]byte, error) {
		return []byte(`{"metadata":{"labels":{"app.kubernetes.io/managed-by":"someone-else"}}}`), nil
	}
	if _, err := ensureApplicationNamespace(foreign, "da-fixture"); err == nil ||
		!strings.Contains(err.Error(), "refusing adoption") {
		t.Fatalf("foreign namespace was adopted: %v", err)
	}
}

func TestDeleteApplicationNamespaceWaitsThenRechecksDataPlane(t *testing.T) {
	var commands []string
	run := func(name string, args ...string) ([]byte, error) {
		command := strings.Join(append([]string{name}, args...), " ")
		commands = append(commands, command)
		if strings.Contains(command, "get namespace da-fixture") {
			return []byte(`{"metadata":{"labels":{"app.kubernetes.io/managed-by":"apprig"}}}`), nil
		}
		return []byte("ok"), nil
	}
	if err := deleteApplicationNamespace(run, "da-fixture"); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(commands, "\n")
	ordered := []string{
		"delete namespace da-fixture --wait=false",
		"wait --for=delete namespace/da-fixture",
		"kube-system wait --for=condition=Ready",
		"kube-system rollout status deployment/coredns",
		"get --raw=/readyz",
	}
	last := -1
	for _, want := range ordered {
		index := strings.Index(got, want)
		if index < 0 || index <= last {
			t.Fatalf("commands do not contain %q in order:\n%s", want, got)
		}
		last = index
	}
}

func TestDeleteApplicationNamespaceRefusesForeignOwner(t *testing.T) {
	run := func(_ string, _ ...string) ([]byte, error) {
		return []byte(`{"metadata":{"labels":{}}}`), nil
	}
	if err := deleteApplicationNamespace(run, "da-fixture"); err == nil ||
		!strings.Contains(err.Error(), "refusing deletion") {
		t.Fatalf("foreign namespace was deleted: %v", err)
	}
}
