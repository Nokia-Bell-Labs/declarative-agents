// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"time"
)

type observerTurnBaseline struct {
	StateIteration   int
	EventsIteration  int
	ChatbotState     string
	ChatbotUpdatedAt string
	ChatbotEventTime string
	RagEventTime     string
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
		if !observerEventAfter(snapshot.ChatbotEvents,
			baseline.ChatbotEventTime, "ParseFailed", "ParsingTier", "Answering") {
			return fmt.Errorf("chatbot has no newer ParsingTier -> Answering event; recent=%s",
				observerEventSummary(snapshot.ChatbotEvents))
		}
		if !observerEventAfter(snapshot.RagEvents,
			baseline.RagEventTime, "QueryResponded", "ResolvingCollection", "Querying") {
			return fmt.Errorf("rag0 has no newer ResolvingCollection -> Querying event")
		}
		fmt.Printf("helmSwap: observer live-turn PASS - chatbot %s and rag0 query events advanced in fleet entry %d\n",
			state, snapshot.EventsIteration)
		return nil
	})
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
