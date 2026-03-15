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
// # Bearer Auth Middleware
//
// BearerAuthMiddleware implements HTTP Bearer Token Authentication per RFC 6750.
// It extracts the token from the Authorization header and validates it using a
// user-provided function. When the token is missing, malformed, or invalid, the
// middleware responds with 401 Unauthorized and a WWW-Authenticate: Bearer header
// per RFC 6750 Section 3. The ValidateFunc receives the full request, allowing
// token validation based on route variables, headers, or other request context.
//
//	mw, err := muxhandlers.BearerAuthMiddleware(muxhandlers.BearerAuthConfig{
//	    Realm: "My API",
//	    ValidateFunc: func(r *http.Request, token string) bool {
//	        return token == expectedToken
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
//
// # Profiler Handler
//
// ProfilerHandler returns an http.Handler that serves the standard
// net/http/pprof endpoints. It is not middleware — it returns an
// http.Handler that serves profiling data directly.
//
//	r.PathPrefix("/debug/pprof").Handler(muxhandlers.ProfilerHandler())
//
// # Sunset Middleware
//
// SunsetMiddleware sets the Sunset response header per RFC 8594 to indicate
// that a resource will become unresponsive at a specific point in time.
// Optionally sets the Deprecation header and a Link header with rel="sunset".
//
//	mw, err := muxhandlers.SunsetMiddleware(muxhandlers.SunsetConfig{
//	    Sunset:      time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC),
//	    Deprecation: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
//	    Link:        "https://example.com/docs/migration",
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	r.Use(mw)
//
// # Idempotency Middleware
//
// IdempotencyMiddleware caches responses keyed by the Idempotency-Key header per
// draft-ietf-httpapi-idempotency-key-header. Duplicate requests with the same key
// replay the cached response without invoking the handler. The middleware requires
// an IdempotencyStore implementation for persistence (e.g. Redis, in-memory).
//
//	mw, err := muxhandlers.IdempotencyMiddleware(muxhandlers.IdempotencyConfig{
//	    Store: redisStore,
//	    TTL:   1 * time.Hour,
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	r.Use(mw)
//
// # Content Negotiation Middleware
//
// ContentNegotiationMiddleware performs proactive content negotiation per
// RFC 9110 Section 12.5.1. It parses the Accept header with quality values,
// selects the best matching type from the offered list, and stores the result
// in the request context. Use NegotiatedType to retrieve the selected type
// inside a handler. When Offered is empty, any media type is accepted.
// When no offered type matches, the middleware responds with 406 Not Acceptable.
//
//	r.Use(muxhandlers.ContentNegotiationMiddleware(muxhandlers.ContentNegotiationConfig{
//	    Offered: []string{"application/json", "application/xml", "text/html"},
//	}))
//
// Inside a handler:
//
//	func handler(w http.ResponseWriter, r *http.Request) {
//	    switch muxhandlers.NegotiatedType(r) {
//	    case "application/json":
//	        mux.ResponseJSON(w, http.StatusOK, data)
//	    case "application/xml":
//	        mux.ResponseXML(w, http.StatusOK, data)
//	    }
//	}
//
// # Problem Details
//
// WriteProblemDetails writes an RFC 9457 Problem Details JSON response with
// Content-Type "application/problem+json". The ProblemDetails struct contains
// the standard members (type, title, status, detail, instance) and supports
// extension members via the Extensions map. NewProblemDetails creates a
// ProblemDetails with the status code and standard status text as title.
//
//	muxhandlers.WriteProblemDetails(w, muxhandlers.ProblemDetails{
//	    Type:   "https://example.com/errors/not-found",
//	    Title:  "Resource not found",
//	    Status: http.StatusNotFound,
//	    Detail: "User with ID 42 was not found",
//	})
//
// With extensions:
//
//	muxhandlers.WriteProblemDetails(w, muxhandlers.ProblemDetails{
//	    Type:   "https://example.com/errors/validation",
//	    Title:  "Validation Error",
//	    Status: http.StatusUnprocessableEntity,
//	    Extensions: map[string]any{
//	        "errors": []string{"email is required"},
//	    },
//	})
//
// Quick error response:
//
//	muxhandlers.WriteProblemDetails(w, muxhandlers.NewProblemDetails(http.StatusForbidden))
//
// # Early Hints Middleware
//
// EarlyHintsMiddleware sends a 103 Early Hints informational response per
// RFC 8297 before the final response. This allows clients to begin preloading
// resources (stylesheets, scripts, fonts) while the server is still processing
// the request. The configured Link headers are sent with the 103 response and
// are not carried over to the final response.
//
//	mw, err := muxhandlers.EarlyHintsMiddleware(muxhandlers.EarlyHintsConfig{
//	    Links: []string{
//	        `</style.css>; rel=preload; as=style`,
//	        `</app.js>; rel=preload; as=script`,
//	    },
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	r.Use(mw)
//
// # IP Allow Middleware
//
// IPAllowMiddleware restricts access to requests originating from a configured
// set of IP addresses and CIDR ranges. Requests from IPs not in the allowed
// list are rejected with 403 Forbidden by default. Use DeniedHandler to
// customize the error response.
//
//	mw, err := muxhandlers.IPAllowMiddleware(muxhandlers.IPAllowConfig{
//	    Allowed: []string{"10.0.0.0/8", "192.168.0.0/16"},
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	r.Use(mw)
package muxhandlers
