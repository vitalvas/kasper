package openapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHeaderConstructors(t *testing.T) {
	t.Run("StringHeader", func(t *testing.T) {
		h := StringHeader("User identifier sent by the IdP")
		assert.Equal(t, "User identifier sent by the IdP", h.Description)
		assert.Equal(t, SchemaTypeString, h.Schema.Type)
	})

	t.Run("IntegerHeader", func(t *testing.T) {
		h := IntegerHeader("Seconds the client should wait")
		assert.Equal(t, SchemaTypeInteger, h.Schema.Type)
	})

	t.Run("HeaderOf uses caller-supplied schema", func(t *testing.T) {
		h := HeaderOf("Content type", EnumSchema("application/json", "application/jwt"))
		assert.Equal(t, SchemaTypeString, h.Schema.Type)
		assert.Equal(t, []any{"application/json", "application/jwt"}, h.Schema.Enum)
	})
}
