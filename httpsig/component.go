package httpsig

import (
	"fmt"
	"net/http"
	"strconv"
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
	ComponentStatus        = "@status"
)

// reqSuffix marks a covered component that resolves against the originating
// request when signing or verifying a response (RFC 9421 Section 2.4), for
// example "@authority;req" or "content-digest;req".
const reqSuffix = ";req"

// sigMessage is the HTTP message whose components are signed or verified: a
// request alone, or a response paired with its originating request.
type sigMessage struct {
	req  *http.Request
	resp *http.Response // nil when the message is the request itself
}

// header returns the header map of the message itself, where Signature and
// Signature-Input live.
func (m sigMessage) header() http.Header {
	if m.resp != nil {
		return m.resp.Header
	}

	return m.req.Header
}

// componentValue extracts the value of a covered component from the message
// per RFC 9421 Section 2.
//
// Components carrying the ";req" parameter resolve against the originating
// request and are only valid for response messages. "@status" is only valid
// for response messages. All other derived components are request-only.
func componentValue(id string, m sigMessage) (string, error) {
	name, isReq := strings.CutSuffix(id, reqSuffix)

	if m.resp == nil {
		if isReq || name == ComponentStatus {
			return "", fmt.Errorf("%w: %s", ErrInvalidComponent, id)
		}

		return requestComponentValue(name, m.req)
	}

	if isReq {
		if m.req == nil {
			return "", fmt.Errorf("%w: %s requires the originating request", ErrInvalidComponent, id)
		}

		return requestComponentValue(name, m.req)
	}

	if name == ComponentStatus {
		return strconv.Itoa(m.resp.StatusCode), nil
	}

	if strings.HasPrefix(name, "@") {
		return "", fmt.Errorf("%w: %s is request-only; use %s%s", ErrInvalidComponent, name, name, reqSuffix)
	}

	return headerComponentValue(name, m.resp.Header)
}

// requestComponentValue extracts the value of a covered component from an
// HTTP request. Derived components start with "@". Header field names are
// lowercased and multi-value headers are joined with ", ".
//
// The "host" header is special-cased because net/http stores it in
// Request.Host rather than in the header map.
func requestComponentValue(id string, r *http.Request) (string, error) {
	if strings.HasPrefix(id, "@") {
		return derivedComponentValue(id, r)
	}

	if len(r.Header[http.CanonicalHeaderKey(id)]) == 0 && strings.EqualFold(id, "host") && r.Host != "" {
		return r.Host, nil
	}

	return headerComponentValue(id, r.Header)
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
		return fmt.Sprintf("?%s", q), nil

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
func headerComponentValue(id string, hdr http.Header) (string, error) {
	values := hdr[http.CanonicalHeaderKey(id)]
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

	uri := fmt.Sprintf("%s://%s%s", s, a, path)
	if r.URL.RawQuery != "" {
		uri = fmt.Sprintf("%s?%s", uri, r.URL.RawQuery)
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
		return fmt.Sprintf("%s?%s", path, r.URL.RawQuery)
	}

	return path
}
