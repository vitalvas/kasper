package openapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSecurityConstructors(t *testing.T) {
	t.Run("BearerAuth with format", func(t *testing.T) {
		s := BearerAuth("JWT")
		assert.Equal(t, SecurityTypeHTTP, s.Type)
		assert.Equal(t, SchemeBearer, s.Scheme)
		assert.Equal(t, "JWT", s.BearerFormat)
	})

	t.Run("BearerAuth without format", func(t *testing.T) {
		s := BearerAuth("")
		assert.Equal(t, SecurityTypeHTTP, s.Type)
		assert.Equal(t, SchemeBearer, s.Scheme)
		assert.Empty(t, s.BearerFormat)
	})

	t.Run("BasicAuth", func(t *testing.T) {
		s := BasicAuth()
		assert.Equal(t, SecurityTypeHTTP, s.Type)
		assert.Equal(t, SchemeBasic, s.Scheme)
	})

	t.Run("APIKeyAuth", func(t *testing.T) {
		s := APIKeyAuth(SecurityInHeader, "X-API-Key")
		assert.Equal(t, SecurityTypeAPIKey, s.Type)
		assert.Equal(t, SecurityInHeader, s.In)
		assert.Equal(t, "X-API-Key", s.Name)
	})

	t.Run("OpenIDConnectAuth", func(t *testing.T) {
		s := OpenIDConnectAuth("https://idp.example.com/.well-known/openid-configuration")
		assert.Equal(t, SecurityTypeOpenIDConnect, s.Type)
		assert.Equal(t, "https://idp.example.com/.well-known/openid-configuration", s.OpenIDConnectURL)
	})
}

func TestRequireScheme(t *testing.T) {
	t.Run("with scopes", func(t *testing.T) {
		req := RequireScheme("oauth2", "read:users", "write:users")
		assert.Equal(t, []string{"read:users", "write:users"}, req["oauth2"])
	})

	t.Run("without scopes is non-nil empty", func(t *testing.T) {
		req := RequireScheme("bearerAuth")
		scopes, ok := req["bearerAuth"]
		assert.True(t, ok)
		assert.NotNil(t, scopes)
		assert.Empty(t, scopes)
	})
}
