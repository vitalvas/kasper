// Package securecookie provides authenticated cookie values in two modes:
//
//   - [SecureCookie] -- AES-GCM authenticated encryption: payload is secret
//     and tamper-proof. Use for sensitive data (PII, credentials, tokens).
//     Three key sizes: 16 (AES-128), 24 (AES-192), or 32 (AES-256) bytes.
//
//   - [SignedCookie] -- HMAC-SHA256 authenticated signing: payload is
//     readable but tamper-proof. Use when the payload is already opaque
//     (JWTs, server-issued opaque IDs) or non-sensitive. Saves the AES key
//     schedule and avoids double-encrypting opaque data. Any non-empty key
//     is accepted; 32 bytes is recommended.
//
// # Basic Usage
//
//	key, _ := securecookie.GenerateKey(32)
//	sc, _ := securecookie.New(key)
//
//	encoded, _ := sc.Encode(map[string]string{"user": "alice"})
//
//	var dst map[string]string
//	_ = sc.Decode(encoded, &dst)
//
// # Configuration
//
// Builder-style methods configure cookie validation:
//
//	sc, _ := securecookie.New(key)
//	sc.MaxAge(3600).     // reject cookies older than 1 hour
//	   MinAge(10).       // reject cookies younger than 10 seconds
//	   MaxLength(8192)   // max encoded value length in bytes
//
// # Signed-Only Mode
//
// [NewSigned] creates a [SignedCookie] for HMAC-SHA256 signing without
// encryption. Same fluent API and same [Codec] interface as [SecureCookie]:
//
//	key, _ := securecookie.GenerateSignedKey(32)
//	sc, _ := securecookie.NewSigned(key)
//	encoded, _ := sc.Encode("eyJhbGciOiJ...") // a JWT
//	var dst string
//	_ = sc.Decode(encoded, &dst)
//
// # Key Rotation
//
// [CodecsFromKeys] (encrypted) and [SignedCodecsFromKeys] (signed) create
// a [Codec] slice from multiple keys. [EncodeMulti] always encodes with
// the first (newest) key. [DecodeMulti] tries each key in order until one
// succeeds, enabling seamless rotation without invalidating existing
// cookies:
//
//	codecs, _ := securecookie.CodecsFromKeys(currentKey, previousKey)
//	encoded, _ := securecookie.EncodeMulti(value, codecs...)
//	_ = securecookie.DecodeMulti(encoded, &dst, codecs...)
//
// During rotation, add the new key at the front of the list and keep old
// keys until all cookies issued with them have expired (MaxAge).
//
// # Additional Authenticated Data (AAD)
//
// By default no AAD is used. Use [SecureCookie.AdditionalData] to bind
// cookies to context such as a user ID, cookie name, or tenant:
//
//	sc.AdditionalData([]byte("user-123"))  // bind to user
//	sc.AdditionalData([]byte("session"))   // bind to cookie name
//	sc.AdditionalData(nil)                 // clear AAD (default)
//
// # Compression
//
// Payloads are automatically compressed with deflate before encryption
// when beneficial. A Shannon entropy check skips compression for
// high-entropy data (> 6.5 bits/byte) and small payloads (< 32 bytes)
// to avoid wasted CPU. This is transparent and requires no configuration.
//
// # Timestamp Validation
//
// Each encoded value embeds a Unix timestamp. On decode, the timestamp is
// checked against configurable MaxAge and MinAge bounds to enforce cookie
// freshness. Cookies with future timestamps beyond 5 minutes of clock
// skew are always rejected.
//
// # Serialization
//
// Values are serialized to JSON by default. A custom [Serializer] can be
// provided via [SecureCookie.SetSerializer]:
//
//	sc.SetSerializer(mySerializer{})
//
// # Security Considerations
//
//   - Always use cryptographically random keys (see [GenerateKey]).
//   - Prefer 32-byte keys (AES-256) for maximum security margin.
//   - Always transmit cookies over HTTPS.
//   - Bind cookies with HttpOnly, Secure, and SameSite attributes at the
//     HTTP layer; this package handles only the value encoding.
//   - Decompressed payloads are limited to 512 KB to prevent zip-bomb attacks.
//
// References:
//   - NIST SP 800-38D: Recommendation for Block Cipher Modes of Operation: GCM
//   - RFC 6265: HTTP State Management Mechanism
package securecookie
