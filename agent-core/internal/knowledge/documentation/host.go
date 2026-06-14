// Copyright (c) 2026 Nokia. All rights reserved.

package docsapi

import (
	"io/fs"
	"log"
	"net/http"
	"strings"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/knowledge/documentation/ui"
)

// HostConfig configures the standalone documentation UI host.
type HostConfig struct {
	Addr    string
	DocsDir string
	Assets  fs.FS
}

// Server serves the Knowledge Manager documentation API and UI assets.
type Server struct {
	addr    string
	docsDir string
	assets  fs.FS
}

// NewServer creates a standalone documentation server.
func NewServer(cfg HostConfig) *Server {
	assets := cfg.Assets
	if assets == nil {
		assets = ui.Assets()
	}
	return &Server{addr: cfg.Addr, docsDir: cfg.DocsDir, assets: assets}
}

// Handler returns a handler with documentation API and UI routes mounted.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	docs := NewHandler(s.docsDir)
	mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	mux.HandleFunc("GET /api/v1/docs", docs.List)
	mux.HandleFunc("GET /api/v1/docs/{path...}", docs.Get)
	mux.HandleFunc("POST /api/v1/docs/search", docs.Search)
	mux.HandleFunc("POST /api/v1/docs/validate", docs.Validate)
	mux.HandleFunc("POST /api/v1/docs/suggestions", docs.Suggest)
	mux.HandleFunc("POST /api/v1/docs/patches/{patch_id}/approve", docs.Approve)
	mux.HandleFunc("POST /api/v1/docs/patches/{patch_id}/reject", docs.Reject)
	mux.Handle("/", spaHandler(s.assets))
	return mux
}

// ListenAndServe starts the documentation UI host.
func (s *Server) ListenAndServe() error {
	log.Printf("documentation UI listening on %s (docs=%s)", s.addr, s.docsDir)
	return http.ListenAndServe(s.addr, s.Handler())
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func spaHandler(assets fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		f, err := assets.Open(path)
		if err != nil {
			serveIndexHTML(w, assets)
			return
		}
		_ = f.Close()
		fileServer.ServeHTTP(w, r)
	})
}

func serveIndexHTML(w http.ResponseWriter, assets fs.FS) {
	data, err := fs.ReadFile(assets, "index.html")
	if err != nil {
		http.Error(w, "index.html not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
