package openapi

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeneratePrimitives(t *testing.T) {
	tests := []struct {
		name         string
		input        any
		expectedType SchemaType
		expectNil    bool
	}{
		{"bool", true, TypeString("boolean"), false},
		{"int", 0, TypeString("integer"), false},
		{"int64", int64(0), TypeString("integer"), false},
		{"uint", uint(0), TypeString("integer"), false},
		{"float64", 0.0, TypeString("number"), false},
		{"float32", float32(0), TypeString("number"), false},
		{"string", "", TypeString("string"), false},
		{"nil", nil, SchemaType{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewSchemaGenerator()
			s := g.Generate(tt.input)
			if tt.expectNil {
				assert.Nil(t, s)
			} else {
				require.NotNil(t, s)
				assert.Equal(t, tt.expectedType, s.Type)
			}
		})
	}
}

func TestGenerateSpecialTypes(t *testing.T) {
	tests := []struct {
		name           string
		input          any
		expectedType   SchemaType
		expectedFormat string
	}{
		{"time.Time", time.Time{}, TypeString("string"), "date-time"},
		{"[]byte", []byte{}, TypeString("string"), "byte"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewSchemaGenerator()
			s := g.Generate(tt.input)
			require.NotNil(t, s)
			assert.Equal(t, tt.expectedType, s.Type)
			assert.Equal(t, tt.expectedFormat, s.Format)
		})
	}
}

func TestGenerateSliceAndArray(t *testing.T) {
	tests := []struct {
		name             string
		input            any
		expectedItemType SchemaType
	}{
		{"[]string", []string{}, TypeString("string")},
		{"[]int", []int{}, TypeString("integer")},
		{"[3]string", [3]string{}, TypeString("string")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewSchemaGenerator()
			s := g.Generate(tt.input)
			require.NotNil(t, s)
			assert.Equal(t, TypeString("array"), s.Type)
			require.NotNil(t, s.Items)
			assert.Equal(t, tt.expectedItemType, s.Items.Type)
		})
	}
}

func TestGenerateMap(t *testing.T) {
	tests := []struct {
		name                  string
		input                 any
		hasAdditionalProps    bool
		additionalPropsType   SchemaType
		checkAdditionalNotNil bool
	}{
		{"map[string]int", map[string]int{}, true, TypeString("integer"), true},
		{"map[string]any", map[string]any{}, true, SchemaType{}, false},
		{"map[int]string", map[int]string{}, false, SchemaType{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewSchemaGenerator()
			s := g.Generate(tt.input)
			require.NotNil(t, s)
			assert.Equal(t, TypeString("object"), s.Type)

			if tt.hasAdditionalProps {
				require.NotNil(t, s.AdditionalProperties)
				if tt.checkAdditionalNotNil {
					assert.Equal(t, tt.additionalPropsType, s.AdditionalProperties.Type)
				}
			} else {
				assert.Nil(t, s.AdditionalProperties)
			}
		})
	}
}

type SimpleStruct struct {
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
	Age   int    `json:"age"`
}

func TestGenerateStruct(t *testing.T) {
	t.Run("simple struct", func(t *testing.T) {
		g := NewSchemaGenerator()
		s := g.Generate(SimpleStruct{})

		assert.Equal(t, "#/components/schemas/SimpleStruct", s.Ref)

		schema := g.Schemas()["SimpleStruct"]
		require.NotNil(t, schema)
		assert.Equal(t, TypeString("object"), schema.Type)
		assert.Contains(t, schema.Properties, "name")
		assert.Contains(t, schema.Properties, "email")
		assert.Contains(t, schema.Properties, "age")
		assert.Contains(t, schema.Required, "name")
		assert.Contains(t, schema.Required, "age")
		assert.NotContains(t, schema.Required, "email")
	})

	t.Run("omitzero field is optional", func(t *testing.T) {
		type WithOmitzero struct {
			Name  string `json:"name"`
			Value int    `json:"value,omitzero"`
		}
		g := NewSchemaGenerator()
		g.Generate(WithOmitzero{})
		schema := g.Schemas()["WithOmitzero"]
		require.NotNil(t, schema)
		assert.Contains(t, schema.Required, "name")
		assert.NotContains(t, schema.Required, "value")
	})

	t.Run("omitzero with omitempty both optional", func(t *testing.T) {
		type Combined struct {
			A string `json:"a,omitempty"`
			B string `json:"b,omitzero"`
			C string `json:"c"`
		}
		g := NewSchemaGenerator()
		g.Generate(Combined{})
		schema := g.Schemas()["Combined"]
		require.NotNil(t, schema)
		assert.NotContains(t, schema.Required, "a")
		assert.NotContains(t, schema.Required, "b")
		assert.Contains(t, schema.Required, "c")
	})

	t.Run("json dash field skipped", func(t *testing.T) {
		type WithDash struct {
			Visible string `json:"visible"`
			Hidden  string `json:"-"`
		}
		g := NewSchemaGenerator()
		g.Generate(WithDash{})
		schema := g.Schemas()["WithDash"]
		require.NotNil(t, schema)
		assert.Contains(t, schema.Properties, "visible")
		assert.NotContains(t, schema.Properties, "Hidden")
	})

	t.Run("unexported fields skipped", func(t *testing.T) {
		type WithUnexported struct {
			Public  string `json:"public"`
			private string //nolint:unused
		}
		g := NewSchemaGenerator()
		g.Generate(WithUnexported{})
		schema := g.Schemas()["WithUnexported"]
		require.NotNil(t, schema)
		assert.Len(t, schema.Properties, 1)
		assert.Contains(t, schema.Properties, "public")
	})

	t.Run("field without json tag uses field name", func(t *testing.T) {
		type NoTag struct {
			FieldName string
		}
		g := NewSchemaGenerator()
		g.Generate(NoTag{})
		schema := g.Schemas()["NoTag"]
		require.NotNil(t, schema)
		assert.Contains(t, schema.Properties, "FieldName")
	})
}

type EmbeddedBase struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
}

type WithEmbedded struct {
	EmbeddedBase
	Name string `json:"name"`
}

func TestGenerateEmbeddedStruct(t *testing.T) {
	t.Run("embedded fields are flattened", func(t *testing.T) {
		g := NewSchemaGenerator()
		g.Generate(WithEmbedded{})
		schema := g.Schemas()["WithEmbedded"]
		require.NotNil(t, schema)
		assert.Contains(t, schema.Properties, "id")
		assert.Contains(t, schema.Properties, "created_at")
		assert.Contains(t, schema.Properties, "name")
	})

	t.Run("embedded with json tag name is not flattened", func(t *testing.T) {
		type Meta struct {
			Version string `json:"version"`
			Source  string `json:"source"`
		}
		type Wrapper struct {
			Meta `json:"meta"`
			Name string `json:"name"`
		}
		g := NewSchemaGenerator()
		g.Generate(Wrapper{})
		schema := g.Schemas()["Wrapper"]
		require.NotNil(t, schema)

		// Meta should appear as "meta" property, not flattened.
		assert.Contains(t, schema.Properties, "meta")
		assert.Contains(t, schema.Properties, "name")
		assert.NotContains(t, schema.Properties, "version")
		assert.NotContains(t, schema.Properties, "source")
	})

	t.Run("embedded pointer with json tag name is not flattened", func(t *testing.T) {
		type Audit struct {
			CreatedBy string `json:"created_by"`
		}
		type Resource struct {
			*Audit `json:"audit"`
			Title  string `json:"title"`
		}
		g := NewSchemaGenerator()
		g.Generate(Resource{})
		schema := g.Schemas()["Resource"]
		require.NotNil(t, schema)

		assert.Contains(t, schema.Properties, "audit")
		assert.Contains(t, schema.Properties, "title")
		assert.NotContains(t, schema.Properties, "created_by")
	})

	t.Run("embedded without json tag is still flattened", func(t *testing.T) {
		type Timestamps struct {
			CreatedAt string `json:"created_at"`
			UpdatedAt string `json:"updated_at"`
		}
		type Record struct {
			Timestamps
			ID string `json:"id"`
		}
		g := NewSchemaGenerator()
		g.Generate(Record{})
		schema := g.Schemas()["Record"]
		require.NotNil(t, schema)

		// Timestamps fields should be inlined.
		assert.Contains(t, schema.Properties, "created_at")
		assert.Contains(t, schema.Properties, "updated_at")
		assert.Contains(t, schema.Properties, "id")
	})

	t.Run("embedded pointer struct fields are all optional", func(t *testing.T) {
		type Audit struct {
			CreatedBy string `json:"created_by"`
			UpdatedBy string `json:"updated_by"`
		}
		type Resource struct {
			*Audit
			Title string `json:"title"`
		}
		g := NewSchemaGenerator()
		g.Generate(Resource{})
		schema := g.Schemas()["Resource"]
		require.NotNil(t, schema)

		// Audit fields are inlined but all optional since *Audit can be nil.
		assert.Contains(t, schema.Properties, "created_by")
		assert.Contains(t, schema.Properties, "updated_by")
		assert.Contains(t, schema.Properties, "title")
		assert.Contains(t, schema.Required, "title")
		assert.NotContains(t, schema.Required, "created_by")
		assert.NotContains(t, schema.Required, "updated_by")
	})

	t.Run("non-pointer embedded struct fields keep required", func(t *testing.T) {
		type Audit struct {
			CreatedBy string `json:"created_by"`
		}
		type Resource struct {
			Audit
			Title string `json:"title"`
		}
		g := NewSchemaGenerator()
		g.Generate(Resource{})
		schema := g.Schemas()["Resource"]
		require.NotNil(t, schema)

		// Non-pointer embed: fields retain their required status.
		assert.Contains(t, schema.Required, "created_by")
		assert.Contains(t, schema.Required, "title")
	})
}

func TestGenerateNullableTypes(t *testing.T) {
	t.Run("pointer to primitive", func(t *testing.T) {
		g := NewSchemaGenerator()
		type WithPtr struct {
			Value *string `json:"value"`
		}
		g.Generate(WithPtr{})
		schema := g.Schemas()["WithPtr"]
		require.NotNil(t, schema)
		valSchema := schema.Properties["value"]
		require.NotNil(t, valSchema)
		assert.Equal(t, TypeArray("string", "null"), valSchema.Type)
	})

	t.Run("pointer to struct", func(t *testing.T) {
		g := NewSchemaGenerator()
		type Inner struct {
			X int `json:"x"`
		}
		type Outer struct {
			Inner *Inner `json:"inner"`
		}
		g.Generate(Outer{})
		schema := g.Schemas()["Outer"]
		require.NotNil(t, schema)
		innerSchema := schema.Properties["inner"]
		require.NotNil(t, innerSchema)
		assert.Len(t, innerSchema.AnyOf, 2)
		assert.Equal(t, "#/components/schemas/Inner", innerSchema.AnyOf[0].Ref)
		assert.Equal(t, TypeString("null"), innerSchema.AnyOf[1].Type)
	})
}

type TaggedStruct struct {
	Name  string `json:"name" openapi:"description=User name,example=John,minLength=1,maxLength=100"`
	Email string `json:"email" openapi:"description=Email address,format=email"`
	Age   int    `json:"age,omitempty" openapi:"minimum=0,maximum=150"`
	Role  string `json:"role" openapi:"enum=admin|user|guest,description=User role"`
}

func TestGenerateOpenAPITags(t *testing.T) {
	t.Run("all tag types", func(t *testing.T) {
		g := NewSchemaGenerator()
		g.Generate(TaggedStruct{})
		schema := g.Schemas()["TaggedStruct"]
		require.NotNil(t, schema)

		// Name field
		nameSchema := schema.Properties["name"]
		assert.Equal(t, "User name", nameSchema.Description)
		assert.Equal(t, "John", nameSchema.Example)
		require.NotNil(t, nameSchema.MinLength)
		assert.Equal(t, 1, *nameSchema.MinLength)
		require.NotNil(t, nameSchema.MaxLength)
		assert.Equal(t, 100, *nameSchema.MaxLength)

		// Email field
		emailSchema := schema.Properties["email"]
		assert.Equal(t, "Email address", emailSchema.Description)
		assert.Equal(t, "email", emailSchema.Format)

		// Age field
		ageSchema := schema.Properties["age"]
		require.NotNil(t, ageSchema.Minimum)
		assert.Equal(t, 0.0, *ageSchema.Minimum)
		require.NotNil(t, ageSchema.Maximum)
		assert.Equal(t, 150.0, *ageSchema.Maximum)

		// Role field
		roleSchema := schema.Properties["role"]
		assert.Equal(t, "User role", roleSchema.Description)
		assert.Equal(t, []any{"admin", "user", "guest"}, roleSchema.Enum)
	})

	t.Run("deprecated and readOnly tags", func(t *testing.T) {
		type DeprecatedField struct {
			Old string `json:"old" openapi:"deprecated"`
			ID  string `json:"id" openapi:"readOnly"`
		}
		g := NewSchemaGenerator()
		g.Generate(DeprecatedField{})
		schema := g.Schemas()["DeprecatedField"]
		require.NotNil(t, schema)
		assert.True(t, schema.Properties["old"].Deprecated)
		assert.True(t, schema.Properties["id"].ReadOnly)
	})

	t.Run("writeOnly tag", func(t *testing.T) {
		type Secret struct {
			Password string `json:"password" openapi:"writeOnly"`
		}
		g := NewSchemaGenerator()
		g.Generate(Secret{})
		schema := g.Schemas()["Secret"]
		require.NotNil(t, schema)
		assert.True(t, schema.Properties["password"].WriteOnly)
	})

	t.Run("pattern tag", func(t *testing.T) {
		type WithPattern struct {
			Code string `json:"code" openapi:"pattern=^[A-Z]{3}$"`
		}
		g := NewSchemaGenerator()
		g.Generate(WithPattern{})
		schema := g.Schemas()["WithPattern"]
		require.NotNil(t, schema)
		assert.Equal(t, `^[A-Z]{3}$`, schema.Properties["code"].Pattern)
	})

	t.Run("integer example parsed", func(t *testing.T) {
		type WithIntExample struct {
			Count int `json:"count" openapi:"example=42"`
		}
		g := NewSchemaGenerator()
		g.Generate(WithIntExample{})
		schema := g.Schemas()["WithIntExample"]
		require.NotNil(t, schema)
		assert.Equal(t, int64(42), schema.Properties["count"].Example)
	})

	t.Run("float example parsed", func(t *testing.T) {
		type WithFloatExample struct {
			Price float64 `json:"price" openapi:"example=9.99"`
		}
		g := NewSchemaGenerator()
		g.Generate(WithFloatExample{})
		schema := g.Schemas()["WithFloatExample"]
		require.NotNil(t, schema)
		assert.Equal(t, 9.99, schema.Properties["price"].Example)
	})

	t.Run("boolean example parsed", func(t *testing.T) {
		type WithBoolExample struct {
			Active bool `json:"active" openapi:"example=true"`
		}
		g := NewSchemaGenerator()
		g.Generate(WithBoolExample{})
		schema := g.Schemas()["WithBoolExample"]
		require.NotNil(t, schema)
		assert.Equal(t, true, schema.Properties["active"].Example)
	})
}

type ExampleUser struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func (ExampleUser) OpenAPIExample() any {
	return ExampleUser{
		ID:    "550e8400-e29b-41d4-a716-446655440000",
		Name:  "Alice",
		Email: "alice@example.com",
	}
}

type NoExampleUser struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func TestOpenAPIExampler(t *testing.T) {
	t.Run("type with vs without OpenAPIExample", func(t *testing.T) {
		tests := []struct {
			name       string
			input      any
			schemaName string
			hasExample bool
		}{
			{"with OpenAPIExample", ExampleUser{}, "ExampleUser", true},
			{"without OpenAPIExample", NoExampleUser{}, "NoExampleUser", false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				g := NewSchemaGenerator()
				g.Generate(tt.input)
				schema := g.Schemas()[tt.schemaName]
				require.NotNil(t, schema)

				if tt.hasExample {
					require.NotNil(t, schema.Example)
				} else {
					assert.Nil(t, schema.Example)
				}
			})
		}
	})

	t.Run("example values are correct", func(t *testing.T) {
		g := NewSchemaGenerator()
		g.Generate(ExampleUser{})

		schema := g.Schemas()["ExampleUser"]
		require.NotNil(t, schema)
		require.NotNil(t, schema.Example)

		ex, ok := schema.Example.(ExampleUser)
		require.True(t, ok)
		assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", ex.ID)
		assert.Equal(t, "Alice", ex.Name)
		assert.Equal(t, "alice@example.com", ex.Email)
	})

	t.Run("example serializes to JSON", func(t *testing.T) {
		g := NewSchemaGenerator()
		g.Generate(ExampleUser{})

		schema := g.Schemas()["ExampleUser"]
		data, err := json.Marshal(schema)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))

		example, ok := parsed["example"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", example["id"])
		assert.Equal(t, "Alice", example["name"])
		assert.Equal(t, "alice@example.com", example["email"])
	})

	t.Run("pointer ref still works with example on component", func(t *testing.T) {
		g := NewSchemaGenerator()
		type Wrapper struct {
			User *ExampleUser `json:"user"`
		}
		s := g.Generate(Wrapper{})

		wrapperSchema := g.Schemas()["Wrapper"]
		require.NotNil(t, wrapperSchema)
		userProp := wrapperSchema.Properties["user"]
		require.NotNil(t, userProp)
		require.Len(t, userProp.AnyOf, 2)
		assert.Equal(t, "#/components/schemas/ExampleUser", userProp.AnyOf[0].Ref)

		exSchema := g.Schemas()["ExampleUser"]
		require.NotNil(t, exSchema)
		assert.NotNil(t, exSchema.Example)

		_ = s
	})
}

func TestSanitizeSchemaName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"plain name unchanged", "User", "User"},
		{"generic simple type", "ResponseData[User]", "ResponseDataUser"},
		{"generic with package path", "ResponseData[github.com/foo/bar.User]", "ResponseDataUser"},
		{"generic slice type", "ResponseData[[]User]", "ResponseDataUserList"},
		{"generic slice with package path", "ResponseData[[]github.com/foo.User]", "ResponseDataUserList"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, sanitizeSchemaName(tt.input))
		})
	}
}

func TestSchemaNameCollision(t *testing.T) {
	t.Run("different packages same type name get unique schema names", func(t *testing.T) {
		type Client struct {
			ID string `json:"id"`
		}

		gen := NewSchemaGenerator()

		// Generate local Client first.
		s1 := gen.Generate(Client{})
		assert.Equal(t, "#/components/schemas/Client", s1.Ref)
		assert.Contains(t, gen.Schemas(), "Client")

		// Generate http.Client which has the same simple name.
		s2 := gen.Generate(http.Client{})
		assert.Equal(t, "#/components/schemas/HttpClient", s2.Ref)
		assert.Contains(t, gen.Schemas(), "HttpClient")

		// Both schemas should be present and distinct.
		assert.Len(t, gen.Schemas(), 2)
	})

	t.Run("same type used twice returns same name", func(t *testing.T) {
		type Item struct {
			ID string `json:"id"`
		}

		gen := NewSchemaGenerator()

		s1 := gen.Generate(Item{})
		s2 := gen.Generate(Item{})
		assert.Equal(t, s1.Ref, s2.Ref)
		assert.Len(t, gen.Schemas(), 1)
	})

	t.Run("triple collision with same package suffix appends numeric suffix", func(t *testing.T) {
		type Client struct {
			ID string `json:"id"`
		}

		gen := NewSchemaGenerator()

		// First: local Client → "Client".
		s1 := gen.Generate(Client{})
		assert.Equal(t, "#/components/schemas/Client", s1.Ref)

		// Pre-seed "HttpClient" to simulate a prior type from a package
		// ending in "http" that already claimed the prefixed name.
		fakeType := reflect.TypeOf(http.Response{})
		gen.nameTypes["HttpClient"] = fakeType
		gen.typeNames[fakeType] = "HttpClient"

		// http.Client: simple "Client" collides, prefixed "HttpClient"
		// also collides → must get numeric suffix "HttpClient2".
		s2 := gen.Generate(http.Client{})
		assert.Equal(t, "#/components/schemas/HttpClient2", s2.Ref)
		assert.Contains(t, gen.Schemas(), "HttpClient2")

		// Same type again returns cached name.
		s3 := gen.Generate(http.Client{})
		assert.Equal(t, s2.Ref, s3.Ref)
	})

	t.Run("generic instantiations with same sanitized name get unique names", func(t *testing.T) {
		gen := NewSchemaGenerator()

		// Simulate two generic types whose sanitized names are identical
		// by pre-seeding the first name, then generating a real type
		// that would produce the same sanitized name.
		//
		// Generate http.Response first → "Response".
		gen.Generate(http.Response{})
		assert.Contains(t, gen.Schemas(), "Response")

		// Pre-seed "HttpResponse" to simulate a generic type instantiation
		// that already claimed the prefixed name.
		type Response struct {
			Code int `json:"code"`
		}
		fakeType := reflect.TypeOf(SimpleStruct{})
		gen.nameTypes["HttpResponse"] = fakeType
		gen.typeNames[fakeType] = "HttpResponse"

		// Local Response: simple "Response" collides, prefixed
		// "OpenapiResponse" (from test package), so it gets that.
		s := gen.Generate(Response{})
		assert.NotEmpty(t, s.Ref)
		// Verify no schema was overwritten — all names are distinct.
		assert.NotEqual(t, gen.nameTypes["Response"], gen.nameTypes[s.Ref[len("#/components/schemas/"):]])
	})
}

func TestPkgPrefix(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"standard library", "net/http", "Http"},
		{"full package path", "github.com/foo/models", "Models"},
		{"hyphenated package", "github.com/foo/go-utils", "Go_utils"},
		{"dotted package", "github.com/foo/v2.api", "V2_api"},
		{"empty string", "", ""},
		{"no slash", "models", "Models"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, pkgPrefix(tt.input))
		})
	}
}

type ResponseWrapper[T any] struct {
	Success  bool     `json:"success"`
	Errors   []string `json:"errors,omitempty"`
	Messages []string `json:"messages,omitempty"`
	Result   T        `json:"result"`
}

func TestGenerateGenericStruct(t *testing.T) {
	t.Run("generic with struct type parameter", func(t *testing.T) {
		g := NewSchemaGenerator()
		s := g.Generate(ResponseWrapper[SimpleStruct]{})

		assert.Equal(t, "#/components/schemas/ResponseWrapperSimpleStruct", s.Ref)

		schema := g.Schemas()["ResponseWrapperSimpleStruct"]
		require.NotNil(t, schema)
		assert.Equal(t, TypeString("object"), schema.Type)
		assert.Contains(t, schema.Properties, "success")
		assert.Contains(t, schema.Properties, "errors")
		assert.Contains(t, schema.Properties, "messages")
		assert.Contains(t, schema.Properties, "result")

		// Result field should be a $ref to SimpleStruct.
		resultProp := schema.Properties["result"]
		assert.Equal(t, "#/components/schemas/SimpleStruct", resultProp.Ref)

		// SimpleStruct should also be in schemas.
		assert.Contains(t, g.Schemas(), "SimpleStruct")
	})

	t.Run("generic with slice type parameter", func(t *testing.T) {
		g := NewSchemaGenerator()
		s := g.Generate(ResponseWrapper[[]SimpleStruct]{})

		assert.Equal(t, "#/components/schemas/ResponseWrapperSimpleStructList", s.Ref)

		schema := g.Schemas()["ResponseWrapperSimpleStructList"]
		require.NotNil(t, schema)

		// Result field should be an array of $ref.
		resultProp := schema.Properties["result"]
		assert.Equal(t, TypeString("array"), resultProp.Type)
		require.NotNil(t, resultProp.Items)
		assert.Equal(t, "#/components/schemas/SimpleStruct", resultProp.Items.Ref)
	})

	t.Run("two generic instantiations produce separate schemas", func(t *testing.T) {
		g := NewSchemaGenerator()
		g.Generate(ResponseWrapper[SimpleStruct]{})
		g.Generate(ResponseWrapper[ExampleUser]{})

		schemas := g.Schemas()
		assert.Contains(t, schemas, "ResponseWrapperSimpleStruct")
		assert.Contains(t, schemas, "ResponseWrapperExampleUser")
	})
}

func TestGenerateTypeDeduplication(t *testing.T) {
	t.Run("same type used twice gets single schema", func(t *testing.T) {
		type Item struct {
			Name string `json:"name"`
		}
		type Container struct {
			Items  []Item `json:"items"`
			Single Item   `json:"single"`
		}
		g := NewSchemaGenerator()
		g.Generate(Container{})
		schemas := g.Schemas()
		assert.Contains(t, schemas, "Item")
		assert.Contains(t, schemas, "Container")
		assert.Len(t, schemas, 2)
	})
}

func TestGenerateSliceOfStructs(t *testing.T) {
	t.Run("slice of named structs uses ref", func(t *testing.T) {
		g := NewSchemaGenerator()
		s := g.Generate([]SimpleStruct{})
		assert.Equal(t, TypeString("array"), s.Type)
		require.NotNil(t, s.Items)
		assert.Equal(t, "#/components/schemas/SimpleStruct", s.Items.Ref)
	})
}

func TestGenerateInterface(t *testing.T) {
	t.Run("any/interface generates empty schema", func(t *testing.T) {
		type WithAny struct {
			Data any `json:"data"`
		}
		g := NewSchemaGenerator()
		g.Generate(WithAny{})
		schema := g.Schemas()["WithAny"]
		require.NotNil(t, schema)
		assert.Contains(t, schema.Properties, "data")
	})
}

func TestSchemaGeneratorJSON(t *testing.T) {
	t.Run("generated schema serializes correctly", func(t *testing.T) {
		g := NewSchemaGenerator()
		g.Generate(TaggedStruct{})
		schema := g.Schemas()["TaggedStruct"]

		data, err := json.Marshal(schema)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Equal(t, "object", parsed["type"])

		props := parsed["properties"].(map[string]any)
		nameProps := props["name"].(map[string]any)
		assert.Equal(t, "User name", nameProps["description"])
		assert.Equal(t, "John", nameProps["example"])
	})
}

func TestSchemaExternalDocs(t *testing.T) {
	t.Run("serializes externalDocs on schema", func(t *testing.T) {
		s := &Schema{
			Type: TypeString("object"),
			ExternalDocs: &ExternalDocs{
				URL:         "https://docs.example.com/user",
				Description: "User schema docs",
			},
		}

		data, err := json.Marshal(s)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		extDocs := parsed["externalDocs"].(map[string]any)
		assert.Equal(t, "https://docs.example.com/user", extDocs["url"])
		assert.Equal(t, "User schema docs", extDocs["description"])
	})
}

func TestGenerateExclusiveMinMax(t *testing.T) {
	t.Run("exclusiveMinimum and exclusiveMaximum", func(t *testing.T) {
		type Ranged struct {
			Score float64 `json:"score" openapi:"exclusiveMinimum=0,exclusiveMaximum=100"`
		}
		g := NewSchemaGenerator()
		g.Generate(Ranged{})
		schema := g.Schemas()["Ranged"]
		require.NotNil(t, schema)
		scoreSchema := schema.Properties["score"]
		require.NotNil(t, scoreSchema.ExclusiveMinimum)
		assert.Equal(t, 0.0, *scoreSchema.ExclusiveMinimum)
		require.NotNil(t, scoreSchema.ExclusiveMaximum)
		assert.Equal(t, 100.0, *scoreSchema.ExclusiveMaximum)
	})
}

func TestGenerateNewTagKeys(t *testing.T) {
	t.Run("title tag", func(t *testing.T) {
		type WithTitle struct {
			Name string `json:"name" openapi:"title=Full Name"`
		}
		g := NewSchemaGenerator()
		g.Generate(WithTitle{})
		schema := g.Schemas()["WithTitle"]
		require.NotNil(t, schema)
		assert.Equal(t, "Full Name", schema.Properties["name"].Title)
	})

	t.Run("multipleOf tag", func(t *testing.T) {
		type WithMultipleOf struct {
			Price float64 `json:"price" openapi:"multipleOf=0.01"`
		}
		g := NewSchemaGenerator()
		g.Generate(WithMultipleOf{})
		schema := g.Schemas()["WithMultipleOf"]
		require.NotNil(t, schema)
		require.NotNil(t, schema.Properties["price"].MultipleOf)
		assert.Equal(t, 0.01, *schema.Properties["price"].MultipleOf)
	})

	t.Run("minItems and maxItems tags", func(t *testing.T) {
		type WithItemConstraints struct {
			Tags []string `json:"tags" openapi:"minItems=1,maxItems=5"`
		}
		g := NewSchemaGenerator()
		g.Generate(WithItemConstraints{})
		schema := g.Schemas()["WithItemConstraints"]
		require.NotNil(t, schema)
		tagsSchema := schema.Properties["tags"]
		require.NotNil(t, tagsSchema.MinItems)
		assert.Equal(t, 1, *tagsSchema.MinItems)
		require.NotNil(t, tagsSchema.MaxItems)
		assert.Equal(t, 5, *tagsSchema.MaxItems)
	})

	t.Run("uniqueItems tag", func(t *testing.T) {
		type WithUnique struct {
			IDs []string `json:"ids" openapi:"uniqueItems"`
		}
		g := NewSchemaGenerator()
		g.Generate(WithUnique{})
		schema := g.Schemas()["WithUnique"]
		require.NotNil(t, schema)
		assert.True(t, schema.Properties["ids"].UniqueItems)
	})

	t.Run("minProperties and maxProperties tags", func(t *testing.T) {
		type WithPropConstraints struct {
			Meta map[string]string `json:"meta" openapi:"minProperties=1,maxProperties=10"`
		}
		g := NewSchemaGenerator()
		g.Generate(WithPropConstraints{})
		schema := g.Schemas()["WithPropConstraints"]
		require.NotNil(t, schema)
		metaSchema := schema.Properties["meta"]
		require.NotNil(t, metaSchema.MinProperties)
		assert.Equal(t, 1, *metaSchema.MinProperties)
		require.NotNil(t, metaSchema.MaxProperties)
		assert.Equal(t, 10, *metaSchema.MaxProperties)
	})

	t.Run("const tag with string", func(t *testing.T) {
		type WithConst struct {
			Type string `json:"type" openapi:"const=user"`
		}
		g := NewSchemaGenerator()
		g.Generate(WithConst{})
		schema := g.Schemas()["WithConst"]
		require.NotNil(t, schema)
		assert.Equal(t, "user", schema.Properties["type"].Const)
	})

	t.Run("const tag with integer", func(t *testing.T) {
		type WithIntConst struct {
			Version int `json:"version" openapi:"const=2"`
		}
		g := NewSchemaGenerator()
		g.Generate(WithIntConst{})
		schema := g.Schemas()["WithIntConst"]
		require.NotNil(t, schema)
		assert.Equal(t, int64(2), schema.Properties["version"].Const)
	})
}

func TestJSONStringTagOverride(t *testing.T) {
	t.Run("int with string tag becomes string type", func(t *testing.T) {
		type WithStringInt struct {
			Count int `json:"count,string"`
		}
		g := NewSchemaGenerator()
		g.Generate(WithStringInt{})
		schema := g.Schemas()["WithStringInt"]
		require.NotNil(t, schema)
		assert.Equal(t, TypeString("string"), schema.Properties["count"].Type)
	})

	t.Run("bool with string tag becomes string type", func(t *testing.T) {
		type WithStringBool struct {
			Active bool `json:"active,string"`
		}
		g := NewSchemaGenerator()
		g.Generate(WithStringBool{})
		schema := g.Schemas()["WithStringBool"]
		require.NotNil(t, schema)
		assert.Equal(t, TypeString("string"), schema.Properties["active"].Type)
	})

	t.Run("float with string tag becomes string type", func(t *testing.T) {
		type WithStringFloat struct {
			Price float64 `json:"price,string"`
		}
		g := NewSchemaGenerator()
		g.Generate(WithStringFloat{})
		schema := g.Schemas()["WithStringFloat"]
		require.NotNil(t, schema)
		assert.Equal(t, TypeString("string"), schema.Properties["price"].Type)
	})

	t.Run("nullable int with string tag becomes nullable string", func(t *testing.T) {
		type WithStringPtr struct {
			Count *int `json:"count,string"`
		}
		g := NewSchemaGenerator()
		g.Generate(WithStringPtr{})
		schema := g.Schemas()["WithStringPtr"]
		require.NotNil(t, schema)
		assert.Equal(t, TypeArray("string", "null"), schema.Properties["count"].Type)
	})

	t.Run("string with string tag stays string", func(t *testing.T) {
		type WithStringString struct {
			Name string `json:"name,string"`
		}
		g := NewSchemaGenerator()
		g.Generate(WithStringString{})
		schema := g.Schemas()["WithStringString"]
		require.NotNil(t, schema)
		assert.Equal(t, TypeString("string"), schema.Properties["name"].Type)
	})

	t.Run("string tag combined with omitempty", func(t *testing.T) {
		type WithStringOmit struct {
			Count int `json:"count,omitempty,string"`
		}
		g := NewSchemaGenerator()
		g.Generate(WithStringOmit{})
		schema := g.Schemas()["WithStringOmit"]
		require.NotNil(t, schema)
		assert.Equal(t, TypeString("string"), schema.Properties["count"].Type)
		assert.NotContains(t, schema.Required, "count")
	})

	t.Run("openapi tags preserved with string override", func(t *testing.T) {
		type WithStringAndOpenAPI struct {
			Count int `json:"count,string" openapi:"description=Item count"`
		}
		g := NewSchemaGenerator()
		g.Generate(WithStringAndOpenAPI{})
		schema := g.Schemas()["WithStringAndOpenAPI"]
		require.NotNil(t, schema)
		assert.Equal(t, TypeString("string"), schema.Properties["count"].Type)
		assert.Equal(t, "Item count", schema.Properties["count"].Description)
	})
}
