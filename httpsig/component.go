package httpsig

import (
	"fmt"
	"net/http"
	"strings"
)

// Derived component identifiers per RFC 9421 Section 2.2.
const (
	ComponentMethod        = "@method"
	ComponentAuthority     = "@authority"
	ComponentPath          = "@path"
	ComponentQuery         = "@query"
	ComponentTargetURI     = "@target-uri"
	ComponentScheme        = "@scheme"
	ComponentRequestTarget = "@request-target"
)

// componentValue extracts the value of a covered component from an HTTP
// request per RFC 9421 Section 2.
//
// Derived components start with "@". Header field names are lowercased and
// multi-value headers are joined with ", ".
func componentValue(id string, r *http.Request) (string, error) {
	if strings.HasPrefix(id, "@") {
		return derivedComponentValue(id, r)
	}

	return headerComponentValue(id, r)
}

// derivedComponentValue extracts the value of a derived component identifier
// per RFC 9421 Section 2.2.
func derivedComponentValue(id string, r *http.Request) (string, error) {
	switch id {
	case ComponentMethod:
		return r.Method, nil

	case ComponentAuthority:
		return authority(r), nil

	case ComponentPath:
		path := r.URL.Path
		if path == "" {
			path = "/"
		}

		return path, nil

	case ComponentQuery:
		q := r.URL.RawQuery
		return "?" + q, nil

	case ComponentTargetURI:
		return targetURI(r), nil

	case ComponentScheme:
		return scheme(r), nil

	case ComponentRequestTarget:
		return requestTarget(r), nil

	default:
		return "", fmt.Errorf("%w: %s", ErrUnknownComponent, id)
	}
}

// headerComponentValue extracts the value of a header field per RFC 9421
// Section 2.1. Multiple values for the same header are joined with ", ".
//
// The "host" header is special-cased because net/http stores it in
// Request.Host rather than in the header map.
func headerComponentValue(id string, r *http.Request) (string, error) {
	canon := http.CanonicalHeaderKey(id)
	values := r.Header[canon]

	if len(values) == 0 && strings.EqualFold(id, "host") && r.Host != "" {
		return r.Host, nil
	}

	if len(values) == 0 {
		return "", fmt.Errorf("%w: header %q not present", ErrUnknownComponent, id)
	}

	return strings.Join(values, ", "), nil
}

// authority returns the authority component (host[:port]) from the request.
func authority(r *http.Request) string {
	if r.Host != "" {
		return strings.ToLower(r.Host)
	}

	if r.URL != nil && r.URL.Host != "" {
		return strings.ToLower(r.URL.Host)
	}

	return ""
}

// scheme returns the request scheme (http or https).
func scheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}

	if r.URL != nil && r.URL.Scheme != "" {
		return strings.ToLower(r.URL.Scheme)
	}

	return "http"
}

// targetURI reconstructs the full target URI for the request.
func targetURI(r *http.Request) string {
	s := scheme(r)
	a := authority(r)
	path := r.URL.Path
	if path == "" {
		path = "/"
	}

	uri := s + "://" + a + path
	if r.URL.RawQuery != "" {
		uri += "?" + r.URL.RawQuery
	}

	return uri
}

// requestTarget returns the request target (path + optional query).
func requestTarget(r *http.Request) string {
	path := r.URL.Path
	if path == "" {
		path = "/"
	}

	if r.URL.RawQuery != "" {
		return path + "?" + r.URL.RawQuery
	}

	return path
}
