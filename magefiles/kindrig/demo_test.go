// Copyright (c) 2026 Nokia. All rights reserved.

package kindrig

import (
	"errors"
	"strings"
	"testing"
)

func TestDemoUpRetainsSuccessfulOwnedCluster(t *testing.T) {
	kind := &fakeKind{}
	deployed := false
	err := DemoUp(kind.run, "da-example-demo", testConfig(t), 0, func(cluster Cluster) error {
		deployed = cluster.Created
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !deployed || kind.issued("delete") {
		t.Fatalf("deployed=%v calls=%v", deployed, kind.calls)
	}
}

func TestDemoUpFailureReleasesOnlyOwnedCluster(t *testing.T) {
	deployErr := errors.New("helm failed")
	tests := []struct {
		name       string
		existing   []string
		wantDelete bool
	}{
		{name: "owned", wantDelete: true},
		{name: "reused", existing: []string{"da-example-demo"}, wantDelete: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kind := &fakeKind{existing: test.existing}
			err := DemoUp(kind.run, "da-example-demo", testConfig(t), 0,
				func(Cluster) error { return deployErr })
			if !errors.Is(err, deployErr) {
				t.Fatalf("error = %v", err)
			}
			if got := kind.issued("delete"); got != test.wantDelete {
				t.Fatalf("delete=%v want=%v calls=%v", got, test.wantDelete, kind.calls)
			}
		})
	}
}

func TestDemoDownDeletesOnlyNamedDemoCluster(t *testing.T) {
	kind := &fakeKind{existing: []string{"developer", "da-example-demo"}}
	if err := DemoDown(kind.run, "da-example-demo"); err != nil {
		t.Fatal(err)
	}
	deleteCall := kind.lastCall("delete")
	if strings.Join(deleteCall, " ") != "delete cluster --name da-example-demo" {
		t.Fatalf("delete call = %v", deleteCall)
	}
}
