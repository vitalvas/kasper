package openapi

import (
	"net/http"
	"strconv"
)

// operationMeta stores metadata collected via the fluent builder
// before the final spec is built.
type operationMeta struct {
	operationID  string
	summary      string
	description  string
	tags         []string
	deprecated   bool
	parameters   []*Parameter
	security     []SecurityRequirement
	externalDocs *ExternalDocs
	callbacks    map[string]*Callback
	servers      []Server

	requestContents      map[string]any                // contentType -> body
	requestDescription   string                        // request body description
	requestRequired      *bool                         // nil = default (true), non-nil = explicit
	responseContents     map[string]map[string]any     // statusKey -> contentType -> body
	responseDescriptions map[string]string             // statusKey -> custom description
	responseHeaders      map[string]map[string]*Header // statusKey -> headerName -> header
	responseLinks        map[string]map[string]*Link   // statusKey -> linkName -> link
}

// OperationBuilder provides a fluent API for attaching OpenAPI metadata
// to a named route.
type OperationBuilder struct {
	meta *operationMeta
}

func newOperationBuilder() *OperationBuilder {
	return &OperationBuilder{
		meta: &operationMeta{
			requestContents:  make(map[string]any),
			responseContents: make(map[string]map[string]any),
		},
	}
}

// OperationID sets a custom operation ID, overriding the auto-detected route
// name. This is useful with Route() where the mux route may not have a name.
func (b *OperationBuilder) OperationID(id string) *OperationBuilder {
	b.meta.operationID = id
	return b
}

// Summary sets the operation summary.
func (b *OperationBuilder) Summary(s string) *OperationBuilder {
	b.meta.summary = s
	return b
}

// Description sets the operation description.
func (b *OperationBuilder) Description(d string) *OperationBuilder {
	b.meta.description = d
	return b
}

// Tags adds one or more tags to the operation.
func (b *OperationBuilder) Tags(tags ...string) *OperationBuilder {
	b.meta.tags = append(b.meta.tags, tags...)
	return b
}

// Deprecated marks the operation as deprecated.
func (b *OperationBuilder) Deprecated() *OperationBuilder {
	b.meta.deprecated = true
	return b
}

// Request registers an application/json request body type for the operation.
// This is a shortcut for RequestContent("application/json", body).
func (b *OperationBuilder) Request(body any) *OperationBuilder {
	b.meta.requestContents["application/json"] = body
	return b
}

// RequestContent registers a request body with the given content type.
// The body can be a Go type (schema generated via reflection), a *Schema
// for explicit schema control, or nil for a content type with no schema.
func (b *OperationBuilder) RequestContent(contentType string, body any) *OperationBuilder {
	b.meta.requestContents[contentType] = body
	return b
}

// RequestDescription sets the description for the request body.
func (b *OperationBuilder) RequestDescription(desc string) *OperationBuilder {
	b.meta.requestDescription = desc
	return b
}

// RequestRequired sets whether the request body is required.
// By default, request bodies are required (true).
func (b *OperationBuilder) RequestRequired(required bool) *OperationBuilder {
	b.meta.requestRequired = &required
	return b
}

// Response registers an application/json response type for the given HTTP
// status code. Pass nil body for responses with no content (e.g., 204).
// This is a shortcut for ResponseContent(statusCode, "application/json", body)
// when body is non-nil.
func (b *OperationBuilder) Response(statusCode int, body any) *OperationBuilder {
	key := strconv.Itoa(statusCode)
	if body != nil {
		if b.meta.responseContents[key] == nil {
			b.meta.responseContents[key] = make(map[string]any)
		}
		b.meta.responseContents[key]["application/json"] = body
	} else if b.meta.responseContents[key] == nil {
		b.meta.responseContents[key] = nil
	}
	return b
}

// ResponseContent registers a response with the given status code and content
// type. The body can be a Go type (schema generated via reflection), a *Schema
// for explicit schema control, or nil for a content type with no schema.
func (b *OperationBuilder) ResponseContent(statusCode int, contentType string, body any) *OperationBuilder {
	key := strconv.Itoa(statusCode)
	if b.meta.responseContents[key] == nil {
		b.meta.responseContents[key] = make(map[string]any)
	}
	b.meta.responseContents[key][contentType] = body
	return b
}

// DefaultResponse registers an application/json response for the "default"
// status key. The default response catches any status code not covered by
// specific responses. Pass nil body for a default response with no content.
func (b *OperationBuilder) DefaultResponse(body any) *OperationBuilder {
	if body != nil {
		if b.meta.responseContents["default"] == nil {
			b.meta.responseContents["default"] = make(map[string]any)
		}
		b.meta.responseContents["default"]["application/json"] = body
	} else if b.meta.responseContents["default"] == nil {
		b.meta.responseContents["default"] = nil
	}
	return b
}

// DefaultResponseContent registers a response with the given content type
// for the "default" status key.
func (b *OperationBuilder) DefaultResponseContent(contentType string, body any) *OperationBuilder {
	if b.meta.responseContents["default"] == nil {
		b.meta.responseContents["default"] = make(map[string]any)
	}
	b.meta.responseContents["default"][contentType] = body
	return b
}

// ResponseHeader adds a header to the response for the given HTTP status code.
func (b *OperationBuilder) ResponseHeader(statusCode int, name string, h *Header) *OperationBuilder {
	key := strconv.Itoa(statusCode)
	if b.meta.responseHeaders == nil {
		b.meta.responseHeaders = make(map[string]map[string]*Header)
	}
	if b.meta.responseHeaders[key] == nil {
		b.meta.responseHeaders[key] = make(map[string]*Header)
	}
	b.meta.responseHeaders[key][name] = h
	return b
}

// ResponseLink adds a link to the response for the given HTTP status code.
func (b *OperationBuilder) ResponseLink(statusCode int, name string, l *Link) *OperationBuilder {
	key := strconv.Itoa(statusCode)
	if b.meta.responseLinks == nil {
		b.meta.responseLinks = make(map[string]map[string]*Link)
	}
	if b.meta.responseLinks[key] == nil {
		b.meta.responseLinks[key] = make(map[string]*Link)
	}
	b.meta.responseLinks[key][name] = l
	return b
}

// DefaultResponseHeader adds a header to the default response.
func (b *OperationBuilder) DefaultResponseHeader(name string, h *Header) *OperationBuilder {
	if b.meta.responseHeaders == nil {
		b.meta.responseHeaders = make(map[string]map[string]*Header)
	}
	if b.meta.responseHeaders["default"] == nil {
		b.meta.responseHeaders["default"] = make(map[string]*Header)
	}
	b.meta.responseHeaders["default"][name] = h
	return b
}

// DefaultResponseLink adds a link to the default response.
func (b *OperationBuilder) DefaultResponseLink(name string, l *Link) *OperationBuilder {
	if b.meta.responseLinks == nil {
		b.meta.responseLinks = make(map[string]map[string]*Link)
	}
	if b.meta.responseLinks["default"] == nil {
		b.meta.responseLinks["default"] = make(map[string]*Link)
	}
	b.meta.responseLinks["default"][name] = l
	return b
}

// ResponseDescription overrides the auto-generated description for a response.
// By default, descriptions are derived from HTTP status text (e.g., "OK", "Not Found").
func (b *OperationBuilder) ResponseDescription(statusCode int, desc string) *OperationBuilder {
	key := strconv.Itoa(statusCode)
	if b.meta.responseDescriptions == nil {
		b.meta.responseDescriptions = make(map[string]string)
	}
	b.meta.responseDescriptions[key] = desc
	return b
}

// DefaultResponseDescription overrides the auto-generated description for the
// default response.
func (b *OperationBuilder) DefaultResponseDescription(desc string) *OperationBuilder {
	if b.meta.responseDescriptions == nil {
		b.meta.responseDescriptions = make(map[string]string)
	}
	b.meta.responseDescriptions["default"] = desc
	return b
}

// Parameter adds a custom parameter to the operation.
func (b *OperationBuilder) Parameter(param *Parameter) *OperationBuilder {
	b.meta.parameters = append(b.meta.parameters, param)
	return b
}

// Security sets operation-level security requirements.
// Call with no arguments to explicitly mark the operation as unauthenticated
// (overrides document-level security).
func (b *OperationBuilder) Security(reqs ...SecurityRequirement) *OperationBuilder {
	if reqs == nil {
		reqs = []SecurityRequirement{}
	}
	b.meta.security = reqs
	return b
}

// ExternalDocs sets external documentation for the operation.
func (b *OperationBuilder) ExternalDocs(url, description string) *OperationBuilder {
	b.meta.externalDocs = &ExternalDocs{URL: url, Description: description}
	return b
}

// Callback adds a callback to the operation.
func (b *OperationBuilder) Callback(name string, cb *Callback) *OperationBuilder {
	if b.meta.callbacks == nil {
		b.meta.callbacks = make(map[string]*Callback)
	}
	b.meta.callbacks[name] = cb
	return b
}

// Server adds a server override for the operation.
func (b *OperationBuilder) Server(server Server) *OperationBuilder {
	b.meta.servers = append(b.meta.servers, server)
	return b
}

// mergeParameters combines auto-generated path parameters with custom
// parameters. Custom parameters with the same name+in override the
// auto-generated ones. Parameters not present in custom are kept from auto.
func mergeParameters(auto, custom []*Parameter) []*Parameter {
	if len(auto) == 0 && len(custom) == 0 {
		return nil
	}

	// Index custom parameters by name+in for O(1) lookup.
	overrides := make(map[[2]string]struct{}, len(custom))
	for _, p := range custom {
		overrides[[2]string{p.Name, p.In}] = struct{}{}
	}

	// Keep auto parameters that are not overridden by custom.
	var merged []*Parameter
	for _, p := range auto {
		if _, ok := overrides[[2]string{p.Name, p.In}]; !ok {
			merged = append(merged, p)
		}
	}

	merged = append(merged, custom...)
	return merged
}

// resolveSchema returns a Schema for the given body value. If body is a
// *Schema it is used directly; otherwise the schema generator produces one
// via reflection.
func resolveSchema(gen *SchemaGenerator, body any) *Schema {
	if body == nil {
		return nil
	}
	if s, ok := body.(*Schema); ok {
		return s
	}
	return gen.Generate(body)
}

// responseDescription returns a human-readable description for a response key.
func responseDescription(key string) string {
	if key == "default" {
		return "Default response"
	}
	code, err := strconv.Atoi(key)
	if err == nil {
		if text := http.StatusText(code); text != "" {
			return text
		}
	}
	return key
}

// buildOperation converts the collected metadata into an Operation object
// using the given schema generator.
func (b *OperationBuilder) buildOperation(gen *SchemaGenerator, operationID string, pathParams []*Parameter) *Operation {
	if b.meta.operationID != "" {
		operationID = b.meta.operationID
	}
	op := &Operation{
		OperationID:  operationID,
		Summary:      b.meta.summary,
		Description:  b.meta.description,
		Tags:         b.meta.tags,
		Deprecated:   b.meta.deprecated,
		Security:     b.meta.security,
		ExternalDocs: b.meta.externalDocs,
		Callbacks:    b.meta.callbacks,
		Servers:      b.meta.servers,
	}

	// Merge path parameters with custom parameters. Custom parameters
	// with the same name+in override auto-generated path parameters
	// to avoid duplicates (OpenAPI requires unique name+in).
	op.Parameters = mergeParameters(pathParams, b.meta.parameters)

	// Build request body.
	if len(b.meta.requestContents) > 0 {
		required := true
		if b.meta.requestRequired != nil {
			required = *b.meta.requestRequired
		}
		op.RequestBody = &RequestBody{
			Description: b.meta.requestDescription,
			Required:    required,
			Content:     make(map[string]*MediaType, len(b.meta.requestContents)),
		}
		for ct, body := range b.meta.requestContents {
			mt := &MediaType{}
			if schema := resolveSchema(gen, body); schema != nil {
				mt.Schema = schema
			}
			op.RequestBody.Content[ct] = mt
		}
	}

	// Build responses.
	if len(b.meta.responseContents) > 0 {
		op.Responses = make(map[string]*Response, len(b.meta.responseContents))
		for key, contents := range b.meta.responseContents {
			desc := responseDescription(key)
			if custom, ok := b.meta.responseDescriptions[key]; ok {
				desc = custom
			}
			resp := &Response{
				Description: desc,
			}
			if len(contents) > 0 {
				resp.Content = make(map[string]*MediaType, len(contents))
				for ct, body := range contents {
					mt := &MediaType{}
					if schema := resolveSchema(gen, body); schema != nil {
						mt.Schema = schema
					}
					resp.Content[ct] = mt
				}
			}
			if headers, ok := b.meta.responseHeaders[key]; ok && len(headers) > 0 {
				resp.Headers = headers
			}
			if links, ok := b.meta.responseLinks[key]; ok && len(links) > 0 {
				resp.Links = links
			}
			op.Responses[key] = resp
		}
	}

	return op
}
