// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package servercmd

import "context"

// Event is one inbound REST event carried on command results and receipts.
type Event struct {
	Source    string                 `json:"source"`
	Queue     string                 `json:"queue,omitempty"`
	Route     string                 `json:"route"`
	Method    string                 `json:"method"`
	Signal    string                 `json:"signal"`
	Payload   map[string]interface{} `json:"payload,omitempty"`
	RequestID string                 `json:"request_id,omitempty"`
}

// Identity is the process-local listener identity a launch receipt records.
type Identity struct {
	Address   string
	Ownership string
}

// ServerHooks is the parent-owned launch/await/stop surface. It is a struct of
// funcs because the operations exceed a 1-3 method interface and because this
// package must not import rest.
type ServerHooks struct {
	Launch            func() (map[string]interface{}, Identity, error)
	Await             func() (Event, string, error)
	AwaitContext      func(context.Context) (Event, string, error)
	Stop              func() (map[string]interface{}, error)
	UndoLaunch        func(server, address, ownership string) (map[string]interface{}, error)
	RestoreEvent      func(server string, event Event) error
	ServerName        string
	ConfiguredAddress string
}

// AwaitHooks is the parent-owned fan-in await surface.
type AwaitHooks struct {
	AwaitAny        func() (Event, string, error)
	AwaitAnyContext func(context.Context) (Event, string, error)
	RestoreEvent    func(server string, event Event) error
}

const (
	initLaunch = "rest_server_launch"
	initAwait  = "rest_server_await"
	initStop   = "rest_server_stop"
)
