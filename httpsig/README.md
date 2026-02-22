# httpsig

HTTP Message Signatures (RFC 9421) for Go with optional Content-Digest (RFC 9530) support.

```bash
go get github.com/vitalvas/kasper/httpsig
```

## Algorithms

| Algorithm | Constant | Key Type |
|-----------|----------|----------|
| Ed25519 | `AlgorithmEd25519` | `ed25519.PrivateKey` / `ed25519.PublicKey` |
| ECDSA P-256 SHA-256 | `AlgorithmECDSAP256SHA256` | `*ecdsa.PrivateKey` / `*ecdsa.PublicKey` (P-256) |
| ECDSA P-384 SHA-384 | `AlgorithmECDSAP384SHA384` | `*ecdsa.PrivateKey` / `*ecdsa.PublicKey` (P-384) |
| RSA-PSS SHA-512 | `AlgorithmRSAPSSSHA512` | `*rsa.PrivateKey` / `*rsa.PublicKey` (2048+ bits) |
| RSA v1.5 SHA-256 | `AlgorithmRSAv15SHA256` | `*rsa.PrivateKey` / `*rsa.PublicKey` (2048+ bits) |
| HMAC SHA-256 | `AlgorithmHMACSHA256` | `[]byte` (32+ bytes) |

## Creating Keys

Each algorithm has a `NewXxxSigner` / `NewXxxVerifier` constructor pair.
Key material is validated at construction time (nil check, curve check,
minimum key size).

```go
// Ed25519
pub, priv, _ := ed25519.GenerateKey(rand.Reader)
signer, err := httpsig.NewEd25519Signer("my-key-id", priv)
verifier, err := httpsig.NewEd25519Verifier("my-key-id", pub)

// ECDSA P-256
ecKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
signer, err := httpsig.NewECDSAP256Signer("ec-key", ecKey)
verifier, err := httpsig.NewECDSAP256Verifier("ec-key", &ecKey.PublicKey)

// ECDSA P-384
ecKey384, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
signer, err := httpsig.NewECDSAP384Signer("ec384-key", ecKey384)
verifier, err := httpsig.NewECDSAP384Verifier("ec384-key", &ecKey384.PublicKey)

// RSA-PSS (2048+ bits)
rsaKey, _ := rsa.GenerateKey(rand.Reader, 2048)
signer, err := httpsig.NewRSAPSSSigner("rsa-key", rsaKey)
verifier, err := httpsig.NewRSAPSSVerifier("rsa-key", &rsaKey.PublicKey)

// RSA v1.5 (2048+ bits)
signer, err := httpsig.NewRSAv15Signer("rsa15-key", rsaKey)
verifier, err := httpsig.NewRSAv15Verifier("rsa15-key", &rsaKey.PublicKey)

// HMAC-SHA256 (32+ byte key)
hmacKey := make([]byte, 32)
rand.Read(hmacKey)
signer, err := httpsig.NewHMACSHA256Signer("hmac-key", hmacKey)
verifier, err := httpsig.NewHMACSHA256Verifier("hmac-key", hmacKey)
```

## Signing Requests

`SignRequest` adds `Signature` and `Signature-Input` headers to an HTTP
request in-place.

```go
signer, err := httpsig.NewEd25519Signer("my-key-id", privateKey)
if err != nil {
    log.Fatal(err)
}

req, _ := http.NewRequest(http.MethodPost, "https://api.example.com/orders", body)

err = httpsig.SignRequest(req, httpsig.SignConfig{
    Signer: signer,
})
if err != nil {
    log.Fatal(err)
}
// req now has Signature and Signature-Input headers.
```

### SignConfig Options

All fields except `Signer` are optional.

```go
err := httpsig.SignRequest(req, httpsig.SignConfig{
    // Required: produces signatures.
    Signer: signer,

    // Signature label in Signature/Signature-Input headers.
    // Default: "sig1".
    Label: "my-sig",

    // Component identifiers to include in the signature base.
    // Default: [ComponentMethod, ComponentAuthority, ComponentPath].
    CoveredComponents: []string{
        httpsig.ComponentMethod,
        httpsig.ComponentAuthority,
        httpsig.ComponentPath,
        httpsig.ComponentQuery,
        "content-type",
    },

    // Optional nonce for replay protection.
    Nonce: "unique-request-id",

    // Optional application-specific tag.
    Tag: "my-app",

    // Signature creation time. Zero value = time.Now().
    // Created: time.Unix(1700000000, 0),

    // Signature expiration time. Zero value = no expiration.
    Expires: time.Now().Add(5 * time.Minute),

    // When set, computes Content-Digest (RFC 9530) and adds
    // "content-digest" to covered components automatically.
    DigestAlgorithm: httpsig.DigestSHA256,
})
```

## Verifying Requests

`VerifyRequest` checks the `Signature` and `Signature-Input` headers on an
incoming request. A `KeyResolver` function looks up the verifier for each
key ID and algorithm.

```go
// Key resolver maps key IDs to verifiers. This is where you load
// public keys from a database, file, or in-memory store.
resolver := func(r *http.Request, keyID string, alg httpsig.Algorithm) (httpsig.Verifier, error) {
    switch keyID {
    case "client-a":
        return httpsig.NewEd25519Verifier(keyID, clientAPublicKey)
    case "client-b":
        return httpsig.NewECDSAP256Verifier(keyID, clientBPublicKey)
    default:
        return nil, fmt.Errorf("unknown key: %s", keyID)
    }
}

err := httpsig.VerifyRequest(req, httpsig.VerifyConfig{
    // Required: looks up the verifier for a given key ID and algorithm.
    Resolver: resolver,

    // When empty, the first signature found is verified.
    // Set to verify a specific signature label.
    Label: "sig1",

    // Verification fails if any of these components are missing
    // from the signature's covered components.
    RequiredComponents: []string{
        httpsig.ComponentMethod, httpsig.ComponentAuthority, httpsig.ComponentPath,
    },

    // Reject signatures older than this duration. Zero = no age check.
    MaxAge: 5 * time.Minute,

    // When true, also verifies the Content-Digest header against
    // the request body before checking the signature.
    RequireDigest: true,
})
if err != nil {
    // Handle verification failure.
    // Possible errors: ErrSignatureNotFound, ErrSignatureInvalid,
    // ErrSignatureExpired, ErrMissingComponent, ErrMalformedHeader,
    // ErrDigestMismatch, ErrDigestNotFound.
    log.Printf("signature verification failed: %v", err)
}
```

## Client Transport

`NewTransport` creates an `http.RoundTripper` that automatically signs
every outgoing request. It accepts an `*http.Transport` for full control
over proxy, TLS, timeouts, and connection pooling. Pass nil for sensible
defaults (cloned from `http.DefaultTransport`). The original request is
cloned before signing, so it is never mutated.

Basic usage with defaults:

```go
signer, err := httpsig.NewEd25519Signer("my-key-id", privateKey)
if err != nil {
    log.Fatal(err)
}

client := &http.Client{
    Transport: httpsig.NewTransport(nil, httpsig.SignConfig{
        Signer: signer,
        CoveredComponents: []string{
            httpsig.ComponentMethod, httpsig.ComponentAuthority,
            httpsig.ComponentPath, "content-type",
        },
        DigestAlgorithm: httpsig.DigestSHA256,
    }),
}

resp, err := client.Post(
    "https://api.example.com/orders",
    "application/json",
    strings.NewReader(`{"item": "widget", "qty": 1}`),
)
if err != nil {
    log.Fatal(err)
}
defer resp.Body.Close()
```

With HTTP/SOCKS proxy, custom TLS, and timeouts:

```go
base := &http.Transport{
    // HTTP proxy from environment (HTTP_PROXY, HTTPS_PROXY, NO_PROXY).
    Proxy: http.ProxyFromEnvironment,

    // Or use a fixed SOCKS5 proxy:
    // Proxy: func(*http.Request) (*url.URL, error) {
    //     return url.Parse("socks5://proxy.example.com:1080")
    // },

    TLSClientConfig: &tls.Config{
        MinVersion: tls.VersionTLS13,
    },
    TLSHandshakeTimeout:   10 * time.Second,
    IdleConnTimeout:        90 * time.Second,
    ResponseHeaderTimeout:  30 * time.Second,
    MaxIdleConns:           100,
    MaxIdleConnsPerHost:    10,
    DisableCompression:     false,
}

client := &http.Client{
    Transport: httpsig.NewTransport(base, httpsig.SignConfig{
        Signer: signer,
    }),
}
```

## Server Middleware

`Middleware` returns a `mux.MiddlewareFunc` for the kasper/mux router.
Unsigned or invalid requests receive a 401 Unauthorized response by
default.

```go
resolver := func(r *http.Request, keyID string, alg httpsig.Algorithm) (httpsig.Verifier, error) {
    // Look up verifier by key ID.
    v, ok := verifiers[keyID]
    if !ok {
        return nil, fmt.Errorf("unknown key: %s", keyID)
    }
    return v, nil
}

mw, err := httpsig.Middleware(httpsig.MiddlewareConfig{
    Verify: httpsig.VerifyConfig{
        Resolver:           resolver,
        RequiredComponents: []string{
                httpsig.ComponentMethod, httpsig.ComponentAuthority, httpsig.ComponentPath,
            },
        MaxAge:             5 * time.Minute,
        RequireDigest:      true,
    },

    // Optional custom error handler. Default: 401 with no body.
    OnError: func(w http.ResponseWriter, r *http.Request, err error) {
        http.Error(w, "signature verification failed", http.StatusUnauthorized)
    },
})
if err != nil {
    log.Fatal(err)
}

router := mux.NewRouter()
router.Use(mw)
router.HandleFunc("/api/v1/orders", handleOrders).Methods(http.MethodPost)
```

## Content-Digest (RFC 9530)

Standalone Content-Digest generation and verification, independent of
message signatures. Supports SHA-256 (`DigestSHA256`) and SHA-512
(`DigestSHA512`).

```go
// Generate: reads the body, sets the Content-Digest header,
// and restores the body for downstream readers.
req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
err := httpsig.SetContentDigest(req, httpsig.DigestSHA256)
// req.Header.Get("Content-Digest") is now "sha-256=:<base64>:"

// Verify: reads the body, checks the Content-Digest header,
// and restores the body.
err = httpsig.VerifyContentDigest(req)
// Returns ErrDigestMismatch if the body was tampered with.
// Returns ErrDigestNotFound if the header is missing.
// Returns ErrUnsupportedDigest if the algorithm is not recognized.
```

When used with `SignRequest`, setting `DigestAlgorithm` computes the digest
and adds `"content-digest"` to the covered components automatically:

```go
err := httpsig.SignRequest(req, httpsig.SignConfig{
    Signer:          signer,
    DigestAlgorithm: httpsig.DigestSHA256,
})
// Both Content-Digest and Signature headers are set.
// The signature covers the digest, binding body integrity to the signature.
```

## End-to-End Example

A complete client-server example using `NewTransport` and `Middleware` together.

```go
package main

import (
    "crypto/ed25519"
    "crypto/rand"
    "fmt"
    "log"
    "net/http"
    "strings"
    "time"

    "github.com/vitalvas/kasper/httpsig"
    "github.com/vitalvas/kasper/mux"
)

func main() {
    // Generate a key pair.
    pub, priv, err := ed25519.GenerateKey(rand.Reader)
    if err != nil {
        log.Fatal(err)
    }

    signer, err := httpsig.NewEd25519Signer("app-key", priv)
    if err != nil {
        log.Fatal(err)
    }

    verifier, err := httpsig.NewEd25519Verifier("app-key", pub)
    if err != nil {
        log.Fatal(err)
    }

    // Server: create a router with signature verification middleware.
    resolver := func(r *http.Request, keyID string, alg httpsig.Algorithm) (httpsig.Verifier, error) {
        if keyID == "app-key" && alg == httpsig.AlgorithmEd25519 {
            return verifier, nil
        }
        return nil, fmt.Errorf("unknown key: %s", keyID)
    }

    mw, err := httpsig.Middleware(httpsig.MiddlewareConfig{
        Verify: httpsig.VerifyConfig{
            Resolver:           resolver,
            RequiredComponents: []string{
                httpsig.ComponentMethod, httpsig.ComponentAuthority, httpsig.ComponentPath,
            },
            MaxAge:             5 * time.Minute,
            RequireDigest:      true,
        },
    })
    if err != nil {
        log.Fatal(err)
    }

    router := mux.NewRouter()
    router.Use(mw)
    router.HandleFunc("/api/v1/orders", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusCreated)
        w.Write([]byte(`{"status":"created"}`))
    }).Methods(http.MethodPost)

    server := &http.Server{Addr: ":8080", Handler: router}
    go server.ListenAndServe()

    // Client: create an HTTP client with automatic signing.
    client := &http.Client{
        Transport: httpsig.NewTransport(nil, httpsig.SignConfig{
            Signer:          signer,
            DigestAlgorithm: httpsig.DigestSHA256,
        }),
    }

    resp, err := client.Post(
        "http://localhost:8080/api/v1/orders",
        "application/json",
        strings.NewReader(`{"item":"widget"}`),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer resp.Body.Close()

    fmt.Println("Status:", resp.StatusCode) // Status: 201
}
```

## Covered Components

Derived components (prefixed with `@`):

| Constant | Value | Description |
|----------|-------|-------------|
| `ComponentMethod` | `@method` | HTTP method (e.g., `GET`, `POST`) |
| `ComponentAuthority` | `@authority` | Host header, lowercased (e.g., `example.com:8080`) |
| `ComponentPath` | `@path` | Request path (e.g., `/api/v1/orders`) |
| `ComponentQuery` | `@query` | Query string with `?` prefix (e.g., `?page=2&sort=name`) |
| `ComponentScheme` | `@scheme` | Request scheme (`http` or `https`) |
| `ComponentTargetURI` | `@target-uri` | Full target URI (e.g., `https://example.com/path?q=1`) |
| `ComponentRequestTarget` | `@request-target` | Path and optional query (e.g., `/path?q=1`) |

Header fields: any HTTP header name, lowercased (e.g., `content-type`,
`content-digest`, `authorization`). Multi-value headers are joined with
`, `.

Default covered components when none specified: `ComponentMethod`,
`ComponentAuthority`, `ComponentPath`.

## Errors

| Error | Description |
|-------|-------------|
| `ErrNoSigner` | `SignConfig.Signer` is nil |
| `ErrNoCoveredComponents` | `SignConfig.CoveredComponents` is empty after defaults |
| `ErrNoResolver` | `VerifyConfig.Resolver` is nil |
| `ErrSignatureNotFound` | Signature label not found in headers |
| `ErrSignatureInvalid` | Cryptographic verification failed |
| `ErrSignatureExpired` | Signature age exceeds `MaxAge` |
| `ErrMissingComponent` | Required component absent from signature |
| `ErrMalformedHeader` | Cannot parse Signature or Signature-Input headers |
| `ErrInvalidKey` | Key material is nil, wrong curve, or too small |
| `ErrDigestMismatch` | Content-Digest does not match body |
| `ErrDigestNotFound` | Content-Digest header missing when required |
| `ErrUnsupportedDigest` | Digest algorithm not recognized |
| `ErrUnknownComponent` | Unrecognized component identifier |

## Standards

- [RFC 9421](https://www.rfc-editor.org/rfc/rfc9421) - HTTP Message Signatures
- [RFC 9530](https://www.rfc-editor.org/rfc/rfc9530) - Digest Fields
- [RFC 8941](https://www.rfc-editor.org/rfc/rfc8941) - Structured Field Values (subset)
