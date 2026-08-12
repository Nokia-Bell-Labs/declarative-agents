// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"net"
	"net/url"
	"strings"
	"testing"
)

func TestCollectorEndpointReservationAvoidsChatbotOwnership(t *testing.T) {
	chatbotPorts := []string{"4317", "18191", "18192", "18193"}
	var chatbotListeners []net.Listener
	for _, port := range chatbotPorts {
		listener, err := net.Listen("tcp", "127.0.0.1:"+port)
		if err == nil {
			chatbotListeners = append(chatbotListeners, listener)
		}
	}
	defer func() {
		for _, listener := range chatbotListeners {
			_ = listener.Close()
		}
	}()

	reservation, err := reserveCollectorEndpoints()
	if err != nil {
		t.Fatal(err)
	}
	defer reservation.release()
	got := endpointPorts(t, reservation.endpoints)
	seen := make(map[string]bool)
	for name, port := range got {
		if seen[port] {
			t.Errorf("collector endpoint %s reuses reserved port %s", name, port)
		}
		seen[port] = true
		for _, chatbotPort := range chatbotPorts {
			if port == chatbotPort {
				t.Errorf("collector endpoint %s collided with Chatbot port %s", name, port)
			}
		}
	}
}

func TestConcurrentCollectorReservationsAreDisjoint(t *testing.T) {
	first, err := reserveCollectorEndpoints()
	if err != nil {
		t.Fatal(err)
	}
	defer first.release()
	second, err := reserveCollectorEndpoints()
	if err != nil {
		t.Fatal(err)
	}
	defer second.release()

	firstPorts := endpointPorts(t, first.endpoints)
	secondPorts := endpointPorts(t, second.endpoints)
	for firstName, firstPort := range firstPorts {
		for secondName, secondPort := range secondPorts {
			if firstPort == secondPort {
				t.Errorf("%s and %s reservations collide on %s",
					firstName, secondName, firstPort)
			}
		}
	}
}

func endpointPorts(t *testing.T, endpoints collectorEndpoints) map[string]string {
	t.Helper()
	urlPort := func(raw string) string {
		parsed, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("parse endpoint %q: %v", raw, err)
		}
		return parsed.Port()
	}
	_, receiver, err := net.SplitHostPort(endpoints.ReceiverAddress)
	if err != nil {
		t.Fatalf("parse receiver %q: %v", endpoints.ReceiverAddress, err)
	}
	ports := map[string]string{
		"receiver": receiver,
		"control":  urlPort(endpoints.ControlAddress),
		"monitor":  urlPort(endpoints.MonitorAddress),
		"query":    urlPort(endpoints.QueryAddress),
	}
	for name, port := range ports {
		if strings.TrimSpace(port) == "" {
			t.Fatalf("%s endpoint has no port: %+v", name, endpoints)
		}
	}
	return ports
}
