// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestObserverTurnBaselineAndLiveSnapshot(t *testing.T) {
	var live bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"labels": observerTurnFixture(live),
		})
	}))
	defer server.Close()

	baseline, err := observerTurnBaselineSnapshot(server.URL, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	live = true
	if err := waitObserverLiveTurn(server.URL, "", baseline, time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestObserverLiveTurnUsesDurableTraceAfterRagEventEviction(t *testing.T) {
	var live bool
	monitor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		labels := observerTurnFixture(live)
		if live {
			removeRagQueryEvent(labels)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"labels": labels})
	}))
	defer monitor.Close()
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/query/traces" {
			_ = json.NewEncoder(w).Encode(observerTraceList{
				Traces: []observerTraceSummary{{
					TraceID: "trace-live", RootService: "chatbot",
					StartTime: time.Now().UTC(),
				}},
				Total: 1, PageSize: 100,
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"trace_id": "trace-live",
			"spans": []map[string]interface{}{
				{"service": "chatbot", "name": "execute_tool rag_query"},
			},
		})
	}))
	defer collector.Close()

	baseline, err := observerTurnBaselineSnapshot(monitor.URL, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	live = true
	if err := waitObserverLiveTurn(
		monitor.URL, collector.URL, baseline, time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestObserverEventAfterRejectsStaleAndWrongTuple(t *testing.T) {
	item := observerFleetItem{Body: map[string]interface{}{
		"recent_events": []interface{}{
			observerEventFixture("2026-08-08T12:00:01Z", "ToolDone", "ParsingTier", "Answering"),
		},
	}}
	if observerEventAfter(item, "2026-08-08T12:00:01Z",
		"ToolDone", "ParsingTier", "Answering") {
		t.Fatal("event at the baseline timestamp was accepted as newer")
	}
	if observerEventAfter(item, "", "ToolDone", "SelectingTier", "Answering") {
		t.Fatal("event with the wrong source state was accepted")
	}
	if !observerEventAfter(item, "", "ToolDone", "ParsingTier", "Answering") {
		t.Fatal("matching newer event was rejected")
	}
}

func observerTurnFixture(live bool) map[string]interface{} {
	chatbot := observerPodFixture("chatbot-0", "10.0.0.1", "chatbot", "18082")
	rag0 := observerPodFixture("rag0-0", "10.0.0.2", "rag-server", "18087")
	rag0["metadata"].(map[string]interface{})["labels"].(map[string]interface{})["chatbot-mesh/rag-unit"] = "rag0"
	pods := []interface{}{chatbot, rag0}
	base := 10
	if live {
		base = 20
	}
	labels := map[string]interface{}{
		"discover_mesh_pods": observerLabelFixture(
			base, map[string]interface{}{"mapped": map[string]interface{}{"pods": pods}}),
		"poll_pod_metrics": observerLabelFixture(
			base+5, map[string]interface{}{"mapped": map[string]interface{}{"pod_metrics": []interface{}{}}}),
	}
	for index, label := range observerFanInLabels {
		items := []interface{}{
			observerItemFixture(pods[0], observerMonitorReadSignal, "http://10.0.0.1:18082"),
			observerItemFixture(pods[1], observerMonitorReadSignal, "http://10.0.0.2:18087"),
		}
		if label == "agent_state_fanin" {
			setObserverItemBody(items[0], observerRunFixture(live, "chatbot"))
			setObserverItemBody(items[1], observerRunFixture(live, "rag0"))
		}
		if label == "agent_events_fanin" {
			setObserverItemBody(items[0], observerEventsFixture(live, "chatbot"))
			setObserverItemBody(items[1], observerEventsFixture(live, "rag0"))
		}
		labels[label] = observerLabelFixture(base+1+index, map[string]interface{}{"items": items})
	}
	return labels
}

func observerRunFixture(live bool, component string) map[string]interface{} {
	state := "AwaitingControl"
	updated := "2026-08-08T12:00:01Z"
	if live && component == "chatbot" {
		state, updated = "Reporting", "2026-08-08T12:00:03Z"
	}
	return map[string]interface{}{
		"run": map[string]interface{}{"state": state, "updated_at": updated},
	}
}

func observerEventsFixture(live bool, component string) map[string]interface{} {
	events := []interface{}{
		observerEventFixture("2026-08-08T12:00:01Z", "AwaitTimedOut", "AwaitingControl", "Discovering"),
	}
	if live && component == "chatbot" {
		events = append(events,
			observerEventFixture("2026-08-08T12:00:03Z", "ParseFailed", "ParsingTier", "Answering"))
	}
	if live && component == "rag0" {
		events = append(events,
			observerEventFixture("2026-08-08T12:00:02Z", "QueryResponded", "ResolvingCollection", "Querying"))
	}
	return map[string]interface{}{"recent_events": events}
}

func observerEventFixture(timestamp, signal, fromState, toState string) map[string]interface{} {
	return map[string]interface{}{
		"timestamp": timestamp, "signal": signal,
		"from_state": fromState, "to_state": toState,
	}
}

func setObserverItemBody(value interface{}, body map[string]interface{}) {
	item := value.(map[string]interface{})
	result := item["result"].(map[string]interface{})
	structured := result["structured_output"].(map[string]interface{})
	structured["body"] = body
}

func removeRagQueryEvent(labels map[string]interface{}) {
	entry := labels["agent_events_fanin"].(map[string]interface{})
	output := entry["output"].(map[string]interface{})
	items := output["items"].([]interface{})
	setObserverItemBody(items[1], observerEventsFixture(false, "rag0"))
}
