// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package corpus

import (
	"reflect"
	"testing"
)

func TestSetFront(t *testing.T) {
	doc := []byte("---\nid: a\ntitle: A\n---\n\nBody\n")
	got, err := SetFront(doc, FrontReviewer, "carol")
	if err != nil || string(got) != "---\nid: a\ntitle: A\nreviewer: carol\n---\n\nBody\n" {
		t.Fatalf("added = %q, %v", got, err)
	}
	if got, _ = SetFront(got, FrontReviewer, "dan"); string(got) != "---\nid: a\ntitle: A\nreviewer: dan\n---\n\nBody\n" {
		t.Fatalf("rewritten = %q", got)
	}
	if got, _ = SetFront(got, FrontReviewer, ""); string(got) != string(doc) {
		t.Fatalf("dropped = %q", got)
	}
	if _, err := SetFront([]byte("no block\n"), FrontReviewer, "x"); err == nil {
		t.Fatal("a document with no frontmatter was changed")
	}
}

func TestFrontNames(t *testing.T) {
	for _, doc := range []string{"---\nid: a\nauthors: alice, bob\n---\n", "---\nid: a\nauthors: [alice, bob]\n---\n"} {
		front, err := Front([]byte(doc))
		if err != nil {
			t.Fatal(err)
		}
		if got := FrontNames(front, FrontAuthors); !reflect.DeepEqual(got, []string{"alice", "bob"}) {
			t.Errorf("%q = %v", doc, got)
		}
	}
	front, _ := Front([]byte("---\nid: a\n---\n"))
	if got := FrontNames(front, FrontAuthors); got != nil {
		t.Errorf("no key = %v", got)
	}
}
