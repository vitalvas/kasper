# privacypass

Privacy Pass publicly verifiable tokens of token type 0x0002 (Blind RSA) per
RFC 9578, with the PrivateToken HTTP authentication scheme of RFC 9577. Built
on the RSA Blind Signature Scheme (RFC 9474) from kasper/blindrsa.

Token type 0x0002 uses `RSABSSA-SHA384-PSS-Deterministic` (RSA-2048, SHA-384,
MGF1-SHA-384, salt length 48). Tokens are publicly verifiable: an origin
verifies a redeemed token with a standard RSA-PSS check against the issuer
public key, with no blinding state and no per-token issuer interaction.

## Roles

- **Issuer** holds the RSA private key and blind-signs token requests.
- **Origin** holds only the issuer public key and verifies redeemed tokens.
- **Client** obtains a blind signature from the issuer and redeems the
  assembled token at the origin.

## Token flow

1. The origin advertises a challenge in a `WWW-Authenticate` header
   (`BuildChallengeHeader`).
2. The client computes `challenge_digest = SHA-256(TokenChallenge)`, picks a
   32-byte nonce, and forms
   `token_input = token_type || nonce || challenge_digest || token_key_id`.
3. The client blinds `token_input` (`blindrsa.Blind`), the issuer signs it
   (`blindrsa.BlindSign`), and the client unblinds it (`blindrsa.Finalize`) to
   recover the authenticator.
4. The client assembles a `Token` and redeems it in an `Authorization` header
   (`BuildAuthorizationHeader`).
5. The origin verifies the token (`VerifyToken`).

## Wire structures

| Type | RFC | Size | Description |
|------|-----|------|-------------|
| `TokenChallenge` | RFC 9577 §2.1 | variable | Challenge advertised in WWW-Authenticate |
| `Token` | RFC 9578 §2 | 354 bytes | Redeemed token |
| `TokenRequest` | RFC 9578 | 259 bytes | Blinded request to the issuer |

Each has `Marshal` and a matching `Unmarshal*` function for byte-exact
encoding:

```go
enc, err := token.Marshal()
got, err := privacypass.UnmarshalToken(enc)
```

`TokenChallenge.RedemptionContext` must be empty or exactly 32 bytes;
otherwise `Marshal` returns `ErrMalformed`.

## Key identity

The issuer public key is encoded as an id-RSASSA-PSS SubjectPublicKeyInfo,
which RFC 9578 requires and which differs from Go's default rsaEncryption
encoding (`x509.MarshalPKIXPublicKey`). The `token_key_id` is the SHA-256 of
that encoding:

```go
keyDER, _ := privacypass.MarshalPublicKey(pub) // id-RSASSA-PSS SPKI
keyID, _ := privacypass.TokenKeyID(pub)         // SHA-256 of the SPKI
```

## Challenge and redemption headers

`BuildChallengeHeader` produces the `WWW-Authenticate` value; the client
redeems with `BuildAuthorizationHeader`; the origin decodes with
`ParseAuthorizationHeader`. All three parameters (`challenge`, `token-key`,
`token`) are base64url with padding, as RFC 9577 requires.

```go
// Origin advertises a challenge.
h, _ := privacypass.BuildChallengeHeader(challenge, pub)
w.Header().Set("WWW-Authenticate", h)

// Client redeems a token.
a, _ := privacypass.BuildAuthorizationHeader(token)
req.Header.Set("Authorization", a)

// Origin parses the redemption.
token, err := privacypass.ParseAuthorizationHeader(req.Header.Get("Authorization"))
```

## Verification

`VerifyToken` checks, in order: the `token_key_id` matches the verifying key,
the `challenge_digest` matches the expected challenge, the authenticator is a
valid RSA-PSS signature over `token_input`, and the nonce has not been
redeemed before. The nonce is recorded only after the signature verifies.

```go
cache := privacypass.NewMemoryNonceCache()
err := privacypass.VerifyToken(pub, challenge, token, cache, time.Hour)
```

## Origin middleware

`Middleware` requires a valid PrivateToken on each request. Requests with no
token, or an invalid or replayed token, receive 401 with a fresh challenge;
valid requests proceed to the next handler.

```go
mw, err := privacypass.Middleware(privacypass.Config{
    PublicKey: pub,
    Challenge: &privacypass.TokenChallenge{
        TokenType:  privacypass.TokenType,
        IssuerName: "issuer.example",
        OriginInfo: "origin.example",
    },
})
if err != nil {
    log.Fatal(err)
}

r := mux.NewRouter()
r.Use(mw)
r.HandleFunc("/api/v1/resource", handler).Methods(http.MethodGet)
```

### Config

| Field | Description |
|-------|-------------|
| `PublicKey` | Issuer public key used to verify redeemed tokens. Required |
| `Challenge` | TokenChallenge advertised to clients and checked on redemption. Required |
| `Cache` | NonceCache for single-use enforcement. Defaults to a process-local MemoryNonceCache |
| `TokenTTL` | Nonce retention window. Defaults to one hour |
| `OnError` | Verification-failure handler. Defaults to 401 with a fresh challenge |

## Cluster deployments

`NonceCache` is an interface:

```go
type NonceCache interface {
    StoreUnique(nonce []byte, ttl time.Duration) bool
}
```

`MemoryNonceCache` is process-local, so in a multi-instance deployment a token
redeemed on one instance is not seen by the others. Implement `NonceCache`
over a shared store with an atomic set-if-absent operation so double-spend is
prevented cluster-wide:

```go
type redisNonceCache struct{ client *redis.Client }

func (c *redisNonceCache) StoreUnique(nonce []byte, ttl time.Duration) bool {
    key := fmt.Sprintf("pp:%x", nonce)
    ok, _ := c.client.SetNX(ctx, key, 1, ttl).Result()
    return ok
}

mw, _ := privacypass.Middleware(privacypass.Config{
    PublicKey: pub,
    Challenge: challenge,
    Cache:     &redisNonceCache{client: rdb},
})
```

## Errors

| Error | Description |
|-------|-------------|
| `ErrMalformed` | A structure or header could not be decoded |
| `ErrWrongTokenType` | Decoded token type is not 0x0002 |
| `ErrNoToken` | Authorization header carries no PrivateToken credential |
| `ErrKeyIDMismatch` | Token key id does not match the verifying key |
| `ErrChallengeMismatch` | Token challenge digest does not match the expected challenge |
| `ErrBadSignature` | Authenticator failed RSA-PSS verification |
| `ErrReplay` | Token nonce has already been redeemed |
| `ErrNoNonceCache` | Verification requested with a nil cache; fails closed |
| `ErrNoPublicKey` | Middleware configured without an issuer public key |

## Standards

- [RFC 9576](https://www.rfc-editor.org/rfc/rfc9576) - Privacy Pass Architecture
- [RFC 9577](https://www.rfc-editor.org/rfc/rfc9577) - Privacy Pass Issuance / HTTP Authentication
- [RFC 9578](https://www.rfc-editor.org/rfc/rfc9578) - Privacy Pass Token Types (Blind RSA, 0x0002)
- [RFC 9474](https://www.rfc-editor.org/rfc/rfc9474) - RSA Blind Signatures (via kasper/blindrsa)
