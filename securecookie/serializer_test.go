package securecookie

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONSerializer(t *testing.T) {
	sz := JSONSerializer{}

	t.Run("round trip", func(t *testing.T) {
		src := map[string]int{"a": 1, "b": 2}
		data, err := sz.Serialize(src)
		require.NoError(t, err)

		var dst map[string]int
		err = sz.Deserialize(data, &dst)
		require.NoError(t, err)
		assert.Equal(t, src, dst)
	})

	t.Run("serialize nil", func(t *testing.T) {
		data, err := sz.Serialize(nil)
		require.NoError(t, err)
		assert.Equal(t, []byte("null"), data)
	})

	t.Run("deserialize invalid", func(t *testing.T) {
		var dst string
		err := sz.Deserialize([]byte("{invalid"), &dst)
		assert.Error(t, err)
	})
}
