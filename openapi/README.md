# openapi

Automatic OpenAPI v3.1.0 specification generation from `mux` router routes.

Converts Go types to JSON Schema (Draft 2020-12) via reflection and `openapi` struct tags. Named struct types are deduplicated into `#/components/schemas` with `$ref` references. Path parameter macros (`{id:uuid}`, `{page:int}`, etc.) are mapped to OpenAPI types automatically.

## Getting started

Create a spec, define routes on the mux router, and attach OpenAPI metadata.

```go
r := mux.NewRouter()
spec := openapi.NewSpec(openapi.Info{Title: "My API", Version: "1.0.0"})
```

### Attaching metadata with Route

`Route` takes a `*mux.Route` directly. Full mux flexibility is preserved -- use any `mux.Route` configuration (Methods, Headers, Queries, Schemes, etc.).

```go
spec.Route(r.HandleFunc("/users", listUsers).Methods(http.MethodGet)).
    Summary("List all users").
    Tags("users").
    Response(http.StatusOK, []User{})

spec.Route(r.HandleFunc("/users", createUser).Methods(http.MethodPost)).
    Summary("Create a user").
    Tags("users").
    Request(CreateUserRequest{}).
    Response(http.StatusCreated, User{}).
    Response(http.StatusBadRequest, ErrorResponse{})
```

### Attaching metadata with Op (named routes)

`Op` matches metadata to routes by name. Define routes first with `.Name()`, then attach metadata separately.

```go
r.HandleFunc("/users", listUsers).Methods(http.MethodGet).Name("listUsers")
r.HandleFunc("/users/{id:uuid}", getUser).Methods(http.MethodGet).Name("getUser")

spec.Op("listUsers").
    Summary("List all users").
    Tags("users").
    Response(http.StatusOK, []User{})

spec.Op("getUser").
    Summary("Get user by ID").
    Tags("users").
    Response(http.StatusOK, User{})
```

Both `Route` and `Op` return an `*OperationBuilder` with the same fluent API.

## Route groups

Use `Group` to apply shared OpenAPI metadata defaults to a logical group of operations. Groups are a metadata concept only -- they do not affect routing.

```go
users := spec.Group().
    Tags("users").
    Security(openapi.SecurityRequirement{"bearerAuth": {}}).
    Response(http.StatusForbidden, ErrorResponse{}).
    ResponseDescription(http.StatusForbidden, "Insufficient permissions").
    Response(http.StatusNotFound, ErrorResponse{})

users.Route(r.HandleFunc("/users", listUsers).Methods(http.MethodGet)).
    Summary("List users").
    Response(http.StatusOK, []User{})
    // inherits 403 and 404 from group

users.Route(r.HandleFunc("/users/{id}", getUser).Methods(http.MethodGet)).
    Summary("Get user").
    Response(http.StatusOK, User{})
    // inherits 403 and 404 from group
```

Groups support `Tags`, `Security`, `Deprecated`, `Server`, `Parameter`, `ExternalDocs`, and the full response API: `Response`, `ResponseContent`, `ResponseDescription`, `ResponseHeader`, `ResponseLink`, `DefaultResponse`, `DefaultResponseDescription`, `DefaultResponseHeader`. Both `Route` and `Op` are available on a group.

An operation-level `Response` call for the same status code overrides the group default for that code.

Override/merge semantics per field:

| Field | Behavior | Description |
|-------|----------|-------------|
| Tags | Append | Group tags + operation tags combined |
| Security | Replace | Operation-level `Security` call overrides group value |
| Deprecated | One-way latch | Group deprecation cannot be undone per-operation |
| Servers | Append | Group servers + operation servers combined |
| Parameters | Append | Group parameters + operation parameters combined |
| Responses | Merge | Group responses + operation responses; operation overrides per status code |
| ExternalDocs | Replace | Operation-level `ExternalDocs` call overrides group value |

## Request and response bodies

### JSON (default)

`Request` and `Response` are JSON shortcuts. Pass a Go type for automatic schema generation via reflection.

```go
spec.Op("create").
    Request(CreateUserRequest{}).
    Response(http.StatusCreated, User{}).
    Response(http.StatusBadRequest, ErrorResponse{})
```

Pass `nil` body to `Response` for status codes with no content:

```go
spec.Op("delete").Response(http.StatusNoContent, nil)
```

### Custom content types

Use `RequestContent` and `ResponseContent` for any content type:

```go
// XML
spec.Op("create").RequestContent("application/xml", Employee{})

// Multiple content types for the same operation
spec.Op("get").
    Response(http.StatusOK, Employee{}).
    ResponseContent(http.StatusOK, "application/xml", Employee{})

// Binary file upload
spec.Op("upload").RequestContent("application/octet-stream", &openapi.Schema{
    Type: openapi.TypeString("string"), Format: "binary",
})

// Image response
spec.Op("avatar").ResponseContent(http.StatusOK, "image/png", &openapi.Schema{
    Type: openapi.TypeString("string"), Format: "binary",
})

// Form data
spec.Op("submit").RequestContent("multipart/form-data", FormInput{})

// Plain text
spec.Op("text").ResponseContent(http.StatusOK, "text/plain", &openapi.Schema{
    Type: openapi.TypeString("string"),
})

// Vendor-specific type
spec.Op("create").RequestContent("application/vnd.mycompany.v2+json", Employee{})
```

Pass a `*Schema` directly for explicit schema control (binary, text, etc.) or a Go type for automatic schema generation via reflection.

### Request body metadata

Set description and required flag on request bodies:

```go
spec.Op("create").
    Request(CreateInput{}).
    RequestDescription("The resource to create").
    RequestRequired(false)
```

By default, request bodies are required (`true`).

### Default response

Use `DefaultResponse` to define a catch-all response for status codes not covered by specific responses:

```go
spec.Op("getUser").
    Response(http.StatusOK, User{}).
    DefaultResponse(ErrorResponse{})
```

`DefaultResponseContent` works like `ResponseContent` for custom content types.

### Response headers and links

Add headers and links to responses:

```go
spec.Op("listUsers").
    Response(http.StatusOK, []User{}).
    ResponseHeader(http.StatusOK, "X-Total-Count", &openapi.Header{
        Description: "Total number of users",
        Schema:      &openapi.Schema{Type: openapi.TypeString("integer")},
    }).
    ResponseLink(http.StatusOK, "GetNext", &openapi.Link{
        OperationID: "listUsers",
        Parameters:  map[string]any{"page": "$response.body#/nextPage"},
    })
```

`DefaultResponseHeader` and `DefaultResponseLink` target the default response:

```go
spec.Op("getUser").
    Response(http.StatusOK, User{}).
    DefaultResponse(ErrorResponse{}).
    DefaultResponseHeader("X-Request-ID", &openapi.Header{
        Schema: &openapi.Schema{Type: openapi.TypeString("string")},
    }).
    DefaultResponseLink("GetError", &openapi.Link{OperationID: "getErrorDetails"})
```

### Response descriptions

Response descriptions are auto-generated from HTTP status text (e.g., "OK", "Not Found"). Override them per status code:

```go
spec.Op("getUser").
    Response(http.StatusOK, User{}).
    ResponseDescription(http.StatusOK, "The requested user").
    DefaultResponse(ErrorResponse{}).
    DefaultResponseDescription("Unexpected error")
```

## Operation ID

When using `Op`, the route name becomes the `operationId` automatically. When using `Route`, the mux route name is used if set. Use `OperationID` to set or override the operation ID explicitly:

```go
spec.Route(r.HandleFunc("/users", listUsers).Methods(http.MethodGet)).
    OperationID("listAllUsers").
    Summary("List users").
    Response(http.StatusOK, []User{})
```

## Parameters

### Path parameters

Path parameters from mux route macros are generated automatically with correct OpenAPI types. No manual parameter registration is needed.

```go
// {id:uuid} -> type: string, format: uuid
spec.Route(r.HandleFunc("/users/{id:uuid}", getUser).Methods(http.MethodGet))
```

### Query, header, and cookie parameters

Add custom parameters at the operation level:

```go
spec.Op("listUsers").
    Parameter(&openapi.Parameter{
        Name: "page", In: "query",
        Schema: &openapi.Schema{Type: openapi.TypeString("integer")},
    }).
    Parameter(&openapi.Parameter{
        Name: "X-Request-ID", In: "header",
        Schema: &openapi.Schema{Type: openapi.TypeString("string")},
    })
```

### Path-level parameters

Parameters shared across all operations under a path:

```go
spec.AddPathParameter("/users/{id}", &openapi.Parameter{
    Name: "X-Tenant-ID", In: "header",
    Schema: &openapi.Schema{Type: openapi.TypeString("string")},
})
```

## Security

Register security schemes and apply them at the document or operation level:

```go
spec.AddSecurityScheme("bearerAuth", &openapi.SecurityScheme{
    Type:         "http",
    Scheme:       "bearer",
    BearerFormat: "JWT",
})
spec.SetSecurity(openapi.SecurityRequirement{"bearerAuth": {}})
```

Override security per operation. Call `Security()` with no arguments to mark an endpoint as public:

```go
spec.Route(r.HandleFunc("/health", healthHandler).Methods(http.MethodGet)).
    Summary("Health check").
    Security()
```

## Servers

Servers can be set at three levels: document, path, and operation. Lower levels override higher levels.

```go
// Document-level (all operations inherit by default).
spec.AddServer(openapi.Server{URL: "https://api.example.com", Description: "Main"})

// Path-level: all operations under /files use the file server.
spec.AddPathServer("/files", openapi.Server{URL: "https://files.example.com"})

// Operation-level: only this operation uses the upload server.
spec.Op("upload").Server(openapi.Server{URL: "https://upload.example.com"})
```

Servers support URL template variables with enum and default values:

```go
spec.AddServer(openapi.Server{
    URL: "https://{environment}.example.com/v2",
    Variables: map[string]*openapi.ServerVariable{
        "environment": {
            Default: "api",
            Enum:    []string{"api", "api.dev", "api.staging"},
        },
    },
})
```

## Tags

Tags used in operations are auto-collected and sorted alphabetically. Use `AddTag` to provide descriptions and external documentation:

```go
spec.AddTag(openapi.Tag{
    Name:         "users",
    Description:  "User management operations",
    ExternalDocs: &openapi.ExternalDocs{URL: "https://docs.example.com/users"},
})
```

User-defined tags take precedence over auto-collected tags. Tags defined via `AddTag` but not used by any operation are still included.

## External documentation

Attach external docs at the document level:

```go
spec.SetExternalDocs("https://docs.example.com", "Full documentation")
```

Or at the operation level:

```go
spec.Op("listUsers").ExternalDocs("https://docs.example.com/users", "User API docs")
```

## Path-level metadata

Set summary, description, and shared parameters on a path. These apply to all operations under the path:

```go
spec.SetPathSummary("/users/{id}", "Represents a user")
spec.SetPathDescription("/users/{id}", "Individual user identified by ID.")
spec.AddPathParameter("/users/{id}", &openapi.Parameter{
    Name: "X-Tenant-ID", In: "header",
    Schema: &openapi.Schema{Type: openapi.TypeString("string")},
})
```

## Reusable components

Register reusable objects in `components`:

```go
spec.AddComponentResponse("NotFound", &openapi.Response{Description: "Not found"})
spec.AddComponentParameter("pageParam", &openapi.Parameter{Name: "page", In: "query"})
spec.AddComponentExample("sample", &openapi.Example{Summary: "A sample", Value: "test"})
spec.AddComponentRequestBody("CreatePet", &openapi.RequestBody{Description: "Pet to create"})
spec.AddComponentHeader("X-Rate-Limit", &openapi.Header{
    Schema: &openapi.Schema{Type: openapi.TypeString("integer")},
})
spec.AddComponentLink("GetUser", &openapi.Link{OperationID: "getUser"})
spec.AddComponentCallback("onEvent", &cb)
spec.AddComponentPathItem("shared", &openapi.PathItem{})
```

## Callbacks

Operations support callbacks:

```go
cb := openapi.Callback{"{$request.body#/callbackUrl}": &openapi.PathItem{}}
spec.Op("subscribe").Callback("onEvent", &cb)
```

## Webhooks

Webhooks describe API-initiated callbacks that are not tied to a specific path on the mux router. They appear in the `webhooks` section of the OpenAPI document.

```go
spec.Webhook("newUser", http.MethodPost).
    Summary("New user notification").
    Tags("webhooks").
    Request(UserEvent{}).
    Response(http.StatusOK, nil)
```

Webhooks support the same fluent API as `Route` and `Op`. Group defaults also apply to webhooks:

```go
events := spec.Group().
    Tags("events").
    Security(openapi.SecurityRequirement{"bearerAuth": {}})

events.Webhook("userCreated", http.MethodPost).
    Summary("User created event").
    Request(UserEvent{})
```

## Struct tags

Use the `openapi` struct tag for schema enrichment:

```go
type CreateUserInput struct {
    Name  string `json:"name" openapi:"description=User name,example=John,minLength=1,maxLength=100"`
    Email string `json:"email" openapi:"description=Email address,format=email"`
    Age   int    `json:"age,omitempty" openapi:"minimum=0,maximum=150"`
    Role  string `json:"role" openapi:"enum=admin|user|guest,description=User role"`
}
```

Supported keys:

| Key | Type | Description |
|-----|------|-------------|
| `description` | string | Field description |
| `example` | any | Example value (type-aware parsing) |
| `format` | string | JSON Schema format (email, uuid, date-time, etc.) |
| `title` | string | Schema title |
| `minimum` | float64 | Minimum value |
| `maximum` | float64 | Maximum value |
| `exclusiveMinimum` | float64 | Exclusive minimum |
| `exclusiveMaximum` | float64 | Exclusive maximum |
| `multipleOf` | float64 | Value must be a multiple of this |
| `minLength` | int | Minimum string length |
| `maxLength` | int | Maximum string length |
| `pattern` | string | Regex pattern |
| `minItems` | int | Minimum array items |
| `maxItems` | int | Maximum array items |
| `uniqueItems` | bool | Array items must be unique |
| `minProperties` | int | Minimum object properties |
| `maxProperties` | int | Maximum object properties |
| `const` | any | Fixed constant value |
| `enum` | string | Pipe-separated enum values |
| `deprecated` | bool | Mark as deprecated |
| `readOnly` | bool | Read-only field |
| `writeOnly` | bool | Write-only field |

## Type-level examples

Implement `openapi.Exampler` to provide a complete example for a type's component schema:

```go
type User struct {
    ID    string `json:"id" openapi:"format=uuid"`
    Name  string `json:"name"`
    Email string `json:"email" openapi:"format=email"`
}

func (User) OpenAPIExample() any {
    return User{
        ID:    "550e8400-e29b-41d4-a716-446655440000",
        Name:  "Alice Smith",
        Email: "alice@example.com",
    }
}
```

The returned value is serialized as the `example` field on the component schema. This works alongside field-level examples set via struct tags.

## Generic response wrappers

Go generics work naturally with the schema generator. Each concrete instantiation produces a distinct component schema with a sanitized name:

```go
type ResponseData[T any] struct {
    Success  bool          `json:"success"`
    Errors   []ErrorDetail `json:"errors,omitempty"`
    Messages []string      `json:"messages,omitempty"`
    Result   T             `json:"result"`
}

// Single user → schema "ResponseDataUser"
spec.Op("getUser").Response(http.StatusOK, ResponseData[User]{})

// User list → schema "ResponseDataUserList"
spec.Op("listUsers").Response(http.StatusOK, ResponseData[[]User]{})
```

Schema name mapping:

| Go type | Component schema name |
|---------|----------------------|
| `ResponseData[User]` | `ResponseDataUser` |
| `ResponseData[[]User]` | `ResponseDataUserList` |
| `ResponseData[pkg.Item]` | `ResponseDataItem` |

## Path parameter macros

| Macro | OpenAPI type | format |
|-------|-------------|--------|
| `uuid` | string | uuid |
| `int` | integer | - |
| `float` | number | - |
| `date` | string | date |
| `domain` | string | hostname |
| `slug` | string | - |
| `alpha` | string | - |
| `alphanum` | string | - |
| `hex` | string | - |

## Type mapping

| Go type | JSON Schema |
|---------|------------|
| `bool` | `{type: "boolean"}` |
| `int`, `int8`..`int64`, `uint`..`uint64` | `{type: "integer"}` |
| `float32`, `float64` | `{type: "number"}` |
| `string` | `{type: "string"}` |
| `[]byte` | `{type: "string", format: "byte"}` |
| `time.Time` | `{type: "string", format: "date-time"}` |
| `*T` | nullable via type array `["<type>", "null"]` |
| `[]T` | `{type: "array", items: schema(T)}` |
| `map[string]V` | `{type: "object", additionalProperties: schema(V)}` |
| `struct` | `{type: "object", properties: {...}, required: [...]}` |

## Serving

`Handle` registers all endpoints under a single base path:

```go
// Swagger UI (default) at /swagger/, schema at /swagger/schema.json and /swagger/schema.yaml
spec.Handle(r, "/swagger", openapi.HandleConfig{})

// RapiDoc
spec.Handle(r, "/swagger", openapi.HandleConfig{UI: openapi.DocsRapiDoc})

// Redoc
spec.Handle(r, "/swagger", openapi.HandleConfig{UI: openapi.DocsRedoc})

// Custom filenames (relative to base path)
spec.Handle(r, "/swagger", openapi.HandleConfig{JSONFilename: "openapi.json", YAMLFilename: "openapi.yaml"})

// Disable YAML endpoint
spec.Handle(r, "/swagger", openapi.HandleConfig{YAMLFilename: "-"})

// Disable interactive docs, serve only spec files
spec.Handle(r, "/swagger", openapi.HandleConfig{DisableDocs: true})
```

### Filename path resolution

Filenames are relative to the base path by default. Use an absolute path (starting with `/`) to serve the schema at an independent location:

```go
// Relative (default): schema at /swagger/schema.json
spec.Handle(r, "/swagger", openapi.HandleConfig{})

// Relative custom name: schema at /swagger/swagger.json
spec.Handle(r, "/swagger", openapi.HandleConfig{JSONFilename: "swagger.json"})

// Relative nested path: schema at /swagger/data/openapi.json
spec.Handle(r, "/swagger", openapi.HandleConfig{JSONFilename: "data/openapi.json"})

// Absolute path: docs at /swagger/, schema at /api/v1/swagger.json
spec.Handle(r, "/swagger", openapi.HandleConfig{
    JSONFilename: "/api/v1/swagger.json",
    YAMLFilename: "-",
})
```

Path resolution examples with base path `/swagger`:

| JSONFilename | Resolved path |
|---|---|
| `"schema.json"` (default) | `/swagger/schema.json` |
| `"swagger.json"` | `/swagger/swagger.json` |
| `"data/openapi.json"` | `/swagger/data/openapi.json` |
| `"/api/v1/swagger.json"` | `/api/v1/swagger.json` |

The same rules apply to `YAMLFilename`. JSON and YAML can use different path styles independently.

Both `<basePath>` and `<basePath>/` serve the docs UI. The docs UI automatically references the correct schema path. All handlers build the document once on first request and cache the result.

## Building the document

`Build` walks the mux router and assembles a complete `*Document`. This is called automatically by `Handle`, but can be used directly for custom serialization or testing:

```go
doc := spec.Build(r)

// Serialize to JSON
data, _ := json.MarshalIndent(doc, "", "  ")

// Serialize to YAML
data, _ := yaml.Marshal(doc)
```

`Build` resolves all registered routes (via `Route` and `Op`), generates JSON schemas for Go types, collects tags, and assembles components. Routes without OpenAPI metadata are skipped.

## Subrouter integration

The openapi package works with mux subrouters. `Build` walks the entire router tree, so routes registered on subrouters appear with their full paths:

```go
r := mux.NewRouter()
api := r.PathPrefix("/api/v1").Subrouter()

spec := openapi.NewSpec(openapi.Info{Title: "My API", Version: "1.0.0"})

users := spec.Group().Tags("users")

users.Route(api.HandleFunc("/users", listUsers).Methods(http.MethodGet)).
    Summary("List users").
    Response(http.StatusOK, []User{})

users.Route(api.HandleFunc("/users/{id:uuid}", getUser).Methods(http.MethodGet)).
    Summary("Get user").
    Response(http.StatusOK, User{})

// Build walks the root router -- subrouter routes appear as /api/v1/users, /api/v1/users/{id}
doc := spec.Build(r)
```

Pass the root router (not the subrouter) to `Build` and `Handle`.
