// Copyright (c) 2026 Nokia. All rights reserved.

package docsapi

import (
	"encoding/json"
	"errors"
	"net/http"
)

// Handler serves documentation repository HTTP routes.
type Handler struct {
	repo    Repository
	patches *PatchStore
}

// NewHandler creates a documentation API handler for one docs root.
func NewHandler(docsDir string) Handler {
	return Handler{repo: NewRepository(docsDir), patches: NewPatchStore()}
}

// List handles GET /api/v1/docs.
func (h Handler) List(w http.ResponseWriter, r *http.Request) {
	docs, err := h.repo.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to scan docs directory")
		return
	}
	writeDataFields(w, http.StatusOK, docs, map[string]interface{}{"count": len(docs)})
}

// Get handles GET /api/v1/docs/{path...}.
func (h Handler) Get(w http.ResponseWriter, r *http.Request) {
	doc, err := h.repo.Get(r.PathValue("path"))
	if err != nil {
		writeDocError(w, err)
		return
	}
	writeDataFields(w, http.StatusOK, doc, detailFields(doc))
}

// Search handles POST /api/v1/docs/search.
func (h Handler) Search(w http.ResponseWriter, r *http.Request) {
	var req SearchRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	result, err := h.repo.Search(req)
	if err != nil {
		writeDocError(w, err)
		return
	}
	writeDataFields(w, http.StatusOK, result, searchFields(result))
}

// Validate handles POST /api/v1/docs/validate.
func (h Handler) Validate(w http.ResponseWriter, r *http.Request) {
	var req ValidationRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	report, err := h.repo.Validate(req)
	if err != nil {
		writeDocError(w, err)
		return
	}
	status := http.StatusOK
	if req.Strict && len(report.Findings) > 0 {
		status = http.StatusUnprocessableEntity
	}
	writeDataFields(w, status, report, validationFields(report))
}

// Suggest handles POST /api/v1/docs/suggestions.
func (h Handler) Suggest(w http.ResponseWriter, r *http.Request) {
	var req SuggestionRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	suggestion, err := h.repo.SuggestChanges(req)
	if err != nil {
		writeDocError(w, err)
		return
	}
	h.patches.Add(suggestion)
	writeDataFields(w, http.StatusAccepted, suggestion, suggestionFields(suggestion))
}

// Approve handles POST /api/v1/docs/patches/{patch_id}/approve.
func (h Handler) Approve(w http.ResponseWriter, r *http.Request) {
	h.decidePatch(w, r, "approved_pending_apply")
}

// Reject handles POST /api/v1/docs/patches/{patch_id}/reject.
func (h Handler) Reject(w http.ResponseWriter, r *http.Request) {
	h.decidePatch(w, r, "rejected")
}

// Reopen handles POST /api/v1/docs/patches/{patch_id}/reopen.
func (h Handler) Reopen(w http.ResponseWriter, r *http.Request) {
	h.decidePatch(w, r, "pending_review")
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

func (h Handler) decidePatch(w http.ResponseWriter, r *http.Request, status string) {
	var req PatchDecisionRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	decision, err := h.patches.Decide(r.PathValue("patch_id"), status, req)
	if err != nil {
		writeDocError(w, err)
		return
	}
	writeDataFields(w, http.StatusOK, decision, decisionFields(decision))
}

func decodeRequest(w http.ResponseWriter, r *http.Request, dst interface{}) bool {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON request body")
		return false
	}
	return true
}

func writeDataFields(w http.ResponseWriter, status int, data interface{}, fields map[string]interface{}) {
	payload := map[string]interface{}{"data": data}
	for key, value := range fields {
		payload[key] = value
	}
	writeJSON(w, status, payload)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, apiResponse{Error: msg})
}

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func detailFields(doc Detail) map[string]interface{} {
	return map[string]interface{}{"path": doc.Path, "content": doc.Content, "raw": doc.Raw}
}

func searchFields(result SearchResult) map[string]interface{} {
	return map[string]interface{}{"query": result.Query, "matches": result.Matches, "count": result.Count}
}

func validationFields(report ValidationReport) map[string]interface{} {
	return map[string]interface{}{"status": report.Status, "findings": report.Findings, "checked_paths": report.CheckedPaths}
}

func suggestionFields(s SuggestionResponse) map[string]interface{} {
	return map[string]interface{}{"patch_id": s.PatchID, "path": s.Path, "status": s.Status, "suggestions": s.Suggestions, "proposed_patch": s.ProposedPatch}
}

func decisionFields(decision PatchDecision) map[string]interface{} {
	return map[string]interface{}{"patch_id": decision.PatchID, "status": decision.Status, "decided_by": decision.DecidedBy}
}
