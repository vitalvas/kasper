// Package openapi provides automatic OpenAPI v3.1.0 specification generation
// from mux router routes using Go reflection and struct tags.
//
// The package targets the OpenAPI Specification v3.1.0 and uses JSON Schema
// Draft 2020-12 for schema generation. It produces a complete OpenAPI document
// from registered routes with zero external schema files.
//
// See: https://spec.openapis.org/oas/v3.1.0
// See: https://json-schema.org/draft/2020-12/json-schema-core
// See: https://json-schema.org/draft/2020-12/json-schema-validation
//
// # Spec Builder
//
// Create a spec, attach metadata to routes, and build the document:
//
//	spec := openapi.NewSpec(openapi.Info{Title: "My API", Version: "1.0.0"})
//
// # Variant B: Attach Metadata to Named Routes
//
// Use Op to annotate existing named routes:
//
//	r := mux.NewRouter()
//	r.HandleFunc("/users", listUsers).Methods(http.MethodGet).Name("listUsers")
//	r.HandleFunc("/users", createUser).Methods(http.MethodPost).Name("createUser")
//
//	spec.Op("listUsers").
//	    Summary("List all users").
//	    Tags("users").
//	    Response(http.StatusOK, []User{})
//
//	spec.Op("createUser").
//	    Summary("Create a user").
//	    Tags("users").
//	    Request(CreateUserInput{}).
//	    Response(http.StatusCreated, User{})
//
// # Route: Attach Metadata to Any Mux Route
//
// Use Route to attach OpenAPI metadata to an already-configured mux route,
// preserving full mux flexibility (Methods, Headers, Queries, Schemes, etc.):
//
//	spec.Route(r.HandleFunc("/users", createUser).Methods(http.MethodPost)).
//	    Summary("Create a user").
//	    Tags("users").
//	    Request(CreateUserInput{}).
//	    Response(http.StatusCreated, User{})
//
// Routes can use any mux.Route configuration:
//
//	spec.Route(r.HandleFunc("/users", listUsers).
//	    Methods(http.MethodGet).
//	    Headers("Accept", "application/json")).
//	    Summary("List users").
//	    Response(http.StatusOK, []User{})
//
// # Route Groups
//
// Use Group to apply shared OpenAPI metadata defaults to a logical group
// of operations. Groups are a metadata concept only -- they do not affect
// routing. Routes created through a group inherit the group's tags, security,
// servers, parameters, responses, and external docs.
//
//	users := spec.Group().
//	    Tags("users").
//	    Security(openapi.SecurityRequirement{"basic": {}}).
//	    Response(http.StatusForbidden, ErrorResponse{}).
//	    Response(http.StatusNotFound, ErrorResponse{})
//
//	users.Route(r.HandleFunc("/users", listUsers).Methods(http.MethodGet)).
//	    Summary("List users").
//	    Response(http.StatusOK, []User{})
//
//	users.Route(r.HandleFunc("/users", createUser).Methods(http.MethodPost)).
//	    Summary("Create user").
//	    Request(CreateUserInput{}).
//	    Response(http.StatusCreated, User{})
//
// Both routes above automatically include 403 and 404 responses from the group.
//
// Groups support the full response builder API: Response, ResponseContent,
// ResponseDescription, ResponseHeader, ResponseLink, DefaultResponse,
// DefaultResponseDescription, and DefaultResponseHeader. For example:
//
//	spec.Group().
//	    Response(http.StatusForbidden, ErrorResponse{}).
//	    ResponseContent(http.StatusForbidden, "application/xml", ErrorResponse{}).
//	    ResponseDescription(http.StatusForbidden, "Insufficient permissions").
//	    DefaultResponse(ErrorResponse{}).
//	    DefaultResponseHeader("X-Request-ID", &openapi.Header{
//	        Schema: &openapi.Schema{Type: openapi.TypeString("string")},
//	    })
//
// Override/merge semantics per field:
//
//   - Tags: append (group tags + operation tags combined)
//   - Security: replace (operation-level Security call overrides group value)
//   - Deprecated: one-way latch (group deprecation cannot be undone per-operation)
//   - Servers: append (group servers + operation servers combined)
//   - Parameters: append (group parameters + operation parameters combined)
//   - Responses: merge (group responses + operation responses; operation overrides per status code)
//   - ExternalDocs: replace (operation-level ExternalDocs call overrides group value)
//
// Groups also support Op for named routes:
//
//	users.Op("listUsers").Summary("List users")
//
// # Security
//
// Register security schemes and apply them at document or operation level:
//
//	spec.AddSecurityScheme("bearerAuth", &openapi.SecurityScheme{
//	    Type:         "http",
//	    Scheme:       "bearer",
//	    BearerFormat: "JWT",
//	})
//	spec.SetSecurity(openapi.SecurityRequirement{"bearerAuth": {}})
//
// Override security per operation (empty Security() marks an endpoint as public):
//
//	spec.Route(r.HandleFunc("/health", healthHandler).Methods(http.MethodGet)).
//	    Summary("Health check").
//	    Security()
//
// # External Documentation
//
// Attach external docs at the document level:
//
//	spec.SetExternalDocs("https://docs.example.com", "Full documentation")
//
// Or at the operation level:
//
//	spec.Op("listUsers").ExternalDocs("https://docs.example.com/users", "User API docs")
//
// # Tags
//
// Tags used in operations are automatically collected into the document-level
// tags list, sorted alphabetically. Use AddTag to provide descriptions and
// external documentation for tags:
//
//	spec.AddTag(openapi.Tag{
//	    Name:         "users",
//	    Description:  "User management operations",
//	    ExternalDocs: &openapi.ExternalDocs{URL: "https://docs.example.com/users"},
//	})
//
// User-defined tags take precedence over auto-collected tags. Tags defined
// via AddTag but not used by any operation are still included in the output.
//
// # Reusable Components
//
// Register reusable objects in components:
//
//	spec.AddComponentResponse("NotFound", &openapi.Response{Description: "Not found"})
//	spec.AddComponentParameter("pageParam", &openapi.Parameter{Name: "page", In: "query"})
//	spec.AddComponentExample("sample", &openapi.Example{Summary: "A sample", Value: "test"})
//	spec.AddComponentRequestBody("CreatePet", &openapi.RequestBody{Description: "Pet to create"})
//	spec.AddComponentHeader("X-Rate-Limit", &openapi.Header{Schema: &openapi.Schema{Type: openapi.TypeString("integer")}})
//	spec.AddComponentLink("GetUser", &openapi.Link{OperationID: "getUser"})
//
// # Path-Level Metadata
//
// Set summary, description, and shared parameters on a path. These apply to
// all operations under the path:
//
//	spec.SetPathSummary("/users/{id}", "Represents a user")
//	spec.SetPathDescription("/users/{id}", "Individual user identified by ID.")
//	spec.AddPathParameter("/users/{id}", &openapi.Parameter{
//	    Name: "X-Tenant-ID", In: "header",
//	    Schema: &openapi.Schema{Type: openapi.TypeString("string")},
//	})
//
// # Server Overrides
//
// Servers can be overridden at the path or operation level. Path-level servers
// apply to all operations under a path, while operation-level servers apply to
// a single operation:
//
//	// Path-level: all operations under /files use the file server.
//	spec.AddPathServer("/files", openapi.Server{URL: "https://files.example.com"})
//
//	// Operation-level: only this operation uses the upload server.
//	spec.Op("upload").Server(openapi.Server{URL: "https://upload.example.com"})
//
// Servers support URL template variables with enum and default values:
//
//	spec.AddServer(openapi.Server{
//	    URL: "https://{environment}.example.com/v2",
//	    Variables: map[string]*openapi.ServerVariable{
//	        "environment": {
//	            Default: "api",
//	            Enum:    []string{"api", "api.dev", "api.staging"},
//	        },
//	    },
//	})
//
// # Media Types
//
// Request and Response are JSON shortcuts. Use RequestContent and
// ResponseContent for any content type:
//
//	// JSON (default)
//	spec.Op("create").Request(Employee{}).Response(http.StatusOK, Employee{})
//
//	// XML
//	spec.Op("create").RequestContent("application/xml", Employee{})
//
//	// Multiple content types for the same operation
//	spec.Op("get").
//	    Response(http.StatusOK, Employee{}).
//	    ResponseContent(http.StatusOK, "application/xml", Employee{})
//
//	// Binary file upload
//	spec.Op("upload").RequestContent("application/octet-stream", &openapi.Schema{
//	    Type: openapi.TypeString("string"), Format: "binary",
//	})
//
//	// Image response
//	spec.Op("avatar").ResponseContent(http.StatusOK, "image/png", &openapi.Schema{
//	    Type: openapi.TypeString("string"), Format: "binary",
//	})
//
//	// Form data
//	spec.Op("submit").RequestContent("multipart/form-data", FormInput{})
//
// Pass a *Schema directly for explicit schema control (binary, text, etc.)
// or a Go type for automatic schema generation via reflection.
//
// # Request Body Metadata
//
// Set description and required flag on request bodies:
//
//	spec.Op("create").
//	    Request(CreateInput{}).
//	    RequestDescription("The resource to create").
//	    RequestRequired(false)
//
// By default, request bodies are required (true).
//
// # Default Response
//
// Use DefaultResponse to define a catch-all response for status codes not
// covered by specific responses:
//
//	spec.Op("getUser").
//	    Response(http.StatusOK, User{}).
//	    DefaultResponse(ErrorResponse{})
//
// DefaultResponseContent works like ResponseContent for custom content types.
//
// # Response Headers and Links
//
// Add headers and links to responses:
//
//	spec.Op("listUsers").
//	    Response(http.StatusOK, []User{}).
//	    ResponseHeader(http.StatusOK, "X-Total-Count", &openapi.Header{
//	        Description: "Total number of users",
//	        Schema:      &openapi.Schema{Type: openapi.TypeString("integer")},
//	    }).
//	    ResponseLink(http.StatusOK, "GetNext", &openapi.Link{
//	        OperationID: "listUsers",
//	        Parameters:  map[string]any{"page": "$response.body#/nextPage"},
//	    })
//
// # Operation ID
//
// When using Op, the route name becomes the operationId automatically.
// When using Route, the mux route name is used if set. Use OperationID
// to set or override the operation ID explicitly:
//
//	spec.Route(r.HandleFunc("/users", listUsers).Methods(http.MethodGet)).
//	    OperationID("listAllUsers").
//	    Summary("List users")
//
// # Response Descriptions
//
// Response descriptions are auto-generated from HTTP status text. Override
// them per status code:
//
//	spec.Op("getUser").
//	    Response(http.StatusOK, User{}).
//	    ResponseDescription(http.StatusOK, "The requested user").
//	    DefaultResponse(ErrorResponse{}).
//	    DefaultResponseDescription("Unexpected error")
//
// # Webhooks
//
// Webhooks describe API-initiated callbacks not tied to a specific path
// on the mux router:
//
//	spec.Webhook("newUser", http.MethodPost).
//	    Summary("New user notification").
//	    Request(UserEvent{}).
//	    Response(http.StatusOK, nil)
//
// Group defaults also apply to webhooks:
//
//	events := spec.Group().Tags("events")
//	events.Webhook("userCreated", http.MethodPost).Summary("User created")
//
// # Operation Extensions
//
// Operations support callbacks:
//
//	cb := openapi.Callback{"{$request.body#/callbackUrl}": &openapi.PathItem{...}}
//	spec.Op("subscribe").Callback("onEvent", &cb)
//
// # Struct Tags
//
// Use the "openapi" struct tag to enrich JSON Schema output:
//
//	type CreateUserInput struct {
//	    Name  string `json:"name" openapi:"description=User name,minLength=1,maxLength=100"`
//	    Email string `json:"email" openapi:"format=email"`
//	    Age   int    `json:"age,omitempty" openapi:"minimum=0,maximum=150"`
//	    Role  string `json:"role" openapi:"enum=admin|user|guest"`
//	}
//
// Supported tag keys: description, example, format, title, minimum, maximum,
// exclusiveMinimum, exclusiveMaximum, minLength, maxLength, pattern,
// multipleOf, minItems, maxItems, uniqueItems, minProperties, maxProperties,
// const, enum (pipe-separated), deprecated, readOnly, writeOnly.
//
// # Path Parameter Typing
//
// Mux route macros are automatically mapped to OpenAPI types:
//
//	{id:uuid}   -> type: string, format: uuid
//	{page:int}  -> type: integer
//	{v:float}   -> type: number
//	{d:date}    -> type: string, format: date
//	{h:domain}  -> type: string, format: hostname
//
// # JSON Schema Generation
//
// Go types are converted to JSON Schema via reflection:
//
//   - bool -> {type: "boolean"}
//   - int/uint variants -> {type: "integer"}
//   - float32/float64 -> {type: "number"}
//   - string -> {type: "string"}
//   - []byte -> {type: "string", format: "byte"}
//   - time.Time -> {type: "string", format: "date-time"}
//   - *T -> nullable type using type arrays (e.g., ["string", "null"])
//   - []T -> {type: "array", items: schema(T)}
//   - map[string]V -> {type: "object", additionalProperties: schema(V)}
//   - struct -> {type: "object", properties: {...}, required: [...]}
//
// Named struct types are deduplicated into #/components/schemas/{TypeName}
// and referenced via $ref.
//
// # Type-Level Examples
//
// Implement the Exampler interface to provide a complete example value
// for a type's component schema:
//
//	func (User) OpenAPIExample() any {
//	    return User{ID: "550e8400-...", Name: "Alice", Email: "alice@example.com"}
//	}
//
// The returned value is serialized as the "example" field on the component
// schema. This works alongside field-level examples set via struct tags.
//
// # Generic Response Wrappers
//
// Go generics work naturally with the schema generator. Each concrete
// instantiation produces a distinct component schema with a sanitized name:
//
//	type ResponseData[T any] struct {
//	    Success bool     `json:"success"`
//	    Errors  []string `json:"errors,omitempty"`
//	    Result  T        `json:"result"`
//	}
//
//	spec.Op("getUser").Response(http.StatusOK, ResponseData[User]{})
//	// → schema "ResponseDataUser" with Result typed as $ref User
//
//	spec.Op("listUsers").Response(http.StatusOK, ResponseData[[]User]{})
//	// → schema "ResponseDataUserList" with Result typed as array of $ref User
//
// # Serving the Specification
//
// Handle registers all OpenAPI endpoints under a base path. The config
// parameter is optional -- pass nil for defaults:
//
//	spec.Handle(r, "/swagger", nil)
//
// This registers three routes:
//
//	/swagger/            - interactive HTML docs
//	/swagger/schema.json - OpenAPI spec as JSON
//	/swagger/schema.yaml - OpenAPI spec as YAML
//
// Both /swagger and /swagger/ serve the docs UI. All handlers build the
// document once on first request using sync.Once.
//
// Filenames are relative to the base path by default. Use an absolute path
// (starting with "/") to serve the schema at an independent location:
//
//	spec.Handle(r, "/swagger", &openapi.HandleConfig{
//	    JSONFilename: "/api/v1/swagger.json",
//	    YAMLFilename: "-",
//	})
//	// /swagger/              -> docs UI pointing to /api/v1/swagger.json
//	// /api/v1/swagger.json   -> JSON spec
//
// Choose the docs UI via HandleConfig:
//
//	openapi.DocsSwaggerUI (default)
//	openapi.DocsRapiDoc
//	openapi.DocsRedoc
//
// Pass additional Swagger UI options via SwaggerUIConfig:
//
//	spec.Handle(r, "/swagger", &openapi.HandleConfig{
//	    SwaggerUIConfig: map[string]any{
//	        "docExpansion": "none",
//	        "deepLinking":  true,
//	    },
//	})
//
// # Building the Document
//
// Build walks the mux router and assembles a complete *Document. This is
// called automatically by Handle, but can be used directly:
//
//	doc := spec.Build(r)
//	data, _ := json.MarshalIndent(doc, "", "  ")
//
// # Subrouter Integration
//
// The openapi package works with mux subrouters. Build walks the entire
// router tree, so routes registered on subrouters appear with full paths:
//
//	api := r.PathPrefix("/api/v1").Subrouter()
//	spec.Route(api.HandleFunc("/users", listUsers).Methods(http.MethodGet)).
//	    Summary("List users")
//	doc := spec.Build(r) // pass root router, not subrouter
package openapi
