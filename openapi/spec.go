package openapi

import (
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/vitalvas/kasper/mux"
)

// macroTypeMap maps mux route macros to OpenAPI type and format.
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
func NewSpec(info Info) *Spec {
	return &Spec{
		info:       info,
		operations: make(map[string]*OperationBuilder),
		routeOps:   make(map[*mux.Route]*OperationBuilder),
	}
}

// AddServer adds a server to the spec.
func (s *Spec) AddServer(server Server) *Spec {
	s.servers = append(s.servers, server)
	return s
}

// AddPathServer adds a server override for a specific path. The path must use
// OpenAPI format (e.g., "/files", "/users/{id}"). All operations under this
// path inherit these servers, overriding the document-level servers.
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
func (s *Spec) AddPathParameter(path string, param *Parameter) *Spec {
	if s.pathParameters == nil {
		s.pathParameters = make(map[string][]*Parameter)
	}
	s.pathParameters[path] = append(s.pathParameters[path], param)
	return s
}

// SetExternalDocs sets the document-level external documentation link.
func (s *Spec) SetExternalDocs(url, description string) *Spec {
	s.externalDocs = &ExternalDocs{URL: url, Description: description}
	return s
}

// SetSecurity sets the document-level security requirements.
func (s *Spec) SetSecurity(reqs ...SecurityRequirement) *Spec {
	s.security = reqs
	return s
}

// AddTag adds a user-defined tag with optional description and external docs.
func (s *Spec) AddTag(tag Tag) *Spec {
	s.tags = append(s.tags, tag)
	return s
}

// AddSecurityScheme registers a reusable security scheme in components.
func (s *Spec) AddSecurityScheme(name string, scheme *SecurityScheme) *Spec {
	if s.securitySchemes == nil {
		s.securitySchemes = make(map[string]*SecurityScheme)
	}
	s.securitySchemes[name] = scheme
	return s
}

// AddComponentResponse registers a reusable response in components.
func (s *Spec) AddComponentResponse(name string, resp *Response) *Spec {
	if s.compResponses == nil {
		s.compResponses = make(map[string]*Response)
	}
	s.compResponses[name] = resp
	return s
}

// AddComponentParameter registers a reusable parameter in components.
func (s *Spec) AddComponentParameter(name string, param *Parameter) *Spec {
	if s.compParameters == nil {
		s.compParameters = make(map[string]*Parameter)
	}
	s.compParameters[name] = param
	return s
}

// AddComponentExample registers a reusable example in components.
func (s *Spec) AddComponentExample(name string, ex *Example) *Spec {
	if s.compExamples == nil {
		s.compExamples = make(map[string]*Example)
	}
	s.compExamples[name] = ex
	return s
}

// AddComponentRequestBody registers a reusable request body in components.
func (s *Spec) AddComponentRequestBody(name string, rb *RequestBody) *Spec {
	if s.compReqBodies == nil {
		s.compReqBodies = make(map[string]*RequestBody)
	}
	s.compReqBodies[name] = rb
	return s
}

// AddComponentHeader registers a reusable header in components.
func (s *Spec) AddComponentHeader(name string, h *Header) *Spec {
	if s.compHeaders == nil {
		s.compHeaders = make(map[string]*Header)
	}
	s.compHeaders[name] = h
	return s
}

// AddComponentLink registers a reusable link in components.
func (s *Spec) AddComponentLink(name string, l *Link) *Spec {
	if s.compLinks == nil {
		s.compLinks = make(map[string]*Link)
	}
	s.compLinks[name] = l
	return s
}

// AddComponentCallback registers a reusable callback in components.
func (s *Spec) AddComponentCallback(name string, cb *Callback) *Spec {
	if s.compCallbacks == nil {
		s.compCallbacks = make(map[string]*Callback)
	}
	s.compCallbacks[name] = cb
	return s
}

// AddComponentPathItem registers a reusable path item in components.
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
func (s *Spec) Group() *RouteGroup {
	return &RouteGroup{spec: s}
}

// Op returns an OperationBuilder for the named route.
// If the route name was not previously registered, a new builder is created.
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
func (s *Spec) Route(route *mux.Route) *OperationBuilder {
	b := newOperationBuilder()
	s.routeOps[route] = b
	return b
}

// Build walks the router and assembles a complete OpenAPI Document.
func (s *Spec) Build(r *mux.Router) *Document {
	gen := NewSchemaGenerator()
	doc := &Document{
		OpenAPI:      "3.1.0",
		Info:         s.info,
		Servers:      s.servers,
		Paths:        make(map[string]*PathItem),
		ExternalDocs: s.externalDocs,
		Security:     s.security,
	}

	_ = r.Walk(func(route *mux.Route, _ *mux.Router, _ []*mux.Route) error {
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

		// Get or create PathItem.
		pathItem, ok := doc.Paths[openAPIPath]
		if !ok {
			pathItem = &PathItem{}
			doc.Paths[openAPIPath] = pathItem
		}

		// Build the operation with route name as operation ID.
		operationID := route.GetName()
		op := builder.buildOperation(gen, operationID, pathParams)

		// Assign to correct HTTP method.
		for _, method := range methods {
			assignOperation(pathItem, method, op)
		}

		return nil
	})

	// Build webhooks.
	if len(s.webhooks) > 0 {
		doc.Webhooks = make(map[string]*PathItem, len(s.webhooks))
		for name, methods := range s.webhooks {
			pathItem := &PathItem{}
			for method, builder := range methods {
				op := builder.buildOperation(gen, "", nil)
				assignOperation(pathItem, method, op)
			}
			doc.Webhooks[name] = pathItem
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

// buildComponents assembles the Components object from generated schemas
// and all user-registered component maps.
func (s *Spec) buildComponents(gen *SchemaGenerator) *Components {
	schemas := gen.Schemas()

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
func parsePath(tpl string) (string, []*Parameter) {
	var params []*Parameter

	openAPIPath := pathVarRegexp.ReplaceAllStringFunc(tpl, func(match string) string {
		inner := match[1 : len(match)-1]
		varName, macroName, _ := strings.Cut(inner, ":")

		param := &Parameter{
			Name:     varName,
			In:       "path",
			Required: true,
			Schema:   &Schema{Type: TypeString("string")},
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
		return "{" + varName + "}"
	})

	return openAPIPath, params
}
