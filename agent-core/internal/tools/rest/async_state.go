// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"fmt"
	"sync"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

const asyncRetentionConsume = "consume"

// AsyncState stores pending REST client requests for send/await tools.
type AsyncState struct {
	mu       sync.Mutex
	requests map[string]*AsyncRequest
}

// AsyncRequest tracks one submitted asynchronous REST operation.
type AsyncRequest struct {
	RequestID        string                 `json:"request_id"`
	OperationID      string                 `json:"operation_id"`
	RestRef          string                 `json:"rest_ref"`
	Resource         string                 `json:"resource,omitempty"`
	IdempotencyToken string                 `json:"idempotency_token,omitempty"`
	Correlation      string                 `json:"correlation,omitempty"`
	SubmittedPayload map[string]interface{} `json:"submitted_payload,omitempty"`
	RetentionPolicy  string                 `json:"retention_policy,omitempty"`
	Done             chan core.Result       `json:"-"`
}

// NewAsyncState creates empty async request state.
func NewAsyncState() *AsyncState {
	return &AsyncState{requests: map[string]*AsyncRequest{}}
}

// Add records a pending async request.
func (s *AsyncState) Add(request *AsyncRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.requests[request.RequestID]; exists {
		return fmt.Errorf("async request %q already exists", request.RequestID)
	}
	s.requests[request.RequestID] = request
	return nil
}

// Get resolves an async request by request ID.
func (s *AsyncState) Get(requestID string) (*AsyncRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	request, ok := s.requests[requestID]
	if !ok {
		return nil, fmt.Errorf("async request %q is not defined", requestID)
	}
	return request, nil
}

// Consume removes an async request when retention policy requires it.
func (s *AsyncState) Consume(request *AsyncRequest) {
	if request.RetentionPolicy != asyncRetentionConsume {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.requests, request.RequestID)
}
