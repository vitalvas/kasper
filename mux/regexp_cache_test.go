package mux

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompileRegexp(t *testing.T) {
	t.Run("compiles valid pattern", func(t *testing.T) {
		re, err := compileRegexp(`^[0-9]+$`)
		require.NoError(t, err)
		assert.True(t, re.MatchString("123"))
		assert.False(t, re.MatchString("abc"))
	})

	t.Run("returns cached instance", func(t *testing.T) {
		re1, err := compileRegexp(`^cached-test-[a-z]+$`)
		require.NoError(t, err)
		re2, err := compileRegexp(`^cached-test-[a-z]+$`)
		require.NoError(t, err)
		assert.Same(t, re1, re2)
	})

	t.Run("invalid pattern returns error", func(t *testing.T) {
		_, err := compileRegexp(`^([0-9+$`)
		assert.Error(t, err)
	})
}

// --- Benchmarks ---

func BenchmarkCompileRegexpCached(b *testing.B) {
	// Prime the cache.
	compileRegexp(`^[0-9]+$`) //nolint:errcheck

	b.ResetTimer()
	for b.Loop() {
		compileRegexp(`^[0-9]+$`) //nolint:errcheck
	}
}

func BenchmarkCompileRegexpColdVsHot(b *testing.B) {
	b.Run("cold", func(b *testing.B) {
		for b.Loop() {
			// Unique pattern each time to bypass cache.
			compileRegexp(`^unique-cold-[0-9]+$`) //nolint:errcheck
		}
	})

	b.Run("hot", func(b *testing.B) {
		compileRegexp(`^unique-hot-[0-9]+$`) //nolint:errcheck
		b.ResetTimer()
		for b.Loop() {
			compileRegexp(`^unique-hot-[0-9]+$`) //nolint:errcheck
		}
	})
}
