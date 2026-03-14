package muxhandlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/vitalvas/kasper/mux"
)

// ErrSunsetZeroTime is returned when SunsetConfig.Sunset is the zero time.
var ErrSunsetZeroTime = errors.New("sunset: sunset time must not be zero")

// SunsetConfig configures the Sunset middleware.
type SunsetConfig struct {
	// Sunset is the point in time when the resource is expected to become
	// unresponsive. Serialized as an HTTP-date per RFC 7231 Section 7.1.1.1.
	// Required.
	//
	// See: https://www.rfc-editor.org/rfc/rfc8594#section-3
	Sunset time.Time

	// Deprecation is the point in time when the resource was deprecated.
	// When non-zero, the Deprecation response header is set.
	Deprecation time.Time

	// Link is an optional URI pointing to documentation about the
	// deprecation or sunset. When non-empty, a Link header with
	// rel="sunset" is added to the response.
	//
	// See: https://www.rfc-editor.org/rfc/rfc8594#section-4
	Link string
}

// SunsetMiddleware returns a middleware that sets the Sunset response header
// per RFC 8594. Optionally sets the Deprecation and Link headers.
//
// See: https://www.rfc-editor.org/rfc/rfc8594
func SunsetMiddleware(cfg SunsetConfig) (mux.MiddlewareFunc, error) {
	if cfg.Sunset.IsZero() {
		return nil, ErrSunsetZeroTime
	}

	sunsetValue := cfg.Sunset.UTC().Format(http.TimeFormat)

	var deprecationValue string
	if !cfg.Deprecation.IsZero() {
		deprecationValue = cfg.Deprecation.UTC().Format(http.TimeFormat)
	}

	var linkValue string
	if cfg.Link != "" {
		linkValue = `<` + cfg.Link + `>; rel="sunset"`
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Sunset", sunsetValue)

			if deprecationValue != "" {
				w.Header().Set("Deprecation", deprecationValue)
			}

			if linkValue != "" {
				w.Header().Add("Link", linkValue)
			}

			next.ServeHTTP(w, r)
		})
	}, nil
}
