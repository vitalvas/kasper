package openapi

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestSchemaType(t *testing.T) {
	t.Run("marshal", func(t *testing.T) {
		tests := []struct {
			name     string
			input    SchemaType
			expected string
		}{
			{"single type marshals as string", TypeString("string"), `"string"`},
			{"multiple types marshal as array", TypeArray("string", "null"), `["string","null"]`},
			{"empty type marshals as null", SchemaType{}, "null"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				data, err := json.Marshal(tt.input)
				require.NoError(t, err)
				assert.JSONEq(t, tt.expected, string(data))
			})
		}
	})

	t.Run("unmarshal", func(t *testing.T) {
		tests := []struct {
			name     string
			input    string
			expected []string
			wantErr  bool
		}{
			{"single string", `"integer"`, []string{"integer"}, false},
			{"array", `["string","null"]`, []string{"string", "null"}, false},
			{"invalid", `123`, nil, true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var st SchemaType
				err := json.Unmarshal([]byte(tt.input), &st)
				if tt.wantErr {
					assert.Error(t, err)
				} else {
					require.NoError(t, err)
					assert.Equal(t, tt.expected, st.Values())
				}
			})
		}
	})

	t.Run("IsEmpty", func(t *testing.T) {
		var empty SchemaType
		assert.True(t, empty.IsEmpty())
		assert.False(t, TypeString("string").IsEmpty())
	})
}

func TestDocumentJSON(t *testing.T) {
	t.Run("minimal document", func(t *testing.T) {
		doc := Document{
			OpenAPI: "3.1.0",
			Info: Info{
				Title:   "Test API",
				Version: "1.0.0",
			},
		}
		data, err := json.Marshal(doc)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Equal(t, "3.1.0", parsed["openapi"])
		assert.Equal(t, "Test API", parsed["info"].(map[string]any)["title"])
		assert.Equal(t, "1.0.0", parsed["info"].(map[string]any)["version"])
	})

	t.Run("full document roundtrip", func(t *testing.T) {
		minLen := 1
		doc := Document{
			OpenAPI: "3.1.0",
			Info: Info{
				Title:       "Pet Store",
				Description: "A sample pet store API",
				Version:     "2.0.0",
				Contact:     &Contact{Name: "API Support", Email: "support@example.com"},
				License:     &License{Name: "MIT"},
			},
			Servers: []Server{
				{URL: "https://api.example.com", Description: "Production"},
			},
			Paths: map[string]*PathItem{
				"/pets": {
					Get: &Operation{
						Tags:        []string{"pets"},
						Summary:     "List pets",
						OperationID: "listPets",
						Responses: map[string]*Response{
							"200": {
								Description: "OK",
								Content: map[string]*MediaType{
									"application/json": {
										Schema: &Schema{
											Type: TypeString("array"),
											Items: &Schema{
												Ref: "#/components/schemas/Pet",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			Components: &Components{
				Schemas: map[string]*Schema{
					"Pet": {
						Type: TypeString("object"),
						Properties: map[string]*Schema{
							"name": {
								Type:      TypeString("string"),
								MinLength: &minLen,
							},
						},
						Required: []string{"name"},
					},
				},
			},
			Tags: []Tag{
				{Name: "pets", Description: "Pet operations"},
			},
		}

		data, err := json.Marshal(doc)
		require.NoError(t, err)

		var roundtrip Document
		require.NoError(t, json.Unmarshal(data, &roundtrip))
		assert.Equal(t, doc.OpenAPI, roundtrip.OpenAPI)
		assert.Equal(t, doc.Info.Title, roundtrip.Info.Title)
		assert.Len(t, roundtrip.Servers, 1)
		assert.Len(t, roundtrip.Tags, 1)
		assert.Contains(t, roundtrip.Paths, "/pets")
		assert.NotNil(t, roundtrip.Components)
		assert.Contains(t, roundtrip.Components.Schemas, "Pet")
	})
}

func TestSchemaJSON(t *testing.T) {
	tests := []struct {
		name      string
		schema    Schema
		checkFunc func(*testing.T, []byte, map[string]any)
	}{
		{
			name:   "ref serializes as $ref",
			schema: Schema{Ref: "#/components/schemas/User"},
			checkFunc: func(t *testing.T, raw []byte, _ map[string]any) {
				assert.Contains(t, string(raw), `"$ref"`)
				assert.Contains(t, string(raw), "#/components/schemas/User")
			},
		},
		{
			name:   "nullable type uses array",
			schema: Schema{Type: TypeArray("string", "null")},
			checkFunc: func(t *testing.T, raw []byte, _ map[string]any) {
				assert.Contains(t, string(raw), `["string","null"]`)
			},
		},
		{
			name: "numeric constraints",
			schema: func() Schema {
				lo, hi := 0.0, 150.0
				return Schema{Type: TypeString("integer"), Minimum: &lo, Maximum: &hi}
			}(),
			checkFunc: func(t *testing.T, _ []byte, parsed map[string]any) {
				assert.Equal(t, "integer", parsed["type"])
				assert.Equal(t, 0.0, parsed["minimum"])
				assert.Equal(t, 150.0, parsed["maximum"])
			},
		},
		{
			name: "string constraints",
			schema: func() Schema {
				minLen, maxLen := 1, 100
				return Schema{Type: TypeString("string"), MinLength: &minLen, MaxLength: &maxLen, Pattern: `^[a-z]+$`}
			}(),
			checkFunc: func(t *testing.T, _ []byte, parsed map[string]any) {
				assert.Equal(t, 1.0, parsed["minLength"])
				assert.Equal(t, 100.0, parsed["maxLength"])
				assert.Equal(t, `^[a-z]+$`, parsed["pattern"])
			},
		},
		{
			name:   "enum values",
			schema: Schema{Type: TypeString("string"), Enum: []any{"admin", "user", "guest"}},
			checkFunc: func(t *testing.T, _ []byte, parsed map[string]any) {
				assert.Len(t, parsed["enum"].([]any), 3)
			},
		},
		{
			name:   "deprecated and readOnly",
			schema: Schema{Type: TypeString("string"), Deprecated: true, ReadOnly: true},
			checkFunc: func(t *testing.T, _ []byte, parsed map[string]any) {
				assert.Equal(t, true, parsed["deprecated"])
				assert.Equal(t, true, parsed["readOnly"])
			},
		},
		{
			name:   "omits empty fields",
			schema: Schema{Type: TypeString("string")},
			checkFunc: func(t *testing.T, _ []byte, parsed map[string]any) {
				for _, key := range []string{"properties", "items", "format", "deprecated", "readOnly", "writeOnly"} {
					assert.NotContains(t, parsed, key)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.schema)
			require.NoError(t, err)

			var parsed map[string]any
			require.NoError(t, json.Unmarshal(data, &parsed))
			tt.checkFunc(t, data, parsed)
		})
	}
}

func TestOperationJSON(t *testing.T) {
	t.Run("full operation", func(t *testing.T) {
		op := Operation{
			Tags:        []string{"users"},
			Summary:     "Create user",
			Description: "Creates a new user",
			OperationID: "createUser",
			Parameters: []*Parameter{
				{
					Name:     "X-Request-ID",
					In:       "header",
					Required: false,
					Schema:   &Schema{Type: TypeString("string")},
				},
			},
			RequestBody: &RequestBody{
				Required: true,
				Content: map[string]*MediaType{
					"application/json": {
						Schema: &Schema{Ref: "#/components/schemas/CreateUser"},
					},
				},
			},
			Responses: map[string]*Response{
				"201": {Description: "Created"},
				"400": {Description: "Bad Request"},
			},
		}

		data, err := json.Marshal(op)
		require.NoError(t, err)

		var roundtrip Operation
		require.NoError(t, json.Unmarshal(data, &roundtrip))
		assert.Equal(t, op.Summary, roundtrip.Summary)
		assert.Equal(t, op.OperationID, roundtrip.OperationID)
		assert.Len(t, roundtrip.Parameters, 1)
		assert.NotNil(t, roundtrip.RequestBody)
		assert.Len(t, roundtrip.Responses, 2)
	})
}

func TestPathItemJSON(t *testing.T) {
	t.Run("multiple methods", func(t *testing.T) {
		pi := PathItem{
			Get:  &Operation{Summary: "List"},
			Post: &Operation{Summary: "Create"},
		}
		data, err := json.Marshal(pi)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Contains(t, parsed, "get")
		assert.Contains(t, parsed, "post")
		assert.NotContains(t, parsed, "put")
		assert.NotContains(t, parsed, "delete")
	})
}

func TestSecurityRequirementJSON(t *testing.T) {
	t.Run("marshal", func(t *testing.T) {
		sr := SecurityRequirement{
			"bearerAuth": {},
		}
		data, err := json.Marshal(sr)
		require.NoError(t, err)
		assert.JSONEq(t, `{"bearerAuth":[]}`, string(data))
	})
}

func TestDocumentNewFields(t *testing.T) {
	tests := []struct {
		name      string
		doc       Document
		checkFunc func(*testing.T, map[string]any)
	}{
		{
			name: "jsonSchemaDialect",
			doc: Document{
				OpenAPI:           "3.1.0",
				Info:              Info{Title: "Test", Version: "1.0.0"},
				JSONSchemaDialect: "https://json-schema.org/draft/2020-12/schema",
			},
			checkFunc: func(t *testing.T, parsed map[string]any) {
				assert.Equal(t, "https://json-schema.org/draft/2020-12/schema", parsed["jsonSchemaDialect"])
			},
		},
		{
			name: "webhooks",
			doc: Document{
				OpenAPI: "3.1.0",
				Info:    Info{Title: "Test", Version: "1.0.0"},
				Webhooks: map[string]*PathItem{
					"newPet": {Post: &Operation{Summary: "New pet notification"}},
				},
			},
			checkFunc: func(t *testing.T, parsed map[string]any) {
				webhooks := parsed["webhooks"].(map[string]any)
				assert.Contains(t, webhooks, "newPet")
			},
		},
		{
			name: "externalDocs",
			doc: Document{
				OpenAPI:      "3.1.0",
				Info:         Info{Title: "Test", Version: "1.0.0"},
				ExternalDocs: &ExternalDocs{URL: "https://docs.example.com", Description: "Full docs"},
			},
			checkFunc: func(t *testing.T, parsed map[string]any) {
				extDocs := parsed["externalDocs"].(map[string]any)
				assert.Equal(t, "https://docs.example.com", extDocs["url"])
				assert.Equal(t, "Full docs", extDocs["description"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.doc)
			require.NoError(t, err)

			var parsed map[string]any
			require.NoError(t, json.Unmarshal(data, &parsed))
			tt.checkFunc(t, parsed)
		})
	}
}

func TestInfoNewFields(t *testing.T) {
	t.Run("summary field", func(t *testing.T) {
		info := Info{
			Title:   "Test API",
			Summary: "A brief summary",
			Version: "1.0.0",
		}
		data, err := json.Marshal(info)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Equal(t, "A brief summary", parsed["summary"])
	})
}

func TestLicenseNewFields(t *testing.T) {
	t.Run("identifier field", func(t *testing.T) {
		lic := License{Name: "Apache 2.0", Identifier: "Apache-2.0"}
		data, err := json.Marshal(lic)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Equal(t, "Apache-2.0", parsed["identifier"])
	})
}

func TestServerVariableJSON(t *testing.T) {
	t.Run("full server variable", func(t *testing.T) {
		sv := ServerVariable{
			Enum:        []string{"v1", "v2"},
			Default:     "v1",
			Description: "API version",
		}
		data, err := json.Marshal(sv)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Equal(t, "v1", parsed["default"])
		assert.Equal(t, "API version", parsed["description"])
		assert.Len(t, parsed["enum"].([]any), 2)
	})

	t.Run("server with variables", func(t *testing.T) {
		s := Server{
			URL: "https://{env}.example.com/{version}",
			Variables: map[string]*ServerVariable{
				"env":     {Default: "prod", Enum: []string{"prod", "staging"}},
				"version": {Default: "v1"},
			},
		}
		data, err := json.Marshal(s)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		vars := parsed["variables"].(map[string]any)
		assert.Contains(t, vars, "env")
		assert.Contains(t, vars, "version")
	})
}

func TestPathItemNewFields(t *testing.T) {
	t.Run("ref and trace", func(t *testing.T) {
		pi := PathItem{
			Ref:   "#/components/pathItems/shared",
			Trace: &Operation{Summary: "Trace operation"},
		}
		data, err := json.Marshal(pi)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Equal(t, "#/components/pathItems/shared", parsed["$ref"])
		assert.Contains(t, parsed, "trace")
	})

	t.Run("servers on path item", func(t *testing.T) {
		pi := PathItem{
			Get:     &Operation{Summary: "Get"},
			Servers: []Server{{URL: "https://override.example.com"}},
		}
		data, err := json.Marshal(pi)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		servers := parsed["servers"].([]any)
		assert.Len(t, servers, 1)
	})
}

func TestOperationNewFields(t *testing.T) {
	cb := Callback{
		"https://callback.example.com": &PathItem{
			Post: &Operation{Summary: "Callback received"},
		},
	}

	tests := []struct {
		name         string
		op           Operation
		expectedKeys []string
	}{
		{
			name: "externalDocs on operation",
			op: Operation{
				Summary:      "Test",
				ExternalDocs: &ExternalDocs{URL: "https://docs.example.com"},
			},
			expectedKeys: []string{"externalDocs"},
		},
		{
			name: "callbacks on operation",
			op: Operation{
				Summary:   "Test",
				Callbacks: map[string]*Callback{"onEvent": &cb},
			},
			expectedKeys: []string{"callbacks"},
		},
		{
			name: "servers on operation",
			op: Operation{
				Summary: "Test",
				Servers: []Server{{URL: "https://override.example.com"}},
			},
			expectedKeys: []string{"servers"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.op)
			require.NoError(t, err)

			var parsed map[string]any
			require.NoError(t, json.Unmarshal(data, &parsed))
			for _, key := range tt.expectedKeys {
				assert.Contains(t, parsed, key)
			}
		})
	}
}

func TestParameterNewFields(t *testing.T) {
	t.Run("style and explode", func(t *testing.T) {
		explode := true
		p := Parameter{
			Name:    "ids",
			In:      "query",
			Style:   "form",
			Explode: &explode,
			Schema:  &Schema{Type: TypeString("array"), Items: &Schema{Type: TypeString("string")}},
		}
		data, err := json.Marshal(p)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Equal(t, "form", parsed["style"])
		assert.Equal(t, true, parsed["explode"])
	})

	t.Run("allowEmptyValue and allowReserved", func(t *testing.T) {
		p := Parameter{
			Name:            "q",
			In:              "query",
			AllowEmptyValue: true,
			AllowReserved:   true,
		}
		data, err := json.Marshal(p)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Equal(t, true, parsed["allowEmptyValue"])
		assert.Equal(t, true, parsed["allowReserved"])
	})

	t.Run("examples on parameter", func(t *testing.T) {
		p := Parameter{
			Name: "id",
			In:   "path",
			Examples: map[string]*Example{
				"example1": {Summary: "First example", Value: "abc"},
			},
		}
		data, err := json.Marshal(p)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Contains(t, parsed, "examples")
	})

	t.Run("content on parameter", func(t *testing.T) {
		p := Parameter{
			Name: "filter",
			In:   "query",
			Content: map[string]*MediaType{
				"application/json": {Schema: &Schema{Type: TypeString("object")}},
			},
		}
		data, err := json.Marshal(p)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Contains(t, parsed, "content")
	})
}

func TestResponseNewFields(t *testing.T) {
	t.Run("links on response", func(t *testing.T) {
		resp := Response{
			Description: "OK",
			Links: map[string]*Link{
				"GetUser": {OperationID: "getUser", Parameters: map[string]any{"userId": "$response.body#/id"}},
			},
		}
		data, err := json.Marshal(resp)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		links := parsed["links"].(map[string]any)
		assert.Contains(t, links, "GetUser")
	})
}

func TestMediaTypeNewFields(t *testing.T) {
	t.Run("examples on media type", func(t *testing.T) {
		mt := MediaType{
			Schema: &Schema{Type: TypeString("object")},
			Examples: map[string]*Example{
				"sample": {Summary: "Sample", Value: map[string]any{"id": "123"}},
			},
		}
		data, err := json.Marshal(mt)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Contains(t, parsed, "examples")
	})

	t.Run("encoding on media type", func(t *testing.T) {
		mt := MediaType{
			Schema: &Schema{Type: TypeString("object")},
			Encoding: map[string]*Encoding{
				"profileImage": {ContentType: "image/png"},
			},
		}
		data, err := json.Marshal(mt)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		encoding := parsed["encoding"].(map[string]any)
		assert.Contains(t, encoding, "profileImage")
	})
}

func TestHeaderNewFields(t *testing.T) {
	t.Run("extended header fields", func(t *testing.T) {
		explode := false
		h := Header{
			Description:     "Rate limit",
			Deprecated:      true,
			AllowEmptyValue: true,
			Style:           "simple",
			Explode:         &explode,
			AllowReserved:   true,
			Schema:          &Schema{Type: TypeString("integer")},
			Example:         42,
		}
		data, err := json.Marshal(h)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Equal(t, true, parsed["deprecated"])
		assert.Equal(t, true, parsed["allowEmptyValue"])
		assert.Equal(t, "simple", parsed["style"])
		assert.Equal(t, false, parsed["explode"])
		assert.Equal(t, true, parsed["allowReserved"])
		assert.Equal(t, 42.0, parsed["example"])
	})

	t.Run("examples and content on header", func(t *testing.T) {
		h := Header{
			Examples: map[string]*Example{
				"ex1": {Value: "test"},
			},
			Content: map[string]*MediaType{
				"application/json": {Schema: &Schema{Type: TypeString("string")}},
			},
		}
		data, err := json.Marshal(h)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Contains(t, parsed, "examples")
		assert.Contains(t, parsed, "content")
	})
}

func TestTagNewFields(t *testing.T) {
	t.Run("externalDocs on tag", func(t *testing.T) {
		tag := Tag{
			Name:         "users",
			Description:  "User operations",
			ExternalDocs: &ExternalDocs{URL: "https://docs.example.com/users"},
		}
		data, err := json.Marshal(tag)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		extDocs := parsed["externalDocs"].(map[string]any)
		assert.Equal(t, "https://docs.example.com/users", extDocs["url"])
	})
}

func TestComponentsNewFields(t *testing.T) {
	t.Run("all component types", func(t *testing.T) {
		cb := Callback{
			"https://example.com": &PathItem{Post: &Operation{Summary: "cb"}},
		}
		comp := Components{
			Schemas:   map[string]*Schema{"Pet": {Type: TypeString("object")}},
			Responses: map[string]*Response{"NotFound": {Description: "Not found"}},
			Parameters: map[string]*Parameter{
				"pageParam": {Name: "page", In: "query", Schema: &Schema{Type: TypeString("integer")}},
			},
			Examples:      map[string]*Example{"ex1": {Summary: "Example", Value: "test"}},
			RequestBodies: map[string]*RequestBody{"CreatePet": {Description: "Pet to create"}},
			Headers:       map[string]*Header{"X-Rate-Limit": {Schema: &Schema{Type: TypeString("integer")}}},
			SecuritySchemes: map[string]*SecurityScheme{
				"bearerAuth": {Type: "http", Scheme: "bearer", BearerFormat: "JWT"},
			},
			Links:     map[string]*Link{"GetUser": {OperationID: "getUser"}},
			Callbacks: map[string]*Callback{"onEvent": &cb},
			PathItems: map[string]*PathItem{"shared": {Get: &Operation{Summary: "Shared"}}},
		}
		data, err := json.Marshal(comp)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Contains(t, parsed, "schemas")
		assert.Contains(t, parsed, "responses")
		assert.Contains(t, parsed, "parameters")
		assert.Contains(t, parsed, "examples")
		assert.Contains(t, parsed, "requestBodies")
		assert.Contains(t, parsed, "headers")
		assert.Contains(t, parsed, "securitySchemes")
		assert.Contains(t, parsed, "links")
		assert.Contains(t, parsed, "callbacks")
		assert.Contains(t, parsed, "pathItems")
	})
}

func TestExternalDocsJSON(t *testing.T) {
	tests := []struct {
		name      string
		input     ExternalDocs
		checkFunc func(*testing.T, map[string]any)
	}{
		{
			name:  "full external docs",
			input: ExternalDocs{URL: "https://docs.example.com", Description: "Full docs"},
			checkFunc: func(t *testing.T, parsed map[string]any) {
				assert.Equal(t, "https://docs.example.com", parsed["url"])
				assert.Equal(t, "Full docs", parsed["description"])
			},
		},
		{
			name:  "omits empty description",
			input: ExternalDocs{URL: "https://docs.example.com"},
			checkFunc: func(t *testing.T, parsed map[string]any) {
				assert.NotContains(t, parsed, "description")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.input)
			require.NoError(t, err)

			var parsed map[string]any
			require.NoError(t, json.Unmarshal(data, &parsed))
			tt.checkFunc(t, parsed)
		})
	}
}

func TestExampleJSON(t *testing.T) {
	tests := []struct {
		name      string
		input     Example
		checkFunc func(*testing.T, map[string]any)
	}{
		{
			name: "full example",
			input: Example{
				Summary:     "A sample",
				Description: "A detailed description",
				Value:       map[string]any{"name": "Fido"},
			},
			checkFunc: func(t *testing.T, parsed map[string]any) {
				assert.Equal(t, "A sample", parsed["summary"])
				assert.Equal(t, "A detailed description", parsed["description"])
				assert.Contains(t, parsed, "value")
			},
		},
		{
			name:  "externalValue",
			input: Example{ExternalValue: "https://example.com/sample.json"},
			checkFunc: func(t *testing.T, parsed map[string]any) {
				assert.Equal(t, "https://example.com/sample.json", parsed["externalValue"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.input)
			require.NoError(t, err)

			var parsed map[string]any
			require.NoError(t, json.Unmarshal(data, &parsed))
			tt.checkFunc(t, parsed)
		})
	}
}

func TestEncodingJSON(t *testing.T) {
	t.Run("full encoding", func(t *testing.T) {
		explode := true
		enc := Encoding{
			ContentType:   "image/png",
			Headers:       map[string]*Header{"X-Custom": {Schema: &Schema{Type: TypeString("string")}}},
			Style:         "form",
			Explode:       &explode,
			AllowReserved: true,
		}
		data, err := json.Marshal(enc)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Equal(t, "image/png", parsed["contentType"])
		assert.Equal(t, "form", parsed["style"])
		assert.Equal(t, true, parsed["explode"])
		assert.Equal(t, true, parsed["allowReserved"])
		assert.Contains(t, parsed, "headers")
	})
}

func TestDiscriminatorJSON(t *testing.T) {
	t.Run("with mapping", func(t *testing.T) {
		d := Discriminator{
			PropertyName: "petType",
			Mapping:      map[string]string{"dog": "#/components/schemas/Dog", "cat": "#/components/schemas/Cat"},
		}
		data, err := json.Marshal(d)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Equal(t, "petType", parsed["propertyName"])
		mapping := parsed["mapping"].(map[string]any)
		assert.Equal(t, "#/components/schemas/Dog", mapping["dog"])
	})
}

func TestXMLJSON(t *testing.T) {
	t.Run("full xml", func(t *testing.T) {
		x := XML{
			Name:      "pet",
			Namespace: "http://example.com/schema",
			Prefix:    "ex",
			Attribute: true,
			Wrapped:   true,
		}
		data, err := json.Marshal(x)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Equal(t, "pet", parsed["name"])
		assert.Equal(t, "http://example.com/schema", parsed["namespace"])
		assert.Equal(t, "ex", parsed["prefix"])
		assert.Equal(t, true, parsed["attribute"])
		assert.Equal(t, true, parsed["wrapped"])
	})
}

func TestSecuritySchemeJSON(t *testing.T) {
	tests := []struct {
		name      string
		scheme    SecurityScheme
		checkFunc func(*testing.T, map[string]any)
	}{
		{
			name: "http bearer",
			scheme: SecurityScheme{
				Type:         "http",
				Scheme:       "bearer",
				BearerFormat: "JWT",
				Description:  "Bearer token auth",
			},
			checkFunc: func(t *testing.T, parsed map[string]any) {
				assert.Equal(t, "http", parsed["type"])
				assert.Equal(t, "bearer", parsed["scheme"])
				assert.Equal(t, "JWT", parsed["bearerFormat"])
			},
		},
		{
			name: "apiKey",
			scheme: SecurityScheme{
				Type: "apiKey",
				Name: "X-API-Key",
				In:   "header",
			},
			checkFunc: func(t *testing.T, parsed map[string]any) {
				assert.Equal(t, "apiKey", parsed["type"])
				assert.Equal(t, "X-API-Key", parsed["name"])
				assert.Equal(t, "header", parsed["in"])
			},
		},
		{
			name: "oauth2",
			scheme: SecurityScheme{
				Type: "oauth2",
				Flows: &OAuthFlows{
					AuthorizationCode: &OAuthFlow{
						AuthorizationURL: "https://example.com/oauth/authorize",
						TokenURL:         "https://example.com/oauth/token",
						Scopes:           map[string]string{"read:pets": "Read pets", "write:pets": "Write pets"},
					},
				},
			},
			checkFunc: func(t *testing.T, parsed map[string]any) {
				assert.Equal(t, "oauth2", parsed["type"])
				flows := parsed["flows"].(map[string]any)
				assert.Contains(t, flows, "authorizationCode")
			},
		},
		{
			name: "openIdConnect",
			scheme: SecurityScheme{
				Type:             "openIdConnect",
				OpenIDConnectURL: "https://example.com/.well-known/openid-configuration",
			},
			checkFunc: func(t *testing.T, parsed map[string]any) {
				assert.Equal(t, "openIdConnect", parsed["type"])
				assert.Equal(t, "https://example.com/.well-known/openid-configuration", parsed["openIdConnectUrl"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.scheme)
			require.NoError(t, err)

			var parsed map[string]any
			require.NoError(t, json.Unmarshal(data, &parsed))
			tt.checkFunc(t, parsed)
		})
	}
}

func TestOAuthFlowsJSON(t *testing.T) {
	t.Run("all flow types", func(t *testing.T) {
		flows := OAuthFlows{
			Implicit: &OAuthFlow{
				AuthorizationURL: "https://example.com/oauth/authorize",
				Scopes:           map[string]string{"read": "Read access"},
			},
			Password: &OAuthFlow{
				TokenURL: "https://example.com/oauth/token",
				Scopes:   map[string]string{"admin": "Admin access"},
			},
			ClientCredentials: &OAuthFlow{
				TokenURL: "https://example.com/oauth/token",
				Scopes:   map[string]string{"service": "Service access"},
			},
			AuthorizationCode: &OAuthFlow{
				AuthorizationURL: "https://example.com/oauth/authorize",
				TokenURL:         "https://example.com/oauth/token",
				RefreshURL:       "https://example.com/oauth/refresh",
				Scopes:           map[string]string{"read": "Read", "write": "Write"},
			},
		}
		data, err := json.Marshal(flows)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Contains(t, parsed, "implicit")
		assert.Contains(t, parsed, "password")
		assert.Contains(t, parsed, "clientCredentials")
		assert.Contains(t, parsed, "authorizationCode")

		authCode := parsed["authorizationCode"].(map[string]any)
		assert.Equal(t, "https://example.com/oauth/refresh", authCode["refreshUrl"])
	})
}

func TestLinkJSON(t *testing.T) {
	t.Run("full link", func(t *testing.T) {
		l := Link{
			OperationRef: "#/paths/~1users~1{id}/get",
			OperationID:  "getUser",
			Parameters:   map[string]any{"userId": "$response.body#/id"},
			RequestBody:  "$request.body",
			Description:  "Link to get user",
			Server:       &Server{URL: "https://api.example.com"},
		}
		data, err := json.Marshal(l)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Equal(t, "#/paths/~1users~1{id}/get", parsed["operationRef"])
		assert.Equal(t, "getUser", parsed["operationId"])
		assert.Contains(t, parsed, "parameters")
		assert.Equal(t, "$request.body", parsed["requestBody"])
		assert.Equal(t, "Link to get user", parsed["description"])
		assert.Contains(t, parsed, "server")
	})
}

func TestCallbackJSON(t *testing.T) {
	t.Run("callback serialization", func(t *testing.T) {
		cb := Callback{
			"{$request.body#/callbackUrl}": &PathItem{
				Post: &Operation{Summary: "Callback notification"},
			},
		}
		data, err := json.Marshal(cb)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Contains(t, parsed, "{$request.body#/callbackUrl}")
	})
}

func TestSchemaNewFields(t *testing.T) {
	t.Run("core identifiers", func(t *testing.T) {
		s := Schema{
			ID:            "https://example.com/schemas/pet",
			SchemaURI:     "https://json-schema.org/draft/2020-12/schema",
			DynamicAnchor: "meta",
			Comment:       "This is a comment",
			Defs: map[string]*Schema{
				"address": {Type: TypeString("object")},
			},
		}
		data, err := json.Marshal(s)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Equal(t, "https://example.com/schemas/pet", parsed["$id"])
		assert.Equal(t, "https://json-schema.org/draft/2020-12/schema", parsed["$schema"])
		assert.Equal(t, "meta", parsed["$dynamicAnchor"])
		assert.Equal(t, "This is a comment", parsed["$comment"])
		assert.Contains(t, parsed, "$defs")
	})

	t.Run("title and multipleOf", func(t *testing.T) {
		mul := 0.01
		s := Schema{
			Type:       TypeString("number"),
			Title:      "Price",
			MultipleOf: &mul,
		}
		data, err := json.Marshal(s)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Equal(t, "Price", parsed["title"])
		assert.Equal(t, 0.01, parsed["multipleOf"])
	})

	t.Run("array constraints", func(t *testing.T) {
		minItems := 1
		maxItems := 10
		s := Schema{
			Type:        TypeString("array"),
			Items:       &Schema{Type: TypeString("string")},
			PrefixItems: []*Schema{{Type: TypeString("integer")}, {Type: TypeString("string")}},
			Contains:    &Schema{Type: TypeString("integer")},
			MinItems:    &minItems,
			MaxItems:    &maxItems,
			UniqueItems: true,
			UnevaluatedItems: &Schema{
				Type: TypeString("boolean"),
			},
		}
		data, err := json.Marshal(s)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Equal(t, 1.0, parsed["minItems"])
		assert.Equal(t, 10.0, parsed["maxItems"])
		assert.Equal(t, true, parsed["uniqueItems"])
		assert.Len(t, parsed["prefixItems"].([]any), 2)
		assert.Contains(t, parsed, "contains")
		assert.Contains(t, parsed, "unevaluatedItems")
	})

	t.Run("object constraints", func(t *testing.T) {
		minProps := 1
		maxProps := 10
		s := Schema{
			Type: TypeString("object"),
			Properties: map[string]*Schema{
				"name": {Type: TypeString("string")},
			},
			PatternProperties: map[string]*Schema{
				"^x-": {Type: TypeString("string")},
			},
			PropertyNames:         &Schema{Pattern: "^[a-z]+$"},
			UnevaluatedProperties: &Schema{Type: TypeString("boolean")},
			MinProperties:         &minProps,
			MaxProperties:         &maxProps,
			DependentRequired: map[string][]string{
				"creditCard": {"billingAddress"},
			},
			DependentSchemas: map[string]*Schema{
				"creditCard": {
					Properties: map[string]*Schema{
						"billingAddress": {Type: TypeString("string")},
					},
					Required: []string{"billingAddress"},
				},
			},
		}
		data, err := json.Marshal(s)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Equal(t, 1.0, parsed["minProperties"])
		assert.Equal(t, 10.0, parsed["maxProperties"])
		assert.Contains(t, parsed, "patternProperties")
		assert.Contains(t, parsed, "propertyNames")
		assert.Contains(t, parsed, "unevaluatedProperties")
		assert.Contains(t, parsed, "dependentRequired")
		assert.Contains(t, parsed, "dependentSchemas")
	})

	t.Run("not schema", func(t *testing.T) {
		s := Schema{
			Not: &Schema{Type: TypeString("null")},
		}
		data, err := json.Marshal(s)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Contains(t, parsed, "not")
	})

	t.Run("const value", func(t *testing.T) {
		s := Schema{
			Const: "fixed_value",
		}
		data, err := json.Marshal(s)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Equal(t, "fixed_value", parsed["const"])
	})

	t.Run("if then else", func(t *testing.T) {
		s := Schema{
			If: &Schema{
				Properties: map[string]*Schema{
					"country": {Const: "US"},
				},
			},
			Then: &Schema{
				Properties: map[string]*Schema{
					"postalCode": {Pattern: `^\d{5}(-\d{4})?$`},
				},
			},
			Else: &Schema{
				Properties: map[string]*Schema{
					"postalCode": {Pattern: `^[A-Z0-9 -]+$`},
				},
			},
		}
		data, err := json.Marshal(s)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Contains(t, parsed, "if")
		assert.Contains(t, parsed, "then")
		assert.Contains(t, parsed, "else")
	})

	t.Run("content encoding", func(t *testing.T) {
		s := Schema{
			ContentEncoding:  "base64",
			ContentMediaType: "image/png",
			ContentSchema:    &Schema{Type: TypeString("string")},
		}
		data, err := json.Marshal(s)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Equal(t, "base64", parsed["contentEncoding"])
		assert.Equal(t, "image/png", parsed["contentMediaType"])
		assert.Contains(t, parsed, "contentSchema")
	})

	t.Run("discriminator and xml", func(t *testing.T) {
		s := Schema{
			Type: TypeString("object"),
			Discriminator: &Discriminator{
				PropertyName: "petType",
				Mapping:      map[string]string{"dog": "#/components/schemas/Dog"},
			},
			XML: &XML{Name: "Pet", Wrapped: true},
		}
		data, err := json.Marshal(s)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Contains(t, parsed, "discriminator")
		assert.Contains(t, parsed, "xml")
	})

	t.Run("examples array", func(t *testing.T) {
		s := Schema{
			Type:     TypeString("string"),
			Examples: []any{"hello", "world"},
		}
		data, err := json.Marshal(s)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		examples := parsed["examples"].([]any)
		assert.Len(t, examples, 2)
	})

	t.Run("omits new empty fields", func(t *testing.T) {
		s := Schema{Type: TypeString("string")}
		data, err := json.Marshal(s)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.NotContains(t, parsed, "title")
		assert.NotContains(t, parsed, "multipleOf")
		assert.NotContains(t, parsed, "minItems")
		assert.NotContains(t, parsed, "maxItems")
		assert.NotContains(t, parsed, "uniqueItems")
		assert.NotContains(t, parsed, "prefixItems")
		assert.NotContains(t, parsed, "contains")
		assert.NotContains(t, parsed, "not")
		assert.NotContains(t, parsed, "if")
		assert.NotContains(t, parsed, "then")
		assert.NotContains(t, parsed, "else")
		assert.NotContains(t, parsed, "const")
		assert.NotContains(t, parsed, "$id")
		assert.NotContains(t, parsed, "$schema")
		assert.NotContains(t, parsed, "$comment")
		assert.NotContains(t, parsed, "$defs")
		assert.NotContains(t, parsed, "$dynamicAnchor")
		assert.NotContains(t, parsed, "discriminator")
		assert.NotContains(t, parsed, "xml")
		assert.NotContains(t, parsed, "contentEncoding")
		assert.NotContains(t, parsed, "contentMediaType")
		assert.NotContains(t, parsed, "examples")
		assert.NotContains(t, parsed, "patternProperties")
		assert.NotContains(t, parsed, "propertyNames")
		assert.NotContains(t, parsed, "unevaluatedProperties")
		assert.NotContains(t, parsed, "unevaluatedItems")
		assert.NotContains(t, parsed, "minProperties")
		assert.NotContains(t, parsed, "maxProperties")
		assert.NotContains(t, parsed, "dependentRequired")
		assert.NotContains(t, parsed, "dependentSchemas")
	})
}

func TestSchemaTypeYAML(t *testing.T) {
	t.Run("marshal", func(t *testing.T) {
		tests := []struct {
			name     string
			input    SchemaType
			expected string
		}{
			{"single type marshals as scalar", TypeString("string"), "string\n"},
			{"multiple types marshal as sequence", TypeArray("string", "null"), "- string\n- \"null\"\n"},
			{"empty type marshals as null", SchemaType{}, "null\n"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				data, err := yaml.Marshal(tt.input)
				require.NoError(t, err)
				assert.Equal(t, tt.expected, string(data))
			})
		}
	})

	t.Run("unmarshal", func(t *testing.T) {
		tests := []struct {
			name     string
			input    string
			expected []string
		}{
			{"scalar", "integer", []string{"integer"}},
			{"sequence", "- string\n- \"null\"\n", []string{"string", "null"}},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var st SchemaType
				require.NoError(t, yaml.Unmarshal([]byte(tt.input), &st))
				assert.Equal(t, tt.expected, st.Values())
			})
		}
	})

	t.Run("schema roundtrip through YAML", func(t *testing.T) {
		s := Schema{
			Type:   TypeString("object"),
			Format: "test",
		}
		data, err := yaml.Marshal(s)
		require.NoError(t, err)
		assert.Contains(t, string(data), "type: object")

		var roundtrip Schema
		require.NoError(t, yaml.Unmarshal(data, &roundtrip))
		assert.Equal(t, TypeString("object"), roundtrip.Type)
		assert.Equal(t, "test", roundtrip.Format)
	})

	t.Run("nullable type roundtrip through YAML", func(t *testing.T) {
		s := Schema{
			Type: TypeArray("string", "null"),
		}
		data, err := yaml.Marshal(s)
		require.NoError(t, err)

		var roundtrip Schema
		require.NoError(t, yaml.Unmarshal(data, &roundtrip))
		assert.Equal(t, []string{"string", "null"}, roundtrip.Type.Values())
	})

	t.Run("empty type omitted from YAML schema", func(t *testing.T) {
		s := Schema{
			Format: "uuid",
		}
		data, err := yaml.Marshal(s)
		require.NoError(t, err)

		yamlStr := string(data)
		assert.NotRegexp(t, `(?m)^type:`, yamlStr)
		assert.Contains(t, yamlStr, "format: uuid")
	})

	t.Run("full document YAML serialization preserves schema types", func(t *testing.T) {
		doc := Document{
			OpenAPI: "3.1.0",
			Info:    Info{Title: "Test", Version: "1.0.0"},
			Components: &Components{
				Schemas: map[string]*Schema{
					"Pet": {
						Type: TypeString("object"),
						Properties: map[string]*Schema{
							"name": {Type: TypeString("string")},
							"age":  {Type: TypeArray("integer", "null")},
						},
					},
				},
			},
		}
		data, err := yaml.Marshal(doc)
		require.NoError(t, err)

		yamlStr := string(data)
		assert.Contains(t, yamlStr, "type: object")
		assert.Contains(t, yamlStr, "type: string")

		var roundtrip Document
		require.NoError(t, yaml.Unmarshal(data, &roundtrip))
		petSchema := roundtrip.Components.Schemas["Pet"]
		require.NotNil(t, petSchema)
		assert.Equal(t, TypeString("object"), petSchema.Type)
		assert.Equal(t, TypeString("string"), petSchema.Properties["name"].Type)
		assert.Equal(t, []string{"integer", "null"}, petSchema.Properties["age"].Type.Values())
	})
}
