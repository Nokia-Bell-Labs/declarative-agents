// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func testServer() (*Server, *MeshView) {
	view := MeshView{
		Rags: []RagView{
			{Name: "rag0", Collection: "corpus", EmbeddingModel: "qwen3-embedding:8b", Replicas: 1},
		},
		LLM:    LLMView{InCluster: false, ExternalURL: "http://ollama:11434", EmbedModel: "qwen3-embedding:8b"},
		Params: ParamsView{NResults: 5, ChunkCap: 0, RouterDefault: "invoke_llm_fast"},
	}
	var applied *MeshView
	s := &Server{
		State:      func() (MeshView, error) { return view, nil },
		Apply:      func(m MeshView) error { applied = &m; return nil },
		Rollout:    func() (RolloutStatus, error) { return RolloutStatus{Phase: "complete", Ready: 1, Desired: 1}, nil },
		ReadToken:  "read-tok",
		ApplyToken: "apply-tok",
	}
	return s, applied
}

func do(s *Server, method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	return rec
}

func TestStateRequiresTokenAndMarshalsView(t *testing.T) {
	s, _ := testServer()

	if rec := do(s, http.MethodGet, "/provisioning/api/state", "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token: status = %d, want 401", rec.Code)
	}
	rec := do(s, http.MethodGet, "/provisioning/api/state", "read-tok", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("read token: status = %d, want 200", rec.Code)
	}
	var view MeshView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	if len(view.Rags) != 1 || view.Rags[0].Name != "rag0" {
		t.Fatalf("state view = %+v, want one rag0 unit", view)
	}
}

func TestApplyRequiresApplyTokenNotReadToken(t *testing.T) {
	s, _ := testServer()
	body := `{"rags":[{"name":"rag0","collection":"c","embeddingModel":"m","replicas":1}],"llm":{"externalURL":"http://o:11434"},"params":{"nResults":5}}`

	// The read token may read but must not apply.
	if rec := do(s, http.MethodPost, "/provisioning/api/apply", "read-tok", body); rec.Code != http.StatusUnauthorized {
		t.Fatalf("read token apply: status = %d, want 401", rec.Code)
	}
	if rec := do(s, http.MethodPost, "/provisioning/api/apply", "apply-tok", body); rec.Code != http.StatusAccepted {
		t.Fatalf("apply token apply: status = %d, want 202: %s", rec.Code, rec.Body)
	}
}

func TestApplyRejectsUnknownFields(t *testing.T) {
	s, _ := testServer()
	// A payload trying to smuggle a per-agent runtime endpoint override must be
	// rejected: the values view admits no such field (R4.2).
	body := `{"rags":[{"name":"rag0","collection":"c","embeddingModel":"m","replicas":1}],"llm":{"externalURL":"http://o:11434"},"params":{"nResults":5},"agentControlURL":"http://rag0:18086/control"}`
	rec := do(s, http.MethodPost, "/provisioning/api/apply", "apply-tok", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown-field apply: status = %d, want 400 (R4.2 must reject a smuggled agent endpoint)", rec.Code)
	}
}

func TestApplyValidatesMeshView(t *testing.T) {
	s, _ := testServer()
	cases := map[string]string{
		"no rags":         `{"rags":[],"llm":{"externalURL":"http://o"},"params":{"nResults":5}}`,
		"bad rag name":    `{"rags":[{"name":"Rag_0","collection":"c","embeddingModel":"m"}],"llm":{"externalURL":"http://o"},"params":{"nResults":5}}`,
		"zero nResults":   `{"rags":[{"name":"rag0","collection":"c","embeddingModel":"m"}],"llm":{"externalURL":"http://o"},"params":{"nResults":0}}`,
		"external no url": `{"rags":[{"name":"rag0","collection":"c","embeddingModel":"m"}],"llm":{"inCluster":false},"params":{"nResults":5}}`,
		"dup rag name":    `{"rags":[{"name":"rag0","collection":"c","embeddingModel":"m"},{"name":"rag0","collection":"d","embeddingModel":"m"}],"llm":{"externalURL":"http://o"},"params":{"nResults":5}}`,
	}
	for name, body := range cases {
		rec := do(s, http.MethodPost, "/provisioning/api/apply", "apply-tok", body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", name, rec.Code)
		}
	}
}

func TestApplyPassesValidatedViewToRollout(t *testing.T) {
	view := MeshView{
		Rags:   []RagView{{Name: "rag0", Collection: "corpus", EmbeddingModel: "m", Replicas: 1}},
		LLM:    LLMView{ExternalURL: "http://o:11434"},
		Params: ParamsView{NResults: 5},
	}
	var got MeshView
	applied := false
	s := &Server{
		State:      func() (MeshView, error) { return view, nil },
		Apply:      func(m MeshView) error { got = m; applied = true; return nil },
		Rollout:    func() (RolloutStatus, error) { return RolloutStatus{}, nil },
		ApplyToken: "apply-tok",
	}
	body, _ := json.Marshal(MeshView{
		Rags:   []RagView{{Name: "rag0", Collection: "corpus", EmbeddingModel: "m", Replicas: 1}, {Name: "rag1", Collection: "corpus2", EmbeddingModel: "m", Replicas: 1}},
		LLM:    LLMView{ExternalURL: "http://o:11434"},
		Params: ParamsView{NResults: 7},
	})
	rec := do(s, http.MethodPost, "/provisioning/api/apply", "apply-tok", string(body))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("apply status = %d, want 202: %s", rec.Code, rec.Body)
	}
	if !applied || len(got.Rags) != 2 || got.Params.NResults != 7 {
		t.Fatalf("rollout received %+v, want the two-rag nResults=7 view", got)
	}
}

func TestHelmSetArgsRendersEachRag(t *testing.T) {
	view := MeshView{
		Rags: []RagView{
			{Name: "alpha", Collection: "ca", EmbeddingModel: "m", Replicas: 2},
			{Name: "bravo", Collection: "cb", EmbeddingModel: "m", Replicas: 1},
		},
		LLM: LLMView{ExternalURL: "http://o:11434", EmbedModel: "m"},
	}
	args := strings.Join(view.HelmSetArgs(), " ")
	for _, want := range []string{"ragUnits[0].name=alpha", "ragUnits[1].name=bravo", "ragUnits[1].collection=cb", "llm.externalURL=http://o:11434"} {
		if !strings.Contains(args, want) {
			t.Errorf("helm set args missing %q; got %s", want, args)
		}
	}
}

func TestRolloutStatusReadable(t *testing.T) {
	s, _ := testServer()
	rec := do(s, http.MethodGet, "/provisioning/api/rollout", "read-tok", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("rollout status = %d, want 200", rec.Code)
	}
	var st RolloutStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatalf("decode rollout: %v", err)
	}
	if st.Phase != "complete" {
		t.Fatalf("rollout phase = %q, want complete", st.Phase)
	}
}

func TestFileStateReadsMeshView(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/mesh.json"
	if err := os.WriteFile(path, []byte(`{"rags":[{"name":"rag0","collection":"c","embeddingModel":"m","replicas":1}],"llm":{"externalURL":"http://o"},"params":{"nResults":5}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	view, err := fileState(path)()
	if err != nil {
		t.Fatalf("fileState: %v", err)
	}
	if len(view.Rags) != 1 || view.Rags[0].Name != "rag0" {
		t.Fatalf("fileState view = %+v", view)
	}
}

func TestParseRolloutFields(t *testing.T) {
	ready, desired, gen := parseRolloutFields("2/3/5")
	if ready != 2 || desired != 3 || gen != 5 {
		t.Fatalf("parseRolloutFields = (%d,%d,%d), want (2,3,5)", ready, desired, gen)
	}
	if r, d, g := parseRolloutFields("//"); r != 0 || d != 0 || g != 0 {
		t.Fatalf("empty parseRolloutFields = (%d,%d,%d), want zeros", r, d, g)
	}
}
