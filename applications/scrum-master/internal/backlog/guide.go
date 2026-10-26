// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package backlog

import (
	"flag"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
)

// Guide is the named guide as this release ships it, docs/guides/<name>.md.
func Guide(name string) (string, error) {
	b, err := scrummaster.Guides.ReadFile(path.Join(scrummaster.GuidesDir, name+".md"))
	if err != nil {
		return "", fmt.Errorf("no guide %q; --name is one of %s", name, strings.Join(guideNames(), ", "))
	}
	return string(b), nil
}

func guideNames() []string {
	var names []string
	entries, _ := fs.ReadDir(scrummaster.Guides, scrummaster.GuidesDir)
	for _, e := range entries {
		names = append(names, strings.TrimSuffix(e.Name(), ".md"))
	}
	return names
}

// GuidedRequest is what a profile's model reads first when the profile
// routes a guide into its context (GH-59): the guide, then the run's
// request, so the request still reaches the model after the guide's step.
func GuidedRequest(name, guide, request string) string {
	return fmt.Sprintf("Read the %s guide below before acting: it is how this program works, and your work follows it.\n\n%s\n\n---\n\nThe request:\n\n%s\n",
		name, strings.TrimSpace(guide), strings.TrimSpace(request))
}

func init() {
	var (
		name        string
		withRequest bool
	)
	cli.Default.Register(&cli.Command{
		Group: "backlog", Name: "guide", Summary: "a guide this release ships, to route into an agent's context; with --with-request, followed by the request read from standard input",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&name, "name", "", "the guide: "+strings.Join(guideNames(), ", ")+" (required)")
			fs.BoolVar(&withRequest, "with-request", false, "append the run's request, read from standard input, after the guide")
		},
		Run: func(ctx *cli.Context) error {
			if name == "" {
				return fmt.Errorf("%w: guide needs --name, one of %s", cli.ErrUsage, strings.Join(guideNames(), ", "))
			}
			g, err := Guide(name)
			if err != nil {
				return err
			}
			text, request := g, ""
			if withRequest {
				b, err := io.ReadAll(ctx.In)
				if err != nil {
					return err
				}
				if request = strings.TrimSpace(string(b)); request == "" {
					return fmt.Errorf("%w: --with-request read no request from standard input", cli.ErrUsage)
				}
				text = GuidedRequest(name, g, request)
			}
			return ctx.Emit(struct {
				Name    string `json:"name"`
				Guide   string `json:"guide"`
				Request string `json:"request,omitempty"`
			}{name, g, request}, text)
		},
	})
}
