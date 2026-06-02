package openapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchemaConstructors(t *testing.T) {
	t.Run("StringSchema", func(t *testing.T) {
		assert.Equal(t, SchemaTypeString, StringSchema().Type)
	})

	t.Run("IntegerSchema", func(t *testing.T) {
		assert.Equal(t, SchemaTypeInteger, IntegerSchema().Type)
	})

	t.Run("NumberSchema", func(t *testing.T) {
		assert.Equal(t, SchemaTypeNumber, NumberSchema().Type)
	})

	t.Run("BooleanSchema", func(t *testing.T) {
		assert.Equal(t, SchemaTypeBoolean, BooleanSchema().Type)
	})

	t.Run("EnumSchema", func(t *testing.T) {
		s := EnumSchema("yes", "no")
		assert.Equal(t, SchemaTypeString, s.Type)
		assert.Equal(t, []any{"yes", "no"}, s.Enum)
	})

	t.Run("ArraySchema", func(t *testing.T) {
		s := ArraySchema(StringSchema())
		assert.Equal(t, SchemaTypeArray, s.Type)
		require.NotNil(t, s.Items)
		assert.Equal(t, SchemaTypeString, s.Items.Type)
	})

	t.Run("ObjectSchema", func(t *testing.T) {
		assert.Equal(t, SchemaTypeObject, ObjectSchema().Type)
	})

	t.Run("distinct instances", func(t *testing.T) {
		// Constructors must not return aliased singletons.
		a := StringSchema()
		b := StringSchema()
		a.Description = "first"
		assert.Empty(t, b.Description)
	})
}
