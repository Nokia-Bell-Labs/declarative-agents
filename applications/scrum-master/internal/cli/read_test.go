// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker/fake"
)

// readingTracker is a tracker that knows which tracker commit it read.
type readingTracker struct {
	*fake.Fake
	read tracker.Read
}

func (r readingTracker) LastRead() (tracker.Read, bool) { return r.read, r.read.Commit != "" }

func runReading(t *testing.T, read tracker.Read, args ...string) string {
	t.Helper()
	reg := &Registry{}
	reg.Register(&Command{Group: "g", Name: "c", Run: func(ctx *Context) error {
		if _, err := ctx.Tracker(); err != nil {
			return err
		}
		return ctx.Emit(map[string]string{"result": "done"}, "done")
	}})
	var out bytes.Buffer
	tracker := func() (tracker.Tracker, error) { return readingTracker{fake.New(), read}, nil }
	if err := reg.Run(append([]string{"g", "c"}, args...), &out, tracker); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

// TestEmitNamesTheTrackerRead: a command run on a tracker that knows what
// it read names the tracker commit in its JSON and its human line, and says
// when it read offline from the cache and when it last pulled (srd042 R3.3,
// rel03.0-uc002 S1). A tracker that reads nothing adds nothing.
func TestEmitNamesTheTrackerRead(t *testing.T) {
	online := tracker.Read{Commit: "4f2a9c1d0e8b7a6f5e4d3c2b1a0f9e8d7c6b5a4f", At: "2026-11-02T09:00:00Z"}
	var got struct {
		Result string       `json:"result"`
		Read   tracker.Read `json:"tracker_read"`
	}
	if err := json.Unmarshal([]byte(runReading(t, online, "--json")), &got); err != nil {
		t.Fatal(err)
	}
	if got.Result != "done" || got.Read != online {
		t.Fatalf("json = %+v, want the result and the read", got)
	}
	if human := runReading(t, online); !strings.Contains(human, "done") || !strings.Contains(human, "tracker 4f2a9c1d0e8b") {
		t.Fatalf("human = %q, want the result and the tracker commit", human)
	}

	offline := tracker.Read{Commit: online.Commit, At: online.At, Offline: true}
	if human := runReading(t, offline); !strings.Contains(human, "offline") || !strings.Contains(human, online.At) {
		t.Fatalf("offline human = %q, want offline and the last pull time", human)
	}

	if human := runReading(t, tracker.Read{}); strings.Contains(human, "tracker") {
		t.Fatalf("human = %q, want no tracker line from a tracker that read nothing", human)
	}
}
