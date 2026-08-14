// Copyright (c) 2026 Nokia. All rights reserved.

package profileaudit

import (
	"fmt"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolrest "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest"
)

func (i *inspector) inspectRESTClient(
	closure loadedClosure,
	commandTimeout time.Duration,
	def catalog.ToolDef,
) error {
	restRef, _ := configString(def.Config, "rest_ref")
	resource, _ := configString(def.Config, "resource")
	operation, _ := configString(def.Config, "operation")
	resolved, err := closure.rest.ResolveClientOperation(toolrest.ClientToolConfig{
		RestRef: restRef, Resource: resource, Operation: operation,
	})
	if err != nil {
		return fmt.Errorf("profile %s action %q REST authority: %w", closure.profilePath, def.Name, err)
	}
	i.inspectRESTClientLimits(closure, commandTimeout, def, resolved)
	if resolved.Client.RetryRef != "" && def.Init != "rest_client_await" {
		i.inspectRESTRetryAggregate(closure, commandTimeout, def, resolved)
	}
	return i.inspectRESTClientAsync(closure, commandTimeout, def, resolved)
}

func (i *inspector) inspectRESTClientLimits(
	closure loadedClosure,
	commandTimeout time.Duration,
	def catalog.ToolDef,
	resolved toolrest.ClientOperationDefinition,
) {
	base := fmt.Sprintf("REST client %s limits", resolved.RestRef)
	if resolved.Limits.Timeout == "" {
		i.addUnsupported(closure, def, commandTimeout, base+".timeout")
	} else {
		i.addDuration(closure, def.Name, base+".timeout", resolved.Limits.Timeout, commandTimeout)
	}
	for field, raw := range map[string]string{
		"connect_timeout": resolved.Limits.ConnectTimeout,
		"read_timeout":    resolved.Limits.ReadTimeout,
	} {
		if raw != "" {
			i.addDuration(closure, def.Name, base+"."+field, raw, commandTimeout)
		}
	}
}

func (i *inspector) inspectRESTRetryAggregate(
	closure loadedClosure,
	commandTimeout time.Duration,
	def catalog.ToolDef,
	resolved toolrest.ClientOperationDefinition,
) {
	attemptTimeout, err := time.ParseDuration(resolved.Limits.Timeout)
	if err != nil || attemptTimeout <= 0 {
		return // The ordinary limits.timeout operation owns this diagnostic.
	}
	backoff := resolved.Retry.Backoff
	if backoff == "" {
		backoff = "fixed"
	}
	source := fmt.Sprintf(
		"REST client %s retry %s aggregate (%d attempts, %s backoff)",
		resolved.RestRef, resolved.Client.RetryRef, resolved.Retry.Attempts, backoff,
	)
	aggregate, err := toolrest.RetryAggregateTimeout(attemptTimeout, resolved.Retry)
	if err != nil {
		i.addInvalid(
			closure, def.Name, source, "unbounded", commandTimeout,
			"retry aggregate has no finite duration: "+err.Error(),
		)
		return
	}
	i.addDuration(closure, def.Name, source, aggregate.String(), commandTimeout)
}

func (i *inspector) inspectRESTClientAsync(
	closure loadedClosure,
	commandTimeout time.Duration,
	def catalog.ToolDef,
	resolved toolrest.ClientOperationDefinition,
) error {
	if def.Init != "rest_client_send" && def.Init != "rest_client_await" {
		return nil
	}
	if resolved.Operation.Async == nil {
		return fmt.Errorf(
			"profile %s action %q requires an async REST operation",
			closure.profilePath, def.Name,
		)
	}
	if def.Init == "rest_client_await" {
		i.addDurationDefault(
			closure, def.Name,
			fmt.Sprintf(
				"REST client %s operation %s async.timeout",
				resolved.RestRef, resolved.OperationName,
			),
			resolved.Operation.Async.Timeout, defaultRESTAwait, commandTimeout,
		)
	}
	return nil
}
