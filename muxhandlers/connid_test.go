package muxhandlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.vitalvas.com/kasper/mux"
)

func TestConnIDFromContext(t *testing.T) {
	t.Run("returns empty for bare context", func(t *testing.T) {
		assert.Empty(t, ConnIDFromContext(context.Background()))
	})

	t.Run("returns stored id", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), connIDKey{}, "conn-1")
		assert.Equal(t, "conn-1", ConnIDFromContext(ctx))
	})
}

func TestConnContext(t *testing.T) {
	t.Run("stamps unique id per call", func(t *testing.T) {
		fn := ConnContext(ConnIDConfig{})
		ctx1 := fn(context.Background(), nil)
		ctx2 := fn(context.Background(), nil)

		assert.Regexp(t, uuidV4Regex, ConnIDFromContext(ctx1))
		assert.NotEqual(t, ConnIDFromContext(ctx1), ConnIDFromContext(ctx2))
	})

	t.Run("custom generate func", func(t *testing.T) {
		fn := ConnContext(ConnIDConfig{GenerateFunc: func() string { return "fixed" }})
		assert.Equal(t, "fixed", ConnIDFromContext(fn(context.Background(), nil)))
	})
}

func TestConnIDMiddleware(t *testing.T) {
	t.Run("surfaces id from context on response and handler", func(t *testing.T) {
		var capturedCtxID string

		r := mux.NewRouter()
		r.HandleFunc("/test", func(_ http.ResponseWriter, req *http.Request) {
			capturedCtxID = ConnIDFromContext(req.Context())
		}).Methods(http.MethodGet)
		r.Use(ConnIDMiddleware(ConnIDConfig{ExposeHeader: true}))

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req = req.WithContext(context.WithValue(req.Context(), connIDKey{}, "conn-xyz"))
		r.ServeHTTP(w, req)

		assert.Equal(t, "conn-xyz", capturedCtxID)
		assert.Equal(t, "conn-xyz", w.Header().Get("X-Connection-ID"))
	})

	t.Run("does not expose header by default", func(t *testing.T) {
		var capturedCtxID string

		r := mux.NewRouter()
		r.HandleFunc("/test", func(_ http.ResponseWriter, req *http.Request) {
			capturedCtxID = ConnIDFromContext(req.Context())
		}).Methods(http.MethodGet)
		r.Use(ConnIDMiddleware(ConnIDConfig{}))

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req = req.WithContext(context.WithValue(req.Context(), connIDKey{}, "conn-xyz"))
		r.ServeHTTP(w, req)

		assert.Equal(t, "conn-xyz", capturedCtxID, "ID still rides the context")
		assert.Empty(t, w.Header().Get("X-Connection-ID"), "but is not exposed on the response")
	})

	t.Run("custom header name", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)
		r.Use(ConnIDMiddleware(ConnIDConfig{HeaderName: "X-Conn", ExposeHeader: true}))

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req = req.WithContext(context.WithValue(req.Context(), connIDKey{}, "conn-xyz"))
		r.ServeHTTP(w, req)

		assert.Equal(t, "conn-xyz", w.Header().Get("X-Conn"))
	})

	t.Run("no header when no connection id present even if exposed", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)
		r.Use(ConnIDMiddleware(ConnIDConfig{ExposeHeader: true}))

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/test", nil))

		assert.Empty(t, w.Header().Get("X-Connection-ID"))
	})
}

// TestConnIDSharedAcrossKeepAlive wires ConnContext and ConnIDMiddleware into a
// real server and verifies two keep-alive requests share one ID while a second
// connection gets a different one.
func TestConnIDSharedAcrossKeepAlive(t *testing.T) {
	cfg := ConnIDConfig{ExposeHeader: true}

	r := mux.NewRouter()
	r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}).Methods(http.MethodGet)
	r.Use(ConnIDMiddleware(cfg))

	srv := httptest.NewUnstartedServer(r)
	srv.Config.ConnContext = ConnContext(cfg)
	srv.Start()
	defer srv.Close()

	get := func(client *http.Client) string {
		resp, err := client.Get(fmt.Sprintf("%s/test", srv.URL))
		require.NoError(t, err)
		defer resp.Body.Close()
		return resp.Header.Get("X-Connection-ID")
	}

	// One client reuses the connection across both requests.
	client1 := &http.Client{Transport: &http.Transport{}}
	id1a := get(client1)
	id1b := get(client1)

	// A fresh transport forces a new connection.
	client2 := &http.Client{Transport: &http.Transport{DisableKeepAlives: false}}
	id2 := get(client2)

	assert.Regexp(t, uuidV4Regex, id1a)
	assert.Equal(t, id1a, id1b, "same keep-alive connection shares one ID")
	assert.NotEqual(t, id1a, id2, "different connection gets a different ID")
}
