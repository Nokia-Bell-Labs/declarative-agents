// Copyright (c) 2026 Nokia. All rights reserved.

package docsapi

import (
	"errors"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strings"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/knowledge/documentation/ui"
)

// HostConfig configures the standalone documentation UI host.
type HostConfig struct {
	Addr       string
	DocsDir    string
	ConfigsDir string
	SourceDir  string
	Assets     fs.FS
	Workflow   WorkflowRunner
}

// Server serves the Knowledge Manager documentation API and UI assets.
type Server struct {
	addr       string
	docsDir    string
	configsDir string
	sourceDir  string
	assets     fs.FS
	workflow   WorkflowRunner
}

// RunningServer is a launched documentation host.
type RunningServer struct {
	Addr   string
	server *http.Server
	done   chan error
}

// NewServer creates a standalone documentation server.
func NewServer(cfg HostConfig) *Server {
	assets := cfg.Assets
	if assets == nil {
		assets = ui.Assets()
	}
	return &Server{
		addr:       cfg.Addr,
		docsDir:    cfg.DocsDir,
		configsDir: cfg.ConfigsDir,
		sourceDir:  cfg.SourceDir,
		assets:     assets,
		workflow:   cfg.Workflow,
	}
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
	mux.HandleFunc("POST /api/v1/docs/patches/{patch_id}/reopen", docs.Reopen)
	mux.HandleFunc("POST /api/v1/actions", s.handleAction)
	mux.HandleFunc("GET /api/v1/configs/{path...}", s.handleGetConfig)
	mux.HandleFunc("GET /api/v1/source/{path...}", s.handleGetSource)
	mux.Handle("/", spaHandler(s.assets))
	return mux
}

// ListenAndServe starts the documentation UI host.
func (s *Server) ListenAndServe() error {
	running, err := s.Start()
	if err != nil {
		return err
	}
	return running.Wait()
}

// Start launches the documentation UI host without blocking.
func (s *Server) Start() (*RunningServer, error) {
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return nil, err
	}
	server := &http.Server{Handler: s.Handler()}
	running := &RunningServer{Addr: listener.Addr().String(), server: server, done: make(chan error, 1)}
	log.Printf("documentation UI listening on %s (docs=%s)", running.Addr, s.docsDir)
	go func() {
		err := server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("documentation UI server stopped with error: %v", err)
		}
		running.done <- err
	}()
	return running, nil
}

// Close stops the launched documentation host.
func (r *RunningServer) Close() error {
	return r.server.Close()
}

// Wait blocks until the launched documentation host stops.
func (r *RunningServer) Wait() error {
	return <-r.done
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleAction(w http.ResponseWriter, r *http.Request) {
	runner := s.workflow
	if runner == nil {
		runner = NewLazyProfileWorkflowRunner(defaultCuratorProfilePath)
	}
	result, err := runner.Run(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
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
