package openapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOperationBuilder(t *testing.T) {
	t.Run("summary and description", func(t *testing.T) {
		b := newOperationBuilder().
			Summary("List users").
			Description("Returns a list of all users")

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "listUsers", nil)

		assert.Equal(t, "listUsers", op.OperationID)
		assert.Equal(t, "List users", op.Summary)
		assert.Equal(t, "Returns a list of all users", op.Description)
	})

	t.Run("tags", func(t *testing.T) {
		b := newOperationBuilder().
			Tags("users", "admin")

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "op1", nil)
		assert.Equal(t, []string{"users", "admin"}, op.Tags)
	})

	t.Run("tags chained", func(t *testing.T) {
		b := newOperationBuilder().
			Tags("users").
			Tags("admin")

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "op1", nil)
		assert.Equal(t, []string{"users", "admin"}, op.Tags)
	})

	t.Run("deprecated", func(t *testing.T) {
		b := newOperationBuilder().Deprecated()

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "op1", nil)
		assert.True(t, op.Deprecated)
	})

	t.Run("request body", func(t *testing.T) {
		type CreateInput struct {
			Name string `json:"name"`
		}
		b := newOperationBuilder().
			Request(CreateInput{})

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "createUser", nil)

		require.NotNil(t, op.RequestBody)
		assert.True(t, op.RequestBody.Required)
		assert.Contains(t, op.RequestBody.Content, "application/json")
		assert.Equal(t, "#/components/schemas/CreateInput", op.RequestBody.Content["application/json"].Schema.Ref)
	})

	t.Run("responses", func(t *testing.T) {
		type User struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		type Error struct {
			Message string `json:"message"`
		}
		b := newOperationBuilder().
			Response(200, User{}).
			Response(400, Error{})

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "getUser", nil)

		require.Len(t, op.Responses, 2)
		assert.Equal(t, "OK", op.Responses["200"].Description)
		assert.Contains(t, op.Responses["200"].Content, "application/json")
		assert.Equal(t, "Bad Request", op.Responses["400"].Description)
	})

	t.Run("response with nil body", func(t *testing.T) {
		b := newOperationBuilder().
			Response(204, nil)

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "deleteUser", nil)

		require.Len(t, op.Responses, 1)
		assert.Equal(t, "No Content", op.Responses["204"].Description)
		assert.Nil(t, op.Responses["204"].Content)
	})

	t.Run("path parameters", func(t *testing.T) {
		b := newOperationBuilder().Summary("Get user")

		pathParams := []*Parameter{
			{
				Name:     "id",
				In:       "path",
				Required: true,
				Schema:   &Schema{Type: TypeString("string"), Format: "uuid"},
			},
		}

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "getUser", pathParams)

		require.Len(t, op.Parameters, 1)
		assert.Equal(t, "id", op.Parameters[0].Name)
		assert.Equal(t, "path", op.Parameters[0].In)
		assert.True(t, op.Parameters[0].Required)
	})

	t.Run("custom parameters appended after path params", func(t *testing.T) {
		b := newOperationBuilder().
			Parameter(&Parameter{
				Name:   "X-Request-ID",
				In:     "header",
				Schema: &Schema{Type: TypeString("string")},
			})

		pathParams := []*Parameter{
			{Name: "id", In: "path", Required: true},
		}

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "op1", pathParams)

		require.Len(t, op.Parameters, 2)
		assert.Equal(t, "id", op.Parameters[0].Name)
		assert.Equal(t, "X-Request-ID", op.Parameters[1].Name)
	})

	t.Run("custom parameter overrides auto-generated path parameter", func(t *testing.T) {
		b := newOperationBuilder().
			Parameter(&Parameter{
				Name:        "id",
				In:          "path",
				Required:    true,
				Description: "User UUID",
				Schema:      &Schema{Type: TypeString("string"), Format: "uuid"},
			})

		pathParams := []*Parameter{
			{Name: "id", In: "path", Required: true},
		}

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "getUser", pathParams)

		// Only one "id" path parameter, no duplicates.
		require.Len(t, op.Parameters, 1)
		assert.Equal(t, "id", op.Parameters[0].Name)
		assert.Equal(t, "User UUID", op.Parameters[0].Description)
		assert.Equal(t, "uuid", op.Parameters[0].Schema.Format)
	})

	t.Run("non-overlapping custom params are appended", func(t *testing.T) {
		b := newOperationBuilder().
			Parameter(&Parameter{
				Name: "page", In: "query",
				Schema: &Schema{Type: TypeString("integer")},
			})

		pathParams := []*Parameter{
			{Name: "id", In: "path", Required: true},
		}

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "op", pathParams)

		require.Len(t, op.Parameters, 2)
		assert.Equal(t, "id", op.Parameters[0].Name)
		assert.Equal(t, "page", op.Parameters[1].Name)
	})

	t.Run("same name different in location are not deduplicated", func(t *testing.T) {
		b := newOperationBuilder().
			Parameter(&Parameter{
				Name: "id", In: "header",
				Schema: &Schema{Type: TypeString("string")},
			})

		pathParams := []*Parameter{
			{Name: "id", In: "path", Required: true},
		}

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "op", pathParams)

		// Both should exist: id in path and id in header.
		require.Len(t, op.Parameters, 2)
		assert.Equal(t, "path", op.Parameters[0].In)
		assert.Equal(t, "header", op.Parameters[1].In)
	})

	t.Run("no parameters when none provided", func(t *testing.T) {
		b := newOperationBuilder().Summary("List all")

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "list", nil)

		assert.Nil(t, op.Parameters)
	})

	t.Run("full fluent chain", func(t *testing.T) {
		type Input struct {
			Name string `json:"name"`
		}
		type Output struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		type ErrorResp struct {
			Error string `json:"error"`
		}
		b := newOperationBuilder().
			Summary("Create resource").
			Description("Creates a new resource").
			Tags("resources").
			Request(Input{}).
			Response(201, Output{}).
			Response(400, ErrorResp{})

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "createResource", nil)

		assert.Equal(t, "Create resource", op.Summary)
		assert.Equal(t, "Creates a new resource", op.Description)
		assert.Equal(t, []string{"resources"}, op.Tags)
		assert.NotNil(t, op.RequestBody)
		assert.Len(t, op.Responses, 2)
		assert.Contains(t, gen.Schemas(), "Input")
		assert.Contains(t, gen.Schemas(), "Output")
		assert.Contains(t, gen.Schemas(), "ErrorResp")
	})

	t.Run("security on operation", func(t *testing.T) {
		b := newOperationBuilder().
			Summary("Public endpoint").
			Security(SecurityRequirement{"apiKey": {}})

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "public", nil)
		require.Len(t, op.Security, 1)
		assert.Contains(t, op.Security[0], "apiKey")
	})

	t.Run("empty security overrides global", func(t *testing.T) {
		b := newOperationBuilder().
			Summary("Public endpoint").
			Security()

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "public", nil)
		assert.NotNil(t, op.Security)
		assert.Empty(t, op.Security)
	})

	t.Run("externalDocs on operation", func(t *testing.T) {
		b := newOperationBuilder().
			Summary("Test op").
			ExternalDocs("https://docs.example.com/test", "Test docs")

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "test", nil)
		require.NotNil(t, op.ExternalDocs)
		assert.Equal(t, "https://docs.example.com/test", op.ExternalDocs.URL)
		assert.Equal(t, "Test docs", op.ExternalDocs.Description)
	})

	t.Run("callback on operation", func(t *testing.T) {
		cb := Callback{
			"{$request.body#/callbackUrl}": &PathItem{
				Post: &Operation{Summary: "Callback received"},
			},
		}
		b := newOperationBuilder().
			Summary("Subscribe").
			Callback("onEvent", &cb)

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "subscribe", nil)
		require.NotNil(t, op.Callbacks)
		assert.Contains(t, op.Callbacks, "onEvent")
	})

	t.Run("server on operation", func(t *testing.T) {
		b := newOperationBuilder().
			Summary("Test").
			Server(Server{URL: "https://override1.example.com", Description: "Override 1"}).
			Server(Server{URL: "https://override2.example.com", Description: "Override 2"})

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "test", nil)
		require.Len(t, op.Servers, 2)
		assert.Equal(t, "https://override1.example.com", op.Servers[0].URL)
		assert.Equal(t, "https://override2.example.com", op.Servers[1].URL)
	})

	t.Run("full chain with new methods", func(t *testing.T) {
		cb := Callback{"{$url}": &PathItem{Post: &Operation{Summary: "cb"}}}
		b := newOperationBuilder().
			Summary("Full operation").
			Description("A full operation").
			Tags("test").
			Deprecated().
			Security(SecurityRequirement{"bearerAuth": {"read"}}).
			ExternalDocs("https://docs.example.com", "Docs").
			Callback("hook", &cb).
			Server(Server{URL: "https://api.example.com", Description: "Main"})

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "fullOp", nil)

		assert.Equal(t, "Full operation", op.Summary)
		assert.True(t, op.Deprecated)
		assert.Len(t, op.Security, 1)
		assert.NotNil(t, op.ExternalDocs)
		assert.Len(t, op.Callbacks, 1)
		assert.Len(t, op.Servers, 1)
	})
}

func TestRequestContent(t *testing.T) {
	type Employee struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}

	t.Run("XML content type", func(t *testing.T) {
		b := newOperationBuilder().
			RequestContent("application/xml", Employee{})

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "createEmployee", nil)

		require.NotNil(t, op.RequestBody)
		assert.True(t, op.RequestBody.Required)
		require.Contains(t, op.RequestBody.Content, "application/xml")
		assert.NotNil(t, op.RequestBody.Content["application/xml"].Schema)
	})

	t.Run("multiple content types", func(t *testing.T) {
		b := newOperationBuilder().
			Request(Employee{}).
			RequestContent("application/xml", Employee{})

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "createEmployee", nil)

		require.NotNil(t, op.RequestBody)
		require.Len(t, op.RequestBody.Content, 2)
		assert.Contains(t, op.RequestBody.Content, "application/json")
		assert.Contains(t, op.RequestBody.Content, "application/xml")
	})

	t.Run("form data", func(t *testing.T) {
		type FileUpload struct {
			Name string `json:"name"`
			File string `json:"file"`
		}
		b := newOperationBuilder().
			RequestContent("multipart/form-data", FileUpload{})

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "upload", nil)

		require.NotNil(t, op.RequestBody)
		require.Contains(t, op.RequestBody.Content, "multipart/form-data")
	})

	t.Run("binary with explicit schema", func(t *testing.T) {
		b := newOperationBuilder().
			RequestContent("application/octet-stream", &Schema{
				Type:   TypeString("string"),
				Format: "binary",
			})

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "upload", nil)

		require.NotNil(t, op.RequestBody)
		require.Contains(t, op.RequestBody.Content, "application/octet-stream")
		schema := op.RequestBody.Content["application/octet-stream"].Schema
		require.NotNil(t, schema)
		assert.Equal(t, TypeString("string"), schema.Type)
		assert.Equal(t, "binary", schema.Format)
	})

	t.Run("nil body produces no schema", func(t *testing.T) {
		b := newOperationBuilder().
			RequestContent("application/octet-stream", nil)

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "upload", nil)

		require.NotNil(t, op.RequestBody)
		require.Contains(t, op.RequestBody.Content, "application/octet-stream")
		assert.Nil(t, op.RequestBody.Content["application/octet-stream"].Schema)
	})

	t.Run("vendor specific type", func(t *testing.T) {
		b := newOperationBuilder().
			RequestContent("application/vnd.mycompany.myapp.v2+json", Employee{})

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "create", nil)

		require.NotNil(t, op.RequestBody)
		assert.Contains(t, op.RequestBody.Content, "application/vnd.mycompany.myapp.v2+json")
	})
}

func TestResponseContent(t *testing.T) {
	type Employee struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}

	t.Run("XML response", func(t *testing.T) {
		b := newOperationBuilder().
			ResponseContent(200, "application/xml", Employee{})

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "getEmployee", nil)

		require.Contains(t, op.Responses, "200")
		require.Contains(t, op.Responses["200"].Content, "application/xml")
		assert.NotNil(t, op.Responses["200"].Content["application/xml"].Schema)
	})

	t.Run("multiple content types for same status", func(t *testing.T) {
		b := newOperationBuilder().
			Response(200, Employee{}).
			ResponseContent(200, "application/xml", Employee{})

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "getEmployee", nil)

		require.Contains(t, op.Responses, "200")
		require.Len(t, op.Responses["200"].Content, 2)
		assert.Contains(t, op.Responses["200"].Content, "application/json")
		assert.Contains(t, op.Responses["200"].Content, "application/xml")
	})

	t.Run("binary response with explicit schema", func(t *testing.T) {
		b := newOperationBuilder().
			ResponseContent(200, "image/png", &Schema{
				Type:   TypeString("string"),
				Format: "binary",
			})

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "getAvatar", nil)

		require.Contains(t, op.Responses, "200")
		require.Contains(t, op.Responses["200"].Content, "image/png")
		schema := op.Responses["200"].Content["image/png"].Schema
		require.NotNil(t, schema)
		assert.Equal(t, TypeString("string"), schema.Type)
		assert.Equal(t, "binary", schema.Format)
	})

	t.Run("text plain response", func(t *testing.T) {
		b := newOperationBuilder().
			ResponseContent(200, "text/plain", &Schema{
				Type: TypeString("string"),
			})

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "getText", nil)

		require.Contains(t, op.Responses, "200")
		require.Contains(t, op.Responses["200"].Content, "text/plain")
	})

	t.Run("wildcard content type", func(t *testing.T) {
		b := newOperationBuilder().
			ResponseContent(200, "image/*", &Schema{
				Type:   TypeString("string"),
				Format: "binary",
			})

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "getImage", nil)

		require.Contains(t, op.Responses, "200")
		assert.Contains(t, op.Responses["200"].Content, "image/*")
	})

	t.Run("nil body with content type", func(t *testing.T) {
		b := newOperationBuilder().
			ResponseContent(200, "application/pdf", nil)

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "getPdf", nil)

		require.Contains(t, op.Responses, "200")
		require.Contains(t, op.Responses["200"].Content, "application/pdf")
		assert.Nil(t, op.Responses["200"].Content["application/pdf"].Schema)
	})

	t.Run("no content still works via Response nil", func(t *testing.T) {
		b := newOperationBuilder().
			Response(204, nil)

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "deleteItem", nil)

		require.Contains(t, op.Responses, "204")
		assert.Equal(t, "No Content", op.Responses["204"].Description)
		assert.Nil(t, op.Responses["204"].Content)
	})

	t.Run("mixed Response and ResponseContent", func(t *testing.T) {
		b := newOperationBuilder().
			Response(200, Employee{}).
			ResponseContent(200, "application/xml", Employee{}).
			Response(204, nil).
			ResponseContent(400, "application/json", nil)

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "op", nil)

		require.Len(t, op.Responses, 3)

		require.Len(t, op.Responses["200"].Content, 2)
		assert.Contains(t, op.Responses["200"].Content, "application/json")
		assert.Contains(t, op.Responses["200"].Content, "application/xml")

		assert.Nil(t, op.Responses["204"].Content)

		require.Len(t, op.Responses["400"].Content, 1)
		assert.Contains(t, op.Responses["400"].Content, "application/json")
	})
}

func TestDefaultResponse(t *testing.T) {
	type ErrorBody struct {
		Message string `json:"message"`
	}

	t.Run("default response with body", func(t *testing.T) {
		b := newOperationBuilder().
			Response(200, nil).
			DefaultResponse(ErrorBody{})

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "op", nil)

		require.Len(t, op.Responses, 2)
		require.Contains(t, op.Responses, "default")
		assert.Equal(t, "Default response", op.Responses["default"].Description)
		require.Contains(t, op.Responses["default"].Content, "application/json")
	})

	t.Run("default response with nil body", func(t *testing.T) {
		b := newOperationBuilder().
			DefaultResponse(nil)

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "op", nil)

		require.Contains(t, op.Responses, "default")
		assert.Nil(t, op.Responses["default"].Content)
	})

	t.Run("default response content with custom type", func(t *testing.T) {
		b := newOperationBuilder().
			DefaultResponseContent("application/xml", ErrorBody{})

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "op", nil)

		require.Contains(t, op.Responses, "default")
		require.Contains(t, op.Responses["default"].Content, "application/xml")
	})

	t.Run("default response alongside numeric responses", func(t *testing.T) {
		b := newOperationBuilder().
			Response(200, nil).
			Response(404, ErrorBody{}).
			DefaultResponse(ErrorBody{})

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "op", nil)

		require.Len(t, op.Responses, 3)
		assert.Contains(t, op.Responses, "200")
		assert.Contains(t, op.Responses, "404")
		assert.Contains(t, op.Responses, "default")
	})
}

func TestResponseHeader(t *testing.T) {
	t.Run("single header on response", func(t *testing.T) {
		b := newOperationBuilder().
			Response(200, nil).
			ResponseHeader(200, "X-Rate-Limit", &Header{
				Description: "Rate limit",
				Schema:      &Schema{Type: TypeString("integer")},
			})

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "op", nil)

		require.Contains(t, op.Responses, "200")
		require.Contains(t, op.Responses["200"].Headers, "X-Rate-Limit")
		assert.Equal(t, "Rate limit", op.Responses["200"].Headers["X-Rate-Limit"].Description)
	})

	t.Run("multiple headers on response", func(t *testing.T) {
		b := newOperationBuilder().
			Response(200, nil).
			ResponseHeader(200, "X-Rate-Limit", &Header{
				Schema: &Schema{Type: TypeString("integer")},
			}).
			ResponseHeader(200, "X-Rate-Remaining", &Header{
				Schema: &Schema{Type: TypeString("integer")},
			})

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "op", nil)

		require.Contains(t, op.Responses, "200")
		require.Len(t, op.Responses["200"].Headers, 2)
		assert.Contains(t, op.Responses["200"].Headers, "X-Rate-Limit")
		assert.Contains(t, op.Responses["200"].Headers, "X-Rate-Remaining")
	})

	t.Run("headers on different status codes", func(t *testing.T) {
		b := newOperationBuilder().
			Response(200, nil).
			Response(429, nil).
			ResponseHeader(200, "X-Request-ID", &Header{
				Schema: &Schema{Type: TypeString("string")},
			}).
			ResponseHeader(429, "Retry-After", &Header{
				Schema: &Schema{Type: TypeString("integer")},
			})

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "op", nil)

		require.Contains(t, op.Responses["200"].Headers, "X-Request-ID")
		require.Contains(t, op.Responses["429"].Headers, "Retry-After")
		assert.NotContains(t, op.Responses["200"].Headers, "Retry-After")
	})
}

func TestResponseLink(t *testing.T) {
	t.Run("single link on response", func(t *testing.T) {
		b := newOperationBuilder().
			Response(201, nil).
			ResponseLink(201, "GetUser", &Link{
				OperationID: "getUser",
				Description: "Get the created user",
			})

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "op", nil)

		require.Contains(t, op.Responses, "201")
		require.Contains(t, op.Responses["201"].Links, "GetUser")
		assert.Equal(t, "getUser", op.Responses["201"].Links["GetUser"].OperationID)
	})

	t.Run("link with parameters", func(t *testing.T) {
		b := newOperationBuilder().
			Response(200, nil).
			ResponseLink(200, "GetNext", &Link{
				OperationID: "listUsers",
				Parameters:  map[string]any{"page": "$response.body#/nextPage"},
			})

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "op", nil)

		require.Contains(t, op.Responses["200"].Links, "GetNext")
		assert.Equal(t, "$response.body#/nextPage", op.Responses["200"].Links["GetNext"].Parameters["page"])
	})

	t.Run("headers and links on same response", func(t *testing.T) {
		b := newOperationBuilder().
			Response(200, nil).
			ResponseHeader(200, "X-Total", &Header{
				Schema: &Schema{Type: TypeString("integer")},
			}).
			ResponseLink(200, "GetNext", &Link{OperationID: "listNext"})

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "op", nil)

		require.Contains(t, op.Responses["200"].Headers, "X-Total")
		require.Contains(t, op.Responses["200"].Links, "GetNext")
	})
}

func TestRequestDescription(t *testing.T) {
	t.Run("sets description on request body", func(t *testing.T) {
		type Input struct {
			Name string `json:"name"`
		}
		b := newOperationBuilder().
			Request(Input{}).
			RequestDescription("The user to create")

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "op", nil)

		require.NotNil(t, op.RequestBody)
		assert.Equal(t, "The user to create", op.RequestBody.Description)
		assert.True(t, op.RequestBody.Required)
	})

	t.Run("default has no description", func(t *testing.T) {
		type Input struct {
			Name string `json:"name"`
		}
		b := newOperationBuilder().
			Request(Input{})

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "op", nil)

		require.NotNil(t, op.RequestBody)
		assert.Empty(t, op.RequestBody.Description)
	})
}

func TestRequestRequired(t *testing.T) {
	t.Run("default is required", func(t *testing.T) {
		type Input struct {
			Name string `json:"name"`
		}
		b := newOperationBuilder().
			Request(Input{})

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "op", nil)

		require.NotNil(t, op.RequestBody)
		assert.True(t, op.RequestBody.Required)
	})

	t.Run("explicit optional", func(t *testing.T) {
		type Input struct {
			Name string `json:"name"`
		}
		b := newOperationBuilder().
			Request(Input{}).
			RequestRequired(false)

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "op", nil)

		require.NotNil(t, op.RequestBody)
		assert.False(t, op.RequestBody.Required)
	})

	t.Run("explicit required", func(t *testing.T) {
		type Input struct {
			Name string `json:"name"`
		}
		b := newOperationBuilder().
			Request(Input{}).
			RequestRequired(true)

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "op", nil)

		require.NotNil(t, op.RequestBody)
		assert.True(t, op.RequestBody.Required)
	})
}

func TestResponseDescription(t *testing.T) {
	t.Run("numeric status code", func(t *testing.T) {
		assert.Equal(t, "OK", responseDescription("200"))
		assert.Equal(t, "Not Found", responseDescription("404"))
	})

	t.Run("default key", func(t *testing.T) {
		assert.Equal(t, "Default response", responseDescription("default"))
	})

	t.Run("unknown code", func(t *testing.T) {
		assert.Equal(t, "999", responseDescription("999"))
	})
}

func TestOperationID(t *testing.T) {
	t.Run("overrides route name", func(t *testing.T) {
		b := newOperationBuilder().
			OperationID("customID").
			Summary("Test")

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "routeName", nil)
		assert.Equal(t, "customID", op.OperationID)
	})

	t.Run("uses route name when not set", func(t *testing.T) {
		b := newOperationBuilder().Summary("Test")

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "routeName", nil)
		assert.Equal(t, "routeName", op.OperationID)
	})

	t.Run("overrides empty route name", func(t *testing.T) {
		b := newOperationBuilder().
			OperationID("myOp").
			Summary("Test")

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "", nil)
		assert.Equal(t, "myOp", op.OperationID)
	})
}

func TestCustomResponseDescription(t *testing.T) {
	t.Run("override status code description", func(t *testing.T) {
		b := newOperationBuilder().
			Response(200, nil).
			ResponseDescription(200, "Successful user retrieval")

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "op", nil)

		require.Contains(t, op.Responses, "200")
		assert.Equal(t, "Successful user retrieval", op.Responses["200"].Description)
	})

	t.Run("default auto-generated when not overridden", func(t *testing.T) {
		b := newOperationBuilder().
			Response(200, nil)

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "op", nil)

		require.Contains(t, op.Responses, "200")
		assert.Equal(t, "OK", op.Responses["200"].Description)
	})

	t.Run("override default response description", func(t *testing.T) {
		b := newOperationBuilder().
			DefaultResponse(nil).
			DefaultResponseDescription("Unexpected error")

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "op", nil)

		require.Contains(t, op.Responses, "default")
		assert.Equal(t, "Unexpected error", op.Responses["default"].Description)
	})

	t.Run("partial override leaves others intact", func(t *testing.T) {
		b := newOperationBuilder().
			Response(200, nil).
			Response(404, nil).
			ResponseDescription(200, "Custom OK")

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "op", nil)

		assert.Equal(t, "Custom OK", op.Responses["200"].Description)
		assert.Equal(t, "Not Found", op.Responses["404"].Description)
	})
}

func TestDefaultResponseHeader(t *testing.T) {
	t.Run("header on default response", func(t *testing.T) {
		b := newOperationBuilder().
			DefaultResponse(nil).
			DefaultResponseHeader("X-Request-ID", &Header{
				Description: "Request tracking ID",
				Schema:      &Schema{Type: TypeString("string")},
			})

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "op", nil)

		require.Contains(t, op.Responses, "default")
		require.Contains(t, op.Responses["default"].Headers, "X-Request-ID")
		assert.Equal(t, "Request tracking ID", op.Responses["default"].Headers["X-Request-ID"].Description)
	})

	t.Run("multiple headers on default response", func(t *testing.T) {
		b := newOperationBuilder().
			DefaultResponse(nil).
			DefaultResponseHeader("X-Request-ID", &Header{
				Schema: &Schema{Type: TypeString("string")},
			}).
			DefaultResponseHeader("X-Error-Code", &Header{
				Schema: &Schema{Type: TypeString("integer")},
			})

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "op", nil)

		require.Len(t, op.Responses["default"].Headers, 2)
		assert.Contains(t, op.Responses["default"].Headers, "X-Request-ID")
		assert.Contains(t, op.Responses["default"].Headers, "X-Error-Code")
	})
}

func TestDefaultResponseLink(t *testing.T) {
	t.Run("link on default response", func(t *testing.T) {
		b := newOperationBuilder().
			DefaultResponse(nil).
			DefaultResponseLink("GetError", &Link{
				OperationID: "getErrorDetails",
				Description: "Get error details",
			})

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "op", nil)

		require.Contains(t, op.Responses, "default")
		require.Contains(t, op.Responses["default"].Links, "GetError")
		assert.Equal(t, "getErrorDetails", op.Responses["default"].Links["GetError"].OperationID)
	})

	t.Run("headers and links on default response", func(t *testing.T) {
		b := newOperationBuilder().
			DefaultResponse(nil).
			DefaultResponseHeader("X-Error-Code", &Header{
				Schema: &Schema{Type: TypeString("integer")},
			}).
			DefaultResponseLink("GetError", &Link{OperationID: "getError"})

		gen := NewSchemaGenerator()
		op := b.buildOperation(gen, "op", nil)

		require.Contains(t, op.Responses["default"].Headers, "X-Error-Code")
		require.Contains(t, op.Responses["default"].Links, "GetError")
	})
}

func TestResolveSchema(t *testing.T) {
	t.Run("nil body returns nil", func(t *testing.T) {
		gen := NewSchemaGenerator()
		assert.Nil(t, resolveSchema(gen, nil))
	})

	t.Run("explicit schema passed through", func(t *testing.T) {
		gen := NewSchemaGenerator()
		s := &Schema{Type: TypeString("string"), Format: "binary"}
		result := resolveSchema(gen, s)
		assert.Same(t, s, result)
	})

	t.Run("go type generates schema", func(t *testing.T) {
		type Item struct {
			ID string `json:"id"`
		}
		gen := NewSchemaGenerator()
		result := resolveSchema(gen, Item{})
		assert.NotNil(t, result)
		assert.Contains(t, gen.Schemas(), "Item")
	})
}
