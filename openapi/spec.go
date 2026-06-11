package openapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/vitalvas/kasper/mux"
)

// macroTypeMap maps mux route macros to OpenAPI type and format.
//
// See: https://spec.openapis.org/oas/v3.1.0#data-types
var macroTypeMap = map[string][2]string{
	"uuid":     {"string", "uuid"},
	"int":      {"integer", ""},
	"float":    {"number", ""},
	"slug":     {"string", ""},
	"alpha":    {"string", ""},
	"alphanum": {"string", ""},
	"date":     {"string", "date"},
	"hex":      {"string", ""},
	"domain":   {"string", "hostname"},
}

// pathVarRegexp matches route variables in the form {name} or {name:macro}.
var pathVarRegexp = regexp.MustCompile(`\{([^}]+)\}`)

// Spec collects OpenAPI metadata for routes and builds a complete Document.
//
// See: https://spec.openapis.org/oas/v3.1.0#openapi-object
type Spec struct {
	info       Info
	servers    []Server
	operations map[string]*OperationBuilder            // keyed by route name (Op)
	routeOps   map[*mux.Route]*OperationBuilder        // keyed by route pointer (Route)
	webhooks   map[string]map[string]*OperationBuilder // name -> method -> builder

	pathServers      map[string][]Server     // keyed by OpenAPI path
	pathSummaries    map[string]string       // keyed by OpenAPI path
	pathDescriptions map[string]string       // keyed by OpenAPI path
	pathParameters   map[string][]*Parameter // keyed by OpenAPI path

	externalDocs    *ExternalDocs
	security        []SecurityRequirement
	tags            []Tag
	securitySchemes map[string]*SecurityScheme

	// ingestedPaths and ingestedWebhooks hold path items merged from
	// Documents passed to SpecFromDocuments. Build seeds the output from
	// these before walking the router, so ingested operations and
	// route-derived operations combine per-method.
	ingestedPaths    map[string]*PathItem
	ingestedWebhooks map[string]*PathItem

	// ingestedSchemas holds component schemas from ingested Documents,
	// merged with reflection-generated schemas at build time.
	ingestedSchemas map[string]*Schema
	compResponses   map[string]*Response
	compParameters  map[string]*Parameter
	compExamples    map[string]*Example
	compReqBodies   map[string]*RequestBody
	compHeaders     map[string]*Header
	compLinks       map[string]*Link
	compCallbacks   map[string]*Callback
	compPathItems   map[string]*PathItem
}

// NewSpec creates a new spec builder with the given API info.
//
// See: https://spec.openapis.org/oas/v3.1.0#openapi-object
func NewSpec(info Info) *Spec {
	return &Spec{
		info:       info,
		operations: make(map[string]*OperationBuilder),
		routeOps:   make(map[*mux.Route]*OperationBuilder),
	}
}

// SpecFromDocuments ingests one or more already-built Documents (for example
// schemas fetched from child services) into a single Spec, merging their
// paths and webhooks per-method. The resulting Spec is served like any other
// via Handle, and may still register additional routes; route-derived
// operations combine with the ingested ones per-method.
//
// Document metadata is combined: the Info, JSON Schema dialect, external docs,
// and security come from the first document that sets them; servers, tags, and
// components are accumulated. Component entries with the same name must be
// identical across documents, matching MergeDocuments.
//
// An error is returned, listing every conflict, when two documents declare the
// same method on the same path (or webhook) or define the same component name
// with different content. On error the returned Spec is nil.
//
// See: https://spec.openapis.org/oas/v3.1.0#openapi-object
func SpecFromDocuments(docs ...*Document) (*Spec, error) {
	s := NewSpec(Info{})
	s.ingestedPaths = make(map[string]*PathItem)
	s.ingestedWebhooks = make(map[string]*PathItem)

	var conflicts []string

	for _, doc := range docs {
		if doc == nil {
			continue
		}

		s.ingestDocumentMeta(doc)
		mergePathItems(s.ingestedPaths, doc.Paths, "paths", &conflicts)
		mergePathItems(s.ingestedWebhooks, doc.Webhooks, "webhooks", &conflicts)
		s.ingestComponents(doc.Components, &conflicts)
	}

	if len(conflicts) > 0 {
		sort.Strings(conflicts)
		return nil, errors.New(strings.Join(conflicts, "; "))
	}

	return s, nil
}

// ingestDocumentMeta copies document-level metadata into the spec. Scalar
// fields take the first value that is set; slice fields accumulate.
func (s *Spec) ingestDocumentMeta(doc *Document) {
	if s.info.Title == "" {
		s.info = doc.Info
	}
	if s.externalDocs == nil {
		s.externalDocs = doc.ExternalDocs
	}
	if len(s.security) == 0 {
		s.security = doc.Security
	}
	s.servers = append(s.servers, doc.Servers...)
	s.tags = append(s.tags, doc.Tags...)
}

// mergePathItems merges src path items into dst per-method. A method already
// present on a path in dst with a different operation produces a conflict.
func mergePathItems(dst, src map[string]*PathItem, kind string, conflicts *[]string) {
	for path, item := range src {
		existing, ok := dst[path]
		if !ok {
			clone := *item
			dst[path] = &clone
			continue
		}
		mergePathItem(existing, item, kind, path, conflicts)
	}
}

// mergePathItem merges the operations of src into dst, reporting a conflict
// when both define the same HTTP method.
func mergePathItem(dst, src *PathItem, kind, path string, conflicts *[]string) {
	ops := []struct {
		method string
		dst    **Operation
		src    *Operation
	}{
		{http.MethodGet, &dst.Get, src.Get},
		{http.MethodPut, &dst.Put, src.Put},
		{http.MethodPost, &dst.Post, src.Post},
		{http.MethodDelete, &dst.Delete, src.Delete},
		{http.MethodOptions, &dst.Options, src.Options},
		{http.MethodHead, &dst.Head, src.Head},
		{http.MethodPatch, &dst.Patch, src.Patch},
		{http.MethodTrace, &dst.Trace, src.Trace},
	}

	for _, o := range ops {
		if o.src == nil {
			continue
		}
		if *o.dst != nil {
			*conflicts = append(*conflicts, fmt.Sprintf("%s: duplicate %s %s", kind, o.method, path))
			continue
		}
		*o.dst = o.src
	}

	if dst.Summary == "" {
		dst.Summary = src.Summary
	}
	if dst.Description == "" {
		dst.Description = src.Description
	}
	dst.Servers = append(dst.Servers, src.Servers...)
	dst.Parameters = append(dst.Parameters, src.Parameters...)
}

// ingestComponents merges a document's components into the spec's component
// maps. Entries with the same name in two documents must be identical, or a
// conflict is reported.
func (s *Spec) ingestComponents(comp *Components, conflicts *[]string) {
	if comp == nil {
		return
	}

	if len(comp.SecuritySchemes) > 0 {
		if s.securitySchemes == nil {
			s.securitySchemes = make(map[string]*SecurityScheme)
		}
		ingestComponentMap("components.securitySchemes", comp.SecuritySchemes, s.securitySchemes, conflicts)
	}
	if len(comp.Responses) > 0 {
		if s.compResponses == nil {
			s.compResponses = make(map[string]*Response)
		}
		ingestComponentMap("components.responses", comp.Responses, s.compResponses, conflicts)
	}
	if len(comp.Parameters) > 0 {
		if s.compParameters == nil {
			s.compParameters = make(map[string]*Parameter)
		}
		ingestComponentMap("components.parameters", comp.Parameters, s.compParameters, conflicts)
	}
	if len(comp.Examples) > 0 {
		if s.compExamples == nil {
			s.compExamples = make(map[string]*Example)
		}
		ingestComponentMap("components.examples", comp.Examples, s.compExamples, conflicts)
	}
	if len(comp.RequestBodies) > 0 {
		if s.compReqBodies == nil {
			s.compReqBodies = make(map[string]*RequestBody)
		}
		ingestComponentMap("components.requestBodies", comp.RequestBodies, s.compReqBodies, conflicts)
	}
	if len(comp.Headers) > 0 {
		if s.compHeaders == nil {
			s.compHeaders = make(map[string]*Header)
		}
		ingestComponentMap("components.headers", comp.Headers, s.compHeaders, conflicts)
	}
	if len(comp.Links) > 0 {
		if s.compLinks == nil {
			s.compLinks = make(map[string]*Link)
		}
		ingestComponentMap("components.links", comp.Links, s.compLinks, conflicts)
	}
	if len(comp.Callbacks) > 0 {
		if s.compCallbacks == nil {
			s.compCallbacks = make(map[string]*Callback)
		}
		ingestComponentMap("components.callbacks", comp.Callbacks, s.compCallbacks, conflicts)
	}
	if len(comp.PathItems) > 0 {
		if s.compPathItems == nil {
			s.compPathItems = make(map[string]*PathItem)
		}
		ingestComponentMap("components.pathItems", comp.PathItems, s.compPathItems, conflicts)
	}
	if len(comp.Schemas) > 0 {
		if s.ingestedSchemas == nil {
			s.ingestedSchemas = make(map[string]*Schema)
		}
		ingestComponentMap("components.schemas", comp.Schemas, s.ingestedSchemas, conflicts)
	}
}

// ingestComponentMap copies src entries into dst. An entry whose name already
// exists in dst with different JSON content is reported as a conflict.
func ingestComponentMap[T any](kind string, src, dst map[string]T, conflicts *[]string) {
	for name, val := range src {
		if prev, ok := dst[name]; ok {
			if !jsonEqual(prev, val) {
				*conflicts = append(*conflicts, fmt.Sprintf("%s: duplicate %q with different definitions", kind, name))
			}
			continue
		}
		dst[name] = val
	}
}

// jsonEqual reports whether two values have identical normalized JSON.
func jsonEqual(a, b any) bool {
	aBytes, err := json.Marshal(a)
	if err != nil {
		return false
	}
	bBytes, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return string(normalizeJSONBytes(aBytes)) == string(normalizeJSONBytes(bBytes))
}

// AddServer adds a server to the spec.
//
// See: https://spec.openapis.org/oas/v3.1.0#openapi-object (servers)
func (s *Spec) AddServer(server Server) *Spec {
	s.servers = append(s.servers, server)
	return s
}

// AddPathServer adds a server override for a specific path. The path must use
// OpenAPI format (e.g., "/files", "/users/{id}"). All operations under this
// path inherit these servers, overriding the document-level servers.
//
// See: https://spec.openapis.org/oas/v3.1.0#path-item-object (servers)
func (s *Spec) AddPathServer(path string, server Server) *Spec {
	if s.pathServers == nil {
		s.pathServers = make(map[string][]Server)
	}
	s.pathServers[path] = append(s.pathServers[path], server)
	return s
}

// SetPathSummary sets a brief summary for a specific path. The path must use
// OpenAPI format (e.g., "/users/{id}"). The summary applies to all operations
// under this path.
//
// See: https://spec.openapis.org/oas/v3.1.0#path-item-object (summary)
func (s *Spec) SetPathSummary(path, summary string) *Spec {
	if s.pathSummaries == nil {
		s.pathSummaries = make(map[string]string)
	}
	s.pathSummaries[path] = summary
	return s
}

// SetPathDescription sets a detailed description for a specific path. The path
// must use OpenAPI format (e.g., "/users/{id}"). The description applies to all
// operations under this path and supports Markdown.
//
// See: https://spec.openapis.org/oas/v3.1.0#path-item-object (description)
func (s *Spec) SetPathDescription(path, description string) *Spec {
	if s.pathDescriptions == nil {
		s.pathDescriptions = make(map[string]string)
	}
	s.pathDescriptions[path] = description
	return s
}

// AddPathParameter adds a shared parameter for a specific path. The path must
// use OpenAPI format (e.g., "/users/{id}"). Path-level parameters apply to all
// operations under this path and can be overridden at the operation level.
//
// See: https://spec.openapis.org/oas/v3.1.0#path-item-object (parameters)
func (s *Spec) AddPathParameter(path string, param *Parameter) *Spec {
	if s.pathParameters == nil {
		s.pathParameters = make(map[string][]*Parameter)
	}
	s.pathParameters[path] = append(s.pathParameters[path], param)
	return s
}

// SetExternalDocs sets the document-level external documentation link.
//
// See: https://spec.openapis.org/oas/v3.1.0#openapi-object (externalDocs)
// See: https://spec.openapis.org/oas/v3.1.0#external-documentation-object
func (s *Spec) SetExternalDocs(url, description string) *Spec {
	s.externalDocs = &ExternalDocs{
		URL:         url,
		Description: description,
	}
	return s
}

// SetSecurity sets the document-level security requirements.
//
// See: https://spec.openapis.org/oas/v3.1.0#openapi-object (security)
// See: https://spec.openapis.org/oas/v3.1.0#security-requirement-object
func (s *Spec) SetSecurity(reqs ...SecurityRequirement) *Spec {
	s.security = reqs
	return s
}

// AddTag adds a user-defined tag with optional description and external docs.
//
// See: https://spec.openapis.org/oas/v3.1.0#tag-object
func (s *Spec) AddTag(tag Tag) *Spec {
	s.tags = append(s.tags, tag)
	return s
}

// AddSecurityScheme registers a reusable security scheme in components.
//
// See: https://spec.openapis.org/oas/v3.1.0#security-scheme-object
func (s *Spec) AddSecurityScheme(name string, scheme *SecurityScheme) *Spec {
	if s.securitySchemes == nil {
		s.securitySchemes = make(map[string]*SecurityScheme)
	}
	s.securitySchemes[name] = scheme
	return s
}

// AddComponentResponse registers a reusable response in components.
//
// See: https://spec.openapis.org/oas/v3.1.0#components-object (responses)
func (s *Spec) AddComponentResponse(name string, resp *Response) *Spec {
	if s.compResponses == nil {
		s.compResponses = make(map[string]*Response)
	}
	s.compResponses[name] = resp
	return s
}

// AddComponentParameter registers a reusable parameter in components.
//
// See: https://spec.openapis.org/oas/v3.1.0#components-object (parameters)
func (s *Spec) AddComponentParameter(name string, param *Parameter) *Spec {
	if s.compParameters == nil {
		s.compParameters = make(map[string]*Parameter)
	}
	s.compParameters[name] = param
	return s
}

// AddComponentExample registers a reusable example in components.
//
// See: https://spec.openapis.org/oas/v3.1.0#components-object (examples)
func (s *Spec) AddComponentExample(name string, ex *Example) *Spec {
	if s.compExamples == nil {
		s.compExamples = make(map[string]*Example)
	}
	s.compExamples[name] = ex
	return s
}

// AddComponentRequestBody registers a reusable request body in components.
//
// See: https://spec.openapis.org/oas/v3.1.0#components-object (requestBodies)
func (s *Spec) AddComponentRequestBody(name string, rb *RequestBody) *Spec {
	if s.compReqBodies == nil {
		s.compReqBodies = make(map[string]*RequestBody)
	}
	s.compReqBodies[name] = rb
	return s
}

// AddComponentHeader registers a reusable header in components.
//
// See: https://spec.openapis.org/oas/v3.1.0#components-object (headers)
func (s *Spec) AddComponentHeader(name string, h *Header) *Spec {
	if s.compHeaders == nil {
		s.compHeaders = make(map[string]*Header)
	}
	s.compHeaders[name] = h
	return s
}

// AddComponentLink registers a reusable link in components.
//
// See: https://spec.openapis.org/oas/v3.1.0#components-object (links)
func (s *Spec) AddComponentLink(name string, l *Link) *Spec {
	if s.compLinks == nil {
		s.compLinks = make(map[string]*Link)
	}
	s.compLinks[name] = l
	return s
}

// AddComponentCallback registers a reusable callback in components.
//
// See: https://spec.openapis.org/oas/v3.1.0#components-object (callbacks)
func (s *Spec) AddComponentCallback(name string, cb *Callback) *Spec {
	if s.compCallbacks == nil {
		s.compCallbacks = make(map[string]*Callback)
	}
	s.compCallbacks[name] = cb
	return s
}

// AddComponentPathItem registers a reusable path item in components.
//
// See: https://spec.openapis.org/oas/v3.1.0#components-object (pathItems)
func (s *Spec) AddComponentPathItem(name string, pi *PathItem) *Spec {
	if s.compPathItems == nil {
		s.compPathItems = make(map[string]*PathItem)
	}
	s.compPathItems[name] = pi
	return s
}

// Webhook registers an OpenAPI webhook with the given name and HTTP method.
// Webhooks describe API-initiated callbacks that are not tied to a specific
// path on the mux router. The returned OperationBuilder has the same fluent
// API as Route and Op.
//
// See: https://spec.openapis.org/oas/v3.1.0#openapi-object (webhooks)
func (s *Spec) Webhook(name, method string) *OperationBuilder {
	if s.webhooks == nil {
		s.webhooks = make(map[string]map[string]*OperationBuilder)
	}
	if s.webhooks[name] == nil {
		s.webhooks[name] = make(map[string]*OperationBuilder)
	}
	b := newOperationBuilder()
	s.webhooks[name][method] = b
	return b
}

// Group creates a new RouteGroup for applying shared OpenAPI metadata defaults
// to a logical group of operations. The returned group provides the same Route
// and Op methods as Spec, but pre-populates each OperationBuilder with the
// group's default tags, security, servers, parameters, and external docs.
//
// See: https://spec.openapis.org/oas/v3.1.0#operation-object
func (s *Spec) Group() *RouteGroup {
	return &RouteGroup{spec: s}
}

// Op returns an OperationBuilder for the named route.
// If the route name was not previously registered, a new builder is created.
//
// See: https://spec.openapis.org/oas/v3.1.0#operation-object (operationId)
func (s *Spec) Op(routeName string) *OperationBuilder {
	if b, ok := s.operations[routeName]; ok {
		return b
	}
	b := newOperationBuilder()
	s.operations[routeName] = b
	return b
}

// Route attaches an OperationBuilder to an existing mux route.
// The route can be configured with any mux features (Methods, Headers, Queries, etc.).
//
// See: https://spec.openapis.org/oas/v3.1.0#path-item-object
func (s *Spec) Route(route *mux.Route) *OperationBuilder {
	b := newOperationBuilder()
	s.routeOps[route] = b
	return b
}

// Build walks the router and assembles a complete OpenAPI Document.
//
// See: https://spec.openapis.org/oas/v3.1.0#openapi-object
func (s *Spec) Build(r *mux.Router) *Document {
	gen := NewSchemaGenerator()
	doc := &Document{
		OpenAPI:      OpenAPIVersion,
		Info:         s.info,
		Servers:      s.servers,
		Paths:        make(map[string]*PathItem),
		ExternalDocs: s.externalDocs,
		Security:     s.security,
	}

	// Seed paths from ingested Documents so route-derived operations merge
	// on top of them per-method.
	for path, item := range s.ingestedPaths {
		clone := *item
		doc.Paths[path] = &clone
	}

	_ = r.Walk(func(route *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		// Skip build-only routes: they are used only for URL building
		// and should not appear in the generated spec.
		if route.IsBuildOnly() {
			return nil
		}

		pathTpl, err := route.GetPathTemplate()
		if err != nil {
			return nil
		}

		methods, err := route.GetMethods()
		if err != nil {
			return nil
		}

		// Look up builder: first by route pointer, then by route name.
		builder, hasOp := s.routeOps[route]
		if !hasOp {
			name := route.GetName()
			builder, hasOp = s.operations[name]
			if !hasOp {
				return nil
			}
		}

		// Parse path variables and convert to OpenAPI path.
		openAPIPath, pathParams := parsePath(pathTpl)

		// Auto-generate header parameters from route header matchers.
		if headers, err := route.GetHeaders(); err == nil {
			for name, value := range headers {
				p := &Parameter{
					Name:     name,
					In:       ParameterInHeader,
					Required: true,
					Schema:   &Schema{Type: SchemaTypeString},
				}
				if value != "" {
					p.Schema.Enum = []any{value}
				}
				pathParams = append(pathParams, p)
			}
		}

		// Get or create PathItem.
		pathItem, ok := doc.Paths[openAPIPath]
		if !ok {
			pathItem = &PathItem{}
			doc.Paths[openAPIPath] = pathItem
		}

		// Collect route-level schemes for auto-generating operation servers.
		schemes, _ := route.GetSchemes()

		// Build one operation per method. When a route registers multiple
		// methods, each gets a distinct operationId to satisfy the OpenAPI
		// uniqueness requirement.
		//
		// See: https://spec.openapis.org/oas/v3.1.0#operation-object (operationId)
		baseID := route.GetName()
		for _, method := range methods {
			opID := baseID
			if len(methods) > 1 && opID != "" {
				opID = fmt.Sprintf("%s%s%s", opID, strings.ToUpper(method[:1]), strings.ToLower(method[1:]))
			}
			op := builder.buildOperation(gen, opID, pathParams)

			// Auto-populate operation servers from route scheme constraints
			// only when no servers are configured at any level (operation,
			// path, or document). Operation-level servers in OpenAPI override
			// path and document servers, so adding incomplete scheme-only
			// URLs would discard configured base URLs.
			if len(schemes) > 0 && len(builder.meta.servers) == 0 &&
				len(s.pathServers[openAPIPath]) == 0 && len(s.servers) == 0 {
				for _, scheme := range schemes {
					op.Servers = append(op.Servers, Server{URL: fmt.Sprintf("%s://", scheme)})
				}
			}

			assignOperation(pathItem, method, op)
		}

		return nil
	})

	// Build webhooks, seeding from ingested Documents.
	if len(s.webhooks) > 0 || len(s.ingestedWebhooks) > 0 {
		doc.Webhooks = make(map[string]*PathItem, len(s.webhooks)+len(s.ingestedWebhooks))
		for name, item := range s.ingestedWebhooks {
			clone := *item
			doc.Webhooks[name] = &clone
		}
		for name, methods := range s.webhooks {
			pathItem, ok := doc.Webhooks[name]
			if !ok {
				pathItem = &PathItem{}
				doc.Webhooks[name] = pathItem
			}
			for method, builder := range methods {
				op := builder.buildOperation(gen, "", nil)
				assignOperation(pathItem, method, op)
			}
		}
	}

	// Apply path-level metadata.
	for path, summary := range s.pathSummaries {
		if pathItem, ok := doc.Paths[path]; ok {
			pathItem.Summary = summary
		}
	}
	for path, description := range s.pathDescriptions {
		if pathItem, ok := doc.Paths[path]; ok {
			pathItem.Description = description
		}
	}
	for path, servers := range s.pathServers {
		if pathItem, ok := doc.Paths[path]; ok {
			pathItem.Servers = append(pathItem.Servers, servers...)
		}
	}
	for path, params := range s.pathParameters {
		if pathItem, ok := doc.Paths[path]; ok {
			pathItem.Parameters = append(pathItem.Parameters, params...)
		}
	}

	// Build components.
	doc.Components = s.buildComponents(gen)

	// Merge tags: user-defined tags take precedence over auto-collected.
	doc.Tags = s.mergeTags(doc.Paths, doc.Webhooks)

	return doc
}

// buildComponents assembles the Components Object from generated schemas
// and all user-registered component maps.
//
// See: https://spec.openapis.org/oas/v3.1.0#components-object
func (s *Spec) buildComponents(gen *SchemaGenerator) *Components {
	schemas := gen.Schemas()

	// Merge schemas ingested from Documents. Reflection-generated schemas
	// take precedence on name collision (the local route is authoritative).
	if len(s.ingestedSchemas) > 0 {
		if schemas == nil {
			schemas = make(map[string]*Schema, len(s.ingestedSchemas))
		}
		for name, schema := range s.ingestedSchemas {
			if _, exists := schemas[name]; !exists {
				schemas[name] = schema
			}
		}
	}

	hasData := len(schemas) > 0 ||
		len(s.securitySchemes) > 0 ||
		len(s.compResponses) > 0 ||
		len(s.compParameters) > 0 ||
		len(s.compExamples) > 0 ||
		len(s.compReqBodies) > 0 ||
		len(s.compHeaders) > 0 ||
		len(s.compLinks) > 0 ||
		len(s.compCallbacks) > 0 ||
		len(s.compPathItems) > 0

	if !hasData {
		return nil
	}

	comp := &Components{}
	if len(schemas) > 0 {
		comp.Schemas = schemas
	}
	if len(s.securitySchemes) > 0 {
		comp.SecuritySchemes = s.securitySchemes
	}
	if len(s.compResponses) > 0 {
		comp.Responses = s.compResponses
	}
	if len(s.compParameters) > 0 {
		comp.Parameters = s.compParameters
	}
	if len(s.compExamples) > 0 {
		comp.Examples = s.compExamples
	}
	if len(s.compReqBodies) > 0 {
		comp.RequestBodies = s.compReqBodies
	}
	if len(s.compHeaders) > 0 {
		comp.Headers = s.compHeaders
	}
	if len(s.compLinks) > 0 {
		comp.Links = s.compLinks
	}
	if len(s.compCallbacks) > 0 {
		comp.Callbacks = s.compCallbacks
	}
	if len(s.compPathItems) > 0 {
		comp.PathItems = s.compPathItems
	}

	return comp
}

// mergeTags combines auto-collected tags from operations with user-defined tags.
// User-defined tags take precedence (their description and externalDocs are kept).
// Tags not seen in operations but defined by the user are still included.
// The result is sorted alphabetically.
//
// See: https://spec.openapis.org/oas/v3.1.0#openapi-object (tags)
// See: https://spec.openapis.org/oas/v3.1.0#tag-object
func (s *Spec) mergeTags(pathMaps ...map[string]*PathItem) []Tag {
	// Build a map of user-defined tags for quick lookup.
	userTags := make(map[string]Tag, len(s.tags))
	for _, tag := range s.tags {
		userTags[tag.Name] = tag
	}

	// Collect tags from operations across all path maps (paths + webhooks).
	seen := make(map[string]bool)
	var tags []Tag

	for _, paths := range pathMaps {
		for _, pathItem := range paths {
			for _, op := range []*Operation{
				pathItem.Get, pathItem.Post, pathItem.Put,
				pathItem.Delete, pathItem.Patch, pathItem.Head,
				pathItem.Options, pathItem.Trace,
			} {
				if op == nil {
					continue
				}
				for _, tagName := range op.Tags {
					if seen[tagName] {
						continue
					}
					seen[tagName] = true
					if userTag, ok := userTags[tagName]; ok {
						tags = append(tags, userTag)
					} else {
						tags = append(tags, Tag{Name: tagName})
					}
				}
			}
		}
	}

	// Add user-defined tags not seen in operations.
	for _, tag := range s.tags {
		if !seen[tag.Name] {
			seen[tag.Name] = true
			tags = append(tags, tag)
		}
	}

	sort.Slice(tags, func(i, j int) bool {
		return tags[i].Name < tags[j].Name
	})

	return tags
}

// assignOperation assigns an operation to the correct HTTP method field
// on the path item.
//
// See: https://spec.openapis.org/oas/v3.1.0#path-item-object
func assignOperation(pathItem *PathItem, method string, op *Operation) {
	switch method {
	case http.MethodGet:
		pathItem.Get = op
	case http.MethodPost:
		pathItem.Post = op
	case http.MethodPut:
		pathItem.Put = op
	case http.MethodDelete:
		pathItem.Delete = op
	case http.MethodPatch:
		pathItem.Patch = op
	case http.MethodHead:
		pathItem.Head = op
	case http.MethodOptions:
		pathItem.Options = op
	case http.MethodTrace:
		pathItem.Trace = op
	}
}

// parsePath extracts variables from a mux path template, converts it to
// OpenAPI format, and generates parameter objects.
//
// See: https://spec.openapis.org/oas/v3.1.0#paths-object
// See: https://spec.openapis.org/oas/v3.1.0#parameter-object
func parsePath(tpl string) (string, []*Parameter) {
	var params []*Parameter

	openAPIPath := pathVarRegexp.ReplaceAllStringFunc(tpl, func(match string) string {
		inner := match[1 : len(match)-1]
		varName, macroName, _ := strings.Cut(inner, ":")

		param := &Parameter{
			Name:     varName,
			In:       ParameterInPath,
			Required: true,
			Schema:   &Schema{Type: SchemaTypeString},
		}

		if macroName != "" {
			if typeInfo, ok := macroTypeMap[macroName]; ok {
				param.Schema = &Schema{Type: TypeString(typeInfo[0])}
				if typeInfo[1] != "" {
					param.Schema.Format = typeInfo[1]
				}
			}
		}

		params = append(params, param)
		return fmt.Sprintf("{%s}", varName)
	})

	return openAPIPath, params
}
