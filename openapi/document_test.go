package openapi

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestDocumentExportJSON(t *testing.T) {
	t.Run("serializes to valid JSON", func(t *testing.T) {
		doc := &Document{
			OpenAPI: "3.1.0",
			Info:    Info{Title: "Test", Version: "1.0.0"},
			Paths: map[string]*PathItem{
				"/users": {Get: &Operation{Summary: "List users"}},
			},
		}

		data, err := doc.JSON()
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Equal(t, "3.1.0", parsed["openapi"])
		info := parsed["info"].(map[string]any)
		assert.Equal(t, "Test", info["title"])
	})

	t.Run("roundtrip via DocumentFromJSON", func(t *testing.T) {
		original := &Document{
			OpenAPI: "3.1.0",
			Info:    Info{Title: "RT", Version: "2.0.0"},
			Components: &Components{
				Schemas: map[string]*Schema{
					"User": {Type: TypeString("object")},
				},
			},
		}

		data, err := original.JSON()
		require.NoError(t, err)

		parsed, err := DocumentFromJSON(data)
		require.NoError(t, err)
		assert.Equal(t, original.OpenAPI, parsed.OpenAPI)
		assert.Equal(t, original.Info, parsed.Info)
		require.NotNil(t, parsed.Components)
		assert.Contains(t, parsed.Components.Schemas, "User")
	})

	t.Run("omits empty fields", func(t *testing.T) {
		doc := &Document{
			OpenAPI: "3.1.0",
			Info:    Info{Title: "Minimal", Version: "1.0.0"},
		}

		data, err := doc.JSON()
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.NotContains(t, parsed, "paths")
		assert.NotContains(t, parsed, "components")
		assert.NotContains(t, parsed, "tags")
	})
}

func TestDocumentExportYAML(t *testing.T) {
	t.Run("serializes to valid YAML", func(t *testing.T) {
		doc := &Document{
			OpenAPI: "3.1.0",
			Info:    Info{Title: "Test", Version: "1.0.0"},
			Tags:    []Tag{{Name: "users", Description: "User ops"}},
		}

		data, err := doc.YAML()
		require.NoError(t, err)

		parsed, err := DocumentFromYAML(data)
		require.NoError(t, err)
		assert.Equal(t, "3.1.0", parsed.OpenAPI)
		assert.Equal(t, "Test", parsed.Info.Title)
		require.Len(t, parsed.Tags, 1)
		assert.Equal(t, "users", parsed.Tags[0].Name)
	})

	t.Run("roundtrip via DocumentFromYAML", func(t *testing.T) {
		original := &Document{
			OpenAPI: "3.1.0",
			Info:    Info{Title: "RT", Version: "3.0.0"},
			Paths: map[string]*PathItem{
				"/health": {Get: &Operation{Summary: "Health"}},
			},
		}

		data, err := original.YAML()
		require.NoError(t, err)

		parsed, err := DocumentFromYAML(data)
		require.NoError(t, err)
		assert.Equal(t, original.OpenAPI, parsed.OpenAPI)
		assert.Equal(t, original.Info, parsed.Info)
		require.Contains(t, parsed.Paths, "/health")
		assert.Equal(t, "Health", parsed.Paths["/health"].Get.Summary)
	})

	t.Run("minimal document", func(t *testing.T) {
		doc := &Document{
			OpenAPI: "3.1.0",
			Info:    Info{Title: "Minimal", Version: "1.0.0"},
		}

		data, err := doc.YAML()
		require.NoError(t, err)

		parsed, err := DocumentFromYAML(data)
		require.NoError(t, err)
		assert.Equal(t, "3.1.0", parsed.OpenAPI)
		assert.Equal(t, "Minimal", parsed.Info.Title)
	})
}

func TestDocumentFromJSON(t *testing.T) {
	t.Run("valid document", func(t *testing.T) {
		data := []byte(`{
			"openapi": "3.1.0",
			"info": {"title": "Test", "version": "1.0.0"},
			"paths": {
				"/users": {
					"get": {"summary": "List users"}
				}
			}
		}`)
		doc, err := DocumentFromJSON(data)
		require.NoError(t, err)
		assert.Equal(t, "3.1.0", doc.OpenAPI)
		assert.Equal(t, "Test", doc.Info.Title)
		assert.Equal(t, "1.0.0", doc.Info.Version)
		require.Contains(t, doc.Paths, "/users")
		assert.Equal(t, "List users", doc.Paths["/users"].Get.Summary)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		_, err := DocumentFromJSON([]byte(`{invalid`))
		assert.Error(t, err)
	})

	t.Run("empty object", func(t *testing.T) {
		doc, err := DocumentFromJSON([]byte(`{}`))
		require.NoError(t, err)
		assert.Empty(t, doc.OpenAPI)
		assert.Nil(t, doc.Paths)
	})

	t.Run("roundtrip", func(t *testing.T) {
		original := &Document{
			OpenAPI: "3.1.0",
			Info:    Info{Title: "Roundtrip", Version: "2.0.0"},
			Paths: map[string]*PathItem{
				"/health": {Get: &Operation{Summary: "Health check"}},
			},
			Components: &Components{
				Schemas: map[string]*Schema{
					"Error": {Type: TypeString("object")},
				},
			},
		}

		data, err := json.Marshal(original)
		require.NoError(t, err)

		parsed, err := DocumentFromJSON(data)
		require.NoError(t, err)
		assert.Equal(t, original.OpenAPI, parsed.OpenAPI)
		assert.Equal(t, original.Info, parsed.Info)
		require.Contains(t, parsed.Paths, "/health")
		assert.Equal(t, "Health check", parsed.Paths["/health"].Get.Summary)
		require.NotNil(t, parsed.Components)
		require.Contains(t, parsed.Components.Schemas, "Error")
		assert.Equal(t, TypeString("object"), parsed.Components.Schemas["Error"].Type)
	})
}

func TestDocumentFromYAML(t *testing.T) {
	t.Run("valid document", func(t *testing.T) {
		data := []byte(`
openapi: "3.1.0"
info:
  title: Test
  version: "1.0.0"
paths:
  /users:
    get:
      summary: List users
`)
		doc, err := DocumentFromYAML(data)
		require.NoError(t, err)
		assert.Equal(t, "3.1.0", doc.OpenAPI)
		assert.Equal(t, "Test", doc.Info.Title)
		assert.Equal(t, "1.0.0", doc.Info.Version)
		require.Contains(t, doc.Paths, "/users")
		assert.Equal(t, "List users", doc.Paths["/users"].Get.Summary)
	})

	t.Run("invalid YAML", func(t *testing.T) {
		_, err := DocumentFromYAML([]byte(`[invalid: yaml: :`))
		assert.Error(t, err)
	})

	t.Run("empty object", func(t *testing.T) {
		doc, err := DocumentFromYAML([]byte(`{}`))
		require.NoError(t, err)
		assert.Empty(t, doc.OpenAPI)
		assert.Nil(t, doc.Paths)
	})

	t.Run("roundtrip", func(t *testing.T) {
		original := &Document{
			OpenAPI: "3.1.0",
			Info:    Info{Title: "Roundtrip", Version: "2.0.0"},
			Tags:    []Tag{{Name: "users", Description: "User operations"}},
		}

		data, err := yaml.Marshal(original)
		require.NoError(t, err)

		parsed, err := DocumentFromYAML(data)
		require.NoError(t, err)
		assert.Equal(t, original.OpenAPI, parsed.OpenAPI)
		assert.Equal(t, original.Info, parsed.Info)
		require.Len(t, parsed.Tags, 1)
		assert.Equal(t, "users", parsed.Tags[0].Name)
		assert.Equal(t, "User operations", parsed.Tags[0].Description)
	})
}
