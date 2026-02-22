package muxhandlers

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/vitalvas/kasper/mux"
)

// ErrInvalidFrameOption is returned when SecurityHeadersConfig.FrameOption is
// not one of the valid values: "DENY", "SAMEORIGIN", or empty string.
var ErrInvalidFrameOption = errors.New("security headers: frame option must be DENY, SAMEORIGIN, or empty")

// SecurityHeadersConfig configures the Security Headers middleware behaviour.
type SecurityHeadersConfig struct {
	// DisableContentTypeNosniff disables the X-Content-Type-Options: nosniff
	// header. The header is set by default (when false).
	DisableContentTypeNosniff bool

	// FrameOption sets the X-Frame-Options header value.
	// Valid values are "DENY", "SAMEORIGIN", or empty string to skip.
	// Defaults to "DENY".
	FrameOption string

	// ReferrerPolicy sets the Referrer-Policy header value.
	// Defaults to "strict-origin-when-cross-origin".
	ReferrerPolicy string

	// HSTSMaxAge sets the max-age directive for the Strict-Transport-Security
	// header in seconds. When zero, the header is not set.
	HSTSMaxAge int

	// HSTSIncludeSubDomains appends the includeSubDomains directive to the
	// Strict-Transport-Security header. Only effective when HSTSMaxAge > 0.
	HSTSIncludeSubDomains bool

	// HSTSPreload appends the preload directive to the
	// Strict-Transport-Security header. Only effective when HSTSMaxAge > 0.
	HSTSPreload bool

	// CrossOriginOpenerPolicy sets the Cross-Origin-Opener-Policy header.
	// When empty, the header is not set.
	CrossOriginOpenerPolicy string

	// ContentSecurityPolicy sets the Content-Security-Policy header.
	// When empty, the header is not set.
	ContentSecurityPolicy string

	// PermissionsPolicy sets the Permissions-Policy header.
	// When empty, the header is not set.
	PermissionsPolicy string
}

// SecurityHeadersMiddleware returns a middleware that sets common security
// response headers. Headers are set before calling the next handler.
//
// It returns ErrInvalidFrameOption if FrameOption is set to a value other than
// "DENY", "SAMEORIGIN", or empty string.
func SecurityHeadersMiddleware(cfg SecurityHeadersConfig) (mux.MiddlewareFunc, error) {
	if cfg.FrameOption != "" && cfg.FrameOption != "DENY" && cfg.FrameOption != "SAMEORIGIN" {
		return nil, ErrInvalidFrameOption
	}

	if cfg.FrameOption == "" {
		cfg.FrameOption = "DENY"
	}

	if cfg.ReferrerPolicy == "" {
		cfg.ReferrerPolicy = "strict-origin-when-cross-origin"
	}

	var hstsValue string
	if cfg.HSTSMaxAge > 0 {
		hstsValue = fmt.Sprintf("max-age=%d", cfg.HSTSMaxAge)
		if cfg.HSTSIncludeSubDomains {
			hstsValue += "; includeSubDomains"
		}
		if cfg.HSTSPreload {
			hstsValue += "; preload"
		}
	}

	nosniff := !cfg.DisableContentTypeNosniff
	frameOption := cfg.FrameOption
	referrerPolicy := cfg.ReferrerPolicy
	crossOriginOpenerPolicy := cfg.CrossOriginOpenerPolicy
	contentSecurityPolicy := cfg.ContentSecurityPolicy
	permissionsPolicy := cfg.PermissionsPolicy

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()

			if nosniff {
				h.Set("X-Content-Type-Options", "nosniff")
			}

			h.Set("X-Frame-Options", frameOption)
			h.Set("Referrer-Policy", referrerPolicy)

			if hstsValue != "" {
				h.Set("Strict-Transport-Security", hstsValue)
			}

			if crossOriginOpenerPolicy != "" {
				h.Set("Cross-Origin-Opener-Policy", crossOriginOpenerPolicy)
			}

			if contentSecurityPolicy != "" {
				h.Set("Content-Security-Policy", contentSecurityPolicy)
			}

			if permissionsPolicy != "" {
				h.Set("Permissions-Policy", permissionsPolicy)
			}

			next.ServeHTTP(w, r)
		})
	}, nil
}
