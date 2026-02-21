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
//
// # Recovery Middleware
//
// RecoveryMiddleware recovers from panics in downstream handlers, returns
// 500 Internal Server Error to the client, and optionally invokes a custom
// log function with the request and recovered value.
//
//	r.Use(muxhandlers.RecoveryMiddleware(muxhandlers.RecoveryConfig{
//	    LogFunc: func(r *http.Request, err any) {
//	        log.Printf("panic: %v %s", err, r.URL.Path)
//	    },
//	}))
//
// # Request ID Middleware
//
// RequestIDMiddleware generates or propagates a unique request identifier.
// The ID is set on the request header, the response header, and the request
// context. Downstream handlers can retrieve it with RequestIDFromContext.
// By default it generates UUID v4 values using github.com/google/uuid.
// Use GenerateUUIDv7 for time-ordered IDs (RFC 9562). The GenerateFunc
// receives the current request, allowing ID generation based on request
// context.
//
//	r.Use(muxhandlers.RequestIDMiddleware(muxhandlers.RequestIDConfig{
//	    TrustIncoming: true,
//	}))
//
// Time-ordered UUID v7:
//
//	r.Use(muxhandlers.RequestIDMiddleware(muxhandlers.RequestIDConfig{
//	    GenerateFunc: muxhandlers.GenerateUUIDv7,
//	}))
//
// # Request Size Limit Middleware
//
// RequestSizeLimitMiddleware rejects request bodies that exceed a maximum
// size. It wraps r.Body with http.MaxBytesReader, which returns 413 Request
// Entity Too Large when the limit is exceeded.
//
//	mw, err := muxhandlers.RequestSizeLimitMiddleware(muxhandlers.RequestSizeLimitConfig{
//	    MaxBytes: 1 << 20, // 1 MiB
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	r.Use(mw)
package muxhandlers
