package httpsig

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComponentValue(t *testing.T) {
	t.Run("derived components", func(t *testing.T) {
		req := httptest.NewRequest("POST", "https://example.com/api/items?page=2&sort=name", nil)
		req.Host = "example.com"

		tests := []struct {
			name string
			id   string
			want string
		}{
			{name: "@method", id: "@method", want: "POST"},
			{name: "@authority", id: "@authority", want: "example.com"},
			{name: "@path", id: "@path", want: "/api/items"},
			{name: "@query", id: "@query", want: "?page=2&sort=name"},
			{name: "@scheme", id: "@scheme", want: "https"},
			{name: "@target-uri", id: "@target-uri", want: "https://example.com/api/items?page=2&sort=name"},
			{name: "@request-target", id: "@request-target", want: "/api/items?page=2&sort=name"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				val, err := componentValue(tt.id, sigMessage{req: req})
				require.NoError(t, err)
				assert.Equal(t, tt.want, val)
			})
		}
	})

	t.Run("header fields", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Custom", "value1")
		req.Header.Add("X-Custom", "value2")

		tests := []struct {
			name string
			id   string
			want string
		}{
			{name: "single header", id: "content-type", want: "application/json"},
			{name: "multi-value header", id: "x-custom", want: "value1, value2"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				val, err := componentValue(tt.id, sigMessage{req: req})
				require.NoError(t, err)
				assert.Equal(t, tt.want, val)
			})
		}
	})

	t.Run("missing header returns error", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)

		_, err := componentValue("x-missing", sigMessage{req: req})
		assert.ErrorIs(t, err, ErrUnknownComponent)
	})

	t.Run("unknown derived component returns error", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)

		_, err := componentValue("@unknown", sigMessage{req: req})
		assert.ErrorIs(t, err, ErrUnknownComponent)
	})

	t.Run("empty path defaults to /", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com", nil)
		req.URL.Path = ""

		val, err := componentValue("@path", sigMessage{req: req})
		require.NoError(t, err)
		assert.Equal(t, "/", val)
	})

	t.Run("empty query", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/path", nil)

		val, err := componentValue("@query", sigMessage{req: req})
		require.NoError(t, err)
		assert.Equal(t, "?", val)
	})

	t.Run("authority from Host header", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://other.com/", nil)
		req.Host = "Example.COM:8080"

		val, err := componentValue("@authority", sigMessage{req: req})
		require.NoError(t, err)
		assert.Equal(t, "example.com:8080", val)
	})

	t.Run("scheme from TLS", func(t *testing.T) {
		req := httptest.NewRequest("GET", "http://example.com/", nil)
		req.TLS = &tls.ConnectionState{}

		val, err := componentValue("@scheme", sigMessage{req: req})
		require.NoError(t, err)
		assert.Equal(t, "https", val)
	})

	t.Run("scheme defaults to http", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/path", nil)
		req.URL.Scheme = ""
		req.TLS = nil

		val, err := componentValue("@scheme", sigMessage{req: req})
		require.NoError(t, err)
		assert.Equal(t, "http", val)
	})

	t.Run("request-target without query", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/api/items", nil)

		val, err := componentValue("@request-target", sigMessage{req: req})
		require.NoError(t, err)
		assert.Equal(t, "/api/items", val)
	})

	t.Run("target-uri without query", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/api/items", nil)
		req.Host = "example.com"

		val, err := componentValue("@target-uri", sigMessage{req: req})
		require.NoError(t, err)
		assert.Equal(t, "https://example.com/api/items", val)
	})

	t.Run("authority from URL.Host when Host header empty", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://url-host.com/", nil)
		req.Host = ""

		val, err := componentValue("@authority", sigMessage{req: req})
		require.NoError(t, err)
		assert.Equal(t, "url-host.com", val)
	})

	t.Run("authority empty when both Host and URL.Host empty", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/relative", nil)
		req.Host = ""
		req.URL.Host = ""

		val, err := componentValue("@authority", sigMessage{req: req})
		require.NoError(t, err)
		assert.Equal(t, "", val)
	})

	t.Run("scheme from URL.Scheme", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)
		req.TLS = nil
		req.URL.Scheme = "https"

		val, err := componentValue("@scheme", sigMessage{req: req})
		require.NoError(t, err)
		assert.Equal(t, "https", val)
	})

	t.Run("target-uri with empty path", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com", nil)
		req.Host = "example.com"
		req.URL.Path = ""

		val, err := componentValue("@target-uri", sigMessage{req: req})
		require.NoError(t, err)
		assert.Equal(t, "https://example.com/", val)
	})

	t.Run("request-target with empty path", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com", nil)
		req.URL.Path = ""

		val, err := componentValue("@request-target", sigMessage{req: req})
		require.NoError(t, err)
		assert.Equal(t, "/", val)
	})

	t.Run("host header reads from r.Host", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)
		req.Host = "example.com"

		val, err := componentValue("host", sigMessage{req: req})
		require.NoError(t, err)
		assert.Equal(t, "example.com", val)
	})

	t.Run("host header prefers header map over r.Host", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)
		req.Host = "from-field.com"
		req.Header.Set("Host", "from-header.com")

		val, err := componentValue("host", sigMessage{req: req})
		require.NoError(t, err)
		assert.Equal(t, "from-header.com", val)
	})

	t.Run("host header missing returns error", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/relative", nil)
		req.Host = ""

		_, err := componentValue("host", sigMessage{req: req})
		assert.ErrorIs(t, err, ErrUnknownComponent)
	})
}

func TestComponentValueResponse(t *testing.T) {
	newMessage := func() sigMessage {
		req := httptest.NewRequest("POST", "https://example.com/api/items?page=2", nil)
		req.Host = "example.com"
		req.Header.Set("X-Request-ID", "req-123")

		resp := &http.Response{StatusCode: http.StatusServiceUnavailable, Header: make(http.Header)}
		resp.Header.Set("Content-Type", "application/json")

		return sigMessage{req: req, resp: resp}
	}

	t.Run("@status", func(t *testing.T) {
		val, err := componentValue("@status", newMessage())
		require.NoError(t, err)
		assert.Equal(t, "503", val)
	})

	t.Run("response header field", func(t *testing.T) {
		val, err := componentValue("content-type", newMessage())
		require.NoError(t, err)
		assert.Equal(t, "application/json", val)
	})

	t.Run("req components resolve against request", func(t *testing.T) {
		tests := []struct {
			id   string
			want string
		}{
			{id: "@method;req", want: "POST"},
			{id: "@authority;req", want: "example.com"},
			{id: "@path;req", want: "/api/items"},
			{id: "@query;req", want: "?page=2"},
			{id: "x-request-id;req", want: "req-123"},
		}

		for _, tt := range tests {
			t.Run(tt.id, func(t *testing.T) {
				val, err := componentValue(tt.id, newMessage())
				require.NoError(t, err)
				assert.Equal(t, tt.want, val)
			})
		}
	})

	t.Run("request-only derived component without req returns error", func(t *testing.T) {
		_, err := componentValue("@method", newMessage())
		assert.ErrorIs(t, err, ErrInvalidComponent)
	})

	t.Run("req component without request returns error", func(t *testing.T) {
		m := newMessage()
		m.req = nil

		_, err := componentValue("@method;req", m)
		assert.ErrorIs(t, err, ErrInvalidComponent)
	})

	t.Run("@status on request returns error", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)

		_, err := componentValue("@status", sigMessage{req: req})
		assert.ErrorIs(t, err, ErrInvalidComponent)
	})

	t.Run("req component on request returns error", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)

		_, err := componentValue("@authority;req", sigMessage{req: req})
		assert.ErrorIs(t, err, ErrInvalidComponent)
	})

	t.Run("missing response header returns error", func(t *testing.T) {
		_, err := componentValue("x-missing", newMessage())
		assert.ErrorIs(t, err, ErrUnknownComponent)
	})
}
