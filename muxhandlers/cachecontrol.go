package muxhandlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/vitalvas/kasper/mux"
)

// ErrNoCacheControlRules is returned when CacheControlConfig.Rules is empty.
var ErrNoCacheControlRules = errors.New("cache control: at least one rule is required")

// CacheControlRule maps a Content-Type prefix to Cache-Control and Expires
// header values.
type CacheControlRule struct {
	// ContentType is a content type prefix to match against the response
	// Content-Type (e.g. "image/", "application/json"). Matching is
	// case-insensitive via strings.HasPrefix on the lowercased value.
	ContentType string

	// Value is the Cache-Control header value to set when this rule
	// matches (e.g. "public, max-age=86400").
	Value string

	// Expires is the duration added to the current time to compute the
	// Expires header value (formatted as HTTP-date per RFC 7231). A zero
	// duration produces a date in the past (epoch), equivalent to
	// "already expired". A negative duration means no Expires header is
	// set for this rule. Positive values produce a future date
	// (e.g. 24*time.Hour sets Expires to 24 hours from now).
	Expires time.Duration
}

// CacheControlConfig configures the CacheControl middleware behaviour.
type CacheControlConfig struct {
	// Rules is the ordered list of content type rules. The first matching
	// rule wins. Required; at least one must be provided.
	Rules []CacheControlRule

	// DefaultValue is the Cache-Control header value for responses that
	// don't match any rule. When empty, no header is set for unmatched
	// types.
	DefaultValue string

	// DefaultExpires is the duration added to the current time to compute
	// the Expires header for responses that don't match any rule. A zero
	// duration produces a date in the past (epoch). A negative duration
	// means no Expires header is set for unmatched types.
	DefaultExpires time.Duration
}

// cacheControlRule is a pre-normalized copy of CacheControlRule used at
// runtime so that the lowercase conversion happens once at factory time.
type cacheControlRule struct {
	contentType string
	value       string
	expires     time.Duration
	hasExpires  bool
}

// CacheControlMiddleware returns a middleware that sets Cache-Control and
// Expires response headers based on the response Content-Type. Rules are
// evaluated in order; the first rule whose ContentType prefix matches wins.
// If no rule matches and DefaultValue/DefaultExpires is non-empty, it is
// used. When the handler already sets a Cache-Control or Expires header,
// the middleware does not overwrite the respective header.
//
// It returns ErrNoCacheControlRules if Rules is empty.
func CacheControlMiddleware(cfg CacheControlConfig) (mux.MiddlewareFunc, error) {
	if len(cfg.Rules) == 0 {
		return nil, ErrNoCacheControlRules
	}

	rules := make([]cacheControlRule, len(cfg.Rules))
	for i, r := range cfg.Rules {
		rules[i] = cacheControlRule{
			contentType: strings.ToLower(r.ContentType),
			value:       r.Value,
			expires:     r.Expires,
			hasExpires:  r.Expires >= 0,
		}
	}

	defaultValue := cfg.DefaultValue
	defaultExpires := cfg.DefaultExpires
	hasDefaultExpires := cfg.DefaultExpires >= 0

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cw := &cacheControlResponseWriter{
				ResponseWriter:    w,
				rules:             rules,
				defaultValue:      defaultValue,
				defaultExpires:    defaultExpires,
				hasDefaultExpires: hasDefaultExpires,
			}

			next.ServeHTTP(cw, r)
		})
	}, nil
}

// cacheControlResponseWriter intercepts WriteHeader to inspect the response
// Content-Type and set Cache-Control and Expires before flushing headers.
type cacheControlResponseWriter struct {
	http.ResponseWriter
	rules             []cacheControlRule
	defaultValue      string
	defaultExpires    time.Duration
	hasDefaultExpires bool
	wroteHeader       bool
}

func (cw *cacheControlResponseWriter) WriteHeader(statusCode int) {
	if cw.wroteHeader {
		return
	}

	cw.wroteHeader = true

	h := cw.Header()

	ccSet := h.Get("Cache-Control") != ""
	exSet := h.Get("Expires") != ""

	if !ccSet || !exSet {
		ct := strings.ToLower(h.Get("Content-Type"))

		var matchedValue string
		var matchedExpires time.Duration
		var setExpires bool

		matched := false
		for _, rule := range cw.rules {
			if strings.HasPrefix(ct, rule.contentType) {
				matchedValue = rule.value
				matchedExpires = rule.expires
				setExpires = rule.hasExpires
				matched = true

				break
			}
		}

		if !matched {
			matchedValue = cw.defaultValue
			matchedExpires = cw.defaultExpires
			setExpires = cw.hasDefaultExpires
		}

		if !ccSet && matchedValue != "" {
			h.Set("Cache-Control", matchedValue)
		}

		if !exSet && setExpires {
			h.Set("Expires", time.Now().UTC().Add(matchedExpires).Format(http.TimeFormat))
		}
	}

	cw.ResponseWriter.WriteHeader(statusCode)
}

func (cw *cacheControlResponseWriter) Write(b []byte) (int, error) {
	if !cw.wroteHeader {
		cw.WriteHeader(http.StatusOK)
	}

	return cw.ResponseWriter.Write(b)
}

// Unwrap returns the underlying ResponseWriter for middleware compatibility.
func (cw *cacheControlResponseWriter) Unwrap() http.ResponseWriter {
	return cw.ResponseWriter
}
