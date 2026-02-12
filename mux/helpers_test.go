package mux

import (
	"net/http"
	"net/http/httptest"
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
	tests := []struct {
		name      string
		input     []string
		expectedN int
		expectErr bool
	}{
		{name: "valid pairs", input: []string{"a", "b", "c", "d"}, expectedN: 2, expectErr: false},
		{name: "empty pairs", input: []string{}, expectedN: 0, expectErr: false},
		{name: "odd number of pairs", input: []string{"a", "b", "c"}, expectedN: 0, expectErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, err := checkPairs(tt.input...)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedN, n)
			}
		})
	}
}

func TestMapFromPairsToString(t *testing.T) {
	tests := []struct {
		name      string
		input     []string
		expected  map[string]string
		expectErr bool
	}{
		{
			name:      "valid pairs",
			input:     []string{"key1", "val1", "key2", "val2"},
			expected:  map[string]string{"key1": "val1", "key2": "val2"},
			expectErr: false,
		},
		{
			name:      "odd number of pairs returns error",
			input:     []string{"key1"},
			expected:  nil,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := mapFromPairsToString(tt.input...)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, m)
			}
		})
	}
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
	tests := []struct {
		name      string
		a         []string
		b         []string
		expectErr bool
	}{
		{name: "no duplicates", a: []string{"a", "b"}, b: []string{"c", "d"}, expectErr: false},
		{name: "with duplicates", a: []string{"a", "b"}, b: []string{"b", "c"}, expectErr: true},
		{name: "empty slices", a: nil, b: nil, expectErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := uniqueVars(tt.a, tt.b)
			if tt.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "duplicated route variable")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMatchInArray(t *testing.T) {
	tests := []struct {
		name     string
		arr      []string
		val      string
		expected bool
	}{
		{name: "found", arr: []string{"a", "b", "c"}, val: "b", expected: true},
		{name: "not found", arr: []string{"a", "b", "c"}, val: "d", expected: false},
		{name: "empty array", arr: nil, val: "a", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, matchInArray(tt.arr, tt.val))
		})
	}
}

func TestMatchMapWithString(t *testing.T) {
	tests := []struct {
		name      string
		toCheck   map[string]string
		toMatch   map[string][]string
		canonical bool
		expected  bool
	}{
		{
			name:      "all keys match",
			toCheck:   map[string]string{"Content-Type": "application/json"},
			toMatch:   map[string][]string{"Content-Type": {"application/json"}},
			canonical: false,
			expected:  true,
		},
		{
			name:      "key missing",
			toCheck:   map[string]string{"X-Custom": "value"},
			toMatch:   map[string][]string{"Content-Type": {"text/plain"}},
			canonical: false,
			expected:  false,
		},
		{
			name:      "value mismatch",
			toCheck:   map[string]string{"Content-Type": "text/html"},
			toMatch:   map[string][]string{"Content-Type": {"application/json"}},
			canonical: false,
			expected:  false,
		},
		{
			name:      "empty value matches any",
			toCheck:   map[string]string{"Content-Type": ""},
			toMatch:   map[string][]string{"Content-Type": {"anything"}},
			canonical: false,
			expected:  true,
		},
		{
			name:      "canonical key matching",
			toCheck:   map[string]string{"content-type": "text/plain"},
			toMatch:   map[string][]string{"Content-Type": {"text/plain"}},
			canonical: true,
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, matchMapWithString(tt.toCheck, tt.toMatch, tt.canonical))
		})
	}
}

func TestMatchMapWithRegex(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		pattern  string
		toMatch  map[string][]string
		expected bool
	}{
		{
			name:     "regex matches",
			key:      "Content-Type",
			pattern:  "^application/.*$",
			toMatch:  map[string][]string{"Content-Type": {"application/json"}},
			expected: true,
		},
		{
			name:     "regex does not match",
			key:      "Content-Type",
			pattern:  "^text/.*$",
			toMatch:  map[string][]string{"Content-Type": {"application/json"}},
			expected: false,
		},
		{
			name:     "key missing",
			key:      "X-Custom",
			pattern:  ".*",
			toMatch:  map[string][]string{"Content-Type": {"text/plain"}},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := mapFromPairsToRegex(tt.key, tt.pattern)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, matchMapWithRegex(m, tt.toMatch, false))
		})
	}
}

func TestSubtractSlice(t *testing.T) {
	tests := []struct {
		name     string
		a        []string
		b        []string
		expected []string
	}{
		{name: "subtracts elements", a: []string{"a", "b", "c", "d"}, b: []string{"b", "d"}, expected: []string{"a", "c"}},
		{name: "nothing to subtract", a: []string{"a", "b"}, b: []string{"c", "d"}, expected: []string{"a", "b"}},
		{name: "empty input", a: nil, b: []string{"a"}, expected: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := subtractSlice(tt.a, tt.b)
			if tt.expected == nil {
				assert.Empty(t, result)
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestRequestURIPath(t *testing.T) {
	tests := []struct {
		name     string
		url      *url.URL
		expected string
	}{
		{
			name:     "uses RawPath when available",
			url:      &url.URL{Path: "/foo/bar", RawPath: "/foo%2Fbar"},
			expected: "/foo%2Fbar",
		},
		{
			name:     "falls back to Path",
			url:      &url.URL{Path: "/foo/bar"},
			expected: "/foo/bar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, requestURIPath(tt.url))
		})
	}
}

func TestAllowedMethods(t *testing.T) {
	t.Run("returns sorted methods that match the path", func(t *testing.T) {
		r := NewRouter()
		r.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).Methods(http.MethodPost)
		r.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).Methods(http.MethodGet)

		req := httptest.NewRequest(http.MethodDelete, "/users", nil)
		allowed := allowedMethods(r, req)
		assert.Equal(t, []string{http.MethodGet, http.MethodPost}, allowed)
	})

	t.Run("excludes the request method", func(t *testing.T) {
		r := NewRouter()
		r.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).Methods(http.MethodGet, http.MethodPost)

		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		allowed := allowedMethods(r, req)
		assert.Equal(t, []string{http.MethodPost}, allowed)
	})

	t.Run("returns empty for path with no matching routes", func(t *testing.T) {
		r := NewRouter()
		r.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).Methods(http.MethodGet)

		req := httptest.NewRequest(http.MethodGet, "/posts", nil)
		allowed := allowedMethods(r, req)
		assert.Empty(t, allowed)
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
