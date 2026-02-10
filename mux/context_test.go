package mux

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVars(t *testing.T) {
	t.Run("returns nil for request without vars", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		assert.Nil(t, Vars(r))
	})

	t.Run("returns vars from request context", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		vars := map[string]string{"id": "42", "name": "test"}
		r = setRouteContext(r, nil, vars)
		result := Vars(r)
		require.NotNil(t, result)
		assert.Equal(t, "42", result["id"])
		assert.Equal(t, "test", result["name"])
	})
}

func TestCurrentRoute(t *testing.T) {
	t.Run("returns nil for request without route", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		assert.Nil(t, CurrentRoute(r))
	})

	t.Run("returns route from request context", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		route := &Route{}
		r = setRouteContext(r, route, nil)
		result := CurrentRoute(r)
		require.NotNil(t, result)
		assert.Equal(t, route, result)
	})
}

func TestSetURLVars(t *testing.T) {
	t.Run("sets vars on request", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		vars := map[string]string{"key": "value"}
		r = SetURLVars(r, vars)
		result := Vars(r)
		require.NotNil(t, result)
		assert.Equal(t, "value", result["key"])
	})

	t.Run("overwrites existing vars", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r = SetURLVars(r, map[string]string{"a": "1"})
		r = SetURLVars(r, map[string]string{"b": "2"})
		result := Vars(r)
		require.NotNil(t, result)
		assert.Empty(t, result["a"])
		assert.Equal(t, "2", result["b"])
	})

	t.Run("preserves existing route", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		route := &Route{}
		r = setRouteContext(r, route, map[string]string{"a": "1"})
		r = SetURLVars(r, map[string]string{"b": "2"})
		assert.Equal(t, route, CurrentRoute(r))
		assert.Equal(t, "2", Vars(r)["b"])
	})
}

func TestMatcherFunc(t *testing.T) {
	t.Run("implements matcher interface", func(t *testing.T) {
		called := false
		fn := MatcherFunc(func(_ *http.Request, _ *RouteMatch) bool {
			called = true
			return true
		})
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		result := fn.Match(r, &RouteMatch{})
		assert.True(t, called)
		assert.True(t, result)
	})

	t.Run("returns false when matcher rejects", func(t *testing.T) {
		fn := MatcherFunc(func(_ *http.Request, _ *RouteMatch) bool {
			return false
		})
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		assert.False(t, fn.Match(r, &RouteMatch{}))
	})
}

func TestMiddlewareFunc(t *testing.T) {
	t.Run("wraps handler", func(t *testing.T) {
		called := false
		mw := MiddlewareFunc(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				next.ServeHTTP(w, r)
			})
		})
		inner := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {})
		handler := mw.Middleware(inner)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		handler.ServeHTTP(w, r)
		assert.True(t, called)
	})
}

// --- Benchmarks ---

func BenchmarkVars(b *testing.B) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	vars := map[string]string{"id": "42", "name": "test", "action": "view"}
	r = setRouteContext(r, nil, vars)
	b.ResetTimer()
	for b.Loop() {
		Vars(r)
	}
}

func BenchmarkSetURLVars(b *testing.B) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	vars := map[string]string{"id": "42", "name": "test"}
	b.ResetTimer()
	for b.Loop() {
		SetURLVars(r, vars)
	}
}

func BenchmarkCurrentRoute(b *testing.B) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	route := &Route{}
	r = setRouteContext(r, route, nil)
	b.ResetTimer()
	for b.Loop() {
		CurrentRoute(r)
	}
}

func TestErrors(t *testing.T) {
	t.Run("ErrMethodMismatch has correct message", func(t *testing.T) {
		assert.Equal(t, "method is not allowed", ErrMethodMismatch.Error())
	})

	t.Run("ErrNotFound has correct message", func(t *testing.T) {
		assert.Equal(t, "no matching route was found", ErrNotFound.Error())
	})

	t.Run("SkipRouter has correct message", func(t *testing.T) {
		assert.Equal(t, "skip this router", SkipRouter.Error())
	})
}
