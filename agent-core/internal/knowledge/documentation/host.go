// Copyright (c) 2026 Nokia. All rights reserved.

package docsapi

import (
	"context"
	"errors"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/knowledge/documentation/ui"
)

// HostConfig configures the standalone documentation UI host.
type HostConfig struct {
	Addr        string
	DocsDir     string
	ConfigsDir  string
	SourceDir   string
	ProfilePath string
	Assets      fs.FS
	Workflow    WorkflowRunner
}

// Server serves the Knowledge Manager documentation API and UI assets.
type Server struct {
	addr        string
	docsDir     string
	configsDir  string
	sourceDir   string
	profilePath string
	assets      fs.FS
	workflow    WorkflowRunner
}

// RunningServer is a launched documentation host.
type RunningServer struct {
	Addr    string
	server  *http.Server
	done    chan error
	cleanup func() error
}

// NewServer creates a standalone documentation server.
func NewServer(cfg HostConfig) *Server {
	assets := cfg.Assets
	if assets == nil {
		assets = ui.Assets()
	}
	return &Server{
		addr:        cfg.Addr,
		docsDir:     cfg.DocsDir,
		configsDir:  cfg.ConfigsDir,
		sourceDir:   cfg.SourceDir,
		profilePath: cfg.ProfilePath,
		assets:      assets,
		workflow:    cfg.Workflow,
	}
}

// Handler returns a handler with documentation API and UI routes mounted.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	docs := NewHandler(s.docsDir)
	requests := NewLazyMachineRequestProxy(s.profilePath, s.docsDir)
	mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	mux.Handle("GET /api/v1/docs", requests)
	mux.Handle("GET /api/v1/docs/{path...}", requests)
	mux.HandleFunc("POST /api/v1/docs/search", docs.Search)
	mux.HandleFunc("POST /api/v1/docs/validate", docs.Validate)
	mux.HandleFunc("POST /api/v1/docs/suggestions", docs.Suggest)
	mux.HandleFunc("POST /api/v1/docs/patches/{patch_id}/approve", docs.Approve)
	mux.HandleFunc("POST /api/v1/docs/patches/{patch_id}/reject", docs.Reject)
	mux.HandleFunc("POST /api/v1/docs/patches/{patch_id}/reopen", docs.Reopen)
	mux.HandleFunc("POST /api/v1/actions", s.handleAction)
	mux.HandleFunc("GET /api/v1/ux", s.handleUX)
	mux.HandleFunc("GET /api/v1/configs/{path...}", s.handleGetConfig)
	mux.HandleFunc("GET /api/v1/source/{path...}", s.handleGetSource)
	mux.Handle("/", docsAPIOrSPAHandler(requests, spaHandler(s.assets)))
	return closeAwareHandler{Handler: mux, close: requests.Close}
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
	handler := s.Handler()
	server := &http.Server{Handler: handler}
	running := &RunningServer{Addr: listener.Addr().String(), server: server, done: make(chan error, 1)}
	if closer, ok := handler.(interface{ Close() error }); ok {
		running.cleanup = closer.Close
	}
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
	err := r.server.Close()
	if cleanupErr := r.cleanupBackend(); err == nil {
		err = cleanupErr
	}
	return err
}

// Shutdown gracefully stops the launched documentation host.
func (r *RunningServer) Shutdown(ctx context.Context) error {
	return r.server.Shutdown(ctx)
}

// Wait blocks until the launched documentation host stops.
func (r *RunningServer) Wait() error {
	return <-r.done
}

// Stop closes the launched host and waits for Serve to return.
func (r *RunningServer) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	shutdownErr := r.Shutdown(ctx)
	waitErr := r.Wait()
	if shutdownErr != nil {
		return shutdownErr
	}
	if waitErr != nil && !errors.Is(waitErr, http.ErrServerClosed) {
		return waitErr
	}
	return r.cleanupBackend()
}

func (r *RunningServer) cleanupBackend() error {
	if r.cleanup == nil {
		return nil
	}
	return r.cleanup()
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleAction(w http.ResponseWriter, r *http.Request) {
	runner := s.workflow
	if runner == nil {
		runner = NewLazyProfileWorkflowRunner(s.profilePath, s.docsDir)
	}
	result, err := runner.Run(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleUX(w http.ResponseWriter, _ *http.Request) {
	cfg, err := LoadCuratorUXConfig(s.profilePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeDataFields(w, http.StatusOK, cfg, nil)
}

type closeAwareHandler struct {
	http.Handler
	close func() error
}

func (h closeAwareHandler) Close() error {
	if h.close == nil {
		return nil
	}
	return h.close()
}

func docsAPIOrSPAHandler(docs http.Handler, spa http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/docs/") {
			docs.ServeHTTP(w, r)
			return
		}
		spa.ServeHTTP(w, r)
	})
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
