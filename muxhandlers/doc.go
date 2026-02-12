// Package muxhandlers provides HTTP middleware handlers for the mux router.
//
// # CORS Middleware
//
// CORSMiddleware implements the full CORS protocol per the Fetch Standard.
// It validates the Origin header (RFC 6454), handles preflight OPTIONS
// requests, and sets the appropriate response headers.
//
//	mw, err := muxhandlers.CORSMiddleware(r, muxhandlers.CORSConfig{
//	    AllowedOrigins:   []string{"https://example.com"},
//	    AllowCredentials: true,
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	r.Use(mw)
//
// # Basic Auth Middleware
//
// BasicAuthMiddleware implements HTTP Basic Authentication per RFC 7617.
// Credentials can be validated via a dynamic callback or a static map.
// Static credential comparison uses constant-time comparison to prevent
// timing attacks.
//
//	mw, err := muxhandlers.BasicAuthMiddleware(muxhandlers.BasicAuthConfig{
//	    Realm: "My App",
//	    Credentials: map[string]string{
//	        "admin": "secret",
//	    },
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	r.Use(mw)
//
// # Proxy Headers Middleware
//
// ProxyHeadersMiddleware populates request fields from reverse proxy headers
// when the request originates from a trusted proxy. It sets r.RemoteAddr from
// X-Forwarded-For or X-Real-IP, r.URL.Scheme from X-Forwarded-Proto or
// X-Forwarded-Scheme, and r.Host from X-Forwarded-Host. When EnableForwarded
// is true, the RFC 7239 Forwarded header is also parsed as a lowest-priority
// fallback. A trusted proxy list (IPs and CIDRs) restricts which peers are
// allowed to set these headers, preventing spoofing from untrusted clients.
// When TrustedProxies is empty, DefaultTrustedProxies (RFC 1918, RFC 4193,
// and loopback ranges) is used.
//
//	mw, err := muxhandlers.ProxyHeadersMiddleware(muxhandlers.ProxyHeadersConfig{
//	    TrustedProxies:  []string{"10.0.0.0/8", "172.16.0.0/12"},
//	    EnableForwarded: true,
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	r.Use(mw)
package muxhandlers
