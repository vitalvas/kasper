package muxhandlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/kasper/mux"
)

func TestSecurityHeadersMiddleware(t *testing.T) {
	t.Run("config validation", func(t *testing.T) {
		tests := []struct {
			name    string
			config  SecurityHeadersConfig
			wantErr error
		}{
			{"invalid frame option", SecurityHeadersConfig{FrameOption: "ALLOW"}, ErrInvalidFrameOption},
			{"invalid frame option lowercase", SecurityHeadersConfig{FrameOption: "deny"}, ErrInvalidFrameOption},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := SecurityHeadersMiddleware(tt.config)
				assert.ErrorIs(t, err, tt.wantErr)
			})
		}

		t.Run("valid configs", func(t *testing.T) {
			validConfigs := []SecurityHeadersConfig{
				{},
				{FrameOption: "DENY"},
				{FrameOption: "SAMEORIGIN"},
				{FrameOption: ""},
			}

			for _, cfg := range validConfigs {
				_, err := SecurityHeadersMiddleware(cfg)
				assert.NoError(t, err)
			}
		})
	})

	tests := []struct {
		name       string
		config     SecurityHeadersConfig
		wantHeader map[string]string
		skipHeader []string
	}{
		{
			name:   "default config",
			config: SecurityHeadersConfig{},
			wantHeader: map[string]string{
				"X-Content-Type-Options": "nosniff",
				"X-Frame-Options":        "DENY",
				"Referrer-Policy":        "strict-origin-when-cross-origin",
			},
			skipHeader: []string{"Strict-Transport-Security", "Cross-Origin-Opener-Policy", "Content-Security-Policy", "Permissions-Policy"},
		},
		{
			name:   "SAMEORIGIN frame option",
			config: SecurityHeadersConfig{FrameOption: "SAMEORIGIN"},
			wantHeader: map[string]string{
				"X-Frame-Options": "SAMEORIGIN",
			},
		},
		{
			name:   "disable nosniff",
			config: SecurityHeadersConfig{DisableContentTypeNosniff: true},
			wantHeader: map[string]string{
				"X-Frame-Options": "DENY",
			},
			skipHeader: []string{"X-Content-Type-Options"},
		},
		{
			name:   "custom referrer policy",
			config: SecurityHeadersConfig{ReferrerPolicy: "no-referrer"},
			wantHeader: map[string]string{
				"Referrer-Policy": "no-referrer",
			},
		},
		{
			name:   "HSTS max-age only",
			config: SecurityHeadersConfig{HSTSMaxAge: 31536000},
			wantHeader: map[string]string{
				"Strict-Transport-Security": "max-age=31536000",
			},
		},
		{
			name: "HSTS with includeSubDomains",
			config: SecurityHeadersConfig{
				HSTSMaxAge:            31536000,
				HSTSIncludeSubDomains: true,
			},
			wantHeader: map[string]string{
				"Strict-Transport-Security": "max-age=31536000; includeSubDomains",
			},
		},
		{
			name: "HSTS with includeSubDomains and preload",
			config: SecurityHeadersConfig{
				HSTSMaxAge:            31536000,
				HSTSIncludeSubDomains: true,
				HSTSPreload:           true,
			},
			wantHeader: map[string]string{
				"Strict-Transport-Security": "max-age=31536000; includeSubDomains; preload",
			},
		},
		{
			name: "HSTS with preload only",
			config: SecurityHeadersConfig{
				HSTSMaxAge:  31536000,
				HSTSPreload: true,
			},
			wantHeader: map[string]string{
				"Strict-Transport-Security": "max-age=31536000; preload",
			},
		},
		{
			name:   "HSTS directives ignored when max-age is zero",
			config: SecurityHeadersConfig{HSTSIncludeSubDomains: true, HSTSPreload: true},
			wantHeader: map[string]string{
				"X-Frame-Options": "DENY",
			},
			skipHeader: []string{"Strict-Transport-Security"},
		},
		{
			name:   "cross-origin opener policy",
			config: SecurityHeadersConfig{CrossOriginOpenerPolicy: "same-origin"},
			wantHeader: map[string]string{
				"Cross-Origin-Opener-Policy": "same-origin",
			},
		},
		{
			name:   "content security policy",
			config: SecurityHeadersConfig{ContentSecurityPolicy: "default-src 'self'"},
			wantHeader: map[string]string{
				"Content-Security-Policy": "default-src 'self'",
			},
		},
		{
			name:   "permissions policy",
			config: SecurityHeadersConfig{PermissionsPolicy: "camera=(), microphone=()"},
			wantHeader: map[string]string{
				"Permissions-Policy": "camera=(), microphone=()",
			},
		},
		{
			name: "all headers enabled",
			config: SecurityHeadersConfig{
				HSTSMaxAge:              63072000,
				HSTSIncludeSubDomains:   true,
				HSTSPreload:             true,
				CrossOriginOpenerPolicy: "same-origin",
				ContentSecurityPolicy:   "default-src 'self'",
				PermissionsPolicy:       "geolocation=()",
			},
			wantHeader: map[string]string{
				"X-Content-Type-Options":     "nosniff",
				"X-Frame-Options":            "DENY",
				"Referrer-Policy":            "strict-origin-when-cross-origin",
				"Strict-Transport-Security":  "max-age=63072000; includeSubDomains; preload",
				"Cross-Origin-Opener-Policy": "same-origin",
				"Content-Security-Policy":    "default-src 'self'",
				"Permissions-Policy":         "geolocation=()",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := mux.NewRouter()
			r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}).Methods(http.MethodGet)

			mw, err := SecurityHeadersMiddleware(tt.config)
			require.NoError(t, err)
			r.Use(mw)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			for header, want := range tt.wantHeader {
				assert.Equal(t, want, w.Header().Get(header), "header %s", header)
			}

			for _, header := range tt.skipHeader {
				assert.Empty(t, w.Header().Get(header), "header %s should not be set", header)
			}
		})
	}

	t.Run("headers set before next handler", func(t *testing.T) {
		var headersInHandler http.Header

		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			headersInHandler = w.Header().Clone()
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := SecurityHeadersMiddleware(SecurityHeadersConfig{})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, "nosniff", headersInHandler.Get("X-Content-Type-Options"))
		assert.Equal(t, "DENY", headersInHandler.Get("X-Frame-Options"))
	})
}

func BenchmarkSecurityHeadersMiddleware(b *testing.B) {
	b.Run("default config", func(b *testing.B) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := SecurityHeadersMiddleware(SecurityHeadersConfig{})
		if err != nil {
			b.Fatal(err)
		}
		r.Use(mw)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)

		b.ResetTimer()
		for b.Loop() {
			r.ServeHTTP(httptest.NewRecorder(), req)
		}
	})

	b.Run("all headers enabled", func(b *testing.B) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := SecurityHeadersMiddleware(SecurityHeadersConfig{
			HSTSMaxAge:              63072000,
			HSTSIncludeSubDomains:   true,
			HSTSPreload:             true,
			CrossOriginOpenerPolicy: "same-origin",
			ContentSecurityPolicy:   "default-src 'self'",
			PermissionsPolicy:       "geolocation=()",
		})
		if err != nil {
			b.Fatal(err)
		}
		r.Use(mw)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)

		b.ResetTimer()
		for b.Loop() {
			r.ServeHTTP(httptest.NewRecorder(), req)
		}
	})
}
