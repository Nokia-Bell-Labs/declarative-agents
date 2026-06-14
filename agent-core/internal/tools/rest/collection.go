// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import "fmt"

// Collection indexes REST definitions loaded for one profile.
type Collection struct {
	Clients map[string]Client
	Servers map[string]Server
	Limits  map[string]LimitProfile
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
		Clients: map[string]Client{},
		Servers: map[string]Server{},
		Limits:  map[string]LimitProfile{},
	}
}

// Add merges a validated REST definition into the collection.
func (c Collection) Add(def Definition) error {
	for name, limits := range def.Limits {
		if _, exists := c.Limits[name]; exists {
			return fmt.Errorf("duplicate REST limits %q", name)
		}
		c.Limits[name] = limits
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
	client, ok := c.Clients[cfg.RestRef]
	if !ok {
		return Operation{}, fmt.Errorf("REST client %q is not defined", cfg.RestRef)
	}
	if cfg.Resource == "" {
		return operationByName(client.Operations, cfg.Operation, "client "+cfg.RestRef)
	}
	resource, ok := client.Resources[cfg.Resource]
	if !ok {
		return Operation{}, fmt.Errorf("REST resource %q is not defined on client %q", cfg.Resource, cfg.RestRef)
	}
	return operationByName(resource.Operations, cfg.Operation, "resource "+cfg.Resource)
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
