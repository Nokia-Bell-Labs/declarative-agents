// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"net"
	"strconv"
)

type collectorEndpoints struct {
	ReceiverAddress string
	ControlAddress  string
	MonitorAddress  string
	QueryAddress    string
	ControlPort     string
	MonitorPort     string
	QueryPort       string
}

type collectorEndpointReservation struct {
	endpoints collectorEndpoints
	listeners []net.Listener
}

func reserveCollectorEndpoints() (*collectorEndpointReservation, error) {
	reservation := &collectorEndpointReservation{}
	addresses := make([]string, 4)
	for index := range addresses {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			reservation.release()
			return nil, fmt.Errorf("reserve collector loopback endpoint: %w", err)
		}
		reservation.listeners = append(reservation.listeners, listener)
		addresses[index] = listener.Addr().String()
	}
	controlPort, err := portFromAddress(addresses[1])
	if err != nil {
		reservation.release()
		return nil, err
	}
	monitorPort, err := portFromAddress(addresses[2])
	if err != nil {
		reservation.release()
		return nil, err
	}
	queryPort, err := portFromAddress(addresses[3])
	if err != nil {
		reservation.release()
		return nil, err
	}
	reservation.endpoints = collectorEndpoints{
		ReceiverAddress: addresses[0],
		ControlAddress:  "http://" + addresses[1],
		MonitorAddress:  "http://" + addresses[2],
		QueryAddress:    "http://" + addresses[3],
		ControlPort:     controlPort,
		MonitorPort:     monitorPort,
		QueryPort:       queryPort,
	}
	return reservation, nil
}

func (reservation *collectorEndpointReservation) release() {
	if reservation == nil {
		return
	}
	for _, listener := range reservation.listeners {
		_ = listener.Close()
	}
	reservation.listeners = nil
}

func portFromAddress(address string) (string, error) {
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", fmt.Errorf("parse collector endpoint %q: %w", address, err)
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 {
		return "", fmt.Errorf("collector endpoint %q has invalid port", address)
	}
	return port, nil
}
