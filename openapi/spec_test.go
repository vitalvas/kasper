package openapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/kasper/mux"
)

func dummyHandler(http.ResponseWriter, *http.Request) {}

func TestNewSpec(t *testing.T) {
	t.Run("creates spec with info", func(t *testing.T) {
		spec := NewSpec(Info{Title: "Test API", Version: "1.0.0"})
		assert.NotNil(t, spec)
		assert.Equal(t, "Test API", spec.info.Title)
	})

	t.Run("add servers", func(t *testing.T) {
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			AddServer(Server{URL: "https://api.example.com", Description: "Production"}).
			AddServer(Server{URL: "http://localhost:8080", Description: "Local"})

		assert.Len(t, spec.servers, 2)
	})
}

func TestParsePath(t *testing.T) {
	t.Run("no variables", func(t *testing.T) {
		path, params := parsePath("/users")
		assert.Equal(t, "/users", path)
		assert.Empty(t, params)
	})

	t.Run("simple variable", func(t *testing.T) {
		path, params := parsePath("/users/{id}")
		assert.Equal(t, "/users/{id}", path)
		require.Len(t, params, 1)
		assert.Equal(t, "id", params[0].Name)
		assert.Equal(t, "path", params[0].In)
		assert.True(t, params[0].Required)
		assert.Equal(t, TypeString("string"), params[0].Schema.Type)
	})

	t.Run("uuid macro", func(t *testing.T) {
		path, params := parsePath("/users/{id:uuid}")
		assert.Equal(t, "/users/{id}", path)
		require.Len(t, params, 1)
		assert.Equal(t, "id", params[0].Name)
		assert.Equal(t, TypeString("string"), params[0].Schema.Type)
		assert.Equal(t, "uuid", params[0].Schema.Format)
	})

	t.Run("int macro", func(t *testing.T) {
		_, params := parsePath("/articles/{page:int}")
		require.Len(t, params, 1)
		assert.Equal(t, TypeString("integer"), params[0].Schema.Type)
		assert.Empty(t, params[0].Schema.Format)
	})

	t.Run("float macro", func(t *testing.T) {
		_, params := parsePath("/values/{v:float}")
		require.Len(t, params, 1)
		assert.Equal(t, TypeString("number"), params[0].Schema.Type)
	})

	t.Run("date macro", func(t *testing.T) {
		_, params := parsePath("/events/{d:date}")
		require.Len(t, params, 1)
		assert.Equal(t, TypeString("string"), params[0].Schema.Type)
		assert.Equal(t, "date", params[0].Schema.Format)
	})

	t.Run("domain macro", func(t *testing.T) {
		_, params := parsePath("/sites/{host:domain}")
		require.Len(t, params, 1)
		assert.Equal(t, TypeString("string"), params[0].Schema.Type)
		assert.Equal(t, "hostname", params[0].Schema.Format)
	})

	t.Run("unknown macro treated as regex", func(t *testing.T) {
		path, params := parsePath("/items/{code:[A-Z]+}")
		assert.Equal(t, "/items/{code}", path)
		require.Len(t, params, 1)
		assert.Equal(t, TypeString("string"), params[0].Schema.Type)
	})

	t.Run("multiple variables", func(t *testing.T) {
		path, params := parsePath("/users/{userId:uuid}/posts/{postId:int}")
		assert.Equal(t, "/users/{userId}/posts/{postId}", path)
		require.Len(t, params, 2)
		assert.Equal(t, "userId", params[0].Name)
		assert.Equal(t, "uuid", params[0].Schema.Format)
		assert.Equal(t, "postId", params[1].Name)
		assert.Equal(t, TypeString("integer"), params[1].Schema.Type)
	})
}

func TestBuildVariantB(t *testing.T) {
	t.Run("named routes with metadata", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/users", dummyHandler).Methods(http.MethodGet).Name("listUsers")
		r.HandleFunc("/users", dummyHandler).Methods(http.MethodPost).Name("createUser")
		r.HandleFunc("/users/{id:uuid}", dummyHandler).Methods(http.MethodGet).Name("getUser")

		type User struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		type CreateUserInput struct {
			Name string `json:"name"`
		}

		spec := NewSpec(Info{Title: "User API", Version: "1.0.0"})

		spec.Op("listUsers").
			Summary("List all users").
			Tags("users").
			Response(http.StatusOK, []User{})

		spec.Op("createUser").
			Summary("Create a user").
			Tags("users").
			Request(CreateUserInput{}).
			Response(http.StatusCreated, User{})

		spec.Op("getUser").
			Summary("Get user by ID").
			Tags("users").
			Response(http.StatusOK, User{})

		doc := spec.Build(r)

		assert.Equal(t, "3.1.0", doc.OpenAPI)
		assert.Equal(t, "User API", doc.Info.Title)

		// Check paths.
		require.Contains(t, doc.Paths, "/users")
		require.Contains(t, doc.Paths, "/users/{id}")

		// GET /users
		require.NotNil(t, doc.Paths["/users"].Get)
		assert.Equal(t, "List all users", doc.Paths["/users"].Get.Summary)
		assert.Equal(t, "listUsers", doc.Paths["/users"].Get.OperationID)
		assert.Equal(t, []string{"users"}, doc.Paths["/users"].Get.Tags)

		// POST /users
		require.NotNil(t, doc.Paths["/users"].Post)
		assert.Equal(t, "Create a user", doc.Paths["/users"].Post.Summary)
		require.NotNil(t, doc.Paths["/users"].Post.RequestBody)

		// GET /users/{id}
		require.NotNil(t, doc.Paths["/users/{id}"].Get)
		assert.Equal(t, "Get user by ID", doc.Paths["/users/{id}"].Get.Summary)
		require.Len(t, doc.Paths["/users/{id}"].Get.Parameters, 1)
		assert.Equal(t, "id", doc.Paths["/users/{id}"].Get.Parameters[0].Name)
		assert.Equal(t, "uuid", doc.Paths["/users/{id}"].Get.Parameters[0].Schema.Format)

		// Check components.
		require.NotNil(t, doc.Components)
		assert.Contains(t, doc.Components.Schemas, "User")
		assert.Contains(t, doc.Components.Schemas, "CreateUserInput")

		// Check tags auto-aggregation.
		require.Len(t, doc.Tags, 1)
		assert.Equal(t, "users", doc.Tags[0].Name)
	})
}

func TestBuildRoute(t *testing.T) {
	t.Run("route with full mux flexibility", func(t *testing.T) {
		r := mux.NewRouter()

		type Item struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}

		spec := NewSpec(Info{Title: "Item API", Version: "1.0.0"})

		spec.Route(r.HandleFunc("/items", dummyHandler).Methods(http.MethodGet)).
			Summary("List items").
			Tags("items").
			Response(http.StatusOK, []Item{})

		spec.Route(r.HandleFunc("/items", dummyHandler).Methods(http.MethodPost)).
			Summary("Create item").
			Tags("items").
			Request(Item{}).
			Response(http.StatusCreated, Item{})

		spec.Route(r.HandleFunc("/items/{id:uuid}", dummyHandler).Methods(http.MethodDelete)).
			Summary("Delete item").
			Tags("items").
			Response(http.StatusNoContent, nil)

		doc := spec.Build(r)

		require.Contains(t, doc.Paths, "/items")
		require.Contains(t, doc.Paths, "/items/{id}")

		require.NotNil(t, doc.Paths["/items"].Get)
		assert.Equal(t, "List items", doc.Paths["/items"].Get.Summary)

		require.NotNil(t, doc.Paths["/items"].Post)
		assert.Equal(t, "Create item", doc.Paths["/items"].Post.Summary)

		require.NotNil(t, doc.Paths["/items/{id}"].Delete)
		assert.Equal(t, "Delete item", doc.Paths["/items/{id}"].Delete.Summary)
	})
}

func TestBuildRouteAllMethods(t *testing.T) {
	t.Run("put patch head", func(t *testing.T) {
		r := mux.NewRouter()

		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})

		spec.Route(r.HandleFunc("/items/{id}", dummyHandler).Methods(http.MethodPut)).
			Summary("Update item")

		spec.Route(r.HandleFunc("/items/{id}", dummyHandler).Methods(http.MethodPatch)).
			Summary("Patch item")

		spec.Route(r.HandleFunc("/items/{id}", dummyHandler).Methods(http.MethodHead)).
			Summary("Head item")

		doc := spec.Build(r)

		require.Contains(t, doc.Paths, "/items/{id}")
		assert.NotNil(t, doc.Paths["/items/{id}"].Put)
		assert.NotNil(t, doc.Paths["/items/{id}"].Patch)
		assert.NotNil(t, doc.Paths["/items/{id}"].Head)
	})
}

func TestBuildTagAutoAggregation(t *testing.T) {
	t.Run("collects unique tags sorted", func(t *testing.T) {
		r := mux.NewRouter()

		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})

		spec.Route(r.HandleFunc("/users", dummyHandler).Methods(http.MethodGet)).Tags("users")
		spec.Route(r.HandleFunc("/posts", dummyHandler).Methods(http.MethodGet)).Tags("posts")
		spec.Route(r.HandleFunc("/admin/users", dummyHandler).Methods(http.MethodGet)).Tags("admin", "users")

		doc := spec.Build(r)

		require.Len(t, doc.Tags, 3)
		assert.Equal(t, "admin", doc.Tags[0].Name)
		assert.Equal(t, "posts", doc.Tags[1].Name)
		assert.Equal(t, "users", doc.Tags[2].Name)
	})
}

func TestBuildServers(t *testing.T) {
	t.Run("servers included in document", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			AddServer(Server{URL: "https://api.example.com", Description: "Production"})

		spec.Route(r.HandleFunc("/health", dummyHandler).Methods(http.MethodGet)).
			Summary("Health check").
			Response(http.StatusOK, nil)

		doc := spec.Build(r)

		require.Len(t, doc.Servers, 1)
		assert.Equal(t, "https://api.example.com", doc.Servers[0].URL)
	})
}

func TestBuildNoComponents(t *testing.T) {
	t.Run("no components when no types registered", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})

		spec.Route(r.HandleFunc("/health", dummyHandler).Methods(http.MethodGet)).
			Summary("Health check").
			Response(http.StatusOK, nil)

		doc := spec.Build(r)
		assert.Nil(t, doc.Components)
	})
}

func TestBuildRouteWithoutOp(t *testing.T) {
	t.Run("route without operation metadata is skipped", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/internal", dummyHandler).Methods(http.MethodGet).Name("internal")

		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})
		doc := spec.Build(r)

		assert.Empty(t, doc.Paths)
	})
}

func TestBuildDocumentJSON(t *testing.T) {
	t.Run("full document serializes to valid JSON", func(t *testing.T) {
		r := mux.NewRouter()

		type User struct {
			ID    string `json:"id"`
			Name  string `json:"name" openapi:"description=User name,minLength=1"`
			Email string `json:"email" openapi:"format=email"`
		}

		spec := NewSpec(Info{Title: "My API", Version: "1.0.0"}).
			AddServer(Server{URL: "https://api.example.com", Description: "Production"})

		spec.Route(r.HandleFunc("/users/{id:uuid}", dummyHandler).Methods(http.MethodGet)).
			Summary("Get user").
			Tags("users").
			Response(http.StatusOK, User{})

		doc := spec.Build(r)

		data, err := json.MarshalIndent(doc, "", "  ")
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Equal(t, "3.1.0", parsed["openapi"])

		paths := parsed["paths"].(map[string]any)
		assert.Contains(t, paths, "/users/{id}")

		components := parsed["components"].(map[string]any)
		schemas := components["schemas"].(map[string]any)
		assert.Contains(t, schemas, "User")
	})
}

func TestSpecBuilderMethods(t *testing.T) {
	t.Run("SetExternalDocs", func(t *testing.T) {
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			SetExternalDocs("https://docs.example.com", "Full docs")
		assert.NotNil(t, spec.externalDocs)
		assert.Equal(t, "https://docs.example.com", spec.externalDocs.URL)
		assert.Equal(t, "Full docs", spec.externalDocs.Description)
	})

	t.Run("SetSecurity", func(t *testing.T) {
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			SetSecurity(SecurityRequirement{"bearerAuth": {}})
		require.Len(t, spec.security, 1)
		assert.Contains(t, spec.security[0], "bearerAuth")
	})

	t.Run("AddTag", func(t *testing.T) {
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			AddTag(Tag{Name: "users", Description: "User operations"}).
			AddTag(Tag{Name: "admin", Description: "Admin operations"})
		assert.Len(t, spec.tags, 2)
	})

	t.Run("AddSecurityScheme", func(t *testing.T) {
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			AddSecurityScheme("bearerAuth", &SecurityScheme{Type: "http", Scheme: "bearer"})
		require.NotNil(t, spec.securitySchemes)
		assert.Contains(t, spec.securitySchemes, "bearerAuth")
	})

	t.Run("AddComponentResponse", func(t *testing.T) {
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			AddComponentResponse("NotFound", &Response{Description: "Not found"})
		require.NotNil(t, spec.compResponses)
		assert.Contains(t, spec.compResponses, "NotFound")
	})

	t.Run("AddComponentParameter", func(t *testing.T) {
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			AddComponentParameter("pageParam", &Parameter{Name: "page", In: "query"})
		require.NotNil(t, spec.compParameters)
		assert.Contains(t, spec.compParameters, "pageParam")
	})

	t.Run("AddComponentExample", func(t *testing.T) {
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			AddComponentExample("sample", &Example{Summary: "Sample", Value: "test"})
		require.NotNil(t, spec.compExamples)
		assert.Contains(t, spec.compExamples, "sample")
	})

	t.Run("AddComponentRequestBody", func(t *testing.T) {
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			AddComponentRequestBody("CreatePet", &RequestBody{Description: "Pet to create"})
		require.NotNil(t, spec.compReqBodies)
		assert.Contains(t, spec.compReqBodies, "CreatePet")
	})

	t.Run("AddComponentHeader", func(t *testing.T) {
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			AddComponentHeader("X-Rate-Limit", &Header{Schema: &Schema{Type: TypeString("integer")}})
		require.NotNil(t, spec.compHeaders)
		assert.Contains(t, spec.compHeaders, "X-Rate-Limit")
	})

	t.Run("AddComponentLink", func(t *testing.T) {
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			AddComponentLink("GetUser", &Link{OperationID: "getUser"})
		require.NotNil(t, spec.compLinks)
		assert.Contains(t, spec.compLinks, "GetUser")
	})

	t.Run("AddComponentCallback", func(t *testing.T) {
		cb := Callback{"{$request.body#/url}": &PathItem{Post: &Operation{Summary: "cb"}}}
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			AddComponentCallback("onEvent", &cb)
		require.NotNil(t, spec.compCallbacks)
		assert.Contains(t, spec.compCallbacks, "onEvent")
	})

	t.Run("AddComponentPathItem", func(t *testing.T) {
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			AddComponentPathItem("shared", &PathItem{Get: &Operation{Summary: "Shared"}})
		require.NotNil(t, spec.compPathItems)
		assert.Contains(t, spec.compPathItems, "shared")
	})

	t.Run("chaining returns spec", func(t *testing.T) {
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			SetExternalDocs("https://docs.example.com", "Docs").
			SetSecurity(SecurityRequirement{"bearerAuth": {}}).
			AddTag(Tag{Name: "users"}).
			AddSecurityScheme("bearerAuth", &SecurityScheme{Type: "http", Scheme: "bearer"}).
			AddComponentResponse("NotFound", &Response{Description: "Not found"}).
			AddComponentParameter("page", &Parameter{Name: "page", In: "query"}).
			AddComponentExample("sample", &Example{Value: "test"}).
			AddComponentRequestBody("Create", &RequestBody{Description: "Create"}).
			AddComponentHeader("X-Rate", &Header{}).
			AddComponentLink("GetUser", &Link{OperationID: "getUser"}).
			AddServer(Server{URL: "https://api.example.com", Description: "Production"})

		assert.NotNil(t, spec.externalDocs)
		assert.Len(t, spec.security, 1)
		assert.Len(t, spec.tags, 1)
		assert.Len(t, spec.securitySchemes, 1)
		assert.Len(t, spec.compResponses, 1)
		assert.Len(t, spec.compParameters, 1)
		assert.Len(t, spec.compExamples, 1)
		assert.Len(t, spec.compReqBodies, 1)
		assert.Len(t, spec.compHeaders, 1)
		assert.Len(t, spec.compLinks, 1)
		assert.Len(t, spec.servers, 1)
	})
}

func TestMergeTags(t *testing.T) {
	t.Run("empty paths", func(t *testing.T) {
		s := &Spec{}
		tags := s.mergeTags(nil)
		assert.Empty(t, tags)
	})

	t.Run("deduplicates and sorts", func(t *testing.T) {
		paths := map[string]*PathItem{
			"/a": {
				Get:  &Operation{Tags: []string{"zebra", "alpha"}},
				Post: &Operation{Tags: []string{"alpha"}},
			},
		}
		s := &Spec{}
		tags := s.mergeTags(paths)
		require.Len(t, tags, 2)
		assert.Equal(t, "alpha", tags[0].Name)
		assert.Equal(t, "zebra", tags[1].Name)
	})

	t.Run("merges tags from multiple path maps", func(t *testing.T) {
		paths := map[string]*PathItem{
			"/a": {Get: &Operation{Tags: []string{"api"}}},
		}
		webhooks := map[string]*PathItem{
			"onEvent": {Post: &Operation{Tags: []string{"webhooks", "api"}}},
		}
		s := &Spec{}
		tags := s.mergeTags(paths, webhooks)
		require.Len(t, tags, 2)
		assert.Equal(t, "api", tags[0].Name)
		assert.Equal(t, "webhooks", tags[1].Name)
	})
}

func TestBuildExternalDocs(t *testing.T) {
	t.Run("appears in document", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			SetExternalDocs("https://docs.example.com", "Full documentation")

		spec.Route(r.HandleFunc("/health", dummyHandler).Methods(http.MethodGet)).
			Summary("Health check").
			Response(http.StatusOK, nil)

		doc := spec.Build(r)
		require.NotNil(t, doc.ExternalDocs)
		assert.Equal(t, "https://docs.example.com", doc.ExternalDocs.URL)
		assert.Equal(t, "Full documentation", doc.ExternalDocs.Description)
	})
}

func TestBuildSecurity(t *testing.T) {
	t.Run("doc-level and operation-level coexist", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			SetSecurity(SecurityRequirement{"bearerAuth": {}}).
			AddSecurityScheme("bearerAuth", &SecurityScheme{Type: "http", Scheme: "bearer"})

		spec.Route(r.HandleFunc("/users", dummyHandler).Methods(http.MethodGet)).
			Summary("List users").
			Tags("users")

		spec.Route(r.HandleFunc("/health", dummyHandler).Methods(http.MethodGet)).
			Summary("Health check").
			Security()

		doc := spec.Build(r)

		// Document-level security is set.
		require.Len(t, doc.Security, 1)
		assert.Contains(t, doc.Security[0], "bearerAuth")

		// Health endpoint has empty security (overrides global).
		require.NotNil(t, doc.Paths["/health"].Get.Security)
		assert.Empty(t, doc.Paths["/health"].Get.Security)

		// Users endpoint has no operation-level security (inherits doc-level).
		assert.Nil(t, doc.Paths["/users"].Get.Security)
	})
}

func TestBuildSecuritySchemes(t *testing.T) {
	t.Run("in doc.Components.SecuritySchemes", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			AddSecurityScheme("bearerAuth", &SecurityScheme{Type: "http", Scheme: "bearer", BearerFormat: "JWT"}).
			AddSecurityScheme("apiKey", &SecurityScheme{Type: "apiKey", Name: "X-API-Key", In: "header"})

		spec.Route(r.HandleFunc("/health", dummyHandler).Methods(http.MethodGet)).
			Summary("Health").
			Response(http.StatusOK, nil)

		doc := spec.Build(r)
		require.NotNil(t, doc.Components)
		require.NotNil(t, doc.Components.SecuritySchemes)
		assert.Contains(t, doc.Components.SecuritySchemes, "bearerAuth")
		assert.Contains(t, doc.Components.SecuritySchemes, "apiKey")
		assert.Equal(t, "JWT", doc.Components.SecuritySchemes["bearerAuth"].BearerFormat)
	})
}

func TestBuildUserDefinedTags(t *testing.T) {
	t.Run("merge with descriptions", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			AddTag(Tag{Name: "users", Description: "User operations"}).
			AddTag(Tag{Name: "admin", Description: "Admin operations", ExternalDocs: &ExternalDocs{URL: "https://docs.example.com/admin"}})

		spec.Route(r.HandleFunc("/users", dummyHandler).Methods(http.MethodGet)).
			Tags("users")
		spec.Route(r.HandleFunc("/admin", dummyHandler).Methods(http.MethodGet)).
			Tags("admin")

		doc := spec.Build(r)

		require.Len(t, doc.Tags, 2)
		// Sorted alphabetically.
		assert.Equal(t, "admin", doc.Tags[0].Name)
		assert.Equal(t, "Admin operations", doc.Tags[0].Description)
		require.NotNil(t, doc.Tags[0].ExternalDocs)
		assert.Equal(t, "https://docs.example.com/admin", doc.Tags[0].ExternalDocs.URL)

		assert.Equal(t, "users", doc.Tags[1].Name)
		assert.Equal(t, "User operations", doc.Tags[1].Description)
	})

	t.Run("tag not in operations still included", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			AddTag(Tag{Name: "experimental", Description: "Experimental endpoints"})

		spec.Route(r.HandleFunc("/users", dummyHandler).Methods(http.MethodGet)).
			Tags("users")

		doc := spec.Build(r)

		require.Len(t, doc.Tags, 2)
		assert.Equal(t, "experimental", doc.Tags[0].Name)
		assert.Equal(t, "Experimental endpoints", doc.Tags[0].Description)
		assert.Equal(t, "users", doc.Tags[1].Name)
	})
}

func TestBuildComponents(t *testing.T) {
	t.Run("responses in components", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			AddComponentResponse("NotFound", &Response{Description: "Not found"})

		spec.Route(r.HandleFunc("/health", dummyHandler).Methods(http.MethodGet)).
			Summary("Health").
			Response(http.StatusOK, nil)

		doc := spec.Build(r)
		require.NotNil(t, doc.Components)
		assert.Contains(t, doc.Components.Responses, "NotFound")
	})

	t.Run("parameters in components", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			AddComponentParameter("pageParam", &Parameter{Name: "page", In: "query", Schema: &Schema{Type: TypeString("integer")}})

		spec.Route(r.HandleFunc("/health", dummyHandler).Methods(http.MethodGet)).
			Summary("Health").
			Response(http.StatusOK, nil)

		doc := spec.Build(r)
		require.NotNil(t, doc.Components)
		assert.Contains(t, doc.Components.Parameters, "pageParam")
	})

	t.Run("examples in components", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			AddComponentExample("sample", &Example{Summary: "A sample", Value: "test"})

		spec.Route(r.HandleFunc("/health", dummyHandler).Methods(http.MethodGet)).
			Summary("Health").
			Response(http.StatusOK, nil)

		doc := spec.Build(r)
		require.NotNil(t, doc.Components)
		assert.Contains(t, doc.Components.Examples, "sample")
	})

	t.Run("request bodies in components", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			AddComponentRequestBody("CreatePet", &RequestBody{Description: "Pet to create"})

		spec.Route(r.HandleFunc("/health", dummyHandler).Methods(http.MethodGet)).
			Summary("Health").
			Response(http.StatusOK, nil)

		doc := spec.Build(r)
		require.NotNil(t, doc.Components)
		assert.Contains(t, doc.Components.RequestBodies, "CreatePet")
	})

	t.Run("headers in components", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			AddComponentHeader("X-Rate-Limit", &Header{Schema: &Schema{Type: TypeString("integer")}})

		spec.Route(r.HandleFunc("/health", dummyHandler).Methods(http.MethodGet)).
			Summary("Health").
			Response(http.StatusOK, nil)

		doc := spec.Build(r)
		require.NotNil(t, doc.Components)
		assert.Contains(t, doc.Components.Headers, "X-Rate-Limit")
	})

	t.Run("links in components", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			AddComponentLink("GetUser", &Link{OperationID: "getUser"})

		spec.Route(r.HandleFunc("/health", dummyHandler).Methods(http.MethodGet)).
			Summary("Health").
			Response(http.StatusOK, nil)

		doc := spec.Build(r)
		require.NotNil(t, doc.Components)
		assert.Contains(t, doc.Components.Links, "GetUser")
	})

	t.Run("callbacks in components", func(t *testing.T) {
		r := mux.NewRouter()
		cb := Callback{"{$url}": &PathItem{Post: &Operation{Summary: "cb"}}}
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			AddComponentCallback("onEvent", &cb)

		spec.Route(r.HandleFunc("/health", dummyHandler).Methods(http.MethodGet)).
			Summary("Health").
			Response(http.StatusOK, nil)

		doc := spec.Build(r)
		require.NotNil(t, doc.Components)
		assert.Contains(t, doc.Components.Callbacks, "onEvent")
	})

	t.Run("path items in components", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			AddComponentPathItem("shared", &PathItem{Get: &Operation{Summary: "Shared"}})

		spec.Route(r.HandleFunc("/health", dummyHandler).Methods(http.MethodGet)).
			Summary("Health").
			Response(http.StatusOK, nil)

		doc := spec.Build(r)
		require.NotNil(t, doc.Components)
		assert.Contains(t, doc.Components.PathItems, "shared")
	})

	t.Run("schemas and security schemes coexist", func(t *testing.T) {
		r := mux.NewRouter()

		type Item struct {
			ID string `json:"id"`
		}

		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			AddSecurityScheme("bearerAuth", &SecurityScheme{Type: "http", Scheme: "bearer"})

		spec.Route(r.HandleFunc("/items", dummyHandler).Methods(http.MethodGet)).
			Summary("List items").
			Response(http.StatusOK, []Item{})

		doc := spec.Build(r)
		require.NotNil(t, doc.Components)
		assert.Contains(t, doc.Components.Schemas, "Item")
		assert.Contains(t, doc.Components.SecuritySchemes, "bearerAuth")
	})
}

func TestBuildOperationCallbacks(t *testing.T) {
	t.Run("callbacks appear in operation", func(t *testing.T) {
		r := mux.NewRouter()
		cb := Callback{
			"{$request.body#/callbackUrl}": &PathItem{
				Post: &Operation{Summary: "Webhook notification"},
			},
		}

		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})
		spec.Route(r.HandleFunc("/subscribe", dummyHandler).Methods(http.MethodPost)).
			Summary("Subscribe to events").
			Callback("onEvent", &cb)

		doc := spec.Build(r)
		require.NotNil(t, doc.Paths["/subscribe"].Post.Callbacks)
		assert.Contains(t, doc.Paths["/subscribe"].Post.Callbacks, "onEvent")
	})
}

func TestBuildOperationServers(t *testing.T) {
	t.Run("servers appear in operation", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})
		spec.Route(r.HandleFunc("/upload", dummyHandler).Methods(http.MethodPost)).
			Summary("Upload file").
			Server(Server{URL: "https://upload.example.com", Description: "Upload server"})

		doc := spec.Build(r)
		require.Len(t, doc.Paths["/upload"].Post.Servers, 1)
		assert.Equal(t, "https://upload.example.com", doc.Paths["/upload"].Post.Servers[0].URL)
	})
}

func TestBuildPathServers(t *testing.T) {
	t.Run("path-level server override", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			AddServer(Server{URL: "https://api.example.com", Description: "Main"}).
			AddPathServer("/files", Server{URL: "https://files.example.com", Description: "File server"})

		spec.Route(r.HandleFunc("/files", dummyHandler).Methods(http.MethodGet)).
			Summary("List files")
		spec.Route(r.HandleFunc("/files", dummyHandler).Methods(http.MethodPost)).
			Summary("Upload file")
		spec.Route(r.HandleFunc("/users", dummyHandler).Methods(http.MethodGet)).
			Summary("List users")

		doc := spec.Build(r)

		require.Contains(t, doc.Paths, "/files")
		require.Len(t, doc.Paths["/files"].Servers, 1)
		assert.Equal(t, "https://files.example.com", doc.Paths["/files"].Servers[0].URL)

		require.Contains(t, doc.Paths, "/users")
		assert.Empty(t, doc.Paths["/users"].Servers)
	})

	t.Run("multiple path servers", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			AddPathServer("/files", Server{URL: "https://files1.example.com", Description: "Primary"}).
			AddPathServer("/files", Server{URL: "https://files2.example.com", Description: "Fallback"})

		spec.Route(r.HandleFunc("/files", dummyHandler).Methods(http.MethodGet)).
			Summary("List files")

		doc := spec.Build(r)

		require.Len(t, doc.Paths["/files"].Servers, 2)
		assert.Equal(t, "https://files1.example.com", doc.Paths["/files"].Servers[0].URL)
		assert.Equal(t, "https://files2.example.com", doc.Paths["/files"].Servers[1].URL)
	})

	t.Run("path server with variables", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			AddPathServer("/files", Server{
				URL:         "https://{region}.files.example.com",
				Description: "Regional file server",
				Variables: map[string]*ServerVariable{
					"region": {
						Default: "us-east",
						Enum:    []string{"us-east", "eu-west"},
					},
				},
			})

		spec.Route(r.HandleFunc("/files", dummyHandler).Methods(http.MethodGet)).
			Summary("List files")

		doc := spec.Build(r)

		require.Len(t, doc.Paths["/files"].Servers, 1)
		srv := doc.Paths["/files"].Servers[0]
		assert.Equal(t, "https://{region}.files.example.com", srv.URL)
		require.Contains(t, srv.Variables, "region")
		assert.Equal(t, "us-east", srv.Variables["region"].Default)
		assert.Equal(t, []string{"us-east", "eu-west"}, srv.Variables["region"].Enum)
	})

	t.Run("path server on parameterized path", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			AddPathServer("/users/{id}", Server{URL: "https://users.example.com"})

		spec.Route(r.HandleFunc("/users/{id:uuid}", dummyHandler).Methods(http.MethodGet)).
			Summary("Get user")

		doc := spec.Build(r)

		require.Contains(t, doc.Paths, "/users/{id}")
		require.Len(t, doc.Paths["/users/{id}"].Servers, 1)
		assert.Equal(t, "https://users.example.com", doc.Paths["/users/{id}"].Servers[0].URL)
	})

	t.Run("unmatched path server ignored", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			AddPathServer("/nonexistent", Server{URL: "https://nowhere.example.com"})

		spec.Route(r.HandleFunc("/users", dummyHandler).Methods(http.MethodGet)).
			Summary("List users")

		doc := spec.Build(r)

		require.Contains(t, doc.Paths, "/users")
		assert.Empty(t, doc.Paths["/users"].Servers)
		assert.NotContains(t, doc.Paths, "/nonexistent")
	})
}

func TestAddPathServer(t *testing.T) {
	t.Run("lazy init and chaining", func(t *testing.T) {
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			AddPathServer("/a", Server{URL: "https://a.example.com"}).
			AddPathServer("/b", Server{URL: "https://b.example.com"})

		require.Len(t, spec.pathServers, 2)
		assert.Len(t, spec.pathServers["/a"], 1)
		assert.Len(t, spec.pathServers["/b"], 1)
	})
}

func TestBuildPathSummaryAndDescription(t *testing.T) {
	t.Run("path-level summary and description", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			SetPathSummary("/users/{id}", "Represents a user").
			SetPathDescription("/users/{id}", "Individual user in the system, identified by numeric `id`.")

		spec.Route(r.HandleFunc("/users/{id}", dummyHandler).Methods(http.MethodGet)).
			Summary("Get user")
		spec.Route(r.HandleFunc("/users/{id}", dummyHandler).Methods(http.MethodDelete)).
			Summary("Delete user")

		doc := spec.Build(r)

		require.Contains(t, doc.Paths, "/users/{id}")
		assert.Equal(t, "Represents a user", doc.Paths["/users/{id}"].Summary)
		assert.Equal(t, "Individual user in the system, identified by numeric `id`.", doc.Paths["/users/{id}"].Description)
		assert.NotNil(t, doc.Paths["/users/{id}"].Get)
		assert.NotNil(t, doc.Paths["/users/{id}"].Delete)
	})

	t.Run("unmatched path ignored", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			SetPathSummary("/nonexistent", "Should not appear")

		spec.Route(r.HandleFunc("/users", dummyHandler).Methods(http.MethodGet)).
			Summary("List users")

		doc := spec.Build(r)

		require.Contains(t, doc.Paths, "/users")
		assert.Empty(t, doc.Paths["/users"].Summary)
		assert.NotContains(t, doc.Paths, "/nonexistent")
	})

	t.Run("chaining returns spec", func(t *testing.T) {
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			SetPathSummary("/a", "Summary A").
			SetPathDescription("/a", "Description A").
			SetPathSummary("/b", "Summary B")

		assert.Len(t, spec.pathSummaries, 2)
		assert.Len(t, spec.pathDescriptions, 1)
	})
}

func TestBuildPathParameters(t *testing.T) {
	t.Run("shared path-level parameter", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			AddPathParameter("/users", &Parameter{
				Name:   "X-Tenant-ID",
				In:     "header",
				Schema: &Schema{Type: TypeString("string")},
			})

		spec.Route(r.HandleFunc("/users", dummyHandler).Methods(http.MethodGet)).
			Summary("List users")
		spec.Route(r.HandleFunc("/users", dummyHandler).Methods(http.MethodPost)).
			Summary("Create user")

		doc := spec.Build(r)

		require.Contains(t, doc.Paths, "/users")
		require.Len(t, doc.Paths["/users"].Parameters, 1)
		assert.Equal(t, "X-Tenant-ID", doc.Paths["/users"].Parameters[0].Name)
		assert.Equal(t, "header", doc.Paths["/users"].Parameters[0].In)
	})

	t.Run("multiple path parameters", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			AddPathParameter("/items", &Parameter{
				Name: "X-Tenant-ID", In: "header",
				Schema: &Schema{Type: TypeString("string")},
			}).
			AddPathParameter("/items", &Parameter{
				Name: "Accept-Language", In: "header",
				Schema: &Schema{Type: TypeString("string")},
			})

		spec.Route(r.HandleFunc("/items", dummyHandler).Methods(http.MethodGet)).
			Summary("List items")

		doc := spec.Build(r)

		require.Len(t, doc.Paths["/items"].Parameters, 2)
		assert.Equal(t, "X-Tenant-ID", doc.Paths["/items"].Parameters[0].Name)
		assert.Equal(t, "Accept-Language", doc.Paths["/items"].Parameters[1].Name)
	})

	t.Run("path parameter on parameterized path", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			AddPathParameter("/users/{id}", &Parameter{
				Name: "X-Request-ID", In: "header",
				Schema: &Schema{Type: TypeString("string")},
			})

		spec.Route(r.HandleFunc("/users/{id:uuid}", dummyHandler).Methods(http.MethodGet)).
			Summary("Get user")

		doc := spec.Build(r)

		require.Contains(t, doc.Paths, "/users/{id}")
		require.Len(t, doc.Paths["/users/{id}"].Parameters, 1)
		assert.Equal(t, "X-Request-ID", doc.Paths["/users/{id}"].Parameters[0].Name)
	})

	t.Run("unmatched path parameter ignored", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			AddPathParameter("/nonexistent", &Parameter{Name: "x", In: "header"})

		spec.Route(r.HandleFunc("/users", dummyHandler).Methods(http.MethodGet)).
			Summary("List users")

		doc := spec.Build(r)

		require.Contains(t, doc.Paths, "/users")
		assert.Nil(t, doc.Paths["/users"].Parameters)
	})

	t.Run("combined path metadata", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			SetPathSummary("/users/{id}", "Represents a user").
			SetPathDescription("/users/{id}", "A single user resource.").
			AddPathServer("/users/{id}", Server{URL: "https://users.example.com"}).
			AddPathParameter("/users/{id}", &Parameter{
				Name: "X-Trace-ID", In: "header",
				Schema: &Schema{Type: TypeString("string")},
			})

		spec.Route(r.HandleFunc("/users/{id:uuid}", dummyHandler).Methods(http.MethodGet)).
			Summary("Get user")

		doc := spec.Build(r)

		pi := doc.Paths["/users/{id}"]
		require.NotNil(t, pi)
		assert.Equal(t, "Represents a user", pi.Summary)
		assert.Equal(t, "A single user resource.", pi.Description)
		require.Len(t, pi.Servers, 1)
		assert.Equal(t, "https://users.example.com", pi.Servers[0].URL)
		require.Len(t, pi.Parameters, 1)
		assert.Equal(t, "X-Trace-ID", pi.Parameters[0].Name)
	})
}

func TestBuildTrace(t *testing.T) {
	t.Run("TRACE method route", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})
		spec.Route(r.HandleFunc("/debug", dummyHandler).Methods(http.MethodTrace)).
			Summary("Trace debug")

		doc := spec.Build(r)
		require.Contains(t, doc.Paths, "/debug")
		require.NotNil(t, doc.Paths["/debug"].Trace)
		assert.Equal(t, "Trace debug", doc.Paths["/debug"].Trace.Summary)
	})
}

func TestBuildOperationIDOverride(t *testing.T) {
	t.Run("Route with custom OperationID", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})

		spec.Route(r.HandleFunc("/users", dummyHandler).Methods(http.MethodGet)).
			OperationID("listAllUsers").
			Summary("List users")

		doc := spec.Build(r)

		require.Contains(t, doc.Paths, "/users")
		require.NotNil(t, doc.Paths["/users"].Get)
		assert.Equal(t, "listAllUsers", doc.Paths["/users"].Get.OperationID)
	})

	t.Run("Op with custom OperationID overrides route name", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/users", dummyHandler).Methods(http.MethodGet).Name("listUsers")

		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})
		spec.Op("listUsers").
			OperationID("customListUsers").
			Summary("List users")

		doc := spec.Build(r)

		require.Contains(t, doc.Paths, "/users")
		assert.Equal(t, "customListUsers", doc.Paths["/users"].Get.OperationID)
	})
}

func TestBuildWebhooks(t *testing.T) {
	t.Run("single webhook", func(t *testing.T) {
		r := mux.NewRouter()

		type UserEvent struct {
			UserID string `json:"userId"`
			Action string `json:"action"`
		}

		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})

		spec.Webhook("newUser", http.MethodPost).
			Summary("New user event").
			Tags("webhooks").
			Request(UserEvent{}).
			Response(http.StatusOK, nil)

		spec.Route(r.HandleFunc("/health", dummyHandler).Methods(http.MethodGet)).
			Summary("Health")

		doc := spec.Build(r)

		require.NotNil(t, doc.Webhooks)
		require.Contains(t, doc.Webhooks, "newUser")
		require.NotNil(t, doc.Webhooks["newUser"].Post)
		assert.Equal(t, "New user event", doc.Webhooks["newUser"].Post.Summary)
		assert.Equal(t, []string{"webhooks"}, doc.Webhooks["newUser"].Post.Tags)
		require.NotNil(t, doc.Webhooks["newUser"].Post.RequestBody)
	})

	t.Run("multiple webhooks", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})

		spec.Webhook("userCreated", http.MethodPost).
			Summary("User created event")
		spec.Webhook("userDeleted", http.MethodPost).
			Summary("User deleted event")

		spec.Route(r.HandleFunc("/health", dummyHandler).Methods(http.MethodGet)).
			Summary("Health")

		doc := spec.Build(r)

		require.Len(t, doc.Webhooks, 2)
		assert.Contains(t, doc.Webhooks, "userCreated")
		assert.Contains(t, doc.Webhooks, "userDeleted")
	})

	t.Run("webhook tags merged into document", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})

		spec.Webhook("onEvent", http.MethodPost).
			Tags("webhooks").
			Summary("Event webhook")

		spec.Route(r.HandleFunc("/users", dummyHandler).Methods(http.MethodGet)).
			Tags("users").
			Summary("List users")

		doc := spec.Build(r)

		require.Len(t, doc.Tags, 2)
		assert.Equal(t, "users", doc.Tags[0].Name)
		assert.Equal(t, "webhooks", doc.Tags[1].Name)
	})

	t.Run("no webhooks when none registered", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})

		spec.Route(r.HandleFunc("/health", dummyHandler).Methods(http.MethodGet)).
			Summary("Health")

		doc := spec.Build(r)
		assert.Nil(t, doc.Webhooks)
	})

	t.Run("webhook with group defaults", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})

		events := spec.Group().
			Tags("events").
			Security(SecurityRequirement{"bearerAuth": {}})

		events.Webhook("userCreated", http.MethodPost).
			Summary("User created")

		spec.Route(r.HandleFunc("/health", dummyHandler).Methods(http.MethodGet)).
			Summary("Health")

		doc := spec.Build(r)

		require.Contains(t, doc.Webhooks, "userCreated")
		wh := doc.Webhooks["userCreated"].Post
		assert.Equal(t, []string{"events"}, wh.Tags)
		require.Len(t, wh.Security, 1)
		assert.Contains(t, wh.Security[0], "bearerAuth")
	})

	t.Run("webhook with schemas generates components", func(t *testing.T) {
		r := mux.NewRouter()

		type WebhookPayload struct {
			Event string `json:"event"`
		}

		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})
		spec.Webhook("onEvent", http.MethodPost).
			Request(WebhookPayload{})

		spec.Route(r.HandleFunc("/health", dummyHandler).Methods(http.MethodGet)).
			Summary("Health")

		doc := spec.Build(r)

		require.NotNil(t, doc.Components)
		assert.Contains(t, doc.Components.Schemas, "WebhookPayload")
	})
}

func TestAssignOperation(t *testing.T) {
	t.Run("all HTTP methods", func(t *testing.T) {
		methods := []struct {
			method string
			check  func(*PathItem) *Operation
		}{
			{http.MethodGet, func(pi *PathItem) *Operation { return pi.Get }},
			{http.MethodPost, func(pi *PathItem) *Operation { return pi.Post }},
			{http.MethodPut, func(pi *PathItem) *Operation { return pi.Put }},
			{http.MethodDelete, func(pi *PathItem) *Operation { return pi.Delete }},
			{http.MethodPatch, func(pi *PathItem) *Operation { return pi.Patch }},
			{http.MethodHead, func(pi *PathItem) *Operation { return pi.Head }},
			{http.MethodOptions, func(pi *PathItem) *Operation { return pi.Options }},
			{http.MethodTrace, func(pi *PathItem) *Operation { return pi.Trace }},
		}

		for _, m := range methods {
			t.Run(m.method, func(t *testing.T) {
				pi := &PathItem{}
				op := &Operation{Summary: m.method}
				assignOperation(pi, m.method, op)
				assert.Equal(t, op, m.check(pi))
			})
		}
	})

	t.Run("unknown method is no-op", func(t *testing.T) {
		pi := &PathItem{}
		op := &Operation{Summary: "unknown"}
		assignOperation(pi, "UNKNOWN", op)
		assert.Nil(t, pi.Get)
		assert.Nil(t, pi.Post)
	})
}
