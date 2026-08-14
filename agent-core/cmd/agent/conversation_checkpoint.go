// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"encoding/json"
	"fmt"

	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	toollm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/validation"
)

type conversationSnapshotResolver struct {
	checkpoint core.ConversationSnapshotResolver
}

func (r conversationSnapshotResolver) ResolveConversationReference(
	reference string,
) ([]modelllm.Message, error) {
	snapshot, err := r.checkpoint.ResolveConversationSnapshot(reference)
	if err != nil {
		return nil, err
	}
	var messages []modelllm.Message
	if err := json.Unmarshal(snapshot, &messages); err != nil {
		return nil, fmt.Errorf("decode authoritative conversation snapshot: %w", err)
	}
	return messages, nil
}

func llmConversationReferencePorts(
	st *agentState,
) (toollm.ConversationReferenceProvider, toollm.ConversationReferenceResolver) {
	if st == nil || st.isolateConversations {
		return nil, nil
	}
	provider, _ := st.checkpoint.(core.ConversationReferenceProvider)
	resolver, ok := st.checkpointForOps().(core.ConversationSnapshotResolver)
	if !ok {
		return provider, nil
	}
	return provider, conversationSnapshotResolver{checkpoint: resolver}
}

type validationSnapshotResolver struct {
	checkpoint core.DomainSnapshotResolver
}

func (r validationSnapshotResolver) ResolveValidationSnapshot(reference string) ([]byte, error) {
	domain, err := r.checkpoint.ResolveDomainSnapshot(reference)
	if err != nil {
		return nil, err
	}
	var snapshot agentDomainSnapshot
	if err := json.Unmarshal(domain, &snapshot); err != nil {
		return nil, fmt.Errorf("decode authoritative agent domain snapshot: %w", err)
	}
	if len(snapshot.Validation) == 0 {
		return nil, fmt.Errorf("authoritative agent domain snapshot has no validation state")
	}
	return append([]byte(nil), snapshot.Validation...), nil
}

func validationReferencePorts(
	st *agentState,
) (validation.DomainReferenceProvider, validation.DomainSnapshotResolver) {
	if st == nil {
		return nil, nil
	}
	provider, _ := st.checkpoint.(core.DomainReferenceProvider)
	resolver, ok := st.checkpointForOps().(core.DomainSnapshotResolver)
	if !ok {
		return provider, nil
	}
	return provider, validationSnapshotResolver{checkpoint: resolver}
}
