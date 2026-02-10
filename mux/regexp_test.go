package mux

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBraceIndices(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  []int
		expectErr bool
	}{
		{name: "no braces", input: "/foo/bar", expected: nil},
		{name: "single variable", input: "/foo/{id}", expected: []int{5, 9}},
		{name: "two variables", input: "/{a}/{b}", expected: []int{1, 4, 5, 8}},
		{name: "variable with pattern", input: "/{id:[0-9]+}", expected: []int{1, 12}},
		{name: "nested braces", input: "/{id:{nested}}", expected: []int{1, 14}},
		{name: "unbalanced open", input: "/{id", expectErr: true},
		{name: "unbalanced close", input: "/id}", expectErr: true},
		{name: "empty string", input: "", expected: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idxs, err := braceIndices(tt.input)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, idxs)
			}
		})
	}
}

func TestCheckDuplicateVars(t *testing.T) {
	t.Run("no duplicates", func(t *testing.T) {
		assert.NoError(t, checkDuplicateVars([]string{"a", "b", "c"}))
	})

	t.Run("with duplicates", func(t *testing.T) {
		err := checkDuplicateVars([]string{"a", "b", "a"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "duplicated route variable")
	})

	t.Run("empty", func(t *testing.T) {
		assert.NoError(t, checkDuplicateVars(nil))
	})
}

func TestGetHost(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		expected string
	}{
		{name: "simple host", host: "example.com", expected: "example.com"},
		{name: "host with port", host: "example.com:8080", expected: "example.com"},
		{name: "uppercase host", host: "Example.COM", expected: "example.com"},
		{name: "host with port uppercase", host: "Example.COM:443", expected: "example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.Host = tt.host
			assert.Equal(t, tt.expected, getHost(r))
		})
	}
}

func TestNewRouteRegexp(t *testing.T) {
	t.Run("simple path", func(t *testing.T) {
		rr, err := newRouteRegexp("/foo/bar", regexpTypePath, routeRegexpOptions{})
		require.NoError(t, err)
		assert.Equal(t, "/foo/bar", rr.template)
		assert.True(t, rr.regexp.MatchString("/foo/bar"))
		assert.False(t, rr.regexp.MatchString("/foo/baz"))
		assert.Empty(t, rr.varsN)
	})

	t.Run("path with variable", func(t *testing.T) {
		rr, err := newRouteRegexp("/users/{id}", regexpTypePath, routeRegexpOptions{})
		require.NoError(t, err)
		assert.Equal(t, "/users/{id}", rr.template)
		assert.True(t, rr.regexp.MatchString("/users/42"))
		assert.True(t, rr.regexp.MatchString("/users/abc"))
		assert.False(t, rr.regexp.MatchString("/users/"))
		assert.Equal(t, []string{"id"}, rr.varsN)
	})

	t.Run("path with pattern variable", func(t *testing.T) {
		rr, err := newRouteRegexp("/users/{id:[0-9]+}", regexpTypePath, routeRegexpOptions{})
		require.NoError(t, err)
		assert.True(t, rr.regexp.MatchString("/users/42"))
		assert.False(t, rr.regexp.MatchString("/users/abc"))
		assert.Equal(t, []string{"id"}, rr.varsN)
	})

	t.Run("multiple variables", func(t *testing.T) {
		rr, err := newRouteRegexp("/users/{id}/posts/{pid}", regexpTypePath, routeRegexpOptions{})
		require.NoError(t, err)
		assert.True(t, rr.regexp.MatchString("/users/42/posts/123"))
		assert.Equal(t, []string{"id", "pid"}, rr.varsN)
	})

	t.Run("host type", func(t *testing.T) {
		rr, err := newRouteRegexp("{subdomain}.example.com", regexpTypeHost, routeRegexpOptions{})
		require.NoError(t, err)
		assert.True(t, rr.matchHost)
		assert.True(t, rr.regexp.MatchString("api.example.com"))
		assert.Equal(t, []string{"subdomain"}, rr.varsN)
	})

	t.Run("prefix type", func(t *testing.T) {
		rr, err := newRouteRegexp("/api/v1", regexpTypePrefix, routeRegexpOptions{})
		require.NoError(t, err)
		assert.True(t, rr.wildcard)
		assert.True(t, rr.regexp.MatchString("/api/v1/users"))
		assert.True(t, rr.regexp.MatchString("/api/v1"))
		assert.False(t, rr.regexp.MatchString("/api/v2"))
	})

	t.Run("strict slash", func(t *testing.T) {
		rr, err := newRouteRegexp("/users", regexpTypePath, routeRegexpOptions{strictSlash: true})
		require.NoError(t, err)
		assert.True(t, rr.regexp.MatchString("/users"))
		assert.True(t, rr.regexp.MatchString("/users/"))
	})

	t.Run("strict slash with trailing slash template", func(t *testing.T) {
		rr, err := newRouteRegexp("/users/", regexpTypePath, routeRegexpOptions{strictSlash: true})
		require.NoError(t, err)
		assert.True(t, rr.regexp.MatchString("/users"))
		assert.True(t, rr.regexp.MatchString("/users/"))
	})

	t.Run("duplicate variables error", func(t *testing.T) {
		_, err := newRouteRegexp("/{id}/{id}", regexpTypePath, routeRegexpOptions{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "duplicated route variable")
	})

	t.Run("empty variable name error", func(t *testing.T) {
		_, err := newRouteRegexp("/{}", regexpTypePath, routeRegexpOptions{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing name")
	})

	t.Run("unbalanced braces error", func(t *testing.T) {
		_, err := newRouteRegexp("/{id", regexpTypePath, routeRegexpOptions{})
		assert.Error(t, err)
	})

	t.Run("invalid regex in variable pattern", func(t *testing.T) {
		_, err := newRouteRegexp("/{id:(}", regexpTypePath, routeRegexpOptions{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid pattern")
	})
}

func TestRouteRegexpMatch(t *testing.T) {
	t.Run("path match", func(t *testing.T) {
		rr, err := newRouteRegexp("/users/{id}", regexpTypePath, routeRegexpOptions{})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
		assert.True(t, rr.Match(req, &RouteMatch{}))
	})

	t.Run("path no match", func(t *testing.T) {
		rr, err := newRouteRegexp("/users/{id:[0-9]+}", regexpTypePath, routeRegexpOptions{})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodGet, "/users/abc", nil)
		assert.False(t, rr.Match(req, &RouteMatch{}))
	})

	t.Run("host match", func(t *testing.T) {
		rr, err := newRouteRegexp("{sub}.example.com", regexpTypeHost, routeRegexpOptions{})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodGet, "http://api.example.com/", nil)
		req.Host = "api.example.com"
		assert.True(t, rr.Match(req, &RouteMatch{}))
	})

	t.Run("query match", func(t *testing.T) {
		rr, err := newRouteRegexp("page={page:[0-9]+}", regexpTypeQuery, routeRegexpOptions{})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodGet, "/search?page=42", nil)
		assert.True(t, rr.Match(req, &RouteMatch{}))
	})

	t.Run("query no match", func(t *testing.T) {
		rr, err := newRouteRegexp("page={page:[0-9]+}", regexpTypeQuery, routeRegexpOptions{})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodGet, "/search?page=abc", nil)
		assert.False(t, rr.Match(req, &RouteMatch{}))
	})

	t.Run("query missing key", func(t *testing.T) {
		rr, err := newRouteRegexp("page={page:[0-9]+}", regexpTypeQuery, routeRegexpOptions{})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodGet, "/search", nil)
		assert.False(t, rr.Match(req, &RouteMatch{}))
	})
}

func TestRouteRegexpURL(t *testing.T) {
	t.Run("builds URL from variables", func(t *testing.T) {
		rr, err := newRouteRegexp("/users/{id}/posts/{pid}", regexpTypePath, routeRegexpOptions{})
		require.NoError(t, err)
		result, err := rr.url(map[string]string{"id": "42", "pid": "123"})
		require.NoError(t, err)
		assert.Equal(t, "/users/42/posts/123", result)
	})

	t.Run("missing variable", func(t *testing.T) {
		rr, err := newRouteRegexp("/users/{id}", regexpTypePath, routeRegexpOptions{})
		require.NoError(t, err)
		_, err = rr.url(map[string]string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing route variable")
	})

	t.Run("variable does not match pattern", func(t *testing.T) {
		rr, err := newRouteRegexp("/users/{id:[0-9]+}", regexpTypePath, routeRegexpOptions{})
		require.NoError(t, err)
		_, err = rr.url(map[string]string{"id": "abc"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "doesn't match")
	})

	t.Run("no variables", func(t *testing.T) {
		rr, err := newRouteRegexp("/static/path", regexpTypePath, routeRegexpOptions{})
		require.NoError(t, err)
		result, err := rr.url(map[string]string{})
		require.NoError(t, err)
		assert.Equal(t, "/static/path", result)
	})
}

func TestRouteRegexpGetURLVars(t *testing.T) {
	t.Run("extracts variables", func(t *testing.T) {
		rr, err := newRouteRegexp("/users/{id}/posts/{pid}", regexpTypePath, routeRegexpOptions{})
		require.NoError(t, err)
		vars := rr.getURLVars("/users/42/posts/123")
		require.NotNil(t, vars)
		assert.Equal(t, "42", vars["id"])
		assert.Equal(t, "123", vars["pid"])
	})

	t.Run("no match returns nil", func(t *testing.T) {
		rr, err := newRouteRegexp("/users/{id:[0-9]+}", regexpTypePath, routeRegexpOptions{})
		require.NoError(t, err)
		assert.Nil(t, rr.getURLVars("/posts/abc"))
	})
}

func TestNewRouteRegexpQueryNoEquals(t *testing.T) {
	t.Run("query template without equals sign", func(t *testing.T) {
		rr, err := newRouteRegexp("key", regexpTypeQuery, routeRegexpOptions{})
		require.NoError(t, err)
		assert.Equal(t, "key", rr.queryKey)
		assert.Empty(t, rr.varsN)
	})
}

func TestMatchQueryStringPresenceCheck(t *testing.T) {
	t.Run("empty template with missing key returns false", func(t *testing.T) {
		// Template "key" with no = sign: queryKey="key", tpl="", no vars, empty template
		rr, err := newRouteRegexp("key", regexpTypeQuery, routeRegexpOptions{})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		assert.False(t, rr.matchQueryString(req))
	})

	t.Run("empty template with key and empty value matches", func(t *testing.T) {
		// Template "key" with no = sign: regexp is ^$ which matches ""
		rr, err := newRouteRegexp("key", regexpTypeQuery, routeRegexpOptions{})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodGet, "/test?key=", nil)
		assert.True(t, rr.matchQueryString(req))
	})

	t.Run("query with vars missing key returns false", func(t *testing.T) {
		rr, err := newRouteRegexp("page={page:[0-9]+}", regexpTypeQuery, routeRegexpOptions{})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		assert.False(t, rr.matchQueryString(req))
	})

	t.Run("query value not matching regexp returns false", func(t *testing.T) {
		rr, err := newRouteRegexp("page={page:[0-9]+}", regexpTypeQuery, routeRegexpOptions{})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodGet, "/test?page=abc", nil)
		assert.False(t, rr.matchQueryString(req))
	})
}

func TestRouteRegexpMatchEncodedPath(t *testing.T) {
	t.Run("uses encoded path when useEncodedPath is true", func(t *testing.T) {
		// With useEncodedPath, Match() uses RawPath which preserves %2F as literal
		rr, err := newRouteRegexp("/users/{id}", regexpTypePath, routeRegexpOptions{useEncodedPath: true})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodGet, "/users/hello%2Fworld", nil)
		// RawPath=/users/hello%2Fworld, [^/]+ matches "hello%2Fworld"
		assert.True(t, rr.Match(req, &RouteMatch{}))
	})
}

// --- Benchmarks ---

func BenchmarkBraceIndices(b *testing.B) {
	inputs := []string{
		"/foo/bar",
		"/{id}",
		"/{a}/{b}",
		"/{id:[0-9]+}",
		"/{sub}.example.com/{path}/{id:[0-9]+}",
	}
	b.ResetTimer()
	for b.Loop() {
		for _, s := range inputs {
			braceIndices(s) //nolint:errcheck
		}
	}
}

func BenchmarkNewRouteRegexp(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		newRouteRegexp("/users/{id:[0-9]+}/posts/{pid}", regexpTypePath, routeRegexpOptions{}) //nolint:errcheck
	}
}

func BenchmarkRouteRegexpMatch(b *testing.B) {
	rr, _ := newRouteRegexp("/users/{id:[0-9]+}/posts/{pid}", regexpTypePath, routeRegexpOptions{})
	req := httptest.NewRequest(http.MethodGet, "/users/42/posts/123", nil)
	match := &RouteMatch{}
	b.ResetTimer()
	for b.Loop() {
		rr.Match(req, match)
	}
}

func BenchmarkRouteRegexpURL(b *testing.B) {
	rr, _ := newRouteRegexp("/users/{id:[0-9]+}/posts/{pid}", regexpTypePath, routeRegexpOptions{})
	values := map[string]string{"id": "42", "pid": "123"}
	b.ResetTimer()
	for b.Loop() {
		rr.url(values) //nolint:errcheck
	}
}

func BenchmarkMatchQueryString(b *testing.B) {
	rr, _ := newRouteRegexp("page={page:[0-9]+}", regexpTypeQuery, routeRegexpOptions{})
	req := httptest.NewRequest(http.MethodGet, "/search?page=42&limit=10", nil)
	b.ResetTimer()
	for b.Loop() {
		rr.matchQueryString(req)
	}
}

func BenchmarkGetHost(b *testing.B) {
	req := httptest.NewRequest(http.MethodGet, "http://api.example.com:8080/", nil)
	req.Host = "api.example.com:8080"
	b.ResetTimer()
	for b.Loop() {
		getHost(req)
	}
}

func BenchmarkRouteRegexpGroupSetMatch(b *testing.B) {
	hostRe, _ := newRouteRegexp("{sub}.example.com", regexpTypeHost, routeRegexpOptions{})
	pathRe, _ := newRouteRegexp("/users/{id:[0-9]+}", regexpTypePath, routeRegexpOptions{})
	queryRe, _ := newRouteRegexp("page={page:[0-9]+}", regexpTypeQuery, routeRegexpOptions{})
	group := &routeRegexpGroup{host: hostRe, path: pathRe, queries: []*routeRegexp{queryRe}}
	req := httptest.NewRequest(http.MethodGet, "http://api.example.com/users/42?page=5", nil)
	req.Host = "api.example.com"
	route := &Route{}
	b.ResetTimer()
	for b.Loop() {
		match := &RouteMatch{}
		group.setMatch(req, match, route)
	}
}

// --- Fuzz ---

func FuzzBraceIndices(f *testing.F) {
	f.Add("")
	f.Add("/foo/bar")
	f.Add("/{id}")
	f.Add("/{a}/{b}")
	f.Add("/{id:[0-9]+}")
	f.Add("/{id:{nested}}")
	f.Add("/{id")
	f.Add("/id}")

	f.Fuzz(func(_ *testing.T, s string) {
		braceIndices(s) //nolint:errcheck
	})
}

func FuzzNewRouteRegexp(f *testing.F) {
	f.Add("/users/{id}")
	f.Add("/users/{id:[0-9]+}")
	f.Add("{sub}.example.com")
	f.Add("/api/v1")
	f.Add("page={page:[0-9]+}")
	f.Add("/{}")
	f.Add("/{a}/{b}/{c}")

	f.Fuzz(func(_ *testing.T, tpl string) {
		newRouteRegexp(tpl, regexpTypePath, routeRegexpOptions{}) //nolint:errcheck
	})
}

func TestRouteRegexpGroupSetMatch(t *testing.T) {
	t.Run("extracts path variables", func(t *testing.T) {
		pathRe, err := newRouteRegexp("/users/{id}", regexpTypePath, routeRegexpOptions{})
		require.NoError(t, err)
		group := &routeRegexpGroup{path: pathRe}
		req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
		match := &RouteMatch{}
		group.setMatch(req, match, &Route{})
		assert.Equal(t, "42", match.Vars["id"])
	})

	t.Run("extracts host variables", func(t *testing.T) {
		hostRe, err := newRouteRegexp("{sub}.example.com", regexpTypeHost, routeRegexpOptions{})
		require.NoError(t, err)
		group := &routeRegexpGroup{host: hostRe}
		req := httptest.NewRequest(http.MethodGet, "http://api.example.com/", nil)
		req.Host = "api.example.com"
		match := &RouteMatch{}
		group.setMatch(req, match, &Route{})
		assert.Equal(t, "api", match.Vars["sub"])
	})

	t.Run("extracts both host and path variables", func(t *testing.T) {
		hostRe, err := newRouteRegexp("{sub}.example.com", regexpTypeHost, routeRegexpOptions{})
		require.NoError(t, err)
		pathRe, err := newRouteRegexp("/users/{id}", regexpTypePath, routeRegexpOptions{})
		require.NoError(t, err)
		group := &routeRegexpGroup{host: hostRe, path: pathRe}
		req := httptest.NewRequest(http.MethodGet, "http://api.example.com/users/42", nil)
		req.Host = "api.example.com"
		match := &RouteMatch{}
		group.setMatch(req, match, &Route{})
		assert.Equal(t, "api", match.Vars["sub"])
		assert.Equal(t, "42", match.Vars["id"])
	})

	t.Run("extracts query variables", func(t *testing.T) {
		queryRe, err := newRouteRegexp("page={page:[0-9]+}", regexpTypeQuery, routeRegexpOptions{})
		require.NoError(t, err)
		group := &routeRegexpGroup{queries: []*routeRegexp{queryRe}}
		req := httptest.NewRequest(http.MethodGet, "/search?page=5", nil)
		match := &RouteMatch{}
		group.setMatch(req, match, &Route{})
		assert.Equal(t, "5", match.Vars["page"])
	})

	t.Run("extracts path variables with useEncodedPath", func(t *testing.T) {
		pathRe, err := newRouteRegexp("/users/{id}", regexpTypePath, routeRegexpOptions{useEncodedPath: true})
		require.NoError(t, err)
		group := &routeRegexpGroup{path: pathRe}
		req := httptest.NewRequest(http.MethodGet, "/users/hello%20world", nil)
		match := &RouteMatch{}
		group.setMatch(req, match, &Route{})
		assert.Equal(t, "hello world", match.Vars["id"])
	})

	t.Run("setVars returns false on no match", func(t *testing.T) {
		pathRe, err := newRouteRegexp("/users/{id:[0-9]+}", regexpTypePath, routeRegexpOptions{})
		require.NoError(t, err)
		dst := make(map[string]string)
		assert.False(t, pathRe.setVars("/posts/abc", dst))
		assert.Empty(t, dst)
	})

	t.Run("setVars writes directly into destination map", func(t *testing.T) {
		pathRe, err := newRouteRegexp("/users/{id}", regexpTypePath, routeRegexpOptions{})
		require.NoError(t, err)
		dst := make(map[string]string)
		assert.True(t, pathRe.setVars("/users/42", dst))
		assert.Equal(t, "42", dst["id"])
	})

	t.Run("query vars missing key skips extraction", func(t *testing.T) {
		queryRe, err := newRouteRegexp("page={page:[0-9]+}", regexpTypeQuery, routeRegexpOptions{})
		require.NoError(t, err)
		group := &routeRegexpGroup{queries: []*routeRegexp{queryRe}}
		req := httptest.NewRequest(http.MethodGet, "/search?other=5", nil)
		match := &RouteMatch{}
		group.setMatch(req, match, &Route{})
		assert.Empty(t, match.Vars["page"])
	})
}
