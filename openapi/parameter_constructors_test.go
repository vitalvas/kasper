package openapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParameterConstructors(t *testing.T) {
	t.Run("QueryParam", func(t *testing.T) {
		p := QueryParam("scope", "Requested OAuth scopes", StringSchema())
		assert.Equal(t, "scope", p.Name)
		assert.Equal(t, ParameterInQuery, p.In)
		assert.Equal(t, "Requested OAuth scopes", p.Description)
		assert.False(t, p.Required)
		assert.Equal(t, SchemaTypeString, p.Schema.Type)
	})

	t.Run("RequiredQueryParam", func(t *testing.T) {
		p := RequiredQueryParam("client_id", "RP identifier", StringSchema())
		assert.True(t, p.Required)
		assert.Equal(t, ParameterInQuery, p.In)
	})

	t.Run("PathParam is always required", func(t *testing.T) {
		p := PathParam("app", "App name", StringSchema())
		assert.Equal(t, ParameterInPath, p.In)
		assert.True(t, p.Required, "OpenAPI requires path parameters to be required")
	})

	t.Run("HeaderParam", func(t *testing.T) {
		p := HeaderParam("X-Trace-Id", "Trace correlator", StringSchema())
		assert.Equal(t, ParameterInHeader, p.In)
		assert.False(t, p.Required)
	})

	t.Run("RequiredHeaderParam", func(t *testing.T) {
		p := RequiredHeaderParam("Sec-Fetch-Dest", "Browser hint", StringSchema())
		assert.True(t, p.Required)
	})

	t.Run("CookieParam", func(t *testing.T) {
		p := CookieParam("session", "Session cookie", StringSchema())
		assert.Equal(t, ParameterInCookie, p.In)
	})
}

func TestQueryParamsFromStruct(t *testing.T) {
	byName := func(params []*Parameter) map[string]*Parameter {
		m := make(map[string]*Parameter, len(params))
		for _, p := range params {
			m[p.Name] = p
		}
		return m
	}

	t.Run("named tags with required and optional", func(t *testing.T) {
		type query struct {
			Page  int      `query:"page,omitempty"`
			Limit int      `query:"limit,omitempty" openapi:"maximum=100"`
			Tags  []string `query:"tag,omitempty"`
			Scope string   `query:"scope"`
		}

		params := QueryParamsFromStruct(query{})
		require.Len(t, params, 4)
		m := byName(params)

		require.Contains(t, m, "page")
		assert.Equal(t, ParameterInQuery, m["page"].In)
		assert.False(t, m["page"].Required)
		assert.Equal(t, SchemaTypeInteger, m["page"].Schema.Type)

		require.Contains(t, m, "limit")
		assert.False(t, m["limit"].Required)
		require.NotNil(t, m["limit"].Schema.Maximum)
		assert.Equal(t, 100.0, *m["limit"].Schema.Maximum)

		require.Contains(t, m, "tag")
		assert.Equal(t, SchemaTypeArray, m["tag"].Schema.Type)
		require.NotNil(t, m["tag"].Schema.Items)
		assert.Equal(t, SchemaTypeString, m["tag"].Schema.Items.Type)

		require.Contains(t, m, "scope")
		assert.True(t, m["scope"].Required, "field without omitempty is required")
	})

	t.Run("untagged exported field uses field name", func(t *testing.T) {
		type query struct {
			Search string
		}

		params := QueryParamsFromStruct(query{})
		require.Len(t, params, 1)
		assert.Equal(t, "Search", params[0].Name)
		assert.True(t, params[0].Required)
	})

	t.Run("tag with only options keeps field name", func(t *testing.T) {
		type query struct {
			Cursor string `query:",omitempty"`
		}

		params := QueryParamsFromStruct(query{})
		require.Len(t, params, 1)
		assert.Equal(t, "Cursor", params[0].Name)
		assert.False(t, params[0].Required)
	})

	t.Run("dash tag is skipped", func(t *testing.T) {
		type query struct {
			Visible string `query:"visible"`
			Hidden  string `query:"-"`
		}

		params := QueryParamsFromStruct(query{})
		require.Len(t, params, 1)
		assert.Equal(t, "visible", params[0].Name)
	})

	t.Run("unexported field is skipped", func(t *testing.T) {
		type query struct {
			Public  string `query:"public"`
			private string `query:"private"` //nolint:unused
		}

		params := QueryParamsFromStruct(query{})
		require.Len(t, params, 1)
		assert.Equal(t, "public", params[0].Name)
	})

	t.Run("accepts pointer to struct", func(t *testing.T) {
		type query struct {
			Page int `query:"page,omitempty"`
		}

		params := QueryParamsFromStruct(&query{})
		require.Len(t, params, 1)
		assert.Equal(t, "page", params[0].Name)
	})

	t.Run("nil returns nil", func(t *testing.T) {
		assert.Nil(t, QueryParamsFromStruct(nil))
	})

	t.Run("non-struct returns nil", func(t *testing.T) {
		assert.Nil(t, QueryParamsFromStruct(42))
		assert.Nil(t, QueryParamsFromStruct("query"))
	})
}
