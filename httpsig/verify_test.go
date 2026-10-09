package httpsig

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifyRequest(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	signer, err := NewEd25519Signer("test-key", priv)
	require.NoError(t, err)

	verifier, err := NewEd25519Verifier("test-key", pub)
	require.NoError(t, err)

	resolver := func(_ *http.Request, keyID string, alg Algorithm) (Verifier, error) {
		if keyID == "test-key" && alg == AlgorithmEd25519 {
			return verifier, nil
		}
		return nil, ErrInvalidKey
	}

	t.Run("nil resolver returns error", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)
		err := VerifyRequest(req, VerifyConfig{})
		assert.ErrorIs(t, err, ErrNoResolver)
	})

	t.Run("missing signature headers returns error", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)
		err := VerifyRequest(req, VerifyConfig{Resolver: resolver})
		assert.ErrorIs(t, err, ErrSignatureNotFound)
	})

	t.Run("sign and verify round trip", func(t *testing.T) {
		req := httptest.NewRequest("POST", "https://example.com/api/items", nil)
		req.Host = "example.com"

		err := SignRequest(req, SignConfig{Signer: signer})
		require.NoError(t, err)

		err = VerifyRequest(req, VerifyConfig{Resolver: resolver})
		assert.NoError(t, err)
	})

	t.Run("tampered header fails verification", func(t *testing.T) {
		req := httptest.NewRequest("POST", "https://example.com/api/items", nil)
		req.Host = "example.com"

		err := SignRequest(req, SignConfig{Signer: signer})
		require.NoError(t, err)

		// Tamper with the request after signing.
		req.Host = "attacker.com"

		err = VerifyRequest(req, VerifyConfig{Resolver: resolver})
		assert.ErrorIs(t, err, ErrSignatureInvalid)
	})

	t.Run("tampered method fails verification", func(t *testing.T) {
		req := httptest.NewRequest("POST", "https://example.com/api/items", nil)
		req.Host = "example.com"

		err := SignRequest(req, SignConfig{Signer: signer})
		require.NoError(t, err)

		req.Method = "DELETE"

		err = VerifyRequest(req, VerifyConfig{Resolver: resolver})
		assert.ErrorIs(t, err, ErrSignatureInvalid)
	})

	t.Run("tampered path fails verification", func(t *testing.T) {
		req := httptest.NewRequest("POST", "https://example.com/api/items", nil)
		req.Host = "example.com"

		err := SignRequest(req, SignConfig{Signer: signer})
		require.NoError(t, err)

		req.URL.Path = "/api/admin"

		err = VerifyRequest(req, VerifyConfig{Resolver: resolver})
		assert.ErrorIs(t, err, ErrSignatureInvalid)
	})

	t.Run("custom label verification", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)
		req.Host = "example.com"

		err := SignRequest(req, SignConfig{
			Signer: signer,
			Label:  "my-sig",
		})
		require.NoError(t, err)

		err = VerifyRequest(req, VerifyConfig{
			Resolver: resolver,
			Label:    "my-sig",
		})
		assert.NoError(t, err)
	})

	t.Run("wrong label not found", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)
		req.Host = "example.com"

		err := SignRequest(req, SignConfig{
			Signer: signer,
			Label:  "real-sig",
		})
		require.NoError(t, err)

		err = VerifyRequest(req, VerifyConfig{
			Resolver: resolver,
			Label:    "wrong-sig",
		})
		assert.ErrorIs(t, err, ErrSignatureNotFound)
	})

	t.Run("required components present", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/path", nil)
		req.Host = "example.com"

		err := SignRequest(req, SignConfig{
			Signer:            signer,
			CoveredComponents: []string{"@method", "@authority", "@path"},
		})
		require.NoError(t, err)

		err = VerifyRequest(req, VerifyConfig{
			Resolver:           resolver,
			RequiredComponents: []string{"@method", "@path"},
		})
		assert.NoError(t, err)
	})

	t.Run("required component missing", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/path", nil)
		req.Host = "example.com"

		err := SignRequest(req, SignConfig{
			Signer:            signer,
			CoveredComponents: []string{"@method"},
		})
		require.NoError(t, err)

		err = VerifyRequest(req, VerifyConfig{
			Resolver:           resolver,
			RequiredComponents: []string{"@authority"},
		})
		assert.ErrorIs(t, err, ErrMissingComponent)
	})

	t.Run("expired signature is rejected", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)
		req.Host = "example.com"

		err := SignRequest(req, SignConfig{
			Signer:  signer,
			Expires: time.Now().Add(-1 * time.Minute),
		})
		require.NoError(t, err)

		err = VerifyRequest(req, VerifyConfig{Resolver: resolver})
		assert.ErrorIs(t, err, ErrSignatureExpired)
	})

	t.Run("non-expired signature is accepted", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)
		req.Host = "example.com"

		err := SignRequest(req, SignConfig{
			Signer:  signer,
			Expires: time.Now().Add(5 * time.Minute),
		})
		require.NoError(t, err)

		err = VerifyRequest(req, VerifyConfig{Resolver: resolver})
		assert.NoError(t, err)
	})

	t.Run("signature age within max age", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)
		req.Host = "example.com"

		err := SignRequest(req, SignConfig{Signer: signer})
		require.NoError(t, err)

		err = VerifyRequest(req, VerifyConfig{
			Resolver: resolver,
			MaxAge:   1 * time.Minute,
		})
		assert.NoError(t, err)
	})

	t.Run("max age with missing created returns ErrCreatedRequired", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)
		req.Host = "example.com"

		// Manually craft Signature-Input without created to test the
		// ErrCreatedRequired path (SignRequest always sets created now).
		req.Header.Set("Signature-Input", `sig1=("@method" "@authority" "@path");alg="ed25519";keyid="test-key"`)
		req.Header.Set("Signature", "sig1=:dGVzdA==:")

		err := VerifyRequest(req, VerifyConfig{
			Resolver: resolver,
			MaxAge:   1 * time.Minute,
		})
		assert.ErrorIs(t, err, ErrCreatedRequired)
	})

	t.Run("max age with future created returns ErrSignatureExpired", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)
		req.Host = "example.com"

		err := SignRequest(req, SignConfig{
			Signer:  signer,
			Created: time.Now().Add(1 * time.Hour),
		})
		require.NoError(t, err)

		err = VerifyRequest(req, VerifyConfig{
			Resolver: resolver,
			MaxAge:   1 * time.Minute,
		})
		assert.ErrorIs(t, err, ErrSignatureExpired)
	})

	t.Run("signature age exceeds max age", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)
		req.Host = "example.com"

		err := SignRequest(req, SignConfig{
			Signer:  signer,
			Created: time.Now().Add(-2 * time.Hour),
		})
		require.NoError(t, err)

		err = VerifyRequest(req, VerifyConfig{
			Resolver: resolver,
			MaxAge:   1 * time.Hour,
		})
		assert.ErrorIs(t, err, ErrSignatureExpired)
	})

	t.Run("with content digest verification", func(t *testing.T) {
		body := "request body content"
		req := httptest.NewRequest("POST", "https://example.com/api", strings.NewReader(body))
		req.Host = "example.com"

		err := SignRequest(req, SignConfig{
			Signer:          signer,
			DigestAlgorithm: DigestSHA256,
		})
		require.NoError(t, err)

		err = VerifyRequest(req, VerifyConfig{
			Resolver:      resolver,
			RequireDigest: true,
		})
		assert.NoError(t, err)
	})

	t.Run("tampered body with digest fails", func(t *testing.T) {
		body := "original body"
		req := httptest.NewRequest("POST", "https://example.com/api", strings.NewReader(body))
		req.Host = "example.com"

		err := SignRequest(req, SignConfig{
			Signer:          signer,
			DigestAlgorithm: DigestSHA256,
		})
		require.NoError(t, err)

		// Replace body after signing.
		req.Body = io.NopCloser(strings.NewReader("tampered body"))

		err = VerifyRequest(req, VerifyConfig{
			Resolver:      resolver,
			RequireDigest: true,
		})
		assert.ErrorIs(t, err, ErrDigestMismatch)
	})

	t.Run("require digest but no digest header", func(t *testing.T) {
		req := httptest.NewRequest("POST", "https://example.com/api", strings.NewReader("body"))
		req.Host = "example.com"

		err := SignRequest(req, SignConfig{Signer: signer})
		require.NoError(t, err)

		err = VerifyRequest(req, VerifyConfig{
			Resolver:      resolver,
			RequireDigest: true,
		})
		assert.ErrorIs(t, err, ErrDigestNotFound)
	})

	t.Run("unknown key ID", func(t *testing.T) {
		_, otherPriv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		otherSigner, err := NewEd25519Signer("unknown-key", otherPriv)
		require.NoError(t, err)

		req := httptest.NewRequest("GET", "https://example.com/", nil)
		req.Host = "example.com"

		err = SignRequest(req, SignConfig{Signer: otherSigner})
		require.NoError(t, err)

		err = VerifyRequest(req, VerifyConfig{Resolver: resolver})
		assert.ErrorIs(t, err, ErrInvalidKey)
	})

	t.Run("signature-input present but signature header missing", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)
		req.Host = "example.com"

		err := SignRequest(req, SignConfig{Signer: signer})
		require.NoError(t, err)

		// Remove only the Signature header.
		req.Header.Del("Signature")

		err = VerifyRequest(req, VerifyConfig{Resolver: resolver})
		assert.ErrorIs(t, err, ErrSignatureNotFound)
	})

	t.Run("unknown component in signature fails verification", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)
		req.Host = "example.com"

		// Manually set headers with a signature that references an unknown component.
		req.Header.Set("Signature-Input", `sig1=("@method" "x-nonexistent");alg="ed25519";keyid="test-key"`)
		req.Header.Set("Signature", "sig1=:dGVzdA==:")

		err := VerifyRequest(req, VerifyConfig{Resolver: resolver})
		assert.ErrorIs(t, err, ErrUnknownComponent)
	})

	t.Run("malformed signature value in Signature header", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)
		req.Host = "example.com"

		err := SignRequest(req, SignConfig{Signer: signer})
		require.NoError(t, err)

		// Replace Signature with a malformed value.
		req.Header.Set("Signature", "sig1=notcolonwrapped")

		err = VerifyRequest(req, VerifyConfig{Resolver: resolver})
		assert.ErrorIs(t, err, ErrMalformedHeader)
	})

	t.Run("malformed signature-input header", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)
		req.Header.Set("Signature-Input", "sig1=noparen")
		req.Header.Set("Signature", "sig1=:dGVzdA==:")

		err := VerifyRequest(req, VerifyConfig{Resolver: resolver})
		assert.ErrorIs(t, err, ErrMalformedHeader)
	})
}

func TestFindSignatureInput(t *testing.T) {
	t.Run("first entry when label empty", func(t *testing.T) {
		header := `sig1=("@method" "@authority");created=123;alg="ed25519";keyid="k"`
		label, value, err := findSignatureInput(header, "")
		require.NoError(t, err)
		assert.Equal(t, "sig1", label)
		assert.Contains(t, value, "@method")
	})

	t.Run("specific label", func(t *testing.T) {
		header := `sig1=("@method");alg="ed25519";keyid="k1", sig2=("@path");alg="ed25519";keyid="k2"`
		label, value, err := findSignatureInput(header, "sig2")
		require.NoError(t, err)
		assert.Equal(t, "sig2", label)
		assert.Contains(t, value, "@path")
	})

	t.Run("comma-only separator", func(t *testing.T) {
		header := `sig1=("@method");alg="ed25519";keyid="k1",sig2=("@path");alg="ed25519";keyid="k2"`
		label, value, err := findSignatureInput(header, "sig2")
		require.NoError(t, err)
		assert.Equal(t, "sig2", label)
		assert.Contains(t, value, "@path")
	})

	t.Run("label not found", func(t *testing.T) {
		header := `sig1=("@method");alg="ed25519";keyid="k"`
		_, _, err := findSignatureInput(header, "sig2")
		assert.ErrorIs(t, err, ErrSignatureNotFound)
	})

	t.Run("entry without equals is skipped", func(t *testing.T) {
		header := `malformed, sig1=("@method");alg="ed25519";keyid="k"`
		label, _, err := findSignatureInput(header, "sig1")
		require.NoError(t, err)
		assert.Equal(t, "sig1", label)
	})

	t.Run("malformed empty entries rejected", func(t *testing.T) {
		// Leading empty dictionary members are not valid RFC 9651 and are
		// rejected by the strict sfv parser.
		header := `, , sig1=("@method");alg="ed25519";keyid="k"`
		_, _, err := findSignatureInput(header, "")
		require.ErrorIs(t, err, ErrMalformedHeader)
	})
}

func TestExtractSignatureValue(t *testing.T) {
	t.Run("valid signature", func(t *testing.T) {
		header := `sig1=:dGVzdA==:`
		sig, err := extractSignatureValue(header, "sig1")
		require.NoError(t, err)
		assert.Equal(t, []byte("test"), sig)
	})

	t.Run("comma-only separator", func(t *testing.T) {
		header := `sig1=:dGVzdA==:,sig2=:YWJj:`
		sig, err := extractSignatureValue(header, "sig2")
		require.NoError(t, err)
		assert.Equal(t, []byte("abc"), sig)
	})

	t.Run("label not found", func(t *testing.T) {
		header := `sig1=:dGVzdA==:`
		_, err := extractSignatureValue(header, "sig2")
		assert.ErrorIs(t, err, ErrSignatureNotFound)
	})

	t.Run("malformed value not byte sequence", func(t *testing.T) {
		header := `sig1=notcolonwrapped`
		_, err := extractSignatureValue(header, "sig1")
		assert.ErrorIs(t, err, ErrMalformedHeader)
	})

	t.Run("invalid base64", func(t *testing.T) {
		header := `sig1=:!!!:`
		_, err := extractSignatureValue(header, "sig1")
		assert.ErrorIs(t, err, ErrMalformedHeader)
	})

	t.Run("entry without equals is skipped", func(t *testing.T) {
		header := `malformed, sig1=:dGVzdA==:`
		sig, err := extractSignatureValue(header, "sig1")
		require.NoError(t, err)
		assert.Equal(t, []byte("test"), sig)
	})

	t.Run("malformed empty entries rejected", func(t *testing.T) {
		// Leading empty dictionary members are not valid RFC 9651 and are
		// rejected by the strict sfv parser.
		header := `, , sig1=:dGVzdA==:`
		_, err := extractSignatureValue(header, "sig1")
		require.ErrorIs(t, err, ErrMalformedHeader)
	})
}

func TestVerifySignedParametersAndMultipleLines(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	verifier, err := NewEd25519Verifier("k", pub)
	require.NoError(t, err)
	cfg := VerifyConfig{Label: "sig", Resolver: func(*http.Request, string, Algorithm) (Verifier, error) { return verifier, nil }}
	raw := `("@method");keyid="k";alg="ed25519";extension="signed"`
	base := fmt.Sprintf("\"@method\": GET\n\"@signature-params\": %s", raw)
	sig := ed25519.Sign(priv, []byte(base))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Add("Signature-Input", `other=("@method");alg="ed25519";keyid="k"`)
	req.Header.Add("Signature-Input", fmt.Sprintf("sig=%s", raw))
	req.Header.Add("Signature", "other=:AA==:")
	req.Header.Add("Signature", fmt.Sprintf("sig=:%s:", base64.StdEncoding.EncodeToString(sig)))
	require.NoError(t, VerifyRequest(req, cfg))
	req.Header.Set("Signature-Input", fmt.Sprintf("sig=%s", strings.Replace(raw, "signed", "changed", 1)))
	require.ErrorIs(t, VerifyRequest(req, cfg), ErrSignatureInvalid)
}

// TestWebBotAuthDraftVector checks interoperability against the official
// test vector from draft-meunier-web-bot-auth-architecture-03 (the signature
// scheme used by Cloudflare Web Bot Auth), which signs with the Ed25519 test
// key from RFC 9421 Appendix B.1.4.
func TestWebBotAuthDraftVector(t *testing.T) {
	pubBytes, err := base64.RawURLEncoding.DecodeString("JrQLj5P_89iXES9-vFgrIy29clF9CC_oPPsw3c5D0bs")
	require.NoError(t, err)

	verifier, err := NewEd25519Verifier("poqkLGiymh_W0uP6PZFw-dvez3QJT5SolqXBCW38r0U", ed25519.PublicKey(pubBytes))
	require.NoError(t, err)

	resolver := func(*http.Request, string, Algorithm) (Verifier, error) { return verifier, nil }

	vectors := []struct {
		name      string
		label     string
		sigInput  string
		sigHeader string
		sigAgent  string
	}{
		{
			name:      "A.2.1 Signature-Agent absent",
			label:     "sig1",
			sigInput:  `sig1=("@authority");created=1735689600;keyid="poqkLGiymh_W0uP6PZFw-dvez3QJT5SolqXBCW38r0U";alg="ed25519";expires=1735693200;nonce="mYotfW3CUjI68sbGw6oKd7kyXqPjZEtU8xFPGWFrqOAf5qC6MDe3pys3SWWCudB0MvwslHy32WXUpkR7u0lt/w==";tag="web-bot-auth"`,
			sigHeader: `sig1=:+NA/cssf4Y2bQTMTkyvTGRCaVzp9quyUevdwwMtMOWhhOOZ2T1subBj0BtvdnrpDEuwSAbiTeElXDzHL3WWKCw==:`,
		},
		{
			name:      "A.2.2 Signature-Agent covered",
			label:     "sig2",
			sigInput:  `sig2=("@authority" "signature-agent");created=1735689600;keyid="poqkLGiymh_W0uP6PZFw-dvez3QJT5SolqXBCW38r0U";alg="ed25519";expires=1735693200;nonce="e8N7S2MFd/qrd6T2R3tdfAuuANngKI7LFtKYI/vowzk4lAZYadIX6wW25MwG7DCT9RUKAJ0qVkU0mEeLElW1qg==";tag="web-bot-auth"`,
			sigHeader: `sig2=:jdq0SqOwHdyHr9+r5jw3iYZH6aNGKijYp/EstF4RQTQdi5N5YYKrD+mCT1HA1nZDsi6nJKuHxUi/5Syp3rLWBA==:`,
			sigAgent:  `"https://signature-agent.test"`,
		},
	}

	for _, v := range vectors {
		t.Run(v.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
			req.Host = "example.com"
			req.Header.Set("Signature-Input", v.sigInput)
			req.Header.Set("Signature", v.sigHeader)
			if v.sigAgent != "" {
				req.Header.Set("Signature-Agent", v.sigAgent)
			}

			// The vector's expires timestamp (2025-01-01) is in the past,
			// so the full VerifyRequest path must reject it as expired -
			// which proves the parameters parse correctly.
			assert.ErrorIs(t, VerifyRequest(req, VerifyConfig{Resolver: resolver}), ErrSignatureExpired)

			// Reconstruct the signature base from the wire headers and
			// verify the vector's signature cryptographically.
			_, raw, err := findSignatureInput(v.sigInput, v.label)
			require.NoError(t, err)

			params, err := parseSignatureParams(raw)
			require.NoError(t, err)

			base, serialized, err := buildSignatureBase(sigMessage{req: req}, params)
			require.NoError(t, err)

			// The signed @signature-params must round-trip byte-exactly,
			// including the vector's parameter order.
			assert.Equal(t, v.sigInput, fmt.Sprintf("%s=%s", v.label, serialized))

			sig, err := extractSignatureValue(v.sigHeader, v.label)
			require.NoError(t, err)

			assert.NoError(t, verifier.Verify(base, sig))
		})
	}
}

func TestVerifyResponse(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	signer, err := NewEd25519Signer("resp-key", priv)
	require.NoError(t, err)

	verifier, err := NewEd25519Verifier("resp-key", pub)
	require.NoError(t, err)

	resolver := func(_ *http.Request, keyID string, alg Algorithm) (Verifier, error) {
		if keyID == "resp-key" && alg == AlgorithmEd25519 {
			return verifier, nil
		}
		return nil, ErrInvalidKey
	}

	signedResponse := func(t *testing.T, status int, body string, req *http.Request, cfg SignConfig) *http.Response {
		t.Helper()

		resp := &http.Response{
			StatusCode: status,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
		}
		cfg.Signer = signer
		require.NoError(t, SignResponse(resp, req, cfg))

		return resp
	}

	t.Run("nil resolver returns error", func(t *testing.T) {
		resp := signedResponse(t, http.StatusOK, "", nil, SignConfig{})
		assert.ErrorIs(t, VerifyResponse(resp, nil, VerifyConfig{}), ErrNoResolver)
	})

	t.Run("round trip with req components and digest", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/api/items", nil)
		req.Host = "example.com"

		resp := signedResponse(t, http.StatusOK, `{"ok":true}`, req, SignConfig{
			CoveredComponents: []string{"@status", "@authority;req", "@path;req"},
			DigestAlgorithm:   DigestSHA256,
		})

		err := VerifyResponse(resp, req, VerifyConfig{
			Resolver:           resolver,
			RequiredComponents: []string{"@status", "@authority;req", "content-digest"},
			RequireDigest:      true,
		})
		require.NoError(t, err)
	})

	t.Run("tampered status fails", func(t *testing.T) {
		resp := signedResponse(t, http.StatusOK, "", nil, SignConfig{})
		resp.StatusCode = http.StatusCreated

		err := VerifyResponse(resp, nil, VerifyConfig{Resolver: resolver})
		assert.ErrorIs(t, err, ErrSignatureInvalid)
	})

	t.Run("tampered request component fails", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/api/items", nil)
		req.Host = "example.com"

		resp := signedResponse(t, http.StatusOK, "", req, SignConfig{
			CoveredComponents: []string{"@status", "@path;req"},
		})
		req.URL.Path = "/api/other"

		err := VerifyResponse(resp, req, VerifyConfig{Resolver: resolver})
		assert.ErrorIs(t, err, ErrSignatureInvalid)
	})

	t.Run("tampered body fails digest check", func(t *testing.T) {
		resp := signedResponse(t, http.StatusOK, "original", nil, SignConfig{DigestAlgorithm: DigestSHA256})
		resp.Body = io.NopCloser(strings.NewReader("tampered"))

		err := VerifyResponse(resp, nil, VerifyConfig{Resolver: resolver, RequireDigest: true})
		assert.ErrorIs(t, err, ErrDigestMismatch)
	})

	t.Run("missing signature returns error", func(t *testing.T) {
		resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: http.NoBody}

		err := VerifyResponse(resp, nil, VerifyConfig{Resolver: resolver})
		assert.ErrorIs(t, err, ErrSignatureNotFound)
	})

	t.Run("nil req falls back to resp.Request", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/x", nil)
		req.Host = "example.com"

		resp := signedResponse(t, http.StatusOK, "", req, SignConfig{
			CoveredComponents: []string{"@status", "@path;req"},
		})
		resp.Request = req

		require.NoError(t, VerifyResponse(resp, nil, VerifyConfig{Resolver: resolver}))
	})

	t.Run("req component without request fails", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/x", nil)

		resp := signedResponse(t, http.StatusOK, "", req, SignConfig{
			CoveredComponents: []string{"@status", "@path;req"},
		})

		err := VerifyResponse(resp, nil, VerifyConfig{Resolver: resolver})
		assert.ErrorIs(t, err, ErrInvalidComponent)
	})

	t.Run("missing required component fails", func(t *testing.T) {
		resp := signedResponse(t, http.StatusOK, "", nil, SignConfig{})

		err := VerifyResponse(resp, nil, VerifyConfig{
			Resolver:           resolver,
			RequiredComponents: []string{"@authority;req"},
		})
		assert.ErrorIs(t, err, ErrMissingComponent)
	})
}
