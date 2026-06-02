package openapi

import "net/http"

// JSONResponse is a (statusCode, body) pair describing one JSON
// response variant. Use with OperationBuilder.ApplyJSONResponses to
// declare several at once.
type JSONResponse struct {
	Status int
	Body   any
}

// ApplyJSONResponses calls Response(status, body) for every entry in
// resps. Returns the builder so it remains chainable.
//
// See: https://spec.openapis.org/oas/v3.1.0#responses-object
func (b *OperationBuilder) ApplyJSONResponses(resps ...JSONResponse) *OperationBuilder {
	for _, r := range resps {
		b.Response(r.Status, r.Body)
	}
	return b
}

// StandardErrorResponses declares the canonical OAuth/OIDC error-status
// set (400, 401, 403) using a single error-body type. body should be
// the JSON shape your TokenError handler emits. Returns the builder
// for chaining.
//
// This is a convenience for the common pattern of mapping every
// machine-readable failure onto the same error envelope.
func (b *OperationBuilder) StandardErrorResponses(body any) *OperationBuilder {
	return b.
		Response(http.StatusBadRequest, body).
		Response(http.StatusUnauthorized, body).
		Response(http.StatusForbidden, body)
}

// ApplyJSONResponses calls Response(status, body) for every entry in
// resps, declaring each as a shared group response. Returns the group
// so it remains chainable.
//
// See: https://spec.openapis.org/oas/v3.1.0#responses-object
func (g *RouteGroup) ApplyJSONResponses(resps ...JSONResponse) *RouteGroup {
	for _, r := range resps {
		g = g.Response(r.Status, r.Body)
	}
	return g
}

// StandardErrorResponses declares the canonical OAuth/OIDC error-status
// set (400, 401, 403) as shared group responses using a single
// error-body type. All operations created through this group inherit
// them. Returns the group for chaining.
//
// This is a convenience for the common pattern of mapping every
// machine-readable failure onto the same error envelope across a group.
func (g *RouteGroup) StandardErrorResponses(body any) *RouteGroup {
	return g.
		Response(http.StatusBadRequest, body).
		Response(http.StatusUnauthorized, body).
		Response(http.StatusForbidden, body)
}
