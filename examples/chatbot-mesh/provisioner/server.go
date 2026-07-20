// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Server is the deployment API. The read path (GET) accepts a read-only token so
// it works with RBAC read-only credentials; apply (POST) requires the apply
// token, separate from the chat path which has no auth. state/apply/rollout are
// injected so the HTTP, auth, and validation surface is testable without a
// cluster; main wires the real implementations.
type Server struct {
	State   func() (MeshView, error)
	Apply   func(MeshView) error
	Rollout func() (RolloutStatus, error)

	ReadToken  string
	ApplyToken string
}

// RolloutStatus is the chatbot Deployment's rollout progress the panel polls.
type RolloutStatus struct {
	Phase    string `json:"phase"` // "progressing" | "complete" | "unknown"
	Ready    int    `json:"ready"`
	Desired  int    `json:"desired"`
	Revision int    `json:"revision"`
	Message  string `json:"message,omitempty"`
}

// Routes returns the deployment-API mux. Everything is under /provisioning/api so
// the chatbot ingress can route /provisioning to this Service same-origin, keeping
// the panel's calls off the GET-only monitor_proxy and out of cross-origin.
func (s *Server) Routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/provisioning/api/state", s.handleState)
	mux.HandleFunc("/provisioning/api/apply", s.handleApply)
	mux.HandleFunc("/provisioning/api/rollout", s.handleRollout)
	mux.HandleFunc("/provisioning/api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	return mux
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "state is read-only")
		return
	}
	if !s.authorized(r, false) {
		writeError(w, http.StatusUnauthorized, "a read or apply token is required")
		return
	}
	view, err := s.State()
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("read mesh state: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "apply is POST")
		return
	}
	if !s.authorized(r, true) {
		writeError(w, http.StatusUnauthorized, "the apply token is required")
		return
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields() // a payload cannot smuggle a field outside the values view (R4.2)
	var view MeshView
	if err := dec.Decode(&view); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("decode mesh patch: %v", err))
		return
	}
	if err := view.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.Apply(view); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("apply rollout: %v", err))
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

func (s *Server) handleRollout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "rollout status is read-only")
		return
	}
	if !s.authorized(r, false) {
		writeError(w, http.StatusUnauthorized, "a read or apply token is required")
		return
	}
	status, err := s.Rollout()
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("rollout status: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// authorized checks the bearer token. The apply path requires the apply token; the
// read paths accept either token, so a read-only credential reads but cannot apply.
func (s *Server) authorized(r *http.Request, needApply bool) bool {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}
	if s.ApplyToken != "" && token == s.ApplyToken {
		return true
	}
	if needApply {
		return false
	}
	return s.ReadToken != "" && token == s.ReadToken
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
