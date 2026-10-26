// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package release

import (
	"flag"
	"fmt"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// PeerReadMarker marks the comment that approves a release issue as a peer
// read (srd015 R1.2).
const PeerReadMarker = "<!-- scrum-master release peer-read -->"

// TaggedMarker marks the comment release tag leaves on the release issue;
// a peer read before it was for the release it tagged (GH-68).
const TaggedMarker = "<!-- scrum-master release tagged -->"

// ApproveResult is what approve reports.
type ApproveResult struct {
	Project string `json:"project"`
	IID     int    `json:"iid"`
	By      string `json:"by"`
	Posted  bool   `json:"posted"` // approve posted the comment now
}

// Approve posts the peer-read approval on a release issue as the person
// running it. A person cannot approve their own release issue, and a
// second approval by the same person posts nothing.
func Approve(t tracker.Tracker, project string, iid int) (ApproveResult, error) {
	r := ApproveResult{Project: project, IID: iid}
	i, err := t.GetIssue(project, iid)
	if err != nil {
		return r, err
	}
	me, err := tracker.UserIn(t, project)
	if err != nil {
		return r, err
	}
	r.By = me.Username
	if me.Username == i.Author {
		return r, fmt.Errorf("%s#%d was filed by %s, its author; a peer read comes from someone else (srd015 R1.2)", project, iid, i.Author)
	}
	notes, err := t.ListNotes(project, iid)
	if err != nil {
		return r, err
	}
	for _, n := range notes {
		if n.Author == me.Username && strings.Contains(n.Body, PeerReadMarker) {
			return r, nil
		}
	}
	r.Posted = true
	return r, t.CreateNote(project, iid, "Peer read: approved.\n\n"+PeerReadMarker)
}

// peerRead is who other than author approved the issue, by a thumbs-up
// award or an approval comment, or nobody.
func peerRead(t tracker.Tracker, project string, iid int, author string) (string, error) {
	notes, err := t.ListNotes(project, iid)
	if err != nil {
		return "", fmt.Errorf("could not read the release issue's comments: %w", err)
	}
	// An issue that already carried a tag needs a peer read after it: one
	// before was for the release that tag cut, and a thumbs-up, which has
	// no time to order it by, no longer counts (GH-68).
	last := -1
	for i, n := range notes {
		if strings.Contains(n.Body, TaggedMarker) || strings.HasPrefix(n.Body, "Tagged ") {
			last = i
		}
	}
	if last < 0 {
		awards, err := t.IssueAwards(project, iid)
		if err != nil {
			return "", fmt.Errorf("could not read the release issue's awards: %w", err)
		}
		for _, a := range awards {
			if a.Name == "thumbsup" && a.User != author {
				return a.User, nil
			}
		}
	}
	for _, n := range notes[last+1:] {
		if n.Author != author && n.Author != "" && strings.Contains(n.Body, PeerReadMarker) {
			return n.Author, nil
		}
	}
	return "", nil
}

func init() {
	var args struct {
		project string
		iid     int
	}
	cli.Default.Register(&cli.Command{
		Group: "release", Name: "approve", Summary: "approve a release issue as its peer read, as the person running it",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&args.project, "project", "", "project path (required)")
			fs.IntVar(&args.iid, "issue", 0, "release issue IID (required)")
		},
		Run: func(ctx *cli.Context) error {
			if args.project == "" || args.iid == 0 {
				return fmt.Errorf("%w: approve needs --project and --issue", cli.ErrUsage)
			}
			if ctx.DryRun {
				return fmt.Errorf("%w: approve is one comment and has no dry run", cli.ErrUsage)
			}
			t, err := ctx.Tracker()
			if err != nil {
				return err
			}
			r, err := Approve(t, args.project, args.iid)
			if err != nil {
				return err
			}
			human := fmt.Sprintf("%s#%d approved by %s", r.Project, r.IID, r.By)
			if !r.Posted {
				human = fmt.Sprintf("%s#%d was already approved by %s", r.Project, r.IID, r.By)
			}
			return ctx.Emit(r, human)
		},
	})
}
