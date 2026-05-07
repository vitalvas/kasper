# securecookie

Authenticated cookie values. Two modes:

- `SecureCookie` -- AES-GCM authenticated **encryption** (payload secret + tamper-proof).
- `SignedCookie` -- HMAC-SHA256 authenticated **signing** (payload readable + tamper-proof).

Use `SecureCookie` for sensitive data (PII, tokens, credentials). Use `SignedCookie` for already-opaque payloads (JWTs, opaque IDs) where confidentiality is unnecessary; you save the AES key schedule and avoid double-encrypting opaque data.

## Features

- AES-GCM authenticated encryption (AES-128, AES-192, AES-256) -- `SecureCookie`
- HMAC-SHA256 authenticated signing -- `SignedCookie`
- Optional additional authenticated data (AAD) binding (user ID, cookie name, tenant, etc.)
- Embedded timestamp with configurable MaxAge, MinAge, and future-timestamp rejection
- Key rotation via multi-codec encode/decode (both modes)
- Pluggable serialization (JSON by default)
- Entropy-adaptive compression (deflate for compressible data, skipped for high-entropy payloads)
- Cryptographically random key generation
- Builder-style configuration

## Installation

```bash
go get github.com/vitalvas/kasper/securecookie
```

## Usage

### Basic Encode/Decode (encrypted)

```go
key, _ := securecookie.GenerateKey(32) // 16 for AES-128, 24 for AES-192, 32 for AES-256
sc, _ := securecookie.New(key)

encoded, _ := sc.Encode(map[string]string{"user": "alice"})

var dst map[string]string
_ = sc.Decode(encoded, &dst)
```

### Signed-only Encode/Decode (HMAC, no encryption)

Use `SignedCookie` when the payload is already opaque (JWTs, server-issued IDs) or non-sensitive. The cookie is tamper-proof but readable.

```go
key, _ := securecookie.GenerateSignedKey(32) // 32 bytes recommended
sc, _ := securecookie.NewSigned(key)

encoded, _ := sc.Encode("eyJhbGciOiJ...") // a JWT
// encoded contains the JWT visible in base64; HMAC-SHA256 prevents tampering.

var dst string
_ = sc.Decode(encoded, &dst)
```

`SignedCookie` shares the same fluent API as `SecureCookie` (`MaxAge`, `MinAge`, `MaxLength`, `SetSerializer`, `AdditionalData`) and satisfies the same `Codec` interface, so it composes with `EncodeMulti` / `DecodeMulti` and graceful key rotation via `SignedCodecsFromKeys`.

### Configuration

```go
sc, _ := securecookie.New(key)
sc.MaxAge(3600).     // cookie expires after 1 hour
   MinAge(10).       // reject cookies younger than 10 seconds
   MaxLength(8192)   // max encoded length in bytes
```

Set to 0 to disable MaxAge, MinAge, or MaxLength checks.

### Key Rotation

`CodecsFromKeys` creates a `Codec` slice from multiple AES keys. `EncodeMulti` always uses the first (newest) key. `DecodeMulti` tries each key in order until one succeeds.

```go
codecs, _ := securecookie.CodecsFromKeys(currentKey, previousKey)

encoded, _ := securecookie.EncodeMulti(value, codecs...)

var dst MySession
_ = securecookie.DecodeMulti(encoded, &dst, codecs...)
```

Rotation strategy:

1. Generate a new key and add it at the front of the list
2. Keep old keys in the list until all cookies issued with them have expired (MaxAge)
3. Remove expired keys from the list

Keys can have different sizes (e.g., rotating from AES-128 to AES-256):

```go
codecs, _ := securecookie.CodecsFromKeys(newKey256, oldKey128)
```

Use `SignedCodecsFromKeys` for the same pattern with signing-only codecs:

```go
codecs, _ := securecookie.SignedCodecsFromKeys(currentKey, previousKey)
```

### Custom Serializer

```go
sc, _ := securecookie.New(key)
sc.SetSerializer(mySerializer{})
```

Any type implementing the `Serializer` interface works:

```go
type Serializer interface {
    Serialize(src any) ([]byte, error)
    Deserialize(src []byte, dst any) error
}
```

### Additional Authenticated Data (AAD)

AAD is bound into the GCM authentication tag. By default no AAD is used. Use `AdditionalData` to bind cookies to context such as a user ID, cookie name, or tenant:

```go
// Bind to user ID
sc.AdditionalData([]byte("user-123"))

// Bind to cookie name
sc.AdditionalData([]byte("session"))

// Clear AAD (default)
sc.AdditionalData(nil)
```

### HTTP Cookie Integration

```go
func setHandler(w http.ResponseWriter, r *http.Request) {
    encoded, _ := sc.Encode(map[string]string{"theme": "dark"})
    http.SetCookie(w, &http.Cookie{
        Name:     "prefs",
        Value:    encoded,
        Path:     "/",
        MaxAge:   3600,
        HttpOnly: true,
        Secure:   true,
        SameSite: http.SameSiteLaxMode,
    })
}

func getHandler(w http.ResponseWriter, r *http.Request) {
    cookie, _ := r.Cookie("prefs")
    var prefs map[string]string
    _ = sc.Decode(cookie.Value, &prefs)
}
```

## Compression

Payloads are automatically compressed with deflate before encryption when beneficial. A fast Shannon entropy check skips compression entirely for high-entropy data (random tokens, encrypted blobs) to avoid wasted CPU.

| Payload | JSON | Encoded | Ratio |
|---|---|---|---|
| 2 strings | 31 B | 91 B | 294% |
| 5 strings | 102 B | 178 B | 175% |
| OIDC session (JWT tokens) | 696 B | 180 B | 26% |
| 50 repeated fields | 2,001 B | 280 B | 14% |
| Keycloak heavy (10 roles, 5 clients) | 5,220 B | 1,538 B | 29% |

Small payloads (< 32 B) and high-entropy data (> 6.5 bits/byte) are stored raw with zero compression overhead. All scenarios above fit within the 4KB browser cookie limit.

## Security

| Property | `SecureCookie` (AES-GCM) | `SignedCookie` (HMAC-SHA256) |
|---|:---:|:---:|
| Tamper detection | Yes | Yes |
| Confidentiality (payload secret) | Yes | **No** -- payload is readable |
| Constant-time signature compare | Yes | Yes |
| AAD binding | Yes | Yes |

Common to both:

- Future timestamps beyond 5 minutes of clock skew are rejected
- Decompressed payloads are limited to 512 KB to prevent zip-bomb attacks
- Generate keys with `GenerateKey(32)` / `GenerateSignedKey(32)` and store them securely
- Always transmit cookies over HTTPS with HttpOnly and Secure flags

Pick `SecureCookie` whenever the cookie body could include anything sensitive. Reach for `SignedCookie` only for opaque or non-sensitive payloads.
