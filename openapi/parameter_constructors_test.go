package openapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
