// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"encoding/json"
	"fmt"
	"strings"

	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
)

const conversationReceiptVersion2 = 2

// legacyConversationReceipt is the original full-conversation receipt. Decode
// support is retained so executions persisted before receipt v2 remain undoable.
type legacyConversationReceipt struct {
	Conversation []modelllm.Message `json:"conversation"`
}

// conversationReceiptV2 is the reference-only rollback context emitted by
// invoke_llm and reset_history. It deliberately contains no conversation text.
// ConversationReference identifies an authoritative checkpoint snapshot when
// one is available; direct same-process undo uses command-local state instead.
type conversationReceiptV2 struct {
	Version                 int    `json:"version"`
	PriorConversationLength int    `json:"prior_conversation_length"`
	ConversationReference   string `json:"conversation_reference,omitempty"`
}

type conversationReceiptV2Wire struct {
	PriorLength *int    `json:"prior_conversation_length"`
	Reference   *string `json:"conversation_reference"`
}

type decodedConversationReceipt struct {
	legacyConversation []modelllm.Message
	priorLength        int
	reference          string
	legacy             bool
}

// ConversationReferenceResolver restores the authoritative conversation
// snapshot identified by a stable reference. Checkpoint implementations wire
// this seam; LLM tools remain independent of their storage details.
type ConversationReferenceResolver interface {
	ResolveConversationReference(reference string) ([]modelllm.Message, error)
}

func encodeConversationReceipt(priorLength int, reference string) string {
	b, err := json.Marshal(conversationReceiptV2{
		Version:                 conversationReceiptVersion2,
		PriorConversationLength: priorLength,
		ConversationReference:   reference,
	})
	if err != nil {
		return ""
	}
	return string(b)
}

func decodeConversationReceipt(receipt string) (decodedConversationReceipt, bool, error) {
	if receipt == "" {
		return decodedConversationReceipt{}, false, nil
	}

	var envelope map[string]json.RawMessage
	if err := json.Unmarshal([]byte(receipt), &envelope); err != nil {
		return decodedConversationReceipt{}, false, err
	}
	rawVersion, versioned := envelope["version"]
	if !versioned {
		return decodeLegacyConversationReceipt(envelope)
	}
	decoded, err := decodeConversationReceiptV2(receipt, envelope, rawVersion)
	return decoded, err == nil, err
}

func decodeLegacyConversationReceipt(envelope map[string]json.RawMessage) (decodedConversationReceipt, bool, error) {
	rawConversation, ok := envelope["conversation"]
	if !ok {
		return decodedConversationReceipt{}, false, fmt.Errorf("legacy conversation receipt is missing conversation field")
	}
	var messages []modelllm.Message
	if err := json.Unmarshal(rawConversation, &messages); err != nil {
		return decodedConversationReceipt{}, false, fmt.Errorf("invalid legacy conversation field: %w", err)
	}
	return decodedConversationReceipt{
		legacyConversation: messages,
		priorLength:        len(messages),
		legacy:             true,
	}, true, nil
}

func decodeConversationReceiptV2(
	receipt string,
	envelope map[string]json.RawMessage,
	rawVersion json.RawMessage,
) (decodedConversationReceipt, error) {
	var version int
	if err := json.Unmarshal(rawVersion, &version); err != nil {
		return decodedConversationReceipt{}, fmt.Errorf("invalid receipt version: %w", err)
	}
	if version != conversationReceiptVersion2 {
		return decodedConversationReceipt{}, fmt.Errorf("unsupported conversation receipt version %d", version)
	}
	if err := validateConversationReceiptV2Fields(envelope); err != nil {
		return decodedConversationReceipt{}, err
	}

	var wire conversationReceiptV2Wire
	if err := json.Unmarshal([]byte(receipt), &wire); err != nil {
		return decodedConversationReceipt{}, err
	}
	return validateConversationReceiptV2(wire)
}

func validateConversationReceiptV2Fields(envelope map[string]json.RawMessage) error {
	for field := range envelope {
		switch field {
		case "version", "prior_conversation_length", "conversation_reference":
			continue
		case "conversation":
			return fmt.Errorf("conversation receipt v2 contains forbidden conversation content")
		default:
			return fmt.Errorf("conversation receipt v2 has unknown field %q", field)
		}
	}
	return nil
}

func validateConversationReceiptV2(wire conversationReceiptV2Wire) (decodedConversationReceipt, error) {
	if wire.PriorLength == nil || *wire.PriorLength < 0 {
		return decodedConversationReceipt{}, fmt.Errorf("conversation receipt v2 has invalid prior conversation length")
	}
	reference := ""
	if wire.Reference != nil {
		reference = strings.TrimSpace(*wire.Reference)
		if reference == "" {
			return decodedConversationReceipt{}, fmt.Errorf("conversation receipt v2 has empty conversation reference")
		}
	}
	return decodedConversationReceipt{
		priorLength: *wire.PriorLength,
		reference:   reference,
	}, nil
}

func resolveConversationReceipt(
	receipt decodedConversationReceipt,
	resolver ConversationReferenceResolver,
) ([]modelllm.Message, error) {
	if receipt.reference == "" {
		if receipt.priorLength == 0 {
			return []modelllm.Message{}, nil
		}
		return nil, fmt.Errorf("receipt has no conversation reference")
	}
	if resolver == nil {
		return nil, fmt.Errorf("conversation reference resolver is not configured")
	}
	messages, err := resolver.ResolveConversationReference(receipt.reference)
	if err != nil {
		return nil, fmt.Errorf("resolve conversation reference %q: %w", receipt.reference, err)
	}
	if len(messages) < receipt.priorLength {
		return nil, fmt.Errorf(
			"conversation reference %q has %d messages, need at least %d",
			receipt.reference, len(messages), receipt.priorLength,
		)
	}
	restored := make([]modelllm.Message, receipt.priorLength)
	copy(restored, messages[:receipt.priorLength])
	return restored, nil
}

const retryReceiptVersion1 = 1

// retryReceipt is the opaque rollback context the parse-retry tools
// (parse_response, report_parse_error) encode: the prior parse-retry counter.
// Its version is independent of the conversation receipt version.
type retryReceipt struct {
	Version           int `json:"retry_receipt_version"`
	ParseRetryCounter int `json:"parse_retry_counter"`
}

func encodeRetryReceipt(retries int) string {
	b, err := json.Marshal(retryReceipt{
		Version:           retryReceiptVersion1,
		ParseRetryCounter: retries,
	})
	if err != nil {
		return ""
	}
	return string(b)
}

func decodeRetryReceipt(receipt string) (int, bool, error) {
	if receipt == "" {
		return 0, false, nil
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal([]byte(receipt), &envelope); err != nil {
		return 0, false, err
	}
	if envelope == nil {
		return 0, false, fmt.Errorf("retry receipt must be a JSON object")
	}
	for field := range envelope {
		switch field {
		case "retry_receipt_version", "parse_retry_counter":
		default:
			return 0, false, fmt.Errorf("retry receipt has unknown field %q", field)
		}
	}
	retries, err := decodeRetryReceiptFields(envelope)
	if err != nil {
		return 0, false, err
	}
	return retries, true, nil
}

func decodeRetryReceiptFields(envelope map[string]json.RawMessage) (int, error) {
	rawCounter, ok := envelope["parse_retry_counter"]
	if !ok {
		return 0, fmt.Errorf("retry receipt is missing parse_retry_counter field")
	}
	var retries *int
	if err := json.Unmarshal(rawCounter, &retries); err != nil {
		return 0, fmt.Errorf("invalid parse_retry_counter: %w", err)
	}
	if retries == nil {
		return 0, fmt.Errorf("retry receipt has null parse_retry_counter")
	}
	if *retries < 0 {
		return 0, fmt.Errorf("retry receipt has negative parse_retry_counter")
	}

	// The unversioned shape was emitted before receipt-driven fresh Undo was
	// available. Keep decoding it so already-persisted executions remain
	// reversible.
	if rawVersion, versioned := envelope["retry_receipt_version"]; versioned {
		var version *int
		if err := json.Unmarshal(rawVersion, &version); err != nil {
			return 0, fmt.Errorf("invalid retry receipt version: %w", err)
		}
		if version == nil {
			return 0, fmt.Errorf("retry receipt has null version")
		}
		if *version != retryReceiptVersion1 {
			return 0, fmt.Errorf("unsupported retry receipt version %d", *version)
		}
	}
	return *retries, nil
}
