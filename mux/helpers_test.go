package mux

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanPath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "empty path", input: "", expected: "/"},
		{name: "root path", input: "/", expected: "/"},
		{name: "simple path", input: "/foo", expected: "/foo"},
		{name: "trailing slash", input: "/foo/", expected: "/foo/"},
		{name: "double slash", input: "/foo//bar", expected: "/foo/bar"},
		{name: "dot segments", input: "/foo/./bar", expected: "/foo/bar"},
		{name: "dotdot segments", input: "/foo/bar/../baz", expected: "/foo/baz"},
		{name: "no leading slash", input: "foo", expected: "/foo"},
		{name: "trailing slash preserved", input: "/foo/bar/", expected: "/foo/bar/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, cleanPath(tt.input))
		})
	}
}

func TestCheckPairs(t *testing.T) {
	t.Run("valid pairs", func(t *testing.T) {
		n, err := checkPairs("a", "b", "c", "d")
		require.NoError(t, err)
		assert.Equal(t, 2, n)
	})

	t.Run("empty pairs", func(t *testing.T) {
		n, err := checkPairs()
		require.NoError(t, err)
		assert.Equal(t, 0, n)
	})

	t.Run("odd number of pairs", func(t *testing.T) {
		_, err := checkPairs("a", "b", "c")
		assert.Error(t, err)
	})
}

func TestMapFromPairsToString(t *testing.T) {
	t.Run("valid pairs", func(t *testing.T) {
		m, err := mapFromPairsToString("key1", "val1", "key2", "val2")
		require.NoError(t, err)
		assert.Equal(t, "val1", m["key1"])
		assert.Equal(t, "val2", m["key2"])
	})

	t.Run("odd number of pairs returns error", func(t *testing.T) {
		_, err := mapFromPairsToString("key1")
		assert.Error(t, err)
	})
}

func TestMapFromPairsToRegex(t *testing.T) {
	t.Run("valid pairs", func(t *testing.T) {
		m, err := mapFromPairsToRegex("key1", "^[0-9]+$", "key2", "^[a-z]+$")
		require.NoError(t, err)
		assert.True(t, m["key1"].MatchString("123"))
		assert.False(t, m["key1"].MatchString("abc"))
		assert.True(t, m["key2"].MatchString("abc"))
	})

	t.Run("invalid regex returns error", func(t *testing.T) {
		_, err := mapFromPairsToRegex("key1", "[invalid")
		assert.Error(t, err)
	})

	t.Run("odd number of pairs returns error", func(t *testing.T) {
		_, err := mapFromPairsToRegex("key1")
		assert.Error(t, err)
	})
}

func TestUniqueVars(t *testing.T) {
	t.Run("no duplicates", func(t *testing.T) {
		err := uniqueVars([]string{"a", "b"}, []string{"c", "d"})
		assert.NoError(t, err)
	})

	t.Run("with duplicates", func(t *testing.T) {
		err := uniqueVars([]string{"a", "b"}, []string{"b", "c"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "duplicated route variable")
	})

	t.Run("empty slices", func(t *testing.T) {
		err := uniqueVars(nil, nil)
		assert.NoError(t, err)
	})
}

func TestMatchInArray(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		assert.True(t, matchInArray([]string{"a", "b", "c"}, "b"))
	})

	t.Run("not found", func(t *testing.T) {
		assert.False(t, matchInArray([]string{"a", "b", "c"}, "d"))
	})

	t.Run("empty array", func(t *testing.T) {
		assert.False(t, matchInArray(nil, "a"))
	})
}

func TestMatchMapWithString(t *testing.T) {
	t.Run("all keys match", func(t *testing.T) {
		toCheck := map[string]string{"Content-Type": "application/json"}
		toMatch := map[string][]string{"Content-Type": {"application/json"}}
		assert.True(t, matchMapWithString(toCheck, toMatch, false))
	})

	t.Run("key missing", func(t *testing.T) {
		toCheck := map[string]string{"X-Custom": "value"}
		toMatch := map[string][]string{"Content-Type": {"text/plain"}}
		assert.False(t, matchMapWithString(toCheck, toMatch, false))
	})

	t.Run("value mismatch", func(t *testing.T) {
		toCheck := map[string]string{"Content-Type": "text/html"}
		toMatch := map[string][]string{"Content-Type": {"application/json"}}
		assert.False(t, matchMapWithString(toCheck, toMatch, false))
	})

	t.Run("empty value matches any", func(t *testing.T) {
		toCheck := map[string]string{"Content-Type": ""}
		toMatch := map[string][]string{"Content-Type": {"anything"}}
		assert.True(t, matchMapWithString(toCheck, toMatch, false))
	})

	t.Run("canonical key matching", func(t *testing.T) {
		toCheck := map[string]string{"content-type": "text/plain"}
		toMatch := map[string][]string{"Content-Type": {"text/plain"}}
		assert.True(t, matchMapWithString(toCheck, toMatch, true))
	})
}

func TestMatchMapWithRegex(t *testing.T) {
	t.Run("regex matches", func(t *testing.T) {
		m, err := mapFromPairsToRegex("Content-Type", "^application/.*$")
		require.NoError(t, err)
		toMatch := map[string][]string{"Content-Type": {"application/json"}}
		assert.True(t, matchMapWithRegex(m, toMatch, false))
	})

	t.Run("regex does not match", func(t *testing.T) {
		m, err := mapFromPairsToRegex("Content-Type", "^text/.*$")
		require.NoError(t, err)
		toMatch := map[string][]string{"Content-Type": {"application/json"}}
		assert.False(t, matchMapWithRegex(m, toMatch, false))
	})

	t.Run("key missing", func(t *testing.T) {
		m, err := mapFromPairsToRegex("X-Custom", ".*")
		require.NoError(t, err)
		toMatch := map[string][]string{"Content-Type": {"text/plain"}}
		assert.False(t, matchMapWithRegex(m, toMatch, false))
	})
}

func TestSubtractSlice(t *testing.T) {
	t.Run("subtracts elements", func(t *testing.T) {
		result := subtractSlice([]string{"a", "b", "c", "d"}, []string{"b", "d"})
		assert.Equal(t, []string{"a", "c"}, result)
	})

	t.Run("nothing to subtract", func(t *testing.T) {
		result := subtractSlice([]string{"a", "b"}, []string{"c", "d"})
		assert.Equal(t, []string{"a", "b"}, result)
	})

	t.Run("empty input", func(t *testing.T) {
		result := subtractSlice(nil, []string{"a"})
		assert.Empty(t, result)
	})
}

func TestRequestURIPath(t *testing.T) {
	t.Run("uses RawPath when available", func(t *testing.T) {
		u := &url.URL{Path: "/foo/bar", RawPath: "/foo%2Fbar"}
		assert.Equal(t, "/foo%2Fbar", requestURIPath(u))
	})

	t.Run("falls back to Path", func(t *testing.T) {
		u := &url.URL{Path: "/foo/bar"}
		assert.Equal(t, "/foo/bar", requestURIPath(u))
	})
}

// --- Benchmarks ---

func BenchmarkCleanPath(b *testing.B) {
	paths := []string{
		"/",
		"/foo/bar",
		"/foo/../bar",
		"/foo/./bar//baz/",
		"/a/b/c/d/e/f/g",
	}
	b.ResetTimer()
	for b.Loop() {
		for _, p := range paths {
			cleanPath(p)
		}
	}
}

func BenchmarkMatchInArray(b *testing.B) {
	arr := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"}
	b.ResetTimer()
	for b.Loop() {
		matchInArray(arr, "DELETE")
	}
}

func BenchmarkMatchMapWithString(b *testing.B) {
	toCheck := map[string]string{
		"Content-Type":    "application/json",
		"Accept":          "text/html",
		"X-Request-Id":    "abc123",
		"Accept-Encoding": "gzip",
	}
	toMatch := map[string][]string{
		"Content-Type":    {"application/json"},
		"Accept":          {"text/html"},
		"X-Request-Id":    {"abc123"},
		"Accept-Encoding": {"gzip", "deflate"},
	}
	b.ResetTimer()
	for b.Loop() {
		matchMapWithString(toCheck, toMatch, true)
	}
}

// --- Fuzz ---

func FuzzCleanPath(f *testing.F) {
	f.Add("")
	f.Add("/")
	f.Add("/foo/bar")
	f.Add("/foo/../bar")
	f.Add("/foo/./bar//baz/")
	f.Add("no-leading-slash")
	f.Add("/a/b/../../../c")

	f.Fuzz(func(_ *testing.T, path string) {
		cleanPath(path)
	})
}
