// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package identity names, each in one place, the two repositories
// scrum-master's code refers to (GH-2755): the runtime it builds on, whose
// agent-core, spec tool, and corpus format a program corpus pins, and
// scrum-master's own home, where its releases, install script, and issues
// live. A test holds install.sh, plugin.json, the Dockerfile, and the
// README to these values, and fails on either name written anywhere else
// in Go.
package identity

// Runtime is the declarative-agents repository scrum-master builds on, as
// a corpus's docs/external-pins.yaml names it.
const Runtime = "github.com/Nokia-Bell-Labs/declarative-agents"

// RuntimeURL is where the runtime publishes each release's binaries and
// source.
const RuntimeURL = "https://" + Runtime

// Home is the repository scrum-master's own releases are published to, as
// owner/name on its forge, and ReleaseAccount the account they are
// published as. A fork releasing elsewhere overrides both at release time
// (eng03).
const (
	Home           = "Nokia-Bell-Labs/declarative-agents"
	ReleaseAccount = "petar-djukic"
)

// HomeURL is the home repository's page; ReleasesURL the page install.sh
// downloads a release from by default.
const (
	HomeURL     = "https://github.com/" + Home
	ReleasesURL = HomeURL + "/releases"
)

// InstallURL is where a person fetches install.sh from: the home
// repository's default branch.
const InstallURL = "https://raw.githubusercontent.com/" + Home + "/main/applications/scrum-master/install.sh"
