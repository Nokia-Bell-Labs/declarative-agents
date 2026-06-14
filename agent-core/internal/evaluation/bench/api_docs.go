// Copyright (c) 2026 Nokia. All rights reserved.

package bench

import (
	"net/http"

	docsapi "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/knowledge/documentation"
)

func (s *Server) handleListDocs(w http.ResponseWriter, r *http.Request) {
	docsapi.NewHandler(s.docsDir).List(w, r)
}

func (s *Server) handleGetDoc(w http.ResponseWriter, r *http.Request) {
	docsapi.NewHandler(s.docsDir).Get(w, r)
}
