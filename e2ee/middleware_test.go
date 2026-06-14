package e2ee

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMiddlewareRequiresKeySet(t *testing.T) {
	_, err := Middleware(MiddlewareConfig{})
	require.ErrorIs(t, err, ErrNoKeySet)
}

func TestMiddlewareEncryptResponseError(t *testing.T) {
	key, _ := GenerateServerKey("kid", time.Hour, DefaultMaxSkew)
	set, _ := NewServerKeySet("https://x", key)

	var capturedErr error

	mw, err := Middleware(MiddlewareConfig{
		Server: ServerConfig{KeySet: set},
		OnError: func(w http.ResponseWriter, _ *http.Request, e error) {
			capturedErr = e
			w.WriteHeader(http.StatusInternalServerError)
		},
	})
	require.NoError(t, err)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Force seal failure during response encryption.
		withFailingGCM(t)
		w.Write([]byte("response"))
	}))

	// Build a valid encrypted request first.
	cs, _ := ParseKeySet(strings.NewReader(string(mustJSON(t, set.Document()))))
	cfg := ClientConfig{KeySet: cs}

	req, _ := http.NewRequest(http.MethodPost, "https://x/api", strings.NewReader("hi"))
	_, err = EncryptRequest(req, cfg)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Error(t, capturedErr)
}

func TestBufferingWriterDoubleWriteHeader(t *testing.T) {
	b := &bufferingWriter{header: make(http.Header), status: http.StatusOK}
	b.WriteHeader(http.StatusTeapot)
	b.WriteHeader(http.StatusGone) // ignored

	assert.Equal(t, http.StatusTeapot, b.status)
}
