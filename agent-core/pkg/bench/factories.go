// Copyright (c) 2026 Nokia. All rights reserved.

package bench

import (
	"io/fs"
	"path/filepath"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/stl"
)

// RegisterFactories registers bench builtin tool factories (serve_ui,
// launch_eval) into the provided BuiltinRegistry. BenchState is lazily
// initialized on first factory call, pulling config from the tool
// definition YAML.
func RegisterFactories(br *stl.BuiltinRegistry, assets fs.FS) {
	var bs *BenchState

	initBS := func(def stl.ToolDef) *BenchState {
		if bs != nil {
			return bs
		}

		var tc stl.ServeUIToolConfig
		_ = stl.DecodeToolConfig(def, &tc)

		dirs := []*string{&tc.DataDir, &tc.ConfigsDir, &tc.DocsDir, &tc.SourceDir, &tc.ProfilesDir}
		for _, p := range dirs {
			if *p != "" {
				if abs, err := filepath.Abs(*p); err == nil {
					*p = abs
				}
			}
		}

		cfg := ServerConfig{
			Addr:        tc.Addr,
			DataDir:     tc.DataDir,
			ConfigsDir:  tc.ConfigsDir,
			ProfilesDir: tc.ProfilesDir,
			DocsDir:     tc.DocsDir,
			SourceDir:   tc.SourceDir,
			Assets:      assets,
		}
		bs = NewBenchState(cfg)
		return bs
	}

	br.Register("serve_ui", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &ServeUIBuilder{BS: initBS(def)}, nil
	})
	br.Register("launch_eval", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		factory := LaunchEvalFactory(initBS(def))
		return factory(def, vars)
	})
}
