// Package privacypass implements Privacy Pass tokens of token type 0x0002
// (Blind RSA) per RFC 9578 and the PrivateToken HTTP authentication scheme
// per RFC 9577.
//
// Type 0x0002 uses the RSA Blind Signature Scheme (RFC 9474) from the
// kasper/blindrsa package with variant RSABSSA-SHA384-PSS-Deterministic:
// RSA-2048 (Nk = 256), SHA-384, MGF1-SHA-384, salt length 48. Tokens are
// publicly verifiable, so the origin verifies with standard RSA-PSS and no
// blinding state.
//
// # Origin Middleware
//
// Middleware requires a valid PrivateToken on each request, issuing a
// WWW-Authenticate challenge to requests without one:
//
//	mw, err := privacypass.Middleware(privacypass.Config{
//	    PublicKey: issuerPublicKey,
//	    Challenge: &privacypass.TokenChallenge{
//	        TokenType:  privacypass.TokenType,
//	        IssuerName: "issuer.example",
//	        OriginInfo: "origin.example",
//	    },
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	r := mux.NewRouter()
//	r.Use(mw)
//
// # Challenge and Redemption Headers
//
// BuildChallengeHeader produces the WWW-Authenticate value advertising a
// challenge and the issuer key:
//
//	header, _ := privacypass.BuildChallengeHeader(challenge, issuerPublicKey)
//	w.Header().Set("WWW-Authenticate", header)
//
// BuildAuthorizationHeader produces the Authorization value that redeems a
// token, and ParseAuthorizationHeader decodes it on the origin:
//
//	auth, _ := privacypass.BuildAuthorizationHeader(token)
//	req.Header.Set("Authorization", auth)
//
//	token, err := privacypass.ParseAuthorizationHeader(req.Header.Get("Authorization"))
//
// # Verification
//
// VerifyToken checks the token key id, challenge digest, and RSA-PSS
// signature, and records the nonce for single-use enforcement. A nil cache
// fails closed with ErrNoNonceCache; a reused nonce returns ErrReplay:
//
//	cache := privacypass.NewMemoryNonceCache()
//	err := privacypass.VerifyToken(issuerPublicKey, challenge, token, cache, time.Hour)
//
// NonceCache is an interface. MemoryNonceCache is process-local; for a
// multi-instance cluster, implement NonceCache over a shared store so a nonce
// redeemed on one instance is rejected on the others:
//
//	type redisNonceCache struct{ client *redis.Client }
//
//	func (c *redisNonceCache) StoreUnique(nonce []byte, ttl time.Duration) bool {
//	    key := fmt.Sprintf("pp:%x", nonce)
//	    ok, _ := c.client.SetNX(ctx, key, 1, ttl).Result()
//	    return ok
//	}
//
// # Key Identity
//
// The issuer key is encoded as an id-RSASSA-PSS SubjectPublicKeyInfo (RFC
// 9578), which differs from Go's default rsaEncryption encoding. TokenKeyID
// returns its SHA-256:
//
//	keyDER, _ := privacypass.MarshalPublicKey(issuerPublicKey)
//	keyID, _ := privacypass.TokenKeyID(issuerPublicKey)
//
// # Wire Structures
//
// TokenChallenge, Token, and TokenRequest provide Marshal and Unmarshal for
// the byte layouts in RFC 9578 and RFC 9577. A Token is 354 bytes; a
// TokenRequest is 259 bytes:
//
//	enc, _ := token.Marshal()
//	got, err := privacypass.UnmarshalToken(enc)
package privacypass
