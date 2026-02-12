package openapi

import (
	"strconv"

	"github.com/vitalvas/kasper/mux"
)

// groupDefaults holds the default metadata that a RouteGroup applies
// to every OperationBuilder it creates.
type groupDefaults struct {
	tags         []string
	security     []SecurityRequirement
	securitySet  bool // distinguishes nil (inherit) from empty (public)
	deprecated   bool
	servers      []Server
	parameters   []*Parameter
	externalDocs *ExternalDocs

	responseContents     map[string]map[string]any     // statusKey -> contentType -> body
	responseDescriptions map[string]string             // statusKey -> custom description
	responseHeaders      map[string]map[string]*Header // statusKey -> headerName -> header
	responseLinks        map[string]map[string]*Link   // statusKey -> linkName -> link
}

// RouteGroup provides shared OpenAPI metadata defaults for a logical group
// of operations. It creates OperationBuilder instances pre-populated with
// the group defaults. Groups register builders into the parent Spec's
// existing maps so Build requires no changes.
type RouteGroup struct {
	spec     *Spec
	defaults groupDefaults
}

// Tags appends tags to the group defaults. Operations created through
// this group will inherit these tags and may add more via their own Tags call.
func (g *RouteGroup) Tags(tags ...string) *RouteGroup {
	g.defaults.tags = append(g.defaults.tags, tags...)
	return g
}

// Security sets the group-level security requirements. Operations created
// through this group inherit these requirements unless they call Security
// themselves, which replaces the group value. Call with no arguments to
// mark the group as public (overrides document-level security).
func (g *RouteGroup) Security(reqs ...SecurityRequirement) *RouteGroup {
	if reqs == nil {
		reqs = []SecurityRequirement{}
	}
	g.defaults.security = reqs
	g.defaults.securitySet = true
	return g
}

// Deprecated marks all operations in this group as deprecated. This is a
// one-way latch: individual operations cannot undo group deprecation.
func (g *RouteGroup) Deprecated() *RouteGroup {
	g.defaults.deprecated = true
	return g
}

// Server adds a server override to the group defaults.
func (g *RouteGroup) Server(server Server) *RouteGroup {
	g.defaults.servers = append(g.defaults.servers, server)
	return g
}

// Parameter adds a common parameter to the group defaults. Operations
// created through this group inherit these parameters and may add more.
func (g *RouteGroup) Parameter(param *Parameter) *RouteGroup {
	g.defaults.parameters = append(g.defaults.parameters, param)
	return g
}

// ExternalDocs sets external documentation for the group. Operations
// created through this group inherit this value unless they call
// ExternalDocs themselves, which replaces it.
func (g *RouteGroup) ExternalDocs(url, description string) *RouteGroup {
	g.defaults.externalDocs = &ExternalDocs{URL: url, Description: description}
	return g
}

// Response adds a shared application/json response for the given HTTP status
// code. All operations created through this group inherit this response.
// An operation-level Response call for the same status code overrides the
// group default.
func (g *RouteGroup) Response(statusCode int, body any) *RouteGroup {
	key := strconv.Itoa(statusCode)
	if g.defaults.responseContents == nil {
		g.defaults.responseContents = make(map[string]map[string]any)
	}
	if body != nil {
		if g.defaults.responseContents[key] == nil {
			g.defaults.responseContents[key] = make(map[string]any)
		}
		g.defaults.responseContents[key]["application/json"] = body
	} else if g.defaults.responseContents[key] == nil {
		g.defaults.responseContents[key] = nil
	}
	return g
}

// ResponseContent adds a shared response with the given status code and content
// type. Use this for non-JSON responses (e.g., "application/xml").
func (g *RouteGroup) ResponseContent(statusCode int, contentType string, body any) *RouteGroup {
	key := strconv.Itoa(statusCode)
	if g.defaults.responseContents == nil {
		g.defaults.responseContents = make(map[string]map[string]any)
	}
	if g.defaults.responseContents[key] == nil {
		g.defaults.responseContents[key] = make(map[string]any)
	}
	g.defaults.responseContents[key][contentType] = body
	return g
}

// ResponseDescription sets a custom description for a shared group response.
func (g *RouteGroup) ResponseDescription(statusCode int, desc string) *RouteGroup {
	key := strconv.Itoa(statusCode)
	if g.defaults.responseDescriptions == nil {
		g.defaults.responseDescriptions = make(map[string]string)
	}
	g.defaults.responseDescriptions[key] = desc
	return g
}

// ResponseHeader adds a shared header to the response for the given HTTP
// status code. All operations in this group inherit this header.
func (g *RouteGroup) ResponseHeader(statusCode int, name string, h *Header) *RouteGroup {
	key := strconv.Itoa(statusCode)
	if g.defaults.responseHeaders == nil {
		g.defaults.responseHeaders = make(map[string]map[string]*Header)
	}
	if g.defaults.responseHeaders[key] == nil {
		g.defaults.responseHeaders[key] = make(map[string]*Header)
	}
	g.defaults.responseHeaders[key][name] = h
	return g
}

// ResponseLink adds a shared link to the response for the given HTTP status
// code. All operations in this group inherit this link.
func (g *RouteGroup) ResponseLink(statusCode int, name string, l *Link) *RouteGroup {
	key := strconv.Itoa(statusCode)
	if g.defaults.responseLinks == nil {
		g.defaults.responseLinks = make(map[string]map[string]*Link)
	}
	if g.defaults.responseLinks[key] == nil {
		g.defaults.responseLinks[key] = make(map[string]*Link)
	}
	g.defaults.responseLinks[key][name] = l
	return g
}

// DefaultResponse adds a shared application/json default response (catch-all
// for status codes not covered by specific responses).
func (g *RouteGroup) DefaultResponse(body any) *RouteGroup {
	if g.defaults.responseContents == nil {
		g.defaults.responseContents = make(map[string]map[string]any)
	}
	if body != nil {
		if g.defaults.responseContents["default"] == nil {
			g.defaults.responseContents["default"] = make(map[string]any)
		}
		g.defaults.responseContents["default"]["application/json"] = body
	} else if g.defaults.responseContents["default"] == nil {
		g.defaults.responseContents["default"] = nil
	}
	return g
}

// DefaultResponseDescription sets a custom description for the shared default
// response.
func (g *RouteGroup) DefaultResponseDescription(desc string) *RouteGroup {
	if g.defaults.responseDescriptions == nil {
		g.defaults.responseDescriptions = make(map[string]string)
	}
	g.defaults.responseDescriptions["default"] = desc
	return g
}

// DefaultResponseHeader adds a shared header to the default response.
func (g *RouteGroup) DefaultResponseHeader(name string, h *Header) *RouteGroup {
	if g.defaults.responseHeaders == nil {
		g.defaults.responseHeaders = make(map[string]map[string]*Header)
	}
	if g.defaults.responseHeaders["default"] == nil {
		g.defaults.responseHeaders["default"] = make(map[string]*Header)
	}
	g.defaults.responseHeaders["default"][name] = h
	return g
}

// Route attaches an OperationBuilder to an existing mux route, pre-populated
// with this group's defaults.
func (g *RouteGroup) Route(route *mux.Route) *OperationBuilder {
	b := g.newBuilderWithDefaults()
	g.spec.routeOps[route] = b
	return b
}

// Webhook registers an OpenAPI webhook with the given name and HTTP method,
// pre-populated with this group's defaults.
func (g *RouteGroup) Webhook(name, method string) *OperationBuilder {
	if g.spec.webhooks == nil {
		g.spec.webhooks = make(map[string]map[string]*OperationBuilder)
	}
	if g.spec.webhooks[name] == nil {
		g.spec.webhooks[name] = make(map[string]*OperationBuilder)
	}
	b := g.newBuilderWithDefaults()
	g.spec.webhooks[name][method] = b
	return b
}

// Op returns an OperationBuilder for the named route, pre-populated with
// this group's defaults. If the route name was previously registered,
// the existing builder is returned (without applying group defaults again).
func (g *RouteGroup) Op(routeName string) *OperationBuilder {
	if b, ok := g.spec.operations[routeName]; ok {
		return b
	}
	b := g.newBuilderWithDefaults()
	g.spec.operations[routeName] = b
	return b
}

// newBuilderWithDefaults creates a new OperationBuilder pre-populated with
// the group's default values.
func (g *RouteGroup) newBuilderWithDefaults() *OperationBuilder {
	b := newOperationBuilder()

	if len(g.defaults.tags) > 0 {
		b.meta.tags = append(b.meta.tags, g.defaults.tags...)
	}

	if g.defaults.securitySet {
		b.meta.security = g.defaults.security
	}

	if g.defaults.deprecated {
		b.meta.deprecated = true
	}

	if len(g.defaults.servers) > 0 {
		b.meta.servers = append(b.meta.servers, g.defaults.servers...)
	}

	if len(g.defaults.parameters) > 0 {
		b.meta.parameters = append(b.meta.parameters, g.defaults.parameters...)
	}

	if g.defaults.externalDocs != nil {
		b.meta.externalDocs = g.defaults.externalDocs
	}

	for key, contents := range g.defaults.responseContents {
		if contents != nil {
			if b.meta.responseContents[key] == nil {
				b.meta.responseContents[key] = make(map[string]any)
			}
			for ct, body := range contents {
				b.meta.responseContents[key][ct] = body
			}
		} else if b.meta.responseContents[key] == nil {
			b.meta.responseContents[key] = nil
		}
	}

	if len(g.defaults.responseDescriptions) > 0 {
		if b.meta.responseDescriptions == nil {
			b.meta.responseDescriptions = make(map[string]string)
		}
		for key, desc := range g.defaults.responseDescriptions {
			b.meta.responseDescriptions[key] = desc
		}
	}

	for key, headers := range g.defaults.responseHeaders {
		if b.meta.responseHeaders == nil {
			b.meta.responseHeaders = make(map[string]map[string]*Header)
		}
		if b.meta.responseHeaders[key] == nil {
			b.meta.responseHeaders[key] = make(map[string]*Header)
		}
		for name, h := range headers {
			b.meta.responseHeaders[key][name] = h
		}
	}

	for key, links := range g.defaults.responseLinks {
		if b.meta.responseLinks == nil {
			b.meta.responseLinks = make(map[string]map[string]*Link)
		}
		if b.meta.responseLinks[key] == nil {
			b.meta.responseLinks[key] = make(map[string]*Link)
		}
		for name, l := range links {
			b.meta.responseLinks[key][name] = l
		}
	}

	return b
}
