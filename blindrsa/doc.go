// Package blindrsa implements RSA Blind Signatures per RFC 9474 (RSABSSA).
//
// RSA Blind Signatures allow a client to obtain an RSA signature from a
// server without the server learning what message was signed. This is used
// for privacy-preserving tokens (e.g., Privacy Pass).
//
// # Protocol Flow
//
//  1. Client calls [Prepare] to prepare the message (adds random prefix for randomized variants).
//  2. Client calls [Blind] to produce a blinded message and blinding state.
//  3. Server calls [BlindSign] to produce a blind signature.
//  4. Client calls [Finalize] to unblind and obtain the final signature.
//  5. Anyone calls [Verify] to verify the signature (standard RSA-PSS).
//
// # Supported Variants
//
// Four variants are supported, all using SHA-384:
//
//   - RSABSSA-SHA384-PSS-Randomized (salt=48, randomized message)
//   - RSABSSA-SHA384-PSSZERO-Randomized (salt=0, randomized message)
//   - RSABSSA-SHA384-PSS-Deterministic (salt=48, deterministic message)
//   - RSABSSA-SHA384-PSSZERO-Deterministic (salt=0, deterministic message)
//
// # HTTP Integration
//
// [IssueHandler] provides an [http.Handler] for token issuance (server side).
// [NewClient] provides an HTTP client for obtaining signatures.
// [VerifyMiddleware] provides a [mux.MiddlewareFunc] for signature verification.
//
// References:
//   - RFC 9474: RSA Blind Signatures
//   - RFC 8017: PKCS #1 (RSA Cryptography Specifications)
package blindrsa
