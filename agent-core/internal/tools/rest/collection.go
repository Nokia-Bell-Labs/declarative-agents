// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import "fmt"

// Collection indexes REST definitions loaded for one profile.
type Collection struct {
	Clients          map[string]Client
	Servers          map[string]Server
	Auth             map[string]AuthProfile
	Limits           map[string]LimitProfile
	RetryPolicies    map[string]RetryPolicy
	ResponseMappings map[string]ResponseMapping
}

// ClientOperationDefinition is a resolved client operation and trusted policy.
type ClientOperationDefinition struct {
	RestRef          string
	Resource         string
	OperationName    string
	Client           Client
	Operation        Operation
	Auth             AuthProfile
	Limits           LimitProfile
	Retry            RetryPolicy
	ResponseMappings map[string]ResponseMapping
}

// ServerDefinition is a resolved server plus its referenced limit profile.
type ServerDefinition struct {
	Name   string
	Server Server
	Limits LimitProfile
}

// NewCollection creates an empty REST definition collection.
func NewCollection() Collection {
	return Collection{
		Clients:          map[string]Client{},
		Servers:          map[string]Server{},
		Auth:             map[string]AuthProfile{},
		Limits:           map[string]LimitProfile{},
		RetryPolicies:    map[string]RetryPolicy{},
		ResponseMappings: map[string]ResponseMapping{},
	}
}

// Add merges a validated REST definition into the collection.
func (c Collection) Add(def Definition) error {
	for name, profile := range def.Auth {
		if _, exists := c.Auth[name]; exists {
			return fmt.Errorf("duplicate REST auth %q", name)
		}
		c.Auth[name] = profile
	}
	for name, limits := range def.Limits {
		if _, exists := c.Limits[name]; exists {
			return fmt.Errorf("duplicate REST limits %q", name)
		}
		c.Limits[name] = limits
	}
	for name, retry := range def.RetryPolicies {
		if _, exists := c.RetryPolicies[name]; exists {
			return fmt.Errorf("duplicate REST retry policy %q", name)
		}
		c.RetryPolicies[name] = retry
	}
	for name, mapping := range def.ResponseMappings {
		if _, exists := c.ResponseMappings[name]; exists {
			return fmt.Errorf("duplicate REST response mapping %q", name)
		}
		c.ResponseMappings[name] = mapping
	}
	for name, client := range def.Clients {
		if _, exists := c.Clients[name]; exists {
			return fmt.Errorf("duplicate REST client %q", name)
		}
		c.Clients[name] = client
	}
	for name, server := range def.Servers {
		if _, exists := c.Servers[name]; exists {
			return fmt.Errorf("duplicate REST server %q", name)
		}
		c.Servers[name] = server
	}
	return nil
}

// ClientOperation resolves a configured client operation.
func (c Collection) ClientOperation(cfg ClientToolConfig) (Operation, error) {
	resolved, err := c.ResolveClientOperation(cfg)
	if err != nil {
		return Operation{}, err
	}
	return resolved.Operation, nil
}

// ResolveClientOperation returns a client operation with trusted policy config.
func (c Collection) ResolveClientOperation(cfg ClientToolConfig) (ClientOperationDefinition, error) {
	client, ok := c.Clients[cfg.RestRef]
	if !ok {
		return ClientOperationDefinition{}, fmt.Errorf("REST client %q is not defined", cfg.RestRef)
	}
	operation, err := c.resolveOperation(client, cfg)
	if err != nil {
		return ClientOperationDefinition{}, err
	}
	return ClientOperationDefinition{
		RestRef: cfg.RestRef, Resource: cfg.Resource, OperationName: cfg.Operation,
		Client: client, Operation: operation, Auth: c.Auth[client.AuthRef],
		Limits: c.Limits[client.LimitsRef], Retry: c.RetryPolicies[client.RetryRef],
		ResponseMappings: c.ResponseMappings,
	}, nil
}

func (c Collection) resolveOperation(client Client, cfg ClientToolConfig) (Operation, error) {
	if cfg.Resource == "" {
		return operationByName(client.Operations, cfg.Operation, "client "+cfg.RestRef)
	}
	resource, ok := client.Resources[cfg.Resource]
	if !ok {
		return Operation{}, fmt.Errorf("REST resource %q is not defined on client %q", cfg.Resource, cfg.RestRef)
	}
	operation, err := operationByName(resource.Operations, cfg.Operation, "resource "+cfg.Resource)
	if err != nil {
		return Operation{}, err
	}
	if operation.Path == "" {
		operation.Path = resource.Path
	}
	return operation, nil
}

// Server resolves a configured server definition.
func (c Collection) Server(name string) (Server, error) {
	resolved, err := c.ResolveServer(name)
	if err != nil {
		return Server{}, err
	}
	return resolved.Server, nil
}

// ResolveServer returns a server with the limit profile it references.
func (c Collection) ResolveServer(name string) (ServerDefinition, error) {
	server, ok := c.Servers[name]
	if !ok {
		return ServerDefinition{}, fmt.Errorf("REST server %q is not defined", name)
	}
	return ServerDefinition{Name: name, Server: server, Limits: c.Limits[server.LimitsRef]}, nil
}

func operationByName(operations map[string]Operation, name, owner string) (Operation, error) {
	operation, ok := operations[name]
	if !ok {
		return Operation{}, fmt.Errorf("REST operation %q is not defined on %s", name, owner)
	}
	return operation, nil
}
