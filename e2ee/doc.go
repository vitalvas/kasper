// Package e2ee implements end-to-end encryption for HTTP API payloads per
// draft-vasylenko-e2ee-http. It protects request and response bodies
// independently of, and in addition to, transport security such as TLS, so
// that TLS-terminating intermediaries (CDNs, reverse proxies, load balancers)
// cannot read or tamper with payloads.
//
// It provides both client-side encryption (via Transport) and server-side
// decryption (via Middleware) for the kasper HTTP toolkit.
//
// # Cryptography
//
// The scheme combines:
//
//   - X25519 key agreement (RFC 7748)
//   - HKDF-SHA256 key derivation (RFC 5869) with separate request and
//     response keys
//   - AES-GCM authenticated encryption: AES-128-GCM, AES-192-GCM, or
//     AES-256-GCM
//
// Each protected body is the concatenation nonce || ciphertext || tag, with a
// 12-octet random nonce and 16-octet tag. Control metadata travels in the
// E2EE-Session HTTP field, an RFC 9651 Structured Field Item whose value is
// the key identifier (kid) and whose parameters carry the AEAD suite, the
// client's ephemeral public key (epk), a timestamp (ts), a per-message replay
// identifier (nid), and an optional inner content type (cty). The serialized
// E2EE-Session field is bound into the AES-GCM additional authenticated data,
// preventing intermediary tampering and response substitution.
//
// # Key Discovery
//
// Servers publish their X25519 public keys as a JSON key set at
// WellKnownPath ("/.well-known/encryption-keys"). ServerKeySet.Handler serves
// this document; clients retrieve and validate it with FetchKeySet, producing
// a ClientKeySet.
//
//	serverKey, _ := e2ee.GenerateServerKey("2026-06", 24*time.Hour, e2ee.DefaultMaxSkew)
//	set, _ := e2ee.NewServerKeySet("https://api.example.com", serverKey)
//	router.Handle(e2ee.WellKnownPath, set.Handler())
//
// # Client Transport
//
// NewTransport returns an http.RoundTripper that encrypts each outgoing
// request body, sets the E2EE-Session header, and decrypts the matching
// response. RFC 9457 Problem Details error responses are returned undecrypted
// so callers can inspect the protocol error.
//
//	keys, _ := e2ee.FetchKeySet(http.DefaultClient, "https://api.example.com")
//	client := &http.Client{
//	    Transport: e2ee.NewTransport(nil, e2ee.ClientConfig{
//	        KeySet:      keys,
//	        ContentType: "application/json",
//	    }),
//	}
//	resp, err := client.Post("https://api.example.com/api/v1/data", "application/json", body)
//
// By default a fresh ephemeral key pair is generated per request. Configure a
// Session to reuse one key pair across requests; sessions must be scoped to a
// single logical client, never persisted, and destroyed on exit or logout.
//
// # Server Middleware
//
// Middleware returns a mux.MiddlewareFunc that decrypts incoming requests and
// encrypts handler responses. Handlers see and produce plaintext. A shared
// in-memory replay cache is installed automatically; supply ServerConfig.Replay
// to plug in a distributed cache.
//
//	mw, _ := e2ee.Middleware(e2ee.MiddlewareConfig{
//	    Server: e2ee.ServerConfig{KeySet: set},
//	})
//	api := router.PathPrefix("/api").Subrouter()
//	api.Use(mw)
//
// # Errors
//
// Protocol failures map to the sentinel errors ErrKeyUnknown, ErrKeyExpired,
// ErrAEADUnsupported, ErrDecryptFailed, ErrTimestampSkew, ErrReplayDetected,
// and ErrMalformed. On the server they are rendered as RFC 9457 Problem
// Details responses (application/problem+json) with the URN type
// urn:ietf:params:e2ee:error:<code>.
//
// # Threat Model
//
// The scheme protects payload confidentiality and integrity against
// TLS-terminating intermediaries and passive observers on plaintext channels
// behind a TLS terminator. It does not protect HTTP metadata (method, path,
// status, non-body headers) or defend against traffic analysis. HTTP request
// and response semantics are intentionally not bound into the AEAD; deployments
// needing end-to-end binding of method, target, or status should layer HTTP
// Message Signatures (see github.com/vitalvas/kasper/httpsig) over the
// E2EE-Session field and a Content-Digest of the ciphertext body. Forward
// secrecy applies to client-side compromise only; compromise of a server
// private key decrypts all past sessions that used it, so rotate keys
// frequently with short validity windows.
package e2ee
