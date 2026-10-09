// Package httpsig implements HTTP Message Signatures per RFC 9421 with
// optional Content-Digest support per RFC 9530.
//
// It covers the full request-response flow for the kasper HTTP toolkit:
// client-side request signing (SignRequest, Transport), server-side request
// verification (VerifyRequest, Middleware), server-side response signing
// (SignResponse, SignMiddleware), and client-side response verification
// (VerifyResponse).
//
// # Supported Algorithms
//
// Six signature algorithms are supported:
//
//   - ed25519 (Edwards-Curve DSA)
//   - ecdsa-p256-sha256 (ECDSA P-256)
//   - ecdsa-p384-sha384 (ECDSA P-384)
//   - rsa-pss-sha512 (RSASSA-PSS)
//   - rsa-v1_5-sha256 (RSASSA-PKCS1-v1_5)
//   - hmac-sha256 (HMAC)
//
// # Signing Requests
//
// Use SignRequest to add Signature and Signature-Input headers to an HTTP
// request:
//
//	signer, err := httpsig.NewEd25519Signer("my-key-id", privateKey)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	err = httpsig.SignRequest(req, httpsig.SignConfig{
//	    Signer:            signer,
//	    CoveredComponents: []string{httpsig.ComponentMethod, httpsig.ComponentAuthority, httpsig.ComponentPath},
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// # Verifying Requests
//
// Use VerifyRequest to verify the signature on an incoming request:
//
//	resolver := func(r *http.Request, keyID string, alg httpsig.Algorithm) (httpsig.Verifier, error) {
//	    // Look up the verifier for the given key ID and algorithm.
//	    return verifier, nil
//	}
//
//	err := httpsig.VerifyRequest(req, httpsig.VerifyConfig{
//	    Resolver:           resolver,
//	    RequiredComponents: []string{httpsig.ComponentMethod, httpsig.ComponentAuthority},
//	    MaxAge:             5 * time.Minute,
//	})
//
// # Signing Responses
//
// Use SignResponse to sign an HTTP response. Covered components may carry
// the ";req" parameter (RFC 9421 Section 2.4) to resolve against the
// originating request, binding the response signature to the request. The
// "@status" derived component covers the response status code and is the
// default when no components are configured:
//
//	err := httpsig.SignResponse(resp, req, httpsig.SignConfig{
//	    Signer:            signer,
//	    CoveredComponents: []string{"@status", "@authority;req", "@path;req"},
//	    DigestAlgorithm:   httpsig.DigestSHA256,
//	})
//
// # Verifying Responses
//
// Use VerifyResponse to verify a signed response. When req is nil, the
// response's Request field (set by net/http clients) is used to resolve
// ";req" components:
//
//	resp, err := client.Get("https://api.example.com/resource")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	err = httpsig.VerifyResponse(resp, nil, httpsig.VerifyConfig{
//	    Resolver:      resolver,
//	    RequireDigest: true,
//	})
//
// # Client Transport
//
// NewTransport creates an http.RoundTripper that automatically signs all
// outgoing requests. Pass an *http.Transport to configure proxy, TLS, and
// timeout settings. Pass nil for sensible defaults:
//
//	client := &http.Client{
//	    Transport: httpsig.NewTransport(nil, httpsig.SignConfig{
//	        Signer: signer,
//	    }),
//	}
//
//	resp, err := client.Get("https://api.example.com/resource")
//
// With custom proxy and TLS:
//
//	base := &http.Transport{
//	    Proxy:           http.ProxyFromEnvironment,
//	    TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13},
//	}
//	client := &http.Client{
//	    Transport: httpsig.NewTransport(base, httpsig.SignConfig{
//	        Signer: signer,
//	    }),
//	}
//
// # Server Middleware
//
// Middleware returns a mux.MiddlewareFunc that verifies signatures on
// incoming requests. It integrates with the kasper/mux router:
//
//	mw, err := httpsig.Middleware(httpsig.MiddlewareConfig{
//	    Verify: httpsig.VerifyConfig{
//	        Resolver: resolver,
//	    },
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	router.Use(mw)
//
// SignMiddleware signs outgoing responses. The handler's response is
// buffered so the signature covers the final status, headers, and body:
//
//	signMW, err := httpsig.SignMiddleware(httpsig.SignConfig{
//	    Signer:            signer,
//	    CoveredComponents: []string{"@status", "@authority;req", "@path;req"},
//	    DigestAlgorithm:   httpsig.DigestSHA256,
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	router.Use(signMW)
//
// # Content-Digest
//
// Optional Content-Digest support (RFC 9530) can be used standalone or
// integrated with signing:
//
//	// Standalone usage:
//	err := httpsig.SetContentDigest(req, httpsig.DigestSHA256)
//
//	// Integrated with signing (adds Content-Digest and includes it
//	// in covered components automatically):
//	err := httpsig.SignRequest(req, httpsig.SignConfig{
//	    Signer:          signer,
//	    DigestAlgorithm: httpsig.DigestSHA256,
//	})
package httpsig
