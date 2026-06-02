package openapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchemaComposition(t *testing.T) {
	t.Run("OneOf", func(t *testing.T) {
		s := OneOf(StringSchema(), IntegerSchema())
		require.Len(t, s.OneOf, 2)
		assert.Equal(t, SchemaTypeString, s.OneOf[0].Type)
		assert.Equal(t, SchemaTypeInteger, s.OneOf[1].Type)
	})

	t.Run("AnyOf", func(t *testing.T) {
		s := AnyOf(StringSchema(), BooleanSchema())
		require.Len(t, s.AnyOf, 2)
	})

	t.Run("AllOf", func(t *testing.T) {
		s := AllOf(RefSchema("Base"), ObjectSchema())
		require.Len(t, s.AllOf, 2)
	})

	t.Run("Not", func(t *testing.T) {
		s := Not(StringSchema())
		require.NotNil(t, s.Not)
		assert.Equal(t, SchemaTypeString, s.Not.Type)
	})

	t.Run("RefSchema", func(t *testing.T) {
		s := RefSchema("User")
		assert.Equal(t, "#/components/schemas/User", s.Ref)
	})

	t.Run("WithDiscriminator", func(t *testing.T) {
		s := OneOf(RefSchema("Cat"), RefSchema("Dog")).
			WithDiscriminator("petType", map[string]string{
				"cat": "#/components/schemas/Cat",
				"dog": "#/components/schemas/Dog",
			})
		require.NotNil(t, s.Discriminator)
		assert.Equal(t, "petType", s.Discriminator.PropertyName)
		assert.Equal(t, "#/components/schemas/Cat", s.Discriminator.Mapping["cat"])
	})

	t.Run("WithDiscriminator implicit mapping", func(t *testing.T) {
		s := OneOf(RefSchema("Cat")).WithDiscriminator("petType", nil)
		require.NotNil(t, s.Discriminator)
		assert.Nil(t, s.Discriminator.Mapping)
	})
}
