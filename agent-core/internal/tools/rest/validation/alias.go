// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package validation checks REST YAML definitions before they enter a Collection.
//
// It imports definition and the exported client/mock helpers it needs. It does
// not import the parent rest package, so the parent can import validation
// without a cycle. Load/parse stay in definition; the parent composes them
// with ValidateDefinition.
package validation

import (
	restclient "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/client"
	restdef "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/definition"
)

type (
	Definition                   = restdef.Definition
	Client                       = restdef.Client
	Resource                     = restdef.Resource
	Operation                    = restdef.Operation
	StatusMapping                = restdef.StatusMapping
	AuthProfile                  = restdef.AuthProfile
	LimitProfile                 = restdef.LimitProfile
	RedirectPolicy               = restdef.RedirectPolicy
	NetworkPolicy                = restdef.NetworkPolicy
	RetryPolicy                  = restdef.RetryPolicy
	RequestBinding               = restdef.RequestBinding
	ResponseMapping              = restdef.ResponseMapping
	AsyncClientConfig            = restdef.AsyncClientConfig
	SideEffect                   = restdef.SideEffect
	Reversibility                = restdef.Reversibility
	Server                       = restdef.Server
	Endpoint                     = restdef.Endpoint
	QueueConfig                  = restdef.QueueConfig
	ShutdownConfig               = restdef.ShutdownConfig
	LifecycleControl             = restdef.LifecycleControl
	StaticAssetsConfig           = restdef.StaticAssetsConfig
	RedirectConfig               = restdef.RedirectConfig
	MonitorProxyConfig           = restdef.MonitorProxyConfig
	MachineRequest               = restdef.MachineRequest
	MachineRequestMapping        = restdef.MachineRequestMapping
	MachineRequestResponse       = restdef.MachineRequestResponse
	MachineResponseMapping       = restdef.MachineResponseMapping
	SignalSourceBinding          = restdef.SignalSourceBinding
	MockConfig                   = restdef.MockConfig
	MockRoute                    = restdef.MockRoute
	MockResponse                 = restdef.MockResponse
	OpenAPIImport                = restdef.OpenAPIImport
	SignalSourceResponseMappings = restdef.SignalSourceResponseMappings
	SignalSourceResponse         = restdef.SignalSourceResponse
	ClientOperationDefinition    = restclient.ClientOperationDefinition
)
