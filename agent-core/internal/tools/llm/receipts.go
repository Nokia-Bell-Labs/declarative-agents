// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"encoding/json"

	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
)

// conversationReceipt is the opaque, tool-owned rollback context the conversation
// tools (invoke_llm, reset_history) encode into Result.Receipt during Execute. It
// carries the prior conversation so a fresh command instance sharing the same
// Conversation can restore it after a process restart (srd035-checkpoint-port R3;
// #44 R2). Only the llm tools decode it; the engine and adapters treat it as
// opaque.
type conversationReceipt struct {
	Conversation []modelllm.Message `json:"conversation"`
}

func encodeConversationReceipt(msgs []modelllm.Message) string {
	b, err := json.Marshal(conversationReceipt{Conversation: msgs})
	if err != nil {
		return ""
	}
	return string(b)
}

func decodeConversationReceipt(receipt string) ([]modelllm.Message, bool, error) {
	if receipt == "" {
		return nil, false, nil
	}
	var r conversationReceipt
	if err := json.Unmarshal([]byte(receipt), &r); err != nil {
		return nil, false, err
	}
	return r.Conversation, true, nil
}

// retryReceipt is the opaque rollback context the parse-retry tools
// (parse_response, report_parse_error) encode: the prior parse-retry counter.
type retryReceipt struct {
	ParseRetryCounter int `json:"parse_retry_counter"`
}

func encodeRetryReceipt(retries int) string {
	b, err := json.Marshal(retryReceipt{ParseRetryCounter: retries})
	if err != nil {
		return ""
	}
	return string(b)
}

func decodeRetryReceipt(receipt string) (int, bool, error) {
	if receipt == "" {
		return 0, false, nil
	}
	var r retryReceipt
	if err := json.Unmarshal([]byte(receipt), &r); err != nil {
		return 0, false, err
	}
	return r.ParseRetryCounter, true, nil
}
