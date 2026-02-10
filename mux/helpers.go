package mux

import (
	"fmt"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
)

// cleanPath returns the canonical path for p, eliminating . and .. elements
// per RFC 3986 Section 5.2.4 (remove dot segments).
func cleanPath(p string) string {
	if p == "" {
		return "/"
	}
	if p[0] != '/' {
		p = "/" + p
	}
	np := path.Clean(p)
	// path.Clean removes trailing slash except for root;
	// put the trailing slash back if necessary.
	if p[len(p)-1] == '/' && np != "/" {
		np += "/"
	}
	return np
}

// checkPairs returns an error if the list of key/value pairs has odd length.
func checkPairs(pairs ...string) (int, error) {
	if len(pairs)%2 != 0 {
		return 0, fmt.Errorf("mux: number of parameters must be multiple of 2, got %v", pairs)
	}
	return len(pairs) / 2, nil
}

// mapFromPairsToString converts variadic string parameters to a string map.
func mapFromPairsToString(pairs ...string) (map[string]string, error) {
	length, err := checkPairs(pairs...)
	if err != nil {
		return nil, err
	}
	m := make(map[string]string, length)
	for i := 0; i < len(pairs); i += 2 {
		m[pairs[i]] = pairs[i+1]
	}
	return m, nil
}

// mapFromPairsToRegex converts variadic string parameters to a map of
// compiled regular expressions.
func mapFromPairsToRegex(pairs ...string) (map[string]*regexp.Regexp, error) {
	length, err := checkPairs(pairs...)
	if err != nil {
		return nil, err
	}
	m := make(map[string]*regexp.Regexp, length)
	for i := 0; i < len(pairs); i += 2 {
		regex, err := regexp.Compile(pairs[i+1])
		if err != nil {
			return nil, err
		}
		m[pairs[i]] = regex
	}
	return m, nil
}

// uniqueVars returns an error if two slices contain duplicated variable names.
func uniqueVars(s1, s2 []string) error {
	for _, v1 := range s1 {
		for _, v2 := range s2 {
			if v1 == v2 {
				return fmt.Errorf("mux: duplicated route variable %q", v2)
			}
		}
	}
	return nil
}

// matchInArray returns true if the given string value is in the array.
func matchInArray(arr []string, value string) bool {
	for _, v := range arr {
		if v == value {
			return true
		}
	}
	return false
}

// matchMapWithString returns true if the given key/value pairs exist in a
// given map. When canonicalKey is true, keys are normalized per
// RFC 7230 Section 3.2 (header field names are case-insensitive).
func matchMapWithString(toCheck map[string]string, toMatch map[string][]string, canonicalKey bool) bool {
	for k, v := range toCheck {
		// Check if key exists.
		if canonicalKey {
			k = http.CanonicalHeaderKey(k)
		}
		values, keyExists := toMatch[k]
		if !keyExists {
			return false
		}
		if v != "" && !matchInArray(values, v) {
			return false
		}
	}
	return true
}

// matchMapWithRegex returns true if the given key/regexp pairs match a
// given map. When canonicalKey is true, keys are normalized per
// RFC 7230 Section 3.2 (header field names are case-insensitive).
func matchMapWithRegex(toCheck map[string]*regexp.Regexp, toMatch map[string][]string, canonicalKey bool) bool {
	for k, v := range toCheck {
		if canonicalKey {
			k = http.CanonicalHeaderKey(k)
		}
		values, keyExists := toMatch[k]
		if !keyExists {
			return false
		}
		if !matchAnyRegexp(v, values) {
			return false
		}
	}
	return true
}

// matchAnyRegexp returns true if the regexp matches any of the given values.
func matchAnyRegexp(re *regexp.Regexp, values []string) bool {
	for _, v := range values {
		if re.MatchString(v) {
			return true
		}
	}
	return false
}

// allowedMethods returns the HTTP methods that match the request path
// but not the request method. Used to populate the Allow header field
// required by RFC 7231 Section 6.5.5 on 405 responses.
// The returned slice is sorted alphabetically per RFC 7231 Section 7.4.1.
func allowedMethods(router *Router, req *http.Request) []string {
	methods := []string{
		http.MethodGet, http.MethodHead, http.MethodPost,
		http.MethodPut, http.MethodPatch, http.MethodDelete,
		http.MethodOptions,
	}
	var allowed []string
	for _, method := range methods {
		if method == req.Method {
			continue
		}
		testReq := req.Clone(req.Context())
		testReq.Method = method
		if router.Match(testReq, &RouteMatch{}) {
			allowed = append(allowed, method)
		}
	}
	sort.Strings(allowed)
	return allowed
}

// methodNotAllowed replies to the request with an HTTP 405 method not allowed.
// RFC 7231 Section 6.5.5: the Allow header is set by the caller (Router.ServeHTTP)
// before this handler is invoked.
func methodNotAllowed(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusMethodNotAllowed)
}

// methodNotAllowedHandler returns a HandlerFunc that replies with 405.
func methodNotAllowedHandler() http.Handler {
	return http.HandlerFunc(methodNotAllowed)
}

// subtractSlice returns elements in a that are not in b.
func subtractSlice(a, b []string) []string {
	result := make([]string, 0, len(a))
	for _, s := range a {
		if !matchInArray(b, s) {
			result = append(result, s)
		}
	}
	return result
}

// requestURIPath returns the percent-encoded path from the request URI
// per RFC 3986 Section 2.1. Falls back to the decoded Path if RawPath
// is empty.
func requestURIPath(u *url.URL) string {
	if u.RawPath != "" {
		return u.RawPath
	}
	return u.Path
}
