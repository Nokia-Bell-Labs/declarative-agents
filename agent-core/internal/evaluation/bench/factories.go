// Copyright (c) 2026 Nokia. All rights reserved.

package bench

import (
	"io/fs"
	"path/filepath"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/catalog"
	toolregistry "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/registry"
)

// RegisterFactories registers bench builtin tool factories (serve_ui,
// launch_eval) into the provided registry. BenchState is lazily
// initialized on first factory call, pulling config from the tool
// definition YAML.
func RegisterFactories(br *toolregistry.BuiltinRegistry, assets fs.FS) {
	var bs *BenchState

	initBS := func(def catalog.ToolDef) *BenchState {
		if bs != nil {
			return bs
		}

		var tc catalog.ServeUIToolConfig
		_ = catalog.DecodeToolConfig(def, &tc)

		dirs := []*string{&tc.DataDir, &tc.ConfigsDir, &tc.SourceDir, &tc.ProfilesDir}
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
			SourceDir:   tc.SourceDir,
			Assets:      assets,
		}
		bs = NewBenchState(cfg)
		return bs
	}

	br.Register("serve_ui", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		return &ServeUIBuilder{BS: initBS(def)}, nil
	})
	br.Register("launch_eval", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		factory := LaunchEvalFactory(initBS(def))
		return factory(def, vars)
	})
}
