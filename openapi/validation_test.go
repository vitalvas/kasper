package openapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParameterValidate(t *testing.T) {
	t.Run("valid query param", func(t *testing.T) {
		require.NoError(t, QueryParam("scope", "", StringSchema()).Validate())
	})

	t.Run("valid path param", func(t *testing.T) {
		require.NoError(t, PathParam("id", "", StringSchema()).Validate())
	})

	t.Run("empty name", func(t *testing.T) {
		err := (&Parameter{In: ParameterInQuery}).Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name must not be empty")
	})

	t.Run("invalid location", func(t *testing.T) {
		err := (&Parameter{Name: "x", In: "body"}).Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid location")
	})

	t.Run("path param not required", func(t *testing.T) {
		err := (&Parameter{Name: "id", In: ParameterInPath}).Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be required")
	})
}

func TestSecuritySchemeValidate(t *testing.T) {
	t.Run("valid bearer", func(t *testing.T) {
		require.NoError(t, BearerAuth("JWT").Validate())
	})

	t.Run("valid basic", func(t *testing.T) {
		require.NoError(t, BasicAuth().Validate())
	})

	t.Run("valid apiKey", func(t *testing.T) {
		require.NoError(t, APIKeyAuth(SecurityInHeader, "X-API-Key").Validate())
	})

	t.Run("valid openIdConnect", func(t *testing.T) {
		require.NoError(t, OpenIDConnectAuth("https://idp.example.com").Validate())
	})

	t.Run("valid oauth2", func(t *testing.T) {
		s := &SecurityScheme{Type: SecurityTypeOAuth2, Flows: &OAuthFlows{}}
		require.NoError(t, s.Validate())
	})

	t.Run("invalid type", func(t *testing.T) {
		err := (&SecurityScheme{Type: "mutualTLS"}).Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid security scheme type")
	})

	t.Run("http without scheme", func(t *testing.T) {
		err := (&SecurityScheme{Type: SecurityTypeHTTP}).Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must set scheme")
	})

	t.Run("apiKey without name", func(t *testing.T) {
		err := (&SecurityScheme{Type: SecurityTypeAPIKey, In: SecurityInHeader}).Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must set name")
	})

	t.Run("apiKey invalid location", func(t *testing.T) {
		err := (&SecurityScheme{Type: SecurityTypeAPIKey, Name: "k", In: "body"}).Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid location")
	})

	t.Run("oauth2 without flows", func(t *testing.T) {
		err := (&SecurityScheme{Type: SecurityTypeOAuth2}).Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must set flows")
	})

	t.Run("openIdConnect without url", func(t *testing.T) {
		err := (&SecurityScheme{Type: SecurityTypeOpenIDConnect}).Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must set openIdConnectUrl")
	})
}
