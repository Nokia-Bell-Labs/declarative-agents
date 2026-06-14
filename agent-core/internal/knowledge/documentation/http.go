// Copyright (c) 2026 Nokia. All rights reserved.

package docsapi

import (
	"encoding/json"
	"errors"
	"net/http"
)

// Handler serves documentation repository HTTP routes.
type Handler struct {
	repo Repository
}

// NewHandler creates a documentation API handler for one docs root.
func NewHandler(docsDir string) Handler {
	return Handler{repo: NewRepository(docsDir)}
}

// List handles GET /api/v1/docs.
func (h Handler) List(w http.ResponseWriter, r *http.Request) {
	docs, err := h.repo.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to scan docs directory")
		return
	}
	writeData(w, docs)
}

// Get handles GET /api/v1/docs/{path...}.
func (h Handler) Get(w http.ResponseWriter, r *http.Request) {
	doc, err := h.repo.Get(r.PathValue("path"))
	if err != nil {
		writeDocError(w, err)
		return
	}
	writeData(w, doc)
}

type apiResponse struct {
	Data  interface{} `json:"data,omitempty"`
	Error string      `json:"error,omitempty"`
}

func writeDocError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotConfigured):
		writeError(w, http.StatusNotFound, ErrNotConfigured.Error())
	case errors.Is(err, ErrPathRequired):
		writeError(w, http.StatusBadRequest, ErrPathRequired.Error())
	case errors.Is(err, ErrInvalidPath):
		writeError(w, http.StatusBadRequest, ErrInvalidPath.Error())
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, ErrNotFound.Error())
	default:
		writeError(w, http.StatusInternalServerError, ErrReadDocument.Error())
	}
}

func writeData(w http.ResponseWriter, data interface{}) {
	writeJSON(w, http.StatusOK, apiResponse{Data: data})
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, apiResponse{Error: msg})
}

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
