// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type observerTurnBaseline struct {
	StateIteration   int
	EventsIteration  int
	ChatbotState     string
	ChatbotUpdatedAt string
	ChatbotEventTime string
	RagEventTime     string
	CapturedAt       time.Time
}

type observerTurnSnapshot struct {
	StateIteration  int
	EventsIteration int
	ChatbotState    observerFleetItem
	RagState        observerFleetItem
	ChatbotEvents   observerFleetItem
	RagEvents       observerFleetItem
}

// observerTurnBaselineSnapshot waits for readable state/event fan-in entries and
// records the clocks that a later live-turn snapshot must advance. It does not
// require a whole-cycle snapshot: command_state exposes the latest output per
// label, and the observer can be between discovery and its joins for most reads
// while each state/event entry remains valid.
func observerTurnBaselineSnapshot(
	monitorURL string,
	timeout time.Duration,
) (observerTurnBaseline, error) {
	var baseline observerTurnBaseline
	err := waitObserverTurnSnapshot(monitorURL, timeout, func(snapshot observerTurnSnapshot) error {
		if snapshot.ChatbotState.Signal != observerMonitorReadSignal ||
			snapshot.RagState.Signal != observerMonitorReadSignal {
			return fmt.Errorf("baseline chatbot or rag0 is unreachable")
		}
		state, updated := observerRunState(snapshot.ChatbotState)
		if updated == "" {
			return fmt.Errorf("baseline chatbot run has no updated_at")
		}
		baseline = observerTurnBaseline{
			StateIteration:   snapshot.StateIteration,
			EventsIteration:  snapshot.EventsIteration,
			ChatbotState:     state,
			ChatbotUpdatedAt: updated,
			ChatbotEventTime: latestObserverEventTime(snapshot.ChatbotEvents),
			RagEventTime:     latestObserverEventTime(snapshot.RagEvents),
			CapturedAt:       time.Now().UTC(),
		}
		return nil
	})
	return baseline, err
}

// waitObserverLiveTurn waits for newer state/event entries that show the
// chatbot entering its answer path and rag0 completing its query while the mock
// holds the actual answer-model request open.
func waitObserverLiveTurn(
	monitorURL string,
	collectorQueryURL string,
	baseline observerTurnBaseline,
	timeout time.Duration,
) error {
	return waitObserverTurnSnapshot(monitorURL, timeout, func(snapshot observerTurnSnapshot) error {
		if snapshot.StateIteration <= baseline.StateIteration ||
			snapshot.EventsIteration <= baseline.EventsIteration {
			return fmt.Errorf("state or event fan-in has not advanced")
		}
		if snapshot.ChatbotState.Signal != observerMonitorReadSignal ||
			snapshot.RagState.Signal != observerMonitorReadSignal {
			return fmt.Errorf("live-turn chatbot or rag0 is unreachable")
		}
		state, updated := observerRunState(snapshot.ChatbotState)
		if updated <= baseline.ChatbotUpdatedAt {
			return fmt.Errorf("chatbot run timestamp has not advanced")
		}
		if state == "" || state == baseline.ChatbotState {
			return fmt.Errorf("chatbot state = %q, unchanged from baseline", state)
		}
		if !observerTierDecisionAfter(snapshot.ChatbotEvents, baseline.ChatbotEventTime) {
			return fmt.Errorf("chatbot has no newer tier decision event; recent=%s",
				observerEventSummary(snapshot.ChatbotEvents))
		}
		ragEvent := observerEventAfter(snapshot.RagEvents,
			baseline.RagEventTime, "QueryResponded", "ResolvingCollection", "Querying")
		traceProof := false
		var traceErr error
		if !ragEvent {
			traceProof, traceErr = observerTraceHasSpanSince(
				collectorQueryURL, baseline.CapturedAt,
				"chatbot", "execute_tool rag_query")
		}
		if !ragEvent && !traceProof {
			return fmt.Errorf(
				"rag0 query has neither a retained fleet event nor a durable rag_query span: recent=%s trace=%v",
				observerEventSummary(snapshot.RagEvents), traceErr)
		}
		evidence := "durable collector trace"
		if ragEvent {
			evidence = "retained query event"
		}
		fmt.Printf("helmSwap: observer live-turn PASS - chatbot %s advanced, rag0 query proven by %s in fleet entry %d\n",
			state, evidence, snapshot.EventsIteration)
		return nil
	})
}

func observerTierDecisionAfter(events observerFleetItem, baseline string) bool {
	return observerEventAfter(events, baseline, "ToolDone", "ParsingTier", "Answering") ||
		observerEventAfter(events, baseline, "ParseFailed", "ParsingTier", "Answering")
}

type observerTraceSummary struct {
	TraceID     string    `json:"trace_id"`
	RootService string    `json:"root_service"`
	StartTime   time.Time `json:"start_time"`
}

type observerTraceList struct {
	Traces   []observerTraceSummary `json:"traces"`
	Total    int                    `json:"total"`
	Offset   int                    `json:"offset"`
	PageSize int                    `json:"page_size"`
}

type observerTraceDetail struct {
	Spans []struct {
		Service string `json:"service"`
		Name    string `json:"name"`
	} `json:"spans"`
}

// observerTraceHasSpanSince searches the collector's durable trace index for a
// post-baseline chatbot span proving the RAG request completed. The held answer
// boundary occurs after rag_query returns, so this persisted tool span backs up
// the observer's bounded recent-event window without requiring request-machine
// spans that the rag0 monitor does not export.
func observerTraceHasSpanSince(
	queryURL string,
	after time.Time,
	service string,
	spanName string,
) (bool, error) {
	const pageSize = 100
	client := &http.Client{Timeout: 5 * time.Second}
	for offset := 0; ; offset += pageSize {
		values := url.Values{
			"offset":    {fmt.Sprint(offset)},
			"page_size": {fmt.Sprint(pageSize)},
		}
		var list observerTraceList
		if err := observerTraceGetJSON(client,
			queryURL+"/query/traces?"+values.Encode(), &list); err != nil {
			return false, err
		}
		reachedBaseline := false
		for _, summary := range list.Traces {
			if summary.StartTime.Before(after) {
				reachedBaseline = true
				continue
			}
			if summary.RootService != "chatbot" {
				continue
			}
			var detail observerTraceDetail
			if err := observerTraceGetJSON(client,
				queryURL+"/query/traces/"+url.PathEscape(summary.TraceID), &detail); err != nil {
				continue
			}
			for _, span := range detail.Spans {
				if span.Service == service && span.Name == spanName {
					return true, nil
				}
			}
		}
		if reachedBaseline || offset+len(list.Traces) >= list.Total ||
			len(list.Traces) == 0 {
			return false, fmt.Errorf(
				"no post-baseline trace contains %s span %q (searched %d of %d)",
				service, spanName, offset+len(list.Traces), list.Total)
		}
	}
}

func observerTraceGetJSON(client *http.Client, endpoint string, target interface{}) error {
	resp, err := client.Get(endpoint)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s status %s", endpoint, resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("decode %s: %w", endpoint, err)
	}
	return nil
}

func waitObserverTurnSnapshot(
	monitorURL string,
	timeout time.Duration,
	check func(observerTurnSnapshot) error,
) error {
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		labels, err := observerFleetLabelsView(monitorURL)
		if err == nil {
			var snapshot observerTurnSnapshot
			snapshot, err = observerTurnSnapshotFromLabels(labels)
			if err == nil {
				err = check(snapshot)
				if err == nil {
					return nil
				}
			}
		}
		last = err
		time.Sleep(500 * time.Millisecond)
	}
	if last == nil {
		last = fmt.Errorf("no observer turn snapshot was available")
	}
	return fmt.Errorf("timed out after %s: %w", timeout, last)
}

func observerTurnSnapshotFromLabels(
	labels map[string]interface{},
) (observerTurnSnapshot, error) {
	stateEntry, stateOutput, err := observerFleetEntry(labels, "agent_state_fanin")
	if err != nil {
		return observerTurnSnapshot{}, err
	}
	eventEntry, eventOutput, err := observerFleetEntry(labels, "agent_events_fanin")
	if err != nil {
		return observerTurnSnapshot{}, err
	}
	stateItems, err := observerItemsFromOutput(stateOutput)
	if err != nil {
		return observerTurnSnapshot{}, err
	}
	eventItems, err := observerItemsFromOutput(eventOutput)
	if err != nil {
		return observerTurnSnapshot{}, err
	}
	chatState, ragState, err := observerTurnItems(stateItems)
	if err != nil {
		return observerTurnSnapshot{}, err
	}
	chatEvents, ragEvents, err := observerTurnItems(eventItems)
	if err != nil {
		return observerTurnSnapshot{}, err
	}
	return observerTurnSnapshot{
		StateIteration:  int(stateEntry["iteration"].(float64)),
		EventsIteration: int(eventEntry["iteration"].(float64)),
		ChatbotState:    chatState, RagState: ragState,
		ChatbotEvents: chatEvents, RagEvents: ragEvents,
	}, nil
}

func observerTurnItems(
	items map[string]observerFleetItem,
) (chatbot, rag0 observerFleetItem, err error) {
	for _, item := range items {
		switch {
		case item.Pod.Component == "chatbot":
			chatbot = item
		case item.Pod.RagUnit == "rag0":
			rag0 = item
		}
	}
	if chatbot.Pod.Name == "" || rag0.Pod.Name == "" {
		return chatbot, rag0, fmt.Errorf("fan-in lacks chatbot or rag0")
	}
	return chatbot, rag0, nil
}

func observerRunState(item observerFleetItem) (state, updatedAt string) {
	run, _ := item.Body["run"].(map[string]interface{})
	return stringValue(run["state"]), stringValue(run["updated_at"])
}

func latestObserverEventTime(item observerFleetItem) string {
	events, _ := item.Body["recent_events"].([]interface{})
	var latest string
	for _, value := range events {
		event, _ := value.(map[string]interface{})
		if timestamp := stringValue(event["timestamp"]); timestamp > latest {
			latest = timestamp
		}
	}
	return latest
}

func observerEventAfter(
	item observerFleetItem,
	after, signal, fromState, toState string,
) bool {
	events, _ := item.Body["recent_events"].([]interface{})
	for _, value := range events {
		event, _ := value.(map[string]interface{})
		if stringValue(event["timestamp"]) > after &&
			stringValue(event["signal"]) == signal &&
			stringValue(event["from_state"]) == fromState &&
			stringValue(event["to_state"]) == toState {
			return true
		}
	}
	return false
}

func observerEventSummary(item observerFleetItem) string {
	events, _ := item.Body["recent_events"].([]interface{})
	summary := make([]string, 0, len(events))
	for _, value := range events {
		event, _ := value.(map[string]interface{})
		summary = append(summary, fmt.Sprintf("%s:%s->%s@%s",
			stringValue(event["signal"]), stringValue(event["from_state"]),
			stringValue(event["to_state"]), stringValue(event["timestamp"])))
	}
	return fmt.Sprint(summary)
}
