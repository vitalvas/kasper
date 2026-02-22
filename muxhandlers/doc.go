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
//
// # Timeout Middleware
//
// TimeoutMiddleware limits handler execution time by wrapping the handler
// with http.TimeoutHandler. It returns 503 Service Unavailable when the
// handler does not complete within the configured duration.
//
//	mw, err := muxhandlers.TimeoutMiddleware(muxhandlers.TimeoutConfig{
//	    Duration: 30 * time.Second,
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	r.Use(mw)
//
// # Compression Middleware
//
// CompressionMiddleware compresses response bodies using gzip or deflate when
// the client advertises support via the Accept-Encoding header. Gzip is
// preferred over deflate when both are accepted. It uses sync.Pool instances
// to reuse writers for performance. Compression is skipped for inherently
// compressed content types (images, video, audio, archives).
//
//	mw, err := muxhandlers.CompressionMiddleware(muxhandlers.CompressionConfig{
//	    Level:     gzip.BestSpeed,
//	    MinLength: 1024,
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	r.Use(mw)
//
// # Security Headers Middleware
//
// SecurityHeadersMiddleware sets common security response headers with
// sensible defaults. Headers are set before calling the next handler.
// By default it sets X-Content-Type-Options: nosniff, X-Frame-Options: DENY,
// and Referrer-Policy: strict-origin-when-cross-origin. HSTS, CSP,
// Permissions-Policy, and Cross-Origin-Opener-Policy headers are opt-in.
//
//	mw, err := muxhandlers.SecurityHeadersMiddleware(muxhandlers.SecurityHeadersConfig{
//	    HSTSMaxAge:            63072000,
//	    HSTSIncludeSubDomains: true,
//	    HSTSPreload:           true,
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	r.Use(mw)
//
// # Method Override Middleware
//
// MethodOverrideMiddleware allows clients to override the HTTP method via a
// configurable header. By default only POST requests are eligible for
// override; use OriginalMethods to allow other methods. The first non-empty
// header value from HeaderNames is uppercased and checked against the allowed
// set. When allowed, r.Method is updated and the header is removed from the
// request. By default it checks X-HTTP-Method-Override, X-Method-Override,
// and X-HTTP-Method in that order.
//
//	mw, err := muxhandlers.MethodOverrideMiddleware(muxhandlers.MethodOverrideConfig{})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	r.Use(mw)
//
// # Content-Type Check Middleware
//
// ContentTypeCheckMiddleware validates that requests carry a matching
// Content-Type header. Matching is case-insensitive and ignores parameters
// such as charset. It returns 415 Unsupported Media Type when the
// Content-Type is missing or does not match any of the allowed types.
// By default it checks POST, PUT, and PATCH requests.
//
//	mw, err := muxhandlers.ContentTypeCheckMiddleware(muxhandlers.ContentTypeCheckConfig{
//	    AllowedTypes: []string{"application/json"},
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	r.Use(mw)
//
// # Server Middleware
//
// ServerMiddleware sets server identification response headers. It sets
// X-Server-Hostname with the machine hostname, resolved once at factory
// time via os.Hostname. Use the Hostname field to provide a static value
// instead.
//
//	mw, err := muxhandlers.ServerMiddleware(muxhandlers.ServerConfig{})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	r.Use(mw)
//
// # Cache-Control Middleware
//
// CacheControlMiddleware sets Cache-Control and Expires response headers
// based on the response Content-Type. Rules are evaluated in order; the
// first rule whose ContentType prefix matches wins. If no rule matches and
// DefaultValue/DefaultExpires is non-empty, it is used. When the handler
// already sets a Cache-Control or Expires header, the middleware does not
// overwrite the respective header.
//
//	mw, err := muxhandlers.CacheControlMiddleware(muxhandlers.CacheControlConfig{
//	    Rules: []muxhandlers.CacheControlRule{
//	        {ContentType: "image/", Value: "public, max-age=86400", Expires: 24 * time.Hour},
//	        {ContentType: "application/json", Value: "no-cache", Expires: 0},
//	    },
//	    DefaultValue:   "no-store",
//	    DefaultExpires: 0,
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	r.Use(mw)
//
// # Static Files Handler
//
// StaticFilesHandler serves static files from any fs.FS implementation
// (os.DirFS, embed.FS, fstest.MapFS, etc.) using http.FileServerFS.
// It is not middleware — it returns an http.Handler that serves files
// directly. Directory listing is disabled by default; when a directory
// has no index.html, a 404 is returned instead of a file listing.
// When SPAFallback is enabled, requests for non-existent paths serve
// the root index.html, allowing client-side routers to handle routing.
//
//	handler, err := muxhandlers.StaticFilesHandler(muxhandlers.StaticFilesConfig{
//	    FS:          os.DirFS("./public"),
//	    SPAFallback: true,
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	r.PathPrefix("/").Handler(handler)
package muxhandlers
