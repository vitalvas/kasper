package mux

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandMacro(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expected       string
		expectCompiled bool
	}{
		{name: "uuid", input: "uuid", expected: `[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`, expectCompiled: true},
		{name: "int", input: "int", expected: `[0-9]+`, expectCompiled: true},
		{name: "float", input: "float", expected: `[0-9]*\.?[0-9]+`, expectCompiled: true},
		{name: "slug", input: "slug", expected: `[a-zA-Z0-9]+(?:-[a-zA-Z0-9]+)*`, expectCompiled: true},
		{name: "alpha", input: "alpha", expected: `[a-zA-Z]+`, expectCompiled: true},
		{name: "alphanum", input: "alphanum", expected: `[a-zA-Z0-9]+`, expectCompiled: true},
		{name: "date", input: "date", expected: `[0-9]{4}-[0-9]{2}-[0-9]{2}`, expectCompiled: true},
		{name: "hex", input: "hex", expected: `[0-9a-fA-F]+`, expectCompiled: true},
		{name: "domain", input: "domain", expected: `(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)*[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?`, expectCompiled: true},
		{name: "unknown returns input unchanged", input: "[0-9]+", expected: `[0-9]+`, expectCompiled: false},
		{name: "empty string", input: "", expected: "", expectCompiled: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pattern, compiled := expandMacro(tt.input)
			assert.Equal(t, tt.expected, pattern)
			if tt.expectCompiled {
				assert.NotNil(t, compiled)
			} else {
				assert.Nil(t, compiled)
			}
		})
	}
}

func TestLengthMatcher(t *testing.T) {
	re := regexp.MustCompile(`^[a-z]+$`)
	m := &lengthMatcher{re: re, maxLen: 5}

	t.Run("matches within limit", func(t *testing.T) {
		assert.True(t, m.MatchString("abc"))
	})

	t.Run("matches at exact limit", func(t *testing.T) {
		assert.True(t, m.MatchString("abcde"))
	})

	t.Run("rejects over limit", func(t *testing.T) {
		assert.False(t, m.MatchString("abcdef"))
	})

	t.Run("rejects regex mismatch within limit", func(t *testing.T) {
		assert.False(t, m.MatchString("123"))
	})

	t.Run("matches empty string", func(t *testing.T) {
		assert.False(t, m.MatchString(""))
	})

	t.Run("String returns regex pattern", func(t *testing.T) {
		assert.Equal(t, `^[a-z]+$`, m.String())
	})
}

func TestRouteMacroPatterns(t *testing.T) {
	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name        string
		path        string
		requestPath string
		shouldMatch bool
	}{
		{name: "uuid matches valid UUID", path: "/users/{id:uuid}", requestPath: "/users/550e8400-e29b-41d4-a716-446655440000", shouldMatch: true},
		{name: "uuid rejects invalid", path: "/users/{id:uuid}", requestPath: "/users/not-a-uuid", shouldMatch: false},
		{name: "int matches digits", path: "/pages/{page:int}", requestPath: "/pages/42", shouldMatch: true},
		{name: "int rejects non-digits", path: "/pages/{page:int}", requestPath: "/pages/abc", shouldMatch: false},
		{name: "float matches decimal", path: "/values/{val:float}", requestPath: "/values/3.14", shouldMatch: true},
		{name: "float matches integer", path: "/values/{val:float}", requestPath: "/values/42", shouldMatch: true},
		{name: "slug matches valid slug", path: "/posts/{s:slug}", requestPath: "/posts/my-post-title", shouldMatch: true},
		{name: "slug rejects leading hyphen", path: "/posts/{s:slug}", requestPath: "/posts/-bad", shouldMatch: false},
		{name: "alpha matches letters", path: "/names/{name:alpha}", requestPath: "/names/hello", shouldMatch: true},
		{name: "alpha rejects digits", path: "/names/{name:alpha}", requestPath: "/names/hello123", shouldMatch: false},
		{name: "alphanum matches mixed", path: "/tokens/{token:alphanum}", requestPath: "/tokens/abc123", shouldMatch: true},
		{name: "alphanum rejects special chars", path: "/tokens/{token:alphanum}", requestPath: "/tokens/abc-123", shouldMatch: false},
		{name: "date matches ISO date", path: "/events/{d:date}", requestPath: "/events/2024-01-15", shouldMatch: true},
		{name: "date rejects invalid format", path: "/events/{d:date}", requestPath: "/events/01-15-2024", shouldMatch: false},
		{name: "hex matches hex string", path: "/colors/{h:hex}", requestPath: "/colors/deadBEEF", shouldMatch: true},
		{name: "hex rejects non-hex", path: "/colors/{h:hex}", requestPath: "/colors/xyz", shouldMatch: false},
		{name: "domain matches simple", path: "/sites/{d:domain}", requestPath: "/sites/example.com", shouldMatch: true},
		{name: "domain matches subdomain", path: "/sites/{d:domain}", requestPath: "/sites/sub.example.com", shouldMatch: true},
		{name: "domain matches hyphenated", path: "/sites/{d:domain}", requestPath: "/sites/my-site.example.co.uk", shouldMatch: true},
		{name: "domain matches single char", path: "/sites/{d:domain}", requestPath: "/sites/a", shouldMatch: true},
		{name: "domain matches single char label with TLD", path: "/sites/{d:domain}", requestPath: "/sites/a.b", shouldMatch: true},
		{name: "domain matches single label", path: "/sites/{d:domain}", requestPath: "/sites/localhost", shouldMatch: true},
		{name: "domain matches 63-char label", path: "/sites/{d:domain}", requestPath: "/sites/a" + strings.Repeat("b", 61) + "c.com", shouldMatch: true},
		{name: "domain rejects 64-char label", path: "/sites/{d:domain}", requestPath: "/sites/a" + strings.Repeat("b", 62) + "c.com", shouldMatch: false},
		{name: "domain rejects leading hyphen", path: "/sites/{d:domain}", requestPath: "/sites/-bad.com", shouldMatch: false},
		{name: "domain rejects trailing hyphen", path: "/sites/{d:domain}", requestPath: "/sites/bad-.com", shouldMatch: false},
		{name: "domain matches 253-char total", path: "/sites/{d:domain}", requestPath: "/sites/" + strings.Repeat("a.", 126) + "b", shouldMatch: true},
		{name: "domain rejects 254-char total", path: "/sites/{d:domain}", requestPath: "/sites/" + strings.Repeat("a.", 126) + "bb", shouldMatch: false},
		{name: "raw regex still works", path: "/items/{id:[0-9]+}", requestPath: "/items/123", shouldMatch: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewRouter()
			router.HandleFunc(tt.path, dummyHandler)

			req := httptest.NewRequest(http.MethodGet, tt.requestPath, nil)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			if tt.shouldMatch {
				assert.Equal(t, http.StatusOK, rr.Code, "expected match for %s -> %s", tt.path, tt.requestPath)
			} else {
				assert.Equal(t, http.StatusNotFound, rr.Code, "expected no match for %s -> %s", tt.path, tt.requestPath)
			}
		})
	}
}

func TestMacroVarsExtracted(t *testing.T) {
	expectedUUID := "550e8400-e29b-41d4-a716-446655440000"
	var extractedVars map[string]string

	router := NewRouter()
	router.HandleFunc("/users/{id:uuid}", func(_ http.ResponseWriter, r *http.Request) {
		extractedVars = Vars(r)
	})

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/users/%s", expectedUUID), nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.NotNil(t, extractedVars)
	assert.Equal(t, expectedUUID, extractedVars["id"])
}

func TestMacroURLBuilding(t *testing.T) {
	router := NewRouter()
	router.HandleFunc("/users/{id:uuid}",
		func(http.ResponseWriter, *http.Request) {},
	).Name("user")

	url, err := router.Get("user").URL("id", "550e8400-e29b-41d4-a716-446655440000")
	require.NoError(t, err)
	assert.Equal(t, "/users/550e8400-e29b-41d4-a716-446655440000", url.Path)
}

// --- Benchmarks ---

func BenchmarkExpandMacro(b *testing.B) {
	macros := []string{"uuid", "int", "float", "slug", "alpha", "alphanum", "date", "hex", "[0-9]+"}
	b.ResetTimer()
	for b.Loop() {
		for _, m := range macros {
			expandMacro(m) //nolint:errcheck
		}
	}
}

func BenchmarkNewRouteRegexpWithMacro(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		newRouteRegexp("/users/{id:uuid}/posts/{page:int}", regexpTypePath, routeRegexpOptions{}) //nolint:errcheck
	}
}
