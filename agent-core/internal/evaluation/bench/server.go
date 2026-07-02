// Copyright (c) 2026 Nokia. All rights reserved.

// Package bench provides the HTTP server and builtin tools for the
// bench agent. The server serves the web UI, experiment results,
// configs, source, and profiles. The serve_ui tool blocks on a channel
// waiting for user actions from the web UI, making it the bench
// equivalent of invoke_llm.
package bench

import (
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"strings"
)

// ServerConfig holds initialization parameters for a Server.
type ServerConfig struct {
	Addr        string
	DataDir     string
	ConfigsDir  string
	ProfilesDir string
	SourceDir   string
	Assets      fs.FS
}

// Server implements the bench HTTP server. When driven by the
// serve_ui tool, user actions from the frontend are sent over
// ActionCh.
type Server struct {
	dataDir     string
	configsDir  string
	profilesDir string
	sourceDir   string
	assets      fs.FS
	ActionCh    chan UserAction
}

// NewServer creates a Server from the given config.
// actionCh may be nil for standalone operation.
func NewServer(cfg ServerConfig, actionCh chan UserAction) *Server {
	return &Server{
		dataDir:     cfg.DataDir,
		configsDir:  cfg.ConfigsDir,
		profilesDir: cfg.ProfilesDir,
		sourceDir:   cfg.SourceDir,
		assets:      cfg.Assets,
		ActionCh:    actionCh,
	}
}

// Handler returns an http.Handler with all routes mounted.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	mux.HandleFunc("GET /api/v1/sessions", s.handleListSessions)
	mux.HandleFunc("GET /api/v1/sessions/{suite}/{ts}", s.handleGetSession)
	mux.HandleFunc("GET /api/v1/sessions/{suite}/{ts}/points", s.handleListPoints)
	mux.HandleFunc("GET /api/v1/sessions/{suite}/{ts}/points/{pointId}", s.handleGetTrace)
	mux.HandleFunc("GET /api/v1/sessions/{suite}/{ts}/points/{pointId}/experiment", s.handleGetExperiment)

	mux.HandleFunc("GET /api/v1/configs", s.handleListConfigs)
	mux.HandleFunc("GET /api/v1/configs/{path...}", s.handleGetConfig)
	mux.HandleFunc("GET /api/v1/profiles", s.handleListProfiles)

	mux.HandleFunc("GET /api/v1/source/{path...}", s.handleGetSource)

	if s.ActionCh != nil {
		mux.HandleFunc("POST /api/v1/actions", s.handleAction)
	}

	if s.assets != nil {
		mux.Handle("/", spaHandler(s.assets))
	}

	return mux
}

// ListenAndServe starts the HTTP server on the given address.
func (s *Server) ListenAndServe(addr string) error {
	log.Printf("bench listening on %s (data=%s, configs=%s, profiles=%s, source=%s)",
		addr, s.dataDir, s.configsDir, s.profilesDir, s.sourceDir)
	return http.ListenAndServe(addr, s.Handler())
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleAction receives user actions from the web UI and forwards
// them to the state machine via ActionCh.
func (s *Server) handleAction(w http.ResponseWriter, r *http.Request) {
	var action UserAction
	if err := json.NewDecoder(r.Body).Decode(&action); err != nil {
		writeError(w, http.StatusBadRequest, "invalid action payload")
		return
	}
	select {
	case s.ActionCh <- action:
		writeJSON(w, http.StatusOK, map[string]string{"status": "accepted"})
	default:
		writeError(w, http.StatusServiceUnavailable, "no active serve_ui listener")
	}
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
		f.Close()

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

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type apiResponse struct {
	Data  interface{} `json:"data,omitempty"`
	Error string      `json:"error,omitempty"`
}

func writeData(w http.ResponseWriter, data interface{}) {
	writeJSON(w, http.StatusOK, apiResponse{Data: data})
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, apiResponse{Error: msg})
}
