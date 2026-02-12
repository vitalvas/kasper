package openapi

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/kasper/mux"
)

func TestRouteGroup(t *testing.T) {
	t.Run("tags from group applied", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})

		g := spec.Group().Tags("users")

		g.Route(r.HandleFunc("/users", dummyHandler).Methods(http.MethodGet)).
			Summary("List users")

		doc := spec.Build(r)

		require.NotNil(t, doc.Paths["/users"].Get)
		assert.Equal(t, []string{"users"}, doc.Paths["/users"].Get.Tags)
	})

	t.Run("tags merge", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})

		g := spec.Group().Tags("users")

		g.Route(r.HandleFunc("/users/admin", dummyHandler).Methods(http.MethodGet)).
			Summary("Admin users").
			Tags("admin")

		doc := spec.Build(r)

		require.NotNil(t, doc.Paths["/users/admin"].Get)
		assert.Equal(t, []string{"users", "admin"}, doc.Paths["/users/admin"].Get.Tags)
	})

	t.Run("security from group", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})

		g := spec.Group().Security(SecurityRequirement{"basic": {}})

		g.Route(r.HandleFunc("/users", dummyHandler).Methods(http.MethodGet)).
			Summary("List users")

		doc := spec.Build(r)

		require.NotNil(t, doc.Paths["/users"].Get)
		require.Len(t, doc.Paths["/users"].Get.Security, 1)
		assert.Contains(t, doc.Paths["/users"].Get.Security[0], "basic")
	})

	t.Run("security override", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})

		g := spec.Group().Security(SecurityRequirement{"basic": {}})

		g.Route(r.HandleFunc("/users", dummyHandler).Methods(http.MethodGet)).
			Summary("List users").
			Security(SecurityRequirement{"oauth2": {"read"}})

		doc := spec.Build(r)

		require.NotNil(t, doc.Paths["/users"].Get)
		require.Len(t, doc.Paths["/users"].Get.Security, 1)
		assert.Contains(t, doc.Paths["/users"].Get.Security[0], "oauth2")
	})

	t.Run("empty security override", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})

		g := spec.Group().Security(SecurityRequirement{"basic": {}})

		g.Route(r.HandleFunc("/health", dummyHandler).Methods(http.MethodGet)).
			Summary("Health check").
			Security()

		doc := spec.Build(r)

		require.NotNil(t, doc.Paths["/health"].Get)
		assert.NotNil(t, doc.Paths["/health"].Get.Security)
		assert.Empty(t, doc.Paths["/health"].Get.Security)
	})

	t.Run("deprecated from group", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})

		g := spec.Group().Deprecated()

		g.Route(r.HandleFunc("/old", dummyHandler).Methods(http.MethodGet)).
			Summary("Old endpoint")

		doc := spec.Build(r)

		require.NotNil(t, doc.Paths["/old"].Get)
		assert.True(t, doc.Paths["/old"].Get.Deprecated)
	})

	t.Run("servers from group", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})

		g := spec.Group().Server(Server{URL: "https://api.example.com", Description: "Main"})

		g.Route(r.HandleFunc("/users", dummyHandler).Methods(http.MethodGet)).
			Summary("List users")

		doc := spec.Build(r)

		require.NotNil(t, doc.Paths["/users"].Get)
		require.Len(t, doc.Paths["/users"].Get.Servers, 1)
		assert.Equal(t, "https://api.example.com", doc.Paths["/users"].Get.Servers[0].URL)
	})

	t.Run("parameters merge", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})

		headerParam := &Parameter{
			Name:   "X-Tenant-ID",
			In:     "header",
			Schema: &Schema{Type: TypeString("string")},
		}
		g := spec.Group().Parameter(headerParam)

		queryParam := &Parameter{
			Name:   "page",
			In:     "query",
			Schema: &Schema{Type: TypeString("integer")},
		}
		g.Route(r.HandleFunc("/users", dummyHandler).Methods(http.MethodGet)).
			Summary("List users").
			Parameter(queryParam)

		doc := spec.Build(r)

		require.NotNil(t, doc.Paths["/users"].Get)
		require.Len(t, doc.Paths["/users"].Get.Parameters, 2)
		assert.Equal(t, "X-Tenant-ID", doc.Paths["/users"].Get.Parameters[0].Name)
		assert.Equal(t, "page", doc.Paths["/users"].Get.Parameters[1].Name)
	})

	t.Run("externalDocs from group", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})

		g := spec.Group().ExternalDocs("https://docs.example.com/users", "User docs")

		g.Route(r.HandleFunc("/users", dummyHandler).Methods(http.MethodGet)).
			Summary("List users")

		doc := spec.Build(r)

		require.NotNil(t, doc.Paths["/users"].Get)
		require.NotNil(t, doc.Paths["/users"].Get.ExternalDocs)
		assert.Equal(t, "https://docs.example.com/users", doc.Paths["/users"].Get.ExternalDocs.URL)
		assert.Equal(t, "User docs", doc.Paths["/users"].Get.ExternalDocs.Description)
	})

	t.Run("externalDocs override", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})

		g := spec.Group().ExternalDocs("https://docs.example.com/group", "Group docs")

		g.Route(r.HandleFunc("/users", dummyHandler).Methods(http.MethodGet)).
			Summary("List users").
			ExternalDocs("https://docs.example.com/users", "User docs")

		doc := spec.Build(r)

		require.NotNil(t, doc.Paths["/users"].Get)
		require.NotNil(t, doc.Paths["/users"].Get.ExternalDocs)
		assert.Equal(t, "https://docs.example.com/users", doc.Paths["/users"].Get.ExternalDocs.URL)
	})

	t.Run("Op named routes", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/users", dummyHandler).Methods(http.MethodGet).Name("listUsers")

		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})
		g := spec.Group().Tags("users").Security(SecurityRequirement{"basic": {}})

		g.Op("listUsers").
			Summary("List users")

		doc := spec.Build(r)

		require.NotNil(t, doc.Paths["/users"].Get)
		assert.Equal(t, []string{"users"}, doc.Paths["/users"].Get.Tags)
		require.Len(t, doc.Paths["/users"].Get.Security, 1)
		assert.Contains(t, doc.Paths["/users"].Get.Security[0], "basic")
	})

	t.Run("multiple independent groups", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})

		users := spec.Group().Tags("users").Security(SecurityRequirement{"basic": {}})
		pets := spec.Group().Tags("pets").Security(SecurityRequirement{"oauth2": {"read"}})

		users.Route(r.HandleFunc("/users", dummyHandler).Methods(http.MethodGet)).
			Summary("List users")

		pets.Route(r.HandleFunc("/pets", dummyHandler).Methods(http.MethodGet)).
			Summary("List pets")

		doc := spec.Build(r)

		require.NotNil(t, doc.Paths["/users"].Get)
		assert.Equal(t, []string{"users"}, doc.Paths["/users"].Get.Tags)
		require.Len(t, doc.Paths["/users"].Get.Security, 1)
		assert.Contains(t, doc.Paths["/users"].Get.Security[0], "basic")

		require.NotNil(t, doc.Paths["/pets"].Get)
		assert.Equal(t, []string{"pets"}, doc.Paths["/pets"].Get.Tags)
		require.Len(t, doc.Paths["/pets"].Get.Security, 1)
		assert.Contains(t, doc.Paths["/pets"].Get.Security[0], "oauth2")
	})

	t.Run("full Build integration", func(t *testing.T) {
		r := mux.NewRouter()

		type User struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}

		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			AddSecurityScheme("bearerAuth", &SecurityScheme{Type: "http", Scheme: "bearer"}).
			SetSecurity(SecurityRequirement{"bearerAuth": {}})

		users := spec.Group().Tags("users")

		users.Route(r.HandleFunc("/users", dummyHandler).Methods(http.MethodGet)).
			Summary("List users").
			Response(http.StatusOK, []User{})

		users.Route(r.HandleFunc("/users/{id:uuid}", dummyHandler).Methods(http.MethodGet)).
			Summary("Get user").
			Response(http.StatusOK, User{})

		spec.Route(r.HandleFunc("/health", dummyHandler).Methods(http.MethodGet)).
			Summary("Health check").
			Tags("health").
			Security().
			Response(http.StatusOK, nil)

		doc := spec.Build(r)

		assert.Equal(t, "3.1.0", doc.OpenAPI)

		require.Contains(t, doc.Paths, "/users")
		require.Contains(t, doc.Paths, "/users/{id}")
		require.Contains(t, doc.Paths, "/health")

		assert.Equal(t, []string{"users"}, doc.Paths["/users"].Get.Tags)
		assert.Equal(t, []string{"users"}, doc.Paths["/users/{id}"].Get.Tags)
		assert.Equal(t, []string{"health"}, doc.Paths["/health"].Get.Tags)

		assert.Nil(t, doc.Paths["/users"].Get.Security)
		assert.NotNil(t, doc.Paths["/health"].Get.Security)
		assert.Empty(t, doc.Paths["/health"].Get.Security)

		require.Len(t, doc.Paths["/users/{id}"].Get.Parameters, 1)
		assert.Equal(t, "id", doc.Paths["/users/{id}"].Get.Parameters[0].Name)

		require.NotNil(t, doc.Components)
		assert.Contains(t, doc.Components.Schemas, "User")
		assert.Contains(t, doc.Components.SecuritySchemes, "bearerAuth")

		require.Len(t, doc.Tags, 2)
		assert.Equal(t, "health", doc.Tags[0].Name)
		assert.Equal(t, "users", doc.Tags[1].Name)
	})

	t.Run("subrouter routes through group", func(t *testing.T) {
		r := mux.NewRouter()
		sub := r.PathPrefix("/api/v1").Subrouter()

		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})
		g := spec.Group().Tags("users")

		g.Route(sub.HandleFunc("/users", dummyHandler).Methods(http.MethodGet)).
			Summary("List users")

		doc := spec.Build(r)

		require.Contains(t, doc.Paths, "/api/v1/users")
		require.NotNil(t, doc.Paths["/api/v1/users"].Get)
		assert.Equal(t, []string{"users"}, doc.Paths["/api/v1/users"].Get.Tags)
	})

	t.Run("unrelated routes unaffected", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})

		g := spec.Group().Tags("users").Security(SecurityRequirement{"basic": {}})

		g.Route(r.HandleFunc("/users", dummyHandler).Methods(http.MethodGet)).
			Summary("List users")

		spec.Route(r.HandleFunc("/health", dummyHandler).Methods(http.MethodGet)).
			Summary("Health check").
			Tags("health")

		doc := spec.Build(r)

		require.NotNil(t, doc.Paths["/users"].Get)
		assert.Equal(t, []string{"users"}, doc.Paths["/users"].Get.Tags)
		require.Len(t, doc.Paths["/users"].Get.Security, 1)

		require.NotNil(t, doc.Paths["/health"].Get)
		assert.Equal(t, []string{"health"}, doc.Paths["/health"].Get.Tags)
		assert.Nil(t, doc.Paths["/health"].Get.Security)
	})

	t.Run("group without security does not set security", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})

		g := spec.Group().Tags("users")

		g.Route(r.HandleFunc("/users", dummyHandler).Methods(http.MethodGet)).
			Summary("List users")

		doc := spec.Build(r)

		require.NotNil(t, doc.Paths["/users"].Get)
		assert.Nil(t, doc.Paths["/users"].Get.Security)
	})

	t.Run("group empty security makes public", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"}).
			SetSecurity(SecurityRequirement{"bearerAuth": {}})

		g := spec.Group().Security()

		g.Route(r.HandleFunc("/public", dummyHandler).Methods(http.MethodGet)).
			Summary("Public endpoint")

		doc := spec.Build(r)

		require.NotNil(t, doc.Paths["/public"].Get)
		assert.NotNil(t, doc.Paths["/public"].Get.Security)
		assert.Empty(t, doc.Paths["/public"].Get.Security)
	})

	t.Run("servers append from group and operation", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})

		g := spec.Group().Server(Server{URL: "https://api1.example.com", Description: "Server 1"})

		g.Route(r.HandleFunc("/upload", dummyHandler).Methods(http.MethodPost)).
			Summary("Upload").
			Server(Server{URL: "https://api2.example.com", Description: "Server 2"})

		doc := spec.Build(r)

		require.NotNil(t, doc.Paths["/upload"].Post)
		require.Len(t, doc.Paths["/upload"].Post.Servers, 2)
		assert.Equal(t, "https://api1.example.com", doc.Paths["/upload"].Post.Servers[0].URL)
		assert.Equal(t, "https://api2.example.com", doc.Paths["/upload"].Post.Servers[1].URL)
	})

	t.Run("group chaining returns group", func(t *testing.T) {
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})

		g := spec.Group().
			Tags("users").
			Security(SecurityRequirement{"basic": {}}).
			Deprecated().
			Server(Server{URL: "https://api.example.com", Description: "Main"}).
			Parameter(&Parameter{Name: "X-Tenant", In: "header"}).
			ExternalDocs("https://docs.example.com", "Docs")

		assert.Equal(t, []string{"users"}, g.defaults.tags)
		assert.True(t, g.defaults.securitySet)
		assert.True(t, g.defaults.deprecated)
		assert.Len(t, g.defaults.servers, 1)
		assert.Len(t, g.defaults.parameters, 1)
		assert.NotNil(t, g.defaults.externalDocs)
	})

	t.Run("shared responses from group", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})

		type ErrResp struct {
			Code string `json:"code"`
		}

		g := spec.Group().
			Tags("api").
			Response(http.StatusForbidden, ErrResp{}).
			Response(http.StatusNotFound, ErrResp{})

		g.Route(r.HandleFunc("/items", dummyHandler).Methods(http.MethodGet)).
			Summary("List items").
			Response(http.StatusOK, []string{})

		doc := spec.Build(r)

		op := doc.Paths["/items"].Get
		require.NotNil(t, op)
		assert.Contains(t, op.Responses, "200")
		assert.Contains(t, op.Responses, "403")
		assert.Contains(t, op.Responses, "404")
		assert.Equal(t, "Forbidden", op.Responses["403"].Description)
		assert.Equal(t, "Not Found", op.Responses["404"].Description)
	})

	t.Run("shared response description from group", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})

		type ErrResp struct {
			Code string `json:"code"`
		}

		g := spec.Group().
			Response(http.StatusForbidden, ErrResp{}).
			ResponseDescription(http.StatusForbidden, "Access denied")

		g.Route(r.HandleFunc("/items", dummyHandler).Methods(http.MethodGet)).
			Summary("List items").
			Response(http.StatusOK, []string{})

		doc := spec.Build(r)

		op := doc.Paths["/items"].Get
		require.NotNil(t, op)
		assert.Equal(t, "Access denied", op.Responses["403"].Description)
	})

	t.Run("operation response overrides group response", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})

		type GenericErr struct {
			Code string `json:"code"`
		}
		type DetailedErr struct {
			Code   string `json:"code"`
			Detail string `json:"detail"`
		}

		g := spec.Group().
			Response(http.StatusNotFound, GenericErr{})

		g.Route(r.HandleFunc("/items/{id}", dummyHandler).Methods(http.MethodGet)).
			Summary("Get item").
			Response(http.StatusOK, map[string]string{}).
			Response(http.StatusNotFound, DetailedErr{})

		doc := spec.Build(r)

		op := doc.Paths["/items/{id}"].Get
		require.NotNil(t, op)
		require.Contains(t, op.Responses, "404")
		schema := op.Responses["404"].Content["application/json"].Schema
		assert.Equal(t, "#/components/schemas/DetailedErr", schema.Ref)
	})

	t.Run("shared response nil body", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})

		g := spec.Group().
			Response(http.StatusNoContent, nil)

		g.Route(r.HandleFunc("/items/{id}", dummyHandler).Methods(http.MethodDelete)).
			Summary("Delete item")

		doc := spec.Build(r)

		op := doc.Paths["/items/{id}"].Delete
		require.NotNil(t, op)
		require.Contains(t, op.Responses, "204")
		assert.Nil(t, op.Responses["204"].Content)
	})

	t.Run("shared response content from group", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})

		type ErrResp struct {
			Code string `json:"code"`
		}

		g := spec.Group().
			Response(http.StatusNotFound, ErrResp{}).
			ResponseContent(http.StatusNotFound, "application/xml", ErrResp{})

		g.Route(r.HandleFunc("/items", dummyHandler).Methods(http.MethodGet)).
			Summary("List items").
			Response(http.StatusOK, []string{})

		doc := spec.Build(r)

		op := doc.Paths["/items"].Get
		require.NotNil(t, op)
		require.Contains(t, op.Responses, "404")
		assert.Contains(t, op.Responses["404"].Content, "application/json")
		assert.Contains(t, op.Responses["404"].Content, "application/xml")
	})

	t.Run("shared response header from group", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})

		type ErrResp struct {
			Code string `json:"code"`
		}

		g := spec.Group().
			Response(http.StatusForbidden, ErrResp{}).
			ResponseHeader(http.StatusForbidden, "X-Request-ID", &Header{
				Schema: &Schema{Type: TypeString("string")},
			})

		g.Route(r.HandleFunc("/items", dummyHandler).Methods(http.MethodGet)).
			Summary("List items").
			Response(http.StatusOK, []string{})

		doc := spec.Build(r)

		op := doc.Paths["/items"].Get
		require.NotNil(t, op)
		require.Contains(t, op.Responses, "403")
		require.Contains(t, op.Responses["403"].Headers, "X-Request-ID")
	})

	t.Run("shared response link from group", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})

		type ErrResp struct {
			Code string `json:"code"`
		}

		g := spec.Group().
			Response(http.StatusNotFound, ErrResp{}).
			ResponseLink(http.StatusNotFound, "Search", &Link{
				OperationID: "search",
			})

		g.Route(r.HandleFunc("/items/{id}", dummyHandler).Methods(http.MethodGet)).
			Summary("Get item").
			Response(http.StatusOK, map[string]string{})

		doc := spec.Build(r)

		op := doc.Paths["/items/{id}"].Get
		require.NotNil(t, op)
		require.Contains(t, op.Responses["404"].Links, "Search")
		assert.Equal(t, "search", op.Responses["404"].Links["Search"].OperationID)
	})

	t.Run("shared default response from group", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})

		type ErrResp struct {
			Code string `json:"code"`
		}

		g := spec.Group().
			DefaultResponse(ErrResp{}).
			DefaultResponseDescription("Unexpected error").
			DefaultResponseHeader("X-Request-ID", &Header{
				Schema: &Schema{Type: TypeString("string"), Format: "uuid"},
			})

		g.Route(r.HandleFunc("/items", dummyHandler).Methods(http.MethodGet)).
			Summary("List items").
			Response(http.StatusOK, []string{})

		doc := spec.Build(r)

		op := doc.Paths["/items"].Get
		require.NotNil(t, op)
		require.Contains(t, op.Responses, "default")
		assert.Equal(t, "Unexpected error", op.Responses["default"].Description)
		assert.Contains(t, op.Responses["default"].Content, "application/json")
		assert.Contains(t, op.Responses["default"].Headers, "X-Request-ID")
	})

	t.Run("shared responses do not leak between groups", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})

		type ErrResp struct {
			Code string `json:"code"`
		}

		g1 := spec.Group().
			Response(http.StatusForbidden, ErrResp{})
		g2 := spec.Group()

		g1.Route(r.HandleFunc("/a", dummyHandler).Methods(http.MethodGet)).
			Summary("A").
			Response(http.StatusOK, nil)

		g2.Route(r.HandleFunc("/b", dummyHandler).Methods(http.MethodGet)).
			Summary("B").
			Response(http.StatusOK, nil)

		doc := spec.Build(r)

		assert.Contains(t, doc.Paths["/a"].Get.Responses, "403")
		assert.NotContains(t, doc.Paths["/b"].Get.Responses, "403")
	})
}
