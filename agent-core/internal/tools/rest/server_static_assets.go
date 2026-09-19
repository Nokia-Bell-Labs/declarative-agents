// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/bundles"
	restdef "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/definition"
)

func (r *serverRuntime) serveStaticAssets(w http.ResponseWriter, req *http.Request, endpoint restdef.Endpoint) {
	cfg := endpoint.StaticAssets
	if cfg == nil {
		http.Error(w, "static_assets is not configured", http.StatusInternalServerError)
		return
	}
	rel, ok := staticAssetsRelativePath(endpoint.Path, req.URL.Path)
	if !ok {
		http.NotFound(w, req)
		return
	}
	// Without a cache policy browsers cache heuristically on Last-Modified,
	// and a SPA keeps running its old bundle after a redeploy until a hard
	// reload. no-cache forces revalidation, which ServeContent answers with
	// cheap 304s, so a normal reload always runs the deployed page (GH-1939).
	w.Header().Set("Cache-Control", "no-cache")
	if cfg.Config != nil && strings.TrimPrefix(path.Clean("/"+rel), "/") == staticAssetsConfigFile {
		writeJSON(w, http.StatusOK, cfg.Config)
		return
	}
	fsys, err := staticAssetsFileSystem(cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	idx := cfg.Index
	if idx == "" {
		idx = "index.html"
	}
	f, info, err := openStaticAssetFile(fsys, rel, idx, cfg.SPA)
	if err != nil {
		w.Header().Del("Cache-Control")
		http.NotFound(w, req)
		return
	}
	defer func() { _ = f.Close() }()
	http.ServeContent(w, req, info.Name(), info.ModTime(), f)
}

// staticAssetsRelativePath is the request path beneath the endpoint's
// catch-all parameter, or false when the request does not match the route.
func staticAssetsRelativePath(route, requestPath string) (string, bool) {
	vars, ok := matchPath(route, requestPath)
	if !ok {
		return "", false
	}
	for _, seg := range pathSegments(route) {
		if n, ok := catchAllParam(seg); ok {
			return vars[n], true
		}
	}
	return "", true
}

// staticAssetsConfigFile is the path, beneath a static_assets mount, at which
// the binding's declared config is served (srd029 R5.10).
const staticAssetsConfigFile = "ui-config.json"

// staticAssetsFileSystem selects the declared file source: a bundle compiled
// into agent-core or a filesystem root (srd029 R5.9).
func staticAssetsFileSystem(cfg *restdef.StaticAssetsConfig) (http.FileSystem, error) {
	if cfg.Bundle != "" {
		fsys, ok := bundles.Lookup(cfg.Bundle)
		if !ok {
			return nil, fmt.Errorf("static_assets bundle %q is not compiled into agent-core", cfg.Bundle)
		}
		return http.FS(fsys), nil
	}
	return http.Dir(filepath.Clean(cfg.Root)), nil
}

func openStaticAssetFile(d http.FileSystem, rel, idx string, spa bool) (http.File, os.FileInfo, error) {
	key := strings.TrimPrefix(path.Clean("/"+rel), "/")
	if key == "." || key == "" {
		return openStaticLeafFile(d, idx)
	}
	f, err := d.Open(key)
	if err != nil {
		if spa {
			return openStaticLeafFile(d, idx)
		}
		return nil, nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		if spa {
			return openStaticLeafFile(d, idx)
		}
		return nil, nil, err
	}
	if !info.IsDir() {
		return f, info, nil
	}
	_ = f.Close()
	if f2, info2, err := openStaticLeafFile(d, path.Join(key, idx)); err == nil {
		return f2, info2, nil
	}
	if spa {
		return openStaticLeafFile(d, idx)
	}
	return nil, nil, os.ErrNotExist
}

func openStaticLeafFile(d http.FileSystem, name string) (http.File, os.FileInfo, error) {
	f, err := d.Open(name)
	if err != nil {
		return nil, nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	if info.IsDir() {
		_ = f.Close()
		return nil, nil, os.ErrNotExist
	}
	return f, info, nil
}
