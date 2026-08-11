// Copyright (c) 2026 Nokia. All rights reserved.

package ollama

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"net/http"
	"os"
	"testing"
)

func chatAPIHandler(content string, promptEval, eval int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		resp := chatResp{
			Message:         msgDTO{Role: "assistant", Content: content},
			EvalCount:       eval,
			PromptEvalCount: promptEval,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func assertFileContains(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(data), want)
}
