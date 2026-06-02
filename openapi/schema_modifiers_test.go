package openapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSchemaModifiers(t *testing.T) {
	t.Run("WithFormat", func(t *testing.T) {
		s := StringSchema().WithFormat(FormatUUID)
		assert.Equal(t, FormatUUID, s.Format)
		assert.Equal(t, SchemaTypeString, s.Type)
	})

	t.Run("WithDescription", func(t *testing.T) {
		s := StringSchema().WithDescription("the name")
		assert.Equal(t, "the name", s.Description)
	})

	t.Run("WithExample", func(t *testing.T) {
		s := IntegerSchema().WithExample(42)
		assert.Equal(t, 42, s.Example)
	})

	t.Run("WithDefault", func(t *testing.T) {
		s := BooleanSchema().WithDefault(true)
		assert.Equal(t, true, s.Default)
	})

	t.Run("chaining", func(t *testing.T) {
		s := StringSchema().
			WithFormat(FormatEmail).
			WithDescription("contact email").
			WithExample("a@b.com")
		assert.Equal(t, FormatEmail, s.Format)
		assert.Equal(t, "contact email", s.Description)
		assert.Equal(t, "a@b.com", s.Example)
	})
}

func TestSchemaNullable(t *testing.T) {
	t.Run("adds null to single type", func(t *testing.T) {
		s := StringSchema().Nullable()
		assert.Equal(t, []string{"string", "null"}, s.Type.Values())
	})

	t.Run("idempotent", func(t *testing.T) {
		s := StringSchema().Nullable().Nullable()
		assert.Equal(t, []string{"string", "null"}, s.Type.Values())
	})

	t.Run("no-op on empty type", func(t *testing.T) {
		s := (&Schema{}).Nullable()
		assert.True(t, s.Type.IsEmpty())
	})
}
