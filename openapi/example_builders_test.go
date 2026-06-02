package openapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExampleBuilders(t *testing.T) {
	t.Run("NewExample", func(t *testing.T) {
		e := NewExample("A user", map[string]any{"id": 1})
		assert.Equal(t, "A user", e.Summary)
		assert.Equal(t, map[string]any{"id": 1}, e.Value)
		assert.Empty(t, e.ExternalValue)
	})

	t.Run("ExternalExample", func(t *testing.T) {
		e := ExternalExample("Big payload", "https://example.com/sample.json")
		assert.Equal(t, "Big payload", e.Summary)
		assert.Equal(t, "https://example.com/sample.json", e.ExternalValue)
		assert.Nil(t, e.Value)
	})

	t.Run("Example WithDescription", func(t *testing.T) {
		e := NewExample("A user", nil).WithDescription("the canonical user")
		assert.Equal(t, "the canonical user", e.Description)
	})

	t.Run("Parameter WithExamples", func(t *testing.T) {
		p := QueryParam("status", "", StringSchema()).
			WithExamples(map[string]*Example{
				"active":   NewExample("Active", "active"),
				"archived": NewExample("Archived", "archived"),
			})
		require.Len(t, p.Examples, 2)
		assert.Equal(t, "active", p.Examples["active"].Value)
	})

	t.Run("Header WithExamples", func(t *testing.T) {
		h := StringHeader("trace id").
			WithExamples(map[string]*Example{"sample": NewExample("Sample", "abc-123")})
		require.Len(t, h.Examples, 1)
		assert.Equal(t, "abc-123", h.Examples["sample"].Value)
	})

	t.Run("MediaType WithExamples", func(t *testing.T) {
		m := (&MediaType{Schema: StringSchema()}).
			WithExamples(map[string]*Example{"sample": NewExample("Sample", "x")})
		require.Len(t, m.Examples, 1)
	})
}
