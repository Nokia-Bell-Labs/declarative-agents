// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

const releasePlatformGateName = "platform"

// releasePlatform owns da-platform for one release run (GH-2215). The platform
// gate boots it fresh after the root audit and before any application gate;
// application integration targets find it running and reuse it without
// ownership. The run tears it down on every exit path, so no cluster state
// survives from one release to the next.
type releasePlatform struct {
	options  kindrig.PlatformOptions
	start    func(kindrig.PlatformOptions) (*kindrig.Platform, error)
	stopFn   func(*kindrig.Platform, bool)
	platform *kindrig.Platform
}

func newReleasePlatform(root, commit string) *releasePlatform {
	revision := commit
	if len(revision) > 12 {
		revision = revision[:12]
	}
	return &releasePlatform{
		options: kindrig.PlatformOptions{
			EvidenceDirectory: filepath.Join(root, "build", "kind-evidence",
				kindrig.PlatformClusterName+"-"+revision),
		},
		start:  kindrig.StartPlatform,
		stopFn: (*kindrig.Platform).Stop,
	}
}

// gate is the exclusive release gate that boots da-platform and runs its
// conformance suite. A platform failure stops the release before any
// application gate can report it as an application failure.
func (p *releasePlatform) gate() releaseGate {
	return releaseGate{
		name:      releasePlatformGateName,
		lane:      releasePlatformGateName,
		exclusive: true,
		run: func() error {
			platform, err := p.start(p.options)
			if err != nil {
				return err
			}
			p.platform = platform
			return nil
		},
	}
}

// stop deletes the release's platform, capturing its evidence first when the
// release failed. It is a no-op when the platform gate never booted one.
func (p *releasePlatform) stop(failed bool) {
	if p.platform == nil {
		return
	}
	started := time.Now()
	p.stopFn(p.platform, failed)
	p.platform = nil
	outcome := "success"
	if failed {
		outcome = "failure"
	}
	fmt.Printf("phase target=release name=platform-teardown elapsed=%s outcome=%s lane=%s\n",
		time.Since(started).Round(time.Millisecond), outcome, releasePlatformGateName)
}

// withPlatformGate places the platform gate after the leading exclusive gates
// (the root audit), so the release audits before it spends a cluster.
func withPlatformGate(gates []releaseGate, platform releaseGate) []releaseGate {
	index := 0
	for index < len(gates) && gates[index].exclusive {
		index++
	}
	withPlatform := make([]releaseGate, 0, len(gates)+1)
	withPlatform = append(withPlatform, gates[:index]...)
	withPlatform = append(withPlatform, platform)
	return append(withPlatform, gates[index:]...)
}

// executePlatformReleaseGates runs the release schedule with the platform gate
// and tears the platform down on every exit path, including a failed gate.
func executePlatformReleaseGates(
	gates []releaseGate,
	platform *releasePlatform,
	run releaseCommandRunner,
) (err error) {
	defer func() { platform.stop(err != nil) }()
	return executeReleaseGates(withPlatformGate(gates, platform.gate()), run)
}
