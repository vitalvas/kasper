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
package muxhandlers
