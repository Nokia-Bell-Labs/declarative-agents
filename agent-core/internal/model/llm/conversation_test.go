// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// stubClient records calls and returns canned responses.
type stubClient struct {
	mu        sync.Mutex
	calls     [][]Message
	responses []ChatResponse
	err       error
	callIdx   int
}

func (s *stubClient) Chat(_ context.Context, msgs []Message, _ ChatOptions) (ChatResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	copied := make([]Message, len(msgs))
	copy(copied, msgs)
	s.calls = append(s.calls, copied)

	if s.err != nil {
		return ChatResponse{}, s.err
	}
	if s.callIdx >= len(s.responses) {
		return ChatResponse{Content: "default"}, nil
	}
	resp := s.responses[s.callIdx]
	s.callIdx++
	return resp, nil
}

func (s *stubClient) ListModels(_ context.Context) ([]ModelInfo, error) {
	return nil, nil
}

var _ Client = (*stubClient)(nil)

type controlledClient struct {
	mu        sync.Mutex
	calls     [][]Message
	entered   chan string
	release   chan struct{}
	failFirst bool
	callIdx   int
}

func newControlledClient(failFirst bool) *controlledClient {
	return &controlledClient{
		entered:   make(chan string, 2),
		release:   make(chan struct{}),
		failFirst: failFirst,
	}
}

func (c *controlledClient) Chat(ctx context.Context, msgs []Message, _ ChatOptions) (ChatResponse, error) {
	c.mu.Lock()
	idx := c.callIdx
	c.callIdx++
	c.calls = append(c.calls, append([]Message(nil), msgs...))
	userMessage := msgs[len(msgs)-1].Content
	c.mu.Unlock()

	c.entered <- userMessage
	if idx == 0 {
		select {
		case <-c.release:
		case <-ctx.Done():
			return ChatResponse{}, ctx.Err()
		}
	}
	if idx == 0 && c.failFirst {
		return ChatResponse{}, errors.New("first call failed")
	}
	return ChatResponse{Content: "response-" + userMessage}, nil
}

func (c *controlledClient) ListModels(_ context.Context) ([]ModelInfo, error) {
	return nil, nil
}

func (c *controlledClient) Calls() [][]Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([][]Message(nil), c.calls...)
}

var _ Client = (*controlledClient)(nil)

func requireConversationSend(t *testing.T, conversation *Conversation, message string) {
	t.Helper()
	_, err := conversation.Send(context.Background(), message)
	require.NoError(t, err)
}

func requireBlocked[T any](t *testing.T, ch <-chan T) bool {
	t.Helper()
	select {
	case <-ch:
		t.Error("operation completed before active conversation turn")
		return false
	case <-time.After(50 * time.Millisecond):
		return true
	}
}

func TestNewConversation(t *testing.T) {
	t.Parallel()
	c := NewConversation(&stubClient{}, "system prompt", ChatOptions{Model: "test"})
	require.Equal(t, 0, c.Len())
	require.Empty(t, c.History())
	require.Equal(t, "system prompt", c.SystemPrompt())
}

func TestConversation_Send(t *testing.T) {
	t.Parallel()
	client := &stubClient{
		responses: []ChatResponse{{Content: "hello back", TokensIn: 10, TokensOut: 5}},
	}
	c := NewConversation(client, "you are helpful", ChatOptions{Model: "m"})

	resp, err := c.Send(context.Background(), "hello")
	require.NoError(t, err)
	require.Equal(t, "hello back", resp.Content)
	require.Equal(t, 10, resp.TokensIn)
	require.Equal(t, 5, resp.TokensOut)

	require.Equal(t, 2, c.Len())

	history := c.History()
	require.Equal(t, User, history[0].Role)
	require.Equal(t, "hello", history[0].Content)
	require.Equal(t, Assistant, history[1].Role)
	require.Equal(t, "hello back", history[1].Content)
}

func TestConversation_SendIncludesSystemPrompt(t *testing.T) {
	t.Parallel()
	client := &stubClient{
		responses: []ChatResponse{{Content: "ok"}},
	}
	c := NewConversation(client, "be concise", ChatOptions{})

	_, err := c.Send(context.Background(), "hi")
	require.NoError(t, err)

	require.Len(t, client.calls, 1)
	msgs := client.calls[0]
	require.Equal(t, System, msgs[0].Role)
	require.Equal(t, "be concise", msgs[0].Content)
	require.Equal(t, User, msgs[1].Role)
	require.Equal(t, "hi", msgs[1].Content)
}

func TestConversation_SendAccumulatesHistory(t *testing.T) {
	t.Parallel()
	client := &stubClient{
		responses: []ChatResponse{
			{Content: "resp1"},
			{Content: "resp2"},
		},
	}
	c := NewConversation(client, "sys", ChatOptions{})

	_, err := c.Send(context.Background(), "msg1")
	require.NoError(t, err)
	_, err = c.Send(context.Background(), "msg2")
	require.NoError(t, err)

	require.Equal(t, 4, c.Len())

	// Second call should include full history
	msgs := client.calls[1]
	require.Len(t, msgs, 4) // system + user1 + assistant1 + user2
	require.Equal(t, "sys", msgs[0].Content)
	require.Equal(t, "msg1", msgs[1].Content)
	require.Equal(t, "resp1", msgs[2].Content)
	require.Equal(t, "msg2", msgs[3].Content)
}

func TestConversation_SendError_KeepsUserMessage(t *testing.T) {
	t.Parallel()
	client := &stubClient{err: errors.New("connection refused")}
	c := NewConversation(client, "", ChatOptions{})

	_, err := c.Send(context.Background(), "hello")
	require.Error(t, err)
	require.Contains(t, err.Error(), "connection refused")

	// User message appended, no assistant message
	require.Equal(t, 1, c.Len())
	require.Equal(t, User, c.History()[0].Role)
}

func TestConversation_ConcurrentSendsPreserveCompleteTurnOrder(t *testing.T) {
	t.Parallel()
	client := newControlledClient(false)
	c := NewConversation(client, "", ChatOptions{})
	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)

	go func() {
		_, err := c.Send(context.Background(), "one")
		firstDone <- err
	}()
	require.Equal(t, "one", <-client.entered)

	secondStarted := make(chan struct{})
	go func() {
		close(secondStarted)
		_, err := c.Send(context.Background(), "two")
		secondDone <- err
	}()
	<-secondStarted
	if !requireBlocked(t, secondDone) {
		close(client.release)
		<-firstDone
		return
	}

	close(client.release)
	require.NoError(t, <-firstDone)
	require.Equal(t, "two", <-client.entered)
	require.NoError(t, <-secondDone)

	require.Equal(t, []Message{
		{Role: User, Content: "one"},
		{Role: Assistant, Content: "response-one"},
		{Role: User, Content: "two"},
		{Role: Assistant, Content: "response-two"},
	}, c.History())
	calls := client.Calls()
	require.Equal(t, []Message{
		{Role: User, Content: "one"},
		{Role: Assistant, Content: "response-one"},
		{Role: User, Content: "two"},
	}, calls[1])
}

func TestConversation_ConcurrentSendAfterFailureKeepsFailedUserOnly(t *testing.T) {
	t.Parallel()
	client := newControlledClient(true)
	c := NewConversation(client, "", ChatOptions{})
	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)

	go func() {
		_, err := c.Send(context.Background(), "failed")
		firstDone <- err
	}()
	require.Equal(t, "failed", <-client.entered)
	go func() {
		_, err := c.Send(context.Background(), "next")
		secondDone <- err
	}()

	close(client.release)
	require.ErrorContains(t, <-firstDone, "first call failed")
	require.Equal(t, "next", <-client.entered)
	require.NoError(t, <-secondDone)
	require.Equal(t, []Message{
		{Role: User, Content: "failed"},
		{Role: User, Content: "next"},
		{Role: Assistant, Content: "response-next"},
	}, c.History())
}

func TestConversation_CancelledSendReleasesNextTurn(t *testing.T) {
	t.Parallel()
	client := newControlledClient(false)
	c := NewConversation(client, "", ChatOptions{})
	ctx, cancel := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)

	go func() {
		_, err := c.Send(ctx, "cancelled")
		firstDone <- err
	}()
	require.Equal(t, "cancelled", <-client.entered)
	go func() {
		_, err := c.Send(context.Background(), "next")
		secondDone <- err
	}()

	cancel()
	require.ErrorIs(t, <-firstDone, context.Canceled)
	require.Equal(t, "next", <-client.entered)
	require.NoError(t, <-secondDone)
	require.Equal(t, []Message{
		{Role: User, Content: "cancelled"},
		{Role: User, Content: "next"},
		{Role: Assistant, Content: "response-next"},
	}, c.History())
}

func TestConversation_SnapshotWaitsForCompleteSend(t *testing.T) {
	t.Parallel()
	client := newControlledClient(false)
	c := NewConversation(client, "", ChatOptions{})
	sendDone := make(chan error, 1)
	snapshotDone := make(chan []Message, 1)

	go func() {
		_, err := c.Send(context.Background(), "one")
		sendDone <- err
	}()
	require.Equal(t, "one", <-client.entered)
	go func() {
		snapshotDone <- c.Snapshot()
	}()
	if !requireBlocked(t, snapshotDone) {
		close(client.release)
		<-sendDone
		return
	}

	close(client.release)
	require.NoError(t, <-sendDone)
	require.Equal(t, []Message{
		{Role: User, Content: "one"},
		{Role: Assistant, Content: "response-one"},
	}, <-snapshotDone)
}

func TestConversation_ResetWaitsForCompleteSend(t *testing.T) {
	t.Parallel()
	client := newControlledClient(false)
	c := NewConversation(client, "", ChatOptions{})
	sendDone := make(chan error, 1)
	resetDone := make(chan struct{})

	go func() {
		_, err := c.Send(context.Background(), "one")
		sendDone <- err
	}()
	require.Equal(t, "one", <-client.entered)
	go func() {
		c.Reset()
		close(resetDone)
	}()
	if !requireBlocked(t, resetDone) {
		close(client.release)
		<-sendDone
		return
	}

	close(client.release)
	require.NoError(t, <-sendDone)
	<-resetDone
	require.Empty(t, c.History())
}

func TestConversation_Reset(t *testing.T) {
	t.Parallel()
	client := &stubClient{
		responses: []ChatResponse{{Content: "r1"}, {Content: "r2"}},
	}
	c := NewConversation(client, "sys", ChatOptions{})

	requireConversationSend(t, c, "a")
	requireConversationSend(t, c, "b")
	require.Equal(t, 4, c.Len())

	c.Reset()
	require.Equal(t, 0, c.Len())
	require.Empty(t, c.History())
}

func TestConversation_ResetThenSend(t *testing.T) {
	t.Parallel()
	client := &stubClient{
		responses: []ChatResponse{{Content: "r1"}, {Content: "r2"}},
	}
	c := NewConversation(client, "sys", ChatOptions{})

	requireConversationSend(t, c, "old")
	c.Reset()
	_, err := c.Send(context.Background(), "fresh")
	require.NoError(t, err)

	require.Equal(t, 2, c.Len())
	require.Equal(t, "fresh", c.History()[0].Content)

	// Chat call after reset should only have system + new user
	lastCall := client.calls[len(client.calls)-1]
	require.Len(t, lastCall, 2) // system + user
}

func TestConversation_HistoryCopySemantics(t *testing.T) {
	t.Parallel()
	client := &stubClient{responses: []ChatResponse{{Content: "r"}}}
	c := NewConversation(client, "", ChatOptions{})

	requireConversationSend(t, c, "msg")

	snapshot := c.History()
	snapshot[0].Content = "mutated"

	fresh := c.History()
	require.Equal(t, "msg", fresh[0].Content)
}

func TestConversation_EmptySystemPrompt(t *testing.T) {
	t.Parallel()
	client := &stubClient{responses: []ChatResponse{{Content: "r"}}}
	c := NewConversation(client, "", ChatOptions{})

	_, err := c.Send(context.Background(), "hi")
	require.NoError(t, err)

	msgs := client.calls[0]
	require.Len(t, msgs, 1) // no system message
	require.Equal(t, User, msgs[0].Role)
}

func TestConversation_SetSystemPrompt(t *testing.T) {
	t.Parallel()
	client := &stubClient{
		responses: []ChatResponse{{Content: "r1"}, {Content: "r2"}},
	}
	c := NewConversation(client, "original", ChatOptions{})

	requireConversationSend(t, c, "a")
	require.Equal(t, "original", client.calls[0][0].Content)

	c.SetSystemPrompt("updated")
	requireConversationSend(t, c, "b")
	require.Equal(t, "updated", client.calls[1][0].Content)
}

func TestConversation_SystemPromptConcurrentAccess(t *testing.T) {
	t.Parallel()
	c := NewConversation(&stubClient{}, "initial", ChatOptions{})
	const goroutines = 8
	const iterations = 1000
	var wg sync.WaitGroup

	for worker := 0; worker < goroutines; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for iteration := 0; iteration < iterations; iteration++ {
				if worker%2 == 0 {
					c.SetSystemPrompt(fmt.Sprintf("%d-%d", worker, iteration))
				} else {
					_ = c.SystemPrompt()
				}
			}
		}(worker)
	}
	wg.Wait()
	require.NotEmpty(t, c.SystemPrompt())
}

// --- Append (manual/append-only mode) ---

func TestConversation_Append(t *testing.T) {
	t.Parallel()
	c := NewConversation(&stubClient{}, "", ChatOptions{})

	c.Append(Message{Role: User, Content: "hello"})
	c.Append(Message{Role: Assistant, Content: "hi"})

	require.Equal(t, 2, c.Len())
	h := c.History()
	require.Equal(t, User, h[0].Role)
	require.Equal(t, "hello", h[0].Content)
	require.Equal(t, Assistant, h[1].Role)
	require.Equal(t, "hi", h[1].Content)
}

func TestConversation_AppendDoesNotCallChat(t *testing.T) {
	t.Parallel()
	client := &stubClient{}
	c := NewConversation(client, "sys", ChatOptions{})

	c.Append(Message{Role: User, Content: "manual"})
	c.Append(Message{Role: Assistant, Content: "response"})

	require.Empty(t, client.calls, "Append must not invoke Chat")
	require.Equal(t, 2, c.Len())
}

func TestConversation_AppendThenSend(t *testing.T) {
	t.Parallel()
	client := &stubClient{
		responses: []ChatResponse{{Content: "r1"}},
	}
	c := NewConversation(client, "sys", ChatOptions{})

	c.Append(Message{Role: User, Content: "preloaded"})
	c.Append(Message{Role: Assistant, Content: "prior"})

	_, err := c.Send(context.Background(), "new question")
	require.NoError(t, err)

	// Chat should have received system + preloaded + prior + new question
	require.Len(t, client.calls, 1)
	msgs := client.calls[0]
	require.Len(t, msgs, 4) // sys + preloaded + prior + new question
	require.Equal(t, "sys", msgs[0].Content)
	require.Equal(t, "preloaded", msgs[1].Content)
	require.Equal(t, "prior", msgs[2].Content)
	require.Equal(t, "new question", msgs[3].Content)
}

func TestConversation_AppendPreservesOrder(t *testing.T) {
	t.Parallel()
	c := NewConversation(&stubClient{}, "", ChatOptions{})

	for i := 0; i < 5; i++ {
		c.Append(Message{Role: User, Content: fmt.Sprintf("msg%d", i)})
	}

	h := c.History()
	require.Len(t, h, 5)
	for i := 0; i < 5; i++ {
		require.Equal(t, fmt.Sprintf("msg%d", i), h[i].Content)
	}
}

func TestConversation_AppendResetAppend(t *testing.T) {
	t.Parallel()
	c := NewConversation(&stubClient{}, "", ChatOptions{})

	c.Append(Message{Role: User, Content: "old"})
	require.Equal(t, 1, c.Len())

	c.Reset()
	require.Equal(t, 0, c.Len())

	c.Append(Message{Role: User, Content: "fresh"})
	require.Equal(t, 1, c.Len())
	require.Equal(t, "fresh", c.History()[0].Content)
}

func TestConversation_TruncateTo(t *testing.T) {
	t.Parallel()
	c := NewConversation(&stubClient{}, "", ChatOptions{})
	c.Append(Message{Role: User, Content: "one"})
	c.Append(Message{Role: Assistant, Content: "two"})
	c.Append(Message{Role: User, Content: "three"})

	require.NoError(t, c.TruncateTo(2))
	require.Equal(t, 2, c.Len())
	require.Equal(t, "two", c.History()[1].Content)
}

func TestConversation_TruncateToRejectsInvalidLength(t *testing.T) {
	t.Parallel()
	c := NewConversation(&stubClient{}, "", ChatOptions{})
	c.Append(Message{Role: User, Content: "one"})

	require.Error(t, c.TruncateTo(2))
	require.Equal(t, 1, c.Len())
}

func TestConversation_SnapshotRestore(t *testing.T) {
	t.Parallel()
	c := NewConversation(&stubClient{}, "", ChatOptions{})
	c.Append(Message{Role: User, Content: "before"})
	snapshot := c.Snapshot()

	c.Append(Message{Role: Assistant, Content: "after"})
	c.Restore(snapshot)

	require.Equal(t, 1, c.Len())
	require.Equal(t, "before", c.History()[0].Content)
	snapshot[0].Content = "mutated"
	require.Equal(t, "before", c.History()[0].Content)
}

// --- Messages (alias for History) ---

func TestConversation_Messages(t *testing.T) {
	t.Parallel()
	client := &stubClient{responses: []ChatResponse{{Content: "r"}}}
	c := NewConversation(client, "", ChatOptions{})

	requireConversationSend(t, c, "hi")

	msgs := c.Messages()
	hist := c.History()
	require.Equal(t, hist, msgs)
}

// --- AssembleMessages ---

func TestConversation_AssembleMessages(t *testing.T) {
	t.Parallel()
	c := NewConversation(&stubClient{}, "system", ChatOptions{})

	c.Append(Message{Role: User, Content: "u1"})
	c.Append(Message{Role: Assistant, Content: "a1"})

	assembled := c.AssembleMessages()
	require.Len(t, assembled, 3) // system + u1 + a1
	require.Equal(t, System, assembled[0].Role)
	require.Equal(t, "system", assembled[0].Content)
	require.Equal(t, User, assembled[1].Role)
	require.Equal(t, Assistant, assembled[2].Role)
}

func TestConversation_AssembleMessages_NoSystemPrompt(t *testing.T) {
	t.Parallel()
	c := NewConversation(&stubClient{}, "", ChatOptions{})

	c.Append(Message{Role: User, Content: "u1"})
	assembled := c.AssembleMessages()
	require.Len(t, assembled, 1) // no system message
	require.Equal(t, User, assembled[0].Role)
}

func TestConversation_AssembleMessages_CopySemantics(t *testing.T) {
	t.Parallel()
	c := NewConversation(&stubClient{}, "sys", ChatOptions{})
	c.Append(Message{Role: User, Content: "original"})

	snapshot := c.AssembleMessages()
	snapshot[1].Content = "mutated"

	fresh := c.AssembleMessages()
	require.Equal(t, "original", fresh[1].Content)
}

// --- Message type tests ---

func TestMessageRoleConstants(t *testing.T) {
	t.Parallel()
	roles := []MessageRole{System, User, Assistant}
	seen := make(map[MessageRole]bool)
	for _, r := range roles {
		require.NotEmpty(t, string(r))
		require.False(t, seen[r])
		seen[r] = true
	}
}

func TestMessage_JSONRoundTrip(t *testing.T) {
	t.Parallel()
	msg := Message{Role: Assistant, Content: "hello"}
	data, err := json.Marshal(msg)
	require.NoError(t, err)

	var decoded Message
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Equal(t, msg, decoded)
}

func TestToolRequest_JSONRoundTrip(t *testing.T) {
	t.Parallel()
	tr := ToolRequest{
		ToolName: "read",
		Params:   json.RawMessage(`{"path":"main.go"}`),
	}
	data, err := json.Marshal(tr)
	require.NoError(t, err)

	var decoded ToolRequest
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Equal(t, "read", decoded.ToolName)
	require.JSONEq(t, `{"path":"main.go"}`, string(decoded.Params))
}

func TestChatResponse_JSONFieldNames(t *testing.T) {
	t.Parallel()
	resp := ChatResponse{Content: "hi", TokensIn: 100, TokensOut: 20}
	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &raw))
	require.Equal(t, "hi", raw["content"])
	require.EqualValues(t, 100, raw["tokens_in"])
	require.EqualValues(t, 20, raw["tokens_out"])
}
