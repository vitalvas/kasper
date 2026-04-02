// Package securecookie provides authenticated and encrypted cookie values
// using AES-GCM.
//
// Cookies are encrypted with AES-GCM (authenticated encryption with
// associated data), which provides both confidentiality and integrity in
// a single pass. Three key sizes are supported:
//
//   - 16 bytes: AES-128-GCM
//   - 24 bytes: AES-192-GCM
//   - 32 bytes: AES-256-GCM
//
// The cookie name is bound as additional authenticated data (AAD),
// preventing cookie value transplant between different cookie names.
//
// # Key Rotation
//
// Multiple codecs can be used for seamless key rotation. When encoding, the
// first codec is always used. When decoding, each codec is tried in order
// until one succeeds:
//
//	codecs := securecookie.CodecsFromKeys(currentKey, previousKey)
//	encoded, _ := securecookie.EncodeMulti("session", value, codecs...)
//	_ = securecookie.DecodeMulti("session", encoded, &dst, codecs...)
//
// # Timestamp Validation
//
// Each encoded value embeds a timestamp. On decode, the timestamp is checked
// against configurable MaxAge and MinAge bounds to enforce cookie freshness
// and prevent replay of stale values. Cookies with future timestamps beyond
// 5 minutes of clock skew are always rejected.
//
// # Serialization
//
// Values are serialized to JSON by default. A custom [Serializer] can be
// provided via [SecureCookie.SetSerializer].
//
// # Security Considerations
//
//   - Always use cryptographically random keys (see [GenerateKey]).
//   - Prefer 32-byte keys (AES-256) for maximum security margin.
//   - Always transmit cookies over HTTPS.
//   - Bind cookies with HttpOnly, Secure, and SameSite attributes at the
//     HTTP layer; this package handles only the value encoding.
//
// References:
//   - NIST SP 800-38D: Recommendation for Block Cipher Modes of Operation: GCM
//   - RFC 6265: HTTP State Management Mechanism
package securecookie
