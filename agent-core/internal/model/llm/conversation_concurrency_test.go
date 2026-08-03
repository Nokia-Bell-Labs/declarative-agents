// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

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

func (c *controlledClient) Calls() [][]Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([][]Message(nil), c.calls...)
}

var _ Client = (*controlledClient)(nil)

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
