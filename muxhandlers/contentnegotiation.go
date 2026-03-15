package muxhandlers

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/vitalvas/kasper/mux"
)

// negotiatedTypeCtxKey is the context key for the negotiated content type.
type negotiatedTypeCtxKey struct{}

// ContentNegotiationConfig configures the Content Negotiation middleware.
//
// Spec reference: https://www.rfc-editor.org/rfc/rfc9110#section-12.5.1
type ContentNegotiationConfig struct {
	// Offered is the list of media types the server can produce, in
	// preference order. When empty, any media type from the Accept header
	// is accepted and the best match is stored in context.
	// Examples: "application/json", "application/xml", "text/html".
	Offered []string
}

// ContentNegotiationMiddleware returns a middleware that performs proactive
// content negotiation per RFC 9110 Section 12.5.1. It parses the Accept
// header, selects the best matching type from the offered list, and stores
// the result in the request context (retrievable via NegotiatedType).
//
// When Offered is empty, any media type is accepted: the highest quality
// type from the Accept header is stored in context, and requests always
// pass through.
//
// When the Accept header is absent or empty, the first offered type is
// selected per RFC 9110 Section 12.5.1 ("A request without any Accept
// header field implies that the user agent will accept any media type").
//
// When no offered type matches the Accept header, the middleware responds
// with 406 Not Acceptable per RFC 9110 Section 15.5.7.
func ContentNegotiationMiddleware(cfg ContentNegotiationConfig) mux.MiddlewareFunc {
	if len(cfg.Offered) == 0 {
		cfg.Offered = []string{"*/*"}
	}

	offered := make([]string, len(cfg.Offered))
	for i, o := range cfg.Offered {
		offered[i] = strings.ToLower(strings.TrimSpace(o))
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			accept := r.Header.Get("Accept")
			selected := negotiate(accept, offered)
			if selected == "" {
				http.Error(w, http.StatusText(http.StatusNotAcceptable), http.StatusNotAcceptable)
				return
			}

			ctx := context.WithValue(r.Context(), negotiatedTypeCtxKey{}, selected)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// NegotiatedType returns the content type selected by ContentNegotiationMiddleware
// from the request context. Returns an empty string if no negotiation was performed.
func NegotiatedType(r *http.Request) string {
	if v, ok := r.Context().Value(negotiatedTypeCtxKey{}).(string); ok {
		return v
	}
	return ""
}

// negotiate selects the best matching offered type for the given Accept header
// value. Returns empty string if no match is found.
func negotiate(accept string, offered []string) string {
	accept = strings.TrimSpace(accept)
	if accept == "" {
		return offered[0]
	}

	ranges := parseAccept(accept)
	if len(ranges) == 0 {
		return offered[0]
	}

	bestType := ""
	bestQuality := -1.0
	bestSpecificity := -1

	for _, o := range offered {
		q, specificity := matchAccept(o, ranges)
		if q > bestQuality || (q == bestQuality && specificity > bestSpecificity) {
			bestQuality = q
			bestType = o
			bestSpecificity = specificity
		}
	}

	if bestQuality <= 0 {
		return ""
	}

	return bestType
}

// acceptRange represents a single media range from an Accept header.
type acceptRange struct {
	mainType    string
	subType     string
	quality     float64
	specificity int // 0=*/*, 1=type/*, 2=type/subtype
}

// parseAccept parses the Accept header value into a list of media ranges
// with quality values per RFC 9110 Section 12.5.1.
func parseAccept(accept string) []acceptRange {
	var ranges []acceptRange

	for part := range strings.SplitSeq(accept, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		mediaType, quality := parseMediaRange(part)
		if mediaType == "" || quality < 0 {
			continue
		}

		mainType, subType, ok := splitMediaType(mediaType)
		if !ok {
			continue
		}

		specificity := 2
		if subType == "*" {
			specificity = 1
		}
		if mainType == "*" {
			specificity = 0
		}

		ranges = append(ranges, acceptRange{
			mainType:    mainType,
			subType:     subType,
			quality:     quality,
			specificity: specificity,
		})
	}

	return ranges
}

// parseMediaRange extracts the media type and quality value from a single
// Accept header media range entry.
func parseMediaRange(s string) (string, float64) {
	quality := 1.0
	mediaType := s

	if mt, params, ok := strings.Cut(s, ";"); ok {
		mediaType = strings.TrimSpace(mt)
		for param := range strings.SplitSeq(params, ";") {
			param = strings.TrimSpace(param)
			if strings.HasPrefix(param, "q=") || strings.HasPrefix(param, "Q=") {
				qVal, err := strconv.ParseFloat(param[2:], 64)
				if err == nil && qVal >= 0 && qVal <= 1 {
					quality = qVal
				}
				break
			}
		}
	}

	return strings.ToLower(strings.TrimSpace(mediaType)), quality
}

// splitMediaType splits "type/subtype" into its components.
func splitMediaType(mt string) (string, string, bool) {
	mainType, subType, ok := strings.Cut(mt, "/")
	return mainType, subType, ok
}

// matchAccept returns the quality and specificity of the best matching
// accept range for the given offered type.
func matchAccept(offered string, ranges []acceptRange) (float64, int) {
	mainType, subType, ok := splitMediaType(offered)
	if !ok {
		return -1, -1
	}

	bestQuality := -1.0
	bestSpecificity := -1

	for _, r := range ranges {
		if r.quality == 0 {
			continue
		}

		matched := false
		switch {
		case r.mainType == "*" && r.subType == "*":
			matched = true
		case mainType == "*" && subType == "*":
			matched = true
		case r.mainType == mainType && r.subType == "*":
			matched = true
		case r.mainType == mainType && subType == "*":
			matched = true
		case r.mainType == mainType && r.subType == subType:
			matched = true
		}

		if matched && (r.specificity > bestSpecificity ||
			(r.specificity == bestSpecificity && r.quality > bestQuality)) {
			bestQuality = r.quality
			bestSpecificity = r.specificity
		}
	}

	return bestQuality, bestSpecificity
}
