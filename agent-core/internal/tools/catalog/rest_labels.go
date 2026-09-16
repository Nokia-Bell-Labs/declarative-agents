// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

// Label types for the words that call a REST client (srd028, srd038 R2.15).
//
// A REST client word publishes the envelope the runtime builds around a
// response, not the payload its output.schema describes, which is why typing
// its label from that schema rejects $from(word).mapped.field. The envelope is
// one shape: twelve fields the runtime fills the same way every time, plus
// mapped and carried, which the operation's own declaration states. Deriving
// the type from that declaration rather than from a signature keeps one
// statement of what mapped holds; a signature would be a second one, free to
// drift from the response mapping the runtime actually reads.

// RESTClientResponse is what a REST client operation declares about its
// response, as far as the type of a label depends on it.
type RESTClientResponse struct {
	// Mapped are the keys published under mapped, united over every status
	// mapping the operation declares. The status decides which mapping applies
	// and the label is published whichever one fired, so a key any of them
	// publishes can legitimately resolve.
	Mapped []string
	// Carried are the request parameters the operation carries forward.
	Carried []string
}

// RESTOperations resolves what a REST client word's configured operation
// declares. The REST package imports this one, so it cannot be imported back;
// the lookup arrives through this interface instead.
type RESTOperations interface {
	ClientResponse(config map[string]interface{}) (RESTClientResponse, bool)
}

// restClientInits are the words whose result is a REST client response
// envelope. A word outside this set keeps whatever type its signature states.
var restClientInits = map[string]bool{
	"rest_client_invoke": true,
	"rest_client_get":    true,
	"rest_client_send":   true,
}

// restClientEnvelopeSchema is the shape clientResultOutput builds. body stays
// undecided: each status mapping may declare its own schema, so no single body
// shape is right for a label published whichever status returned, and a wrong
// one rejects a selector that resolves.
func restClientEnvelopeSchema(response RESTClientResponse) map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"rest_ref":           map[string]interface{}{"type": "string"},
			"resource":           map[string]interface{}{"type": "string"},
			"operation":          map[string]interface{}{"type": "string"},
			"status":             map[string]interface{}{"type": "integer"},
			"headers":            map[string]interface{}{"type": "object"},
			"body":               describedValue("the decoded response, whose shape the status mapping decides"),
			"mapped":             namedFields(response.Mapped, "a field the operation's response mapping publishes"),
			"carried":            namedFields(response.Carried, "a request parameter the operation carries forward"),
			"resource_id":        describedValue("the resource identity the response mapping selects"),
			"request_id":         describedValue("the request identity the response mapping selects"),
			"retry_count":        map[string]interface{}{"type": "integer"},
			"domain_error_code":  map[string]interface{}{"type": "string"},
			"selected_authority": map[string]interface{}{"type": "string"},
		},
	}
}

// namedFields is an object whose declared fields are exactly the names given,
// each of a shape the response decides.
func namedFields(names []string, description string) map[string]interface{} {
	properties := make(map[string]interface{}, len(names))
	for _, name := range names {
		properties[name] = describedValue(description)
	}
	return map[string]interface{}{"type": "object", "properties": properties}
}

// describedValue is a schema that names what a value is without constraining
// it, so a path stops there rather than being decided against a shape no
// declaration states.
func describedValue(description string) map[string]interface{} {
	return map[string]interface{}{"description": description}
}

// restClientLabelSchema returns the envelope a REST client word publishes, and
// reports false for any other word or for an operation that does not resolve.
// An unresolvable operation leaves the label untyped rather than failing here:
// the load reports that separately, and typing is gradual.
func restClientLabelSchema(def ToolDef, operations RESTOperations) (map[string]interface{}, bool) {
	if operations == nil || !restClientInits[def.Init] {
		return nil, false
	}
	response, ok := operations.ClientResponse(def.Config)
	if !ok {
		return nil, false
	}
	return restClientEnvelopeSchema(response), true
}
