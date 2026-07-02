// Copyright (c) 2026 Nokia. All rights reserved.

package docsapi

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// SearchRequest scopes a documentation corpus search.
type SearchRequest struct {
	Query    string `json:"query"`
	Category string `json:"category,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

// SearchResult contains structured matches for a documentation search.
type SearchResult struct {
	Query   string        `json:"query"`
	Matches []SearchMatch `json:"matches"`
	Count   int           `json:"count"`
}

// SearchMatch identifies one matched document.
type SearchMatch struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Snippet  string `json:"snippet"`
}

// ValidationRequest scopes documentation validation.
type ValidationRequest struct {
	Paths  []string `json:"paths,omitempty"`
	Strict bool     `json:"strict,omitempty"`
}

// ValidationReport is an auditable validation result.
type ValidationReport struct {
	Status       string    `json:"status"`
	Findings     []Finding `json:"findings"`
	CheckedPaths []string  `json:"checked_paths"`
}

// Finding is one validation or drift finding.
type Finding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Path     string `json:"path"`
	Message  string `json:"message"`
}

// SuggestionRequest asks for a reviewable patch proposal.
type SuggestionRequest struct {
	Path        string `json:"path"`
	Instruction string `json:"instruction"`
	Context     string `json:"context,omitempty"`
}

// SuggestionResponse records a generated patch proposal.
type SuggestionResponse struct {
	PatchID       string    `json:"patch_id"`
	Path          string    `json:"path"`
	Status        string    `json:"status"`
	Suggestions   []string  `json:"suggestions"`
	ProposedPatch string    `json:"proposed_patch"`
	Findings      []Finding `json:"findings,omitempty"`
	CreatedAt     string    `json:"created_at"`
}

// PatchDecisionRequest records an explicit patch review decision.
type PatchDecisionRequest struct {
	DecidedBy string `json:"decided_by"`
	Note      string `json:"note,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// PatchDecision is a review record. It never applies documentation writes.
type PatchDecision struct {
	PatchID   string `json:"patch_id"`
	Status    string `json:"status"`
	DecidedBy string `json:"decided_by"`
	Note      string `json:"note,omitempty"`
	Reason    string `json:"reason,omitempty"`
	Applied   bool   `json:"applied"`
}

// PatchStore holds pending suggestion decisions for the running host.
type PatchStore struct {
	mu      sync.Mutex
	patches map[string]SuggestionResponse
}

// NewPatchStore creates an empty in-memory patch store.
func NewPatchStore() *PatchStore {
	return &PatchStore{patches: map[string]SuggestionResponse{}}
}

// Search returns documents whose path or raw content matches the query.
func (r Repository) Search(req SearchRequest) (SearchResult, error) {
	if strings.TrimSpace(req.Query) == "" {
		return SearchResult{}, ErrPathRequired
	}
	entries, err := r.List()
	if err != nil {
		return SearchResult{}, err
	}
	matches := r.searchEntries(entries, req)
	return SearchResult{Query: req.Query, Matches: matches, Count: len(matches)}, nil
}

// Validate checks parseability, required fields, and index path drift.
func (r Repository) Validate(req ValidationRequest) (ValidationReport, error) {
	details, err := r.requestedDetails(req.Paths)
	if err != nil {
		return ValidationReport{}, err
	}
	report := ValidationReport{Status: "valid"}
	for _, detail := range details {
		report.CheckedPaths = append(report.CheckedPaths, detail.Path)
		report.Findings = append(report.Findings, validateDetail(detail)...)
		if detail.Path == "docs/SPECIFICATIONS.yaml" || detail.Path == "SPECIFICATIONS.yaml" {
			report.Findings = append(report.Findings, r.validateIndexPaths(detail)...)
		}
	}
	if len(report.Findings) > 0 {
		report.Status = "findings"
	}
	sort.Strings(report.CheckedPaths)
	return report, nil
}

// SuggestChanges creates a patch proposal and leaves writes unapplied.
func (r Repository) SuggestChanges(req SuggestionRequest) (SuggestionResponse, error) {
	if strings.TrimSpace(req.Instruction) == "" {
		return SuggestionResponse{}, ErrPathRequired
	}
	detail, err := r.Get(req.Path)
	if err != nil {
		return SuggestionResponse{}, err
	}
	report, _ := r.Validate(ValidationRequest{Paths: []string{req.Path}})
	return suggestionFromRequest(req, detail, report.Findings), nil
}

// Add records a pending suggestion.
func (s *PatchStore) Add(suggestion SuggestionResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.patches[suggestion.PatchID] = suggestion
}

// Decide records a review decision without applying writes.
func (s *PatchStore) Decide(patchID, status string, req PatchDecisionRequest) (PatchDecision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.patches[patchID]; !ok {
		return PatchDecision{}, ErrNotFound
	}
	return PatchDecision{
		PatchID: patchID, Status: status, DecidedBy: req.DecidedBy,
		Note: req.Note, Reason: req.Reason, Applied: false,
	}, nil
}

func (r Repository) searchEntries(entries []Entry, req SearchRequest) []SearchMatch {
	query := strings.ToLower(req.Query)
	limit := normalizedLimit(req.Limit)
	matches := []SearchMatch{}
	for _, entry := range entries {
		if req.Category != "" && entry.Category != req.Category {
			continue
		}
		detail, err := r.Get(entry.Path)
		if err == nil && matchesQuery(entry, detail.Raw, query) {
			matches = append(matches, SearchMatch{
				Path: entry.Path, Name: entry.Name, Category: entry.Category,
				Snippet: snippetFor(detail.Raw, query),
			})
		}
		if len(matches) == limit {
			return matches
		}
	}
	return matches
}

func normalizedLimit(limit int) int {
	if limit <= 0 || limit > 50 {
		return 50
	}
	return limit
}

func matchesQuery(entry Entry, raw, query string) bool {
	return strings.Contains(strings.ToLower(entry.Path), query) ||
		strings.Contains(strings.ToLower(entry.Name), query) ||
		strings.Contains(strings.ToLower(raw), query)
}

func snippetFor(raw, query string) string {
	lower := strings.ToLower(raw)
	idx := strings.Index(lower, query)
	if idx < 0 {
		return firstLine(raw)
	}
	start := max(0, idx-40)
	end := min(len(raw), idx+len(query)+80)
	return strings.TrimSpace(raw[start:end])
}

func firstLine(raw string) string {
	line, _, _ := strings.Cut(raw, "\n")
	return strings.TrimSpace(line)
}

func (r Repository) requestedDetails(paths []string) ([]Detail, error) {
	if len(paths) == 0 {
		return r.allDetails()
	}
	details := make([]Detail, 0, len(paths))
	for _, path := range paths {
		detail, err := r.Get(path)
		if err != nil {
			return nil, err
		}
		details = append(details, detail)
	}
	return details, nil
}

func (r Repository) allDetails() ([]Detail, error) {
	entries, err := r.List()
	if err != nil {
		return nil, err
	}
	details := make([]Detail, 0, len(entries))
	for _, entry := range entries {
		detail, err := r.Get(entry.Path)
		if err != nil {
			return nil, err
		}
		details = append(details, detail)
	}
	return details, nil
}

func validateDetail(detail Detail) []Finding {
	content, ok := detail.Content.(map[string]interface{})
	if !ok {
		return []Finding{finding("yaml_parse_error", "error", detail.Path, "document YAML could not be parsed")}
	}
	return missingFieldFindings(detail.Path, content, requiredFields(detail.Path))
}

func missingFieldFindings(path string, content map[string]interface{}, fields []string) []Finding {
	findings := []Finding{}
	for _, field := range fields {
		if _, ok := content[field]; !ok {
			findings = append(findings, finding("missing_required_field", "error", path, "missing required field "+field))
		}
	}
	return findings
}

func requiredFields(path string) []string {
	switch {
	case path == "VISION.yaml":
		return []string{"id", "title", "executive_summary", "problem", "what_this_does", "why_we_build_this", "success_criteria", "not"}
	case path == "ARCHITECTURE.yaml":
		return []string{"id", "title", "overview", "interfaces", "components", "design_decisions", "technology_choices", "project_structure", "implementation_status", "related_documents"}
	case path == "SPECIFICATIONS.yaml":
		return []string{"id", "title", "overview", "roadmap_summary", "foundation_document_index", "srd_index", "config_format_index", "semantic_model_index", "use_case_index", "test_suite_index", "coverage_gaps"}
	case path == "road-map.yaml":
		return []string{"id", "title", "releases"}
	case strings.HasPrefix(path, "specs/software-requirements/"):
		return []string{"id", "title", "problem", "goals", "requirements", "non_goals", "acceptance_criteria"}
	case strings.HasPrefix(path, "specs/use-cases/"):
		return []string{"id", "title", "summary", "actor", "trigger", "flow", "touchpoints", "success_criteria", "out_of_scope"}
	case strings.HasPrefix(path, "specs/test-suites/"):
		return []string{"id", "title", "release", "traces", "preconditions", "test_cases"}
	default:
		return nil
	}
}

func (r Repository) validateIndexPaths(detail Detail) []Finding {
	refs := map[string]bool{}
	collectPathRefs(detail.Content, refs)
	findings := []Finding{}
	for ref := range refs {
		cleaned := strings.TrimPrefix(ref, "docs/")
		if _, err := r.Get(cleaned); err != nil {
			findings = append(findings, finding("index_path_missing", "error", detail.Path, "referenced path missing: "+ref))
		}
	}
	return findings
}

func collectPathRefs(value interface{}, refs map[string]bool) {
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, child := range typed {
			if key == "path" {
				if path, ok := child.(string); ok && strings.HasPrefix(path, "docs/") {
					refs[path] = true
				}
			}
			collectPathRefs(child, refs)
		}
	case []interface{}:
		for _, child := range typed {
			collectPathRefs(child, refs)
		}
	}
}

func finding(code, severity, path, message string) Finding {
	return Finding{Code: code, Severity: severity, Path: path, Message: message}
}

func suggestionFromRequest(req SuggestionRequest, detail Detail, findings []Finding) SuggestionResponse {
	created := time.Now().UTC().Format(time.RFC3339)
	suggestions := suggestionMessages(req, findings)
	return SuggestionResponse{
		PatchID: patchID(req, detail.Raw), Path: detail.Path, Status: "pending_review",
		Suggestions: suggestions, ProposedPatch: proposedPatch(req, findings),
		Findings: findings, CreatedAt: created,
	}
}

func suggestionMessages(req SuggestionRequest, findings []Finding) []string {
	messages := []string{"Review requested change: " + req.Instruction}
	for _, f := range findings {
		messages = append(messages, fmt.Sprintf("Address %s in %s: %s", f.Code, f.Path, f.Message))
	}
	return messages
}

func proposedPatch(req SuggestionRequest, findings []Finding) string {
	lines := []string{"# Proposed documentation patch", "# Path: " + req.Path, "# Instruction: " + req.Instruction}
	for _, f := range findings {
		lines = append(lines, "# Finding: "+f.Message)
	}
	lines = append(lines, "# Approval required before any file write.")
	return strings.Join(lines, "\n") + "\n"
}

func patchID(req SuggestionRequest, raw string) string {
	sum := sha256.Sum256([]byte(req.Path + "\n" + req.Instruction + "\n" + req.Context + "\n" + raw))
	return "patch-" + hex.EncodeToString(sum[:])[:12]
}
