// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import "net/http"

// MockEngine is the mock binding's request and log surface. The engine lives in
// rest/mock; the parent stores this interface so Serve/WriteLog do not hang off
// *serverRuntime (GH-1821). Two methods keep the interface inside the go-style
// 1-3 method bound.
type MockEngine interface {
	Serve(w http.ResponseWriter, req *http.Request, maxRequestBytes int)
	WriteLog(w http.ResponseWriter)
}
