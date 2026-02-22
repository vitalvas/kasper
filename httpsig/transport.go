package httpsig

import "net/http"

// Transport is an http.RoundTripper that signs outgoing requests using
// HTTP Message Signatures (RFC 9421).
//
// Use NewTransport to create a Transport with a configured *http.Transport
// for proxy, TLS, and timeout settings.
type Transport struct {
	base   http.RoundTripper
	config SignConfig
}

// NewTransport creates a signing Transport that delegates to base after
// signing each request. When base is nil, a clone of http.DefaultTransport
// is used, giving an independent connection pool with default proxy, TLS,
// and timeout settings.
//
// Configure base for custom proxy (HTTP/SOCKS), TLS, timeouts, and
// connection pool settings:
//
//	base := &http.Transport{
//	    Proxy:           http.ProxyFromEnvironment,
//	    TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13},
//	    IdleConnTimeout: 90 * time.Second,
//	}
//	transport := httpsig.NewTransport(base, httpsig.SignConfig{Signer: signer})
func NewTransport(base *http.Transport, cfg SignConfig) *Transport {
	var rt http.RoundTripper
	if base != nil {
		rt = base
	} else {
		rt = http.DefaultTransport.(*http.Transport).Clone()
	}

	return &Transport{
		base:   rt,
		config: cfg,
	}
}

// RoundTrip signs the request and then delegates to the base transport.
// The original request is cloned before signing to avoid mutation.
// When GetBody is available, the clone receives its own body copy so
// that digest computation does not consume the caller's body.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())

	if clone.Body != nil && req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, err
		}

		clone.Body = body
	}

	if err := SignRequest(clone, t.config); err != nil {
		return nil, err
	}

	return t.base.RoundTrip(clone)
}
