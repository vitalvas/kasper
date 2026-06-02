package openapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchemaConstraints(t *testing.T) {
	t.Run("numeric bounds", func(t *testing.T) {
		s := IntegerSchema().WithMinimum(1).WithMaximum(100)
		require.NotNil(t, s.Minimum)
		require.NotNil(t, s.Maximum)
		assert.Equal(t, 1.0, *s.Minimum)
		assert.Equal(t, 100.0, *s.Maximum)
	})

	t.Run("exclusive bounds and multipleOf", func(t *testing.T) {
		s := NumberSchema().
			WithExclusiveMinimum(0).
			WithExclusiveMaximum(10).
			WithMultipleOf(0.5)
		require.NotNil(t, s.ExclusiveMinimum)
		require.NotNil(t, s.ExclusiveMaximum)
		require.NotNil(t, s.MultipleOf)
		assert.Equal(t, 0.0, *s.ExclusiveMinimum)
		assert.Equal(t, 10.0, *s.ExclusiveMaximum)
		assert.Equal(t, 0.5, *s.MultipleOf)
	})

	t.Run("string length and pattern", func(t *testing.T) {
		s := StringSchema().WithMinLength(1).WithMaxLength(200).WithPattern("^[a-z]+$")
		require.NotNil(t, s.MinLength)
		require.NotNil(t, s.MaxLength)
		assert.Equal(t, 1, *s.MinLength)
		assert.Equal(t, 200, *s.MaxLength)
		assert.Equal(t, "^[a-z]+$", s.Pattern)
	})

	t.Run("array constraints", func(t *testing.T) {
		s := ArraySchema(StringSchema()).
			WithMinItems(1).
			WithMaxItems(10).
			WithUniqueItems()
		require.NotNil(t, s.MinItems)
		require.NotNil(t, s.MaxItems)
		assert.Equal(t, 1, *s.MinItems)
		assert.Equal(t, 10, *s.MaxItems)
		assert.True(t, s.UniqueItems)
	})

	t.Run("WithItems", func(t *testing.T) {
		s := (&Schema{Type: SchemaTypeArray}).WithItems(IntegerSchema())
		require.NotNil(t, s.Items)
		assert.Equal(t, SchemaTypeInteger, s.Items.Type)
	})

	t.Run("WithEnum on any type", func(t *testing.T) {
		s := IntegerSchema().WithEnum(1, 2, 3)
		assert.Equal(t, []any{1, 2, 3}, s.Enum)
		assert.Equal(t, SchemaTypeInteger, s.Type)
	})

	t.Run("chaining with metadata modifiers", func(t *testing.T) {
		s := StringSchema().
			WithDescription("username").
			WithMinLength(3).
			WithMaxLength(32).
			WithPattern("^[a-z0-9_]+$")
		assert.Equal(t, "username", s.Description)
		require.NotNil(t, s.MinLength)
		assert.Equal(t, 3, *s.MinLength)
	})
}
