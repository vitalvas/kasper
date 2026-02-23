package openapi

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeDocuments(t *testing.T) {
	info := Info{Title: "Merged API", Version: "1.0.0"}

	t.Run("non-overlapping merge", func(t *testing.T) {
		doc1 := &Document{
			Paths: map[string]*PathItem{
				"/users": {Get: &Operation{Summary: "List users"}},
			},
		}
		doc2 := &Document{
			Paths: map[string]*PathItem{
				"/billing": {Get: &Operation{Summary: "List invoices"}},
			},
		}

		result, err := MergeDocuments(info, doc1, doc2)
		require.NoError(t, err)
		assert.Equal(t, "3.1.0", result.OpenAPI)
		assert.Equal(t, info, result.Info)
		assert.Contains(t, result.Paths, "/users")
		assert.Contains(t, result.Paths, "/billing")
	})

	t.Run("duplicate paths error", func(t *testing.T) {
		doc1 := &Document{
			Paths: map[string]*PathItem{
				"/users": {Get: &Operation{Summary: "List users v1"}},
			},
		}
		doc2 := &Document{
			Paths: map[string]*PathItem{
				"/users": {Get: &Operation{Summary: "List users v2"}},
			},
		}

		_, err := MergeDocuments(info, doc1, doc2)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `paths: duplicate "/users"`)
	})

	t.Run("duplicate webhooks error", func(t *testing.T) {
		doc1 := &Document{
			Webhooks: map[string]*PathItem{
				"userCreated": {Post: &Operation{Summary: "v1"}},
			},
		}
		doc2 := &Document{
			Webhooks: map[string]*PathItem{
				"userCreated": {Post: &Operation{Summary: "v2"}},
			},
		}

		_, err := MergeDocuments(info, doc1, doc2)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `webhooks: duplicate "userCreated"`)
	})

	t.Run("conflicting component schemas error", func(t *testing.T) {
		doc1 := &Document{
			Components: &Components{
				Schemas: map[string]*Schema{
					"User": {Type: TypeString("object")},
				},
			},
		}
		doc2 := &Document{
			Components: &Components{
				Schemas: map[string]*Schema{
					"User": {Type: TypeString("string")},
				},
			},
		}

		_, err := MergeDocuments(info, doc1, doc2)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `components.schemas: duplicate "User"`)
	})

	t.Run("identical component schemas deduplicated", func(t *testing.T) {
		doc1 := &Document{
			Components: &Components{
				Schemas: map[string]*Schema{
					"User": {Type: TypeString("object"), Description: "A user"},
				},
			},
		}
		doc2 := &Document{
			Components: &Components{
				Schemas: map[string]*Schema{
					"User": {Type: TypeString("object"), Description: "A user"},
				},
			},
		}

		result, err := MergeDocuments(info, doc1, doc2)
		require.NoError(t, err)
		require.NotNil(t, result.Components)
		assert.Len(t, result.Components.Schemas, 1)
		assert.Contains(t, result.Components.Schemas, "User")
	})

	t.Run("all 10 component types merged", func(t *testing.T) {
		doc1 := &Document{
			Components: &Components{
				Schemas:         map[string]*Schema{"S1": {Type: TypeString("object")}},
				Responses:       map[string]*Response{"R1": {Description: "OK"}},
				Parameters:      map[string]*Parameter{"P1": {Name: "p1", In: "query"}},
				Examples:        map[string]*Example{"E1": {Summary: "ex1"}},
				RequestBodies:   map[string]*RequestBody{"RB1": {Description: "body1"}},
				Headers:         map[string]*Header{"H1": {Description: "header1"}},
				SecuritySchemes: map[string]*SecurityScheme{"SS1": {Type: "http", Scheme: "bearer"}},
				Links:           map[string]*Link{"L1": {OperationID: "op1"}},
				Callbacks:       map[string]*Callback{"C1": {"url": &PathItem{}}},
				PathItems:       map[string]*PathItem{"PI1": {Summary: "pi1"}},
			},
		}
		doc2 := &Document{
			Components: &Components{
				Schemas:         map[string]*Schema{"S2": {Type: TypeString("string")}},
				Responses:       map[string]*Response{"R2": {Description: "Created"}},
				Parameters:      map[string]*Parameter{"P2": {Name: "p2", In: "header"}},
				Examples:        map[string]*Example{"E2": {Summary: "ex2"}},
				RequestBodies:   map[string]*RequestBody{"RB2": {Description: "body2"}},
				Headers:         map[string]*Header{"H2": {Description: "header2"}},
				SecuritySchemes: map[string]*SecurityScheme{"SS2": {Type: "apiKey", Name: "key", In: "header"}},
				Links:           map[string]*Link{"L2": {OperationID: "op2"}},
				Callbacks:       map[string]*Callback{"C2": {"url2": &PathItem{}}},
				PathItems:       map[string]*PathItem{"PI2": {Summary: "pi2"}},
			},
		}

		result, err := MergeDocuments(info, doc1, doc2)
		require.NoError(t, err)
		require.NotNil(t, result.Components)

		assert.Len(t, result.Components.Schemas, 2)
		assert.Len(t, result.Components.Responses, 2)
		assert.Len(t, result.Components.Parameters, 2)
		assert.Len(t, result.Components.Examples, 2)
		assert.Len(t, result.Components.RequestBodies, 2)
		assert.Len(t, result.Components.Headers, 2)
		assert.Len(t, result.Components.SecuritySchemes, 2)
		assert.Len(t, result.Components.Links, 2)
		assert.Len(t, result.Components.Callbacks, 2)
		assert.Len(t, result.Components.PathItems, 2)
	})

	t.Run("tag deduplication keeps description", func(t *testing.T) {
		doc1 := &Document{
			Tags: []Tag{
				{Name: "users"},
				{Name: "billing", Description: "Billing ops"},
			},
		}
		doc2 := &Document{
			Tags: []Tag{
				{Name: "users", Description: "User management"},
				{Name: "billing"},
			},
		}

		result, err := MergeDocuments(info, doc1, doc2)
		require.NoError(t, err)
		require.Len(t, result.Tags, 2)

		// Tags should be sorted alphabetically.
		assert.Equal(t, "billing", result.Tags[0].Name)
		assert.Equal(t, "Billing ops", result.Tags[0].Description)
		assert.Equal(t, "users", result.Tags[1].Name)
		assert.Equal(t, "User management", result.Tags[1].Description)
	})

	t.Run("security union and deduplication", func(t *testing.T) {
		doc1 := &Document{
			Security: []SecurityRequirement{
				{"bearer": {}},
				{"apiKey": {}},
			},
		}
		doc2 := &Document{
			Security: []SecurityRequirement{
				{"bearer": {}},
				{"oauth2": {"read", "write"}},
			},
		}

		result, err := MergeDocuments(info, doc1, doc2)
		require.NoError(t, err)
		assert.Len(t, result.Security, 3)
	})

	t.Run("servers dropped", func(t *testing.T) {
		doc1 := &Document{
			Servers: []Server{{URL: "https://api1.example.com"}},
			Paths:   map[string]*PathItem{"/a": {Get: &Operation{Summary: "A"}}},
		}
		doc2 := &Document{
			Servers: []Server{{URL: "https://api2.example.com"}},
			Paths:   map[string]*PathItem{"/b": {Get: &Operation{Summary: "B"}}},
		}

		result, err := MergeDocuments(info, doc1, doc2)
		require.NoError(t, err)
		assert.Nil(t, result.Servers)
	})

	t.Run("nil components handled", func(t *testing.T) {
		doc1 := &Document{
			Paths: map[string]*PathItem{"/a": {Get: &Operation{Summary: "A"}}},
		}
		doc2 := &Document{
			Components: &Components{
				Schemas: map[string]*Schema{"User": {Type: TypeString("object")}},
			},
		}

		result, err := MergeDocuments(info, doc1, doc2)
		require.NoError(t, err)
		require.NotNil(t, result.Components)
		assert.Contains(t, result.Components.Schemas, "User")
	})

	t.Run("empty docs", func(t *testing.T) {
		result, err := MergeDocuments(info)
		require.NoError(t, err)
		assert.Equal(t, "3.1.0", result.OpenAPI)
		assert.Equal(t, info, result.Info)
		assert.Nil(t, result.Paths)
		assert.Nil(t, result.Components)
		assert.Nil(t, result.Tags)
		assert.Nil(t, result.Security)
	})

	t.Run("multiple conflicts in single error", func(t *testing.T) {
		doc1 := &Document{
			Paths: map[string]*PathItem{
				"/users": {Get: &Operation{Summary: "v1"}},
			},
			Components: &Components{
				Schemas: map[string]*Schema{
					"User": {Type: TypeString("object")},
				},
			},
		}
		doc2 := &Document{
			Paths: map[string]*PathItem{
				"/users": {Get: &Operation{Summary: "v2"}},
			},
			Components: &Components{
				Schemas: map[string]*Schema{
					"User": {Type: TypeString("string")},
				},
			},
		}

		_, err := MergeDocuments(info, doc1, doc2)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "paths")
		assert.Contains(t, err.Error(), "components.schemas")
	})

	t.Run("identical paths deduplicated", func(t *testing.T) {
		doc1 := &Document{
			Paths: map[string]*PathItem{
				"/users": {Get: &Operation{Summary: "List users"}},
			},
		}
		doc2 := &Document{
			Paths: map[string]*PathItem{
				"/users": {Get: &Operation{Summary: "List users"}},
			},
		}

		result, err := MergeDocuments(info, doc1, doc2)
		require.NoError(t, err)
		assert.Len(t, result.Paths, 1)
	})

	t.Run("nil documents skipped", func(t *testing.T) {
		doc1 := &Document{
			Paths: map[string]*PathItem{
				"/users": {Get: &Operation{Summary: "List users"}},
			},
		}

		result, err := MergeDocuments(info, nil, doc1, nil)
		require.NoError(t, err)
		assert.Len(t, result.Paths, 1)
		assert.Contains(t, result.Paths, "/users")
	})

	t.Run("tag external docs preserved", func(t *testing.T) {
		doc1 := &Document{
			Tags: []Tag{
				{Name: "users"},
			},
		}
		doc2 := &Document{
			Tags: []Tag{
				{Name: "users", ExternalDocs: &ExternalDocs{URL: "https://docs.example.com"}},
			},
		}

		result, err := MergeDocuments(info, doc1, doc2)
		require.NoError(t, err)
		require.Len(t, result.Tags, 1)
		require.NotNil(t, result.Tags[0].ExternalDocs)
		assert.Equal(t, "https://docs.example.com", result.Tags[0].ExternalDocs.URL)
	})

	t.Run("schemas with different required order deduplicated", func(t *testing.T) {
		doc1 := &Document{
			Components: &Components{
				Schemas: map[string]*Schema{
					"User": {
						Type:     TypeString("object"),
						Required: []string{"name", "id"},
					},
				},
			},
		}
		doc2 := &Document{
			Components: &Components{
				Schemas: map[string]*Schema{
					"User": {
						Type:     TypeString("object"),
						Required: []string{"id", "name"},
					},
				},
			},
		}

		result, err := MergeDocuments(info, doc1, doc2)
		require.NoError(t, err)
		require.NotNil(t, result.Components)
		assert.Len(t, result.Components.Schemas, 1)
	})

	t.Run("security dedup ignores scope order", func(t *testing.T) {
		doc1 := &Document{
			Security: []SecurityRequirement{
				{"oauth2": {"write", "read"}},
			},
		}
		doc2 := &Document{
			Security: []SecurityRequirement{
				{"oauth2": {"read", "write"}},
			},
		}

		result, err := MergeDocuments(info, doc1, doc2)
		require.NoError(t, err)
		assert.Len(t, result.Security, 1)
	})

	t.Run("rejects incompatible openapi version", func(t *testing.T) {
		versions := []string{"3.0.3", "2.0", "3.1", "3.10.0"}
		for _, v := range versions {
			doc := &Document{
				OpenAPI: v,
				Paths:   map[string]*PathItem{"/a": {Get: &Operation{Summary: "A"}}},
			}

			_, err := MergeDocuments(info, doc)
			require.Error(t, err, "version %q should be rejected", v)
			assert.Contains(t, err.Error(), fmt.Sprintf("unsupported version %q", v))
		}
	})

	t.Run("accepts 3.1.x versions", func(t *testing.T) {
		doc1 := &Document{
			OpenAPI: "3.1.0",
			Paths:   map[string]*PathItem{"/a": {Get: &Operation{Summary: "A"}}},
		}
		doc2 := &Document{
			OpenAPI: "3.1.1",
			Paths:   map[string]*PathItem{"/b": {Get: &Operation{Summary: "B"}}},
		}

		result, err := MergeDocuments(info, doc1, doc2)
		require.NoError(t, err)
		assert.Len(t, result.Paths, 2)
	})

	t.Run("empty openapi version accepted", func(t *testing.T) {
		doc1 := &Document{
			Paths: map[string]*PathItem{"/a": {Get: &Operation{Summary: "A"}}},
		}

		result, err := MergeDocuments(info, doc1)
		require.NoError(t, err)
		assert.Equal(t, "3.1.0", result.OpenAPI)
	})

	t.Run("jsonSchemaDialect preserved when identical", func(t *testing.T) {
		dialect := "https://json-schema.org/draft/2020-12/schema"
		doc1 := &Document{
			JSONSchemaDialect: dialect,
			Paths:             map[string]*PathItem{"/a": {Get: &Operation{Summary: "A"}}},
		}
		doc2 := &Document{
			JSONSchemaDialect: dialect,
			Paths:             map[string]*PathItem{"/b": {Get: &Operation{Summary: "B"}}},
		}

		result, err := MergeDocuments(info, doc1, doc2)
		require.NoError(t, err)
		assert.Equal(t, dialect, result.JSONSchemaDialect)
	})

	t.Run("jsonSchemaDialect conflict", func(t *testing.T) {
		doc1 := &Document{
			JSONSchemaDialect: "https://json-schema.org/draft/2020-12/schema",
		}
		doc2 := &Document{
			JSONSchemaDialect: "https://json-schema.org/draft/2019-09/schema",
		}

		_, err := MergeDocuments(info, doc1, doc2)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "jsonSchemaDialect")
	})

	t.Run("different operation tag order is a conflict", func(t *testing.T) {
		doc1 := &Document{
			Paths: map[string]*PathItem{
				"/users": {Get: &Operation{Summary: "List", Tags: []string{"users", "admin"}}},
			},
		}
		doc2 := &Document{
			Paths: map[string]*PathItem{
				"/users": {Get: &Operation{Summary: "List", Tags: []string{"admin", "users"}}},
			},
		}

		_, err := MergeDocuments(info, doc1, doc2)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `paths: duplicate "/users"`)
	})

	t.Run("jsonSchemaDialect from single doc", func(t *testing.T) {
		dialect := "https://json-schema.org/draft/2020-12/schema"
		doc1 := &Document{
			JSONSchemaDialect: dialect,
			Paths:             map[string]*PathItem{"/a": {Get: &Operation{Summary: "A"}}},
		}
		doc2 := &Document{
			Paths: map[string]*PathItem{"/b": {Get: &Operation{Summary: "B"}}},
		}

		result, err := MergeDocuments(info, doc1, doc2)
		require.NoError(t, err)
		assert.Equal(t, dialect, result.JSONSchemaDialect)
	})

	t.Run("security dedup nil vs empty scopes", func(t *testing.T) {
		doc1 := &Document{
			Security: []SecurityRequirement{
				{"bearer": nil},
			},
		}
		doc2 := &Document{
			Security: []SecurityRequirement{
				{"bearer": {}},
			},
		}

		result, err := MergeDocuments(info, doc1, doc2)
		require.NoError(t, err)
		assert.Len(t, result.Security, 1)
	})

	t.Run("conflicting tag descriptions error", func(t *testing.T) {
		doc1 := &Document{
			Tags: []Tag{
				{Name: "users", Description: "User management"},
			},
		}
		doc2 := &Document{
			Tags: []Tag{
				{Name: "users", Description: "User operations"},
			},
		}

		_, err := MergeDocuments(info, doc1, doc2)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `tags: "users" has conflicting descriptions`)
	})

	t.Run("conflicting tag externalDocs error", func(t *testing.T) {
		doc1 := &Document{
			Tags: []Tag{
				{Name: "users", ExternalDocs: &ExternalDocs{URL: "https://a.example.com"}},
			},
		}
		doc2 := &Document{
			Tags: []Tag{
				{Name: "users", ExternalDocs: &ExternalDocs{URL: "https://b.example.com"}},
			},
		}

		_, err := MergeDocuments(info, doc1, doc2)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `tags: "users" has conflicting externalDocs`)
	})

	t.Run("tag complementary metadata merged", func(t *testing.T) {
		doc1 := &Document{
			Tags: []Tag{
				{Name: "users", Description: "User ops"},
			},
		}
		doc2 := &Document{
			Tags: []Tag{
				{Name: "users", ExternalDocs: &ExternalDocs{URL: "https://docs.example.com"}},
			},
		}

		result, err := MergeDocuments(info, doc1, doc2)
		require.NoError(t, err)
		require.Len(t, result.Tags, 1)
		assert.Equal(t, "User ops", result.Tags[0].Description)
		require.NotNil(t, result.Tags[0].ExternalDocs)
		assert.Equal(t, "https://docs.example.com", result.Tags[0].ExternalDocs.URL)
	})
}
