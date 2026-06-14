package e2ee

import (
	"bytes"
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/kasper/mux"
)

func TestGenerateServerKey(t *testing.T) {
	key, err := GenerateServerKey("2026-06", time.Hour, DefaultMaxSkew)
	require.NoError(t, err)
	assert.Equal(t, "2026-06", key.KID)
	assert.NotNil(t, key.Priv)
	assert.Equal(t, []AEAD{AEADAES256GCM, AEADAES128GCM}, key.AEADs)
	assert.True(t, key.validAt(time.Now()))
}

func TestGenerateServerKeyValidation(t *testing.T) {
	_, err := GenerateServerKey("bad kid!", time.Hour, 0)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidKey))

	_, err = GenerateServerKey("ok", time.Hour, -1)
	require.Error(t, err)

	_, err = GenerateServerKey("ok", time.Hour, 0, AEAD("nope"))
	require.Error(t, err)
}

func TestServerKeySetDocumentRoundTrip(t *testing.T) {
	k1, err := GenerateServerKey("2026-06", time.Hour, DefaultMaxSkew, AEADAES256GCM, AEADAES128GCM)
	require.NoError(t, err)

	set, err := NewServerKeySet("https://api.example.com", k1)
	require.NoError(t, err)

	doc := set.Document()
	assert.Equal(t, "https://api.example.com", doc.Issuer)
	require.Len(t, doc.Keys, 1)
	assert.Equal(t, "X25519", doc.Keys[0].Alg)
	assert.NotEmpty(t, doc.Keys[0].PublicKey)
	assert.NotEmpty(t, doc.Keys[0].Fingerprint)

	// Render handler output and parse it as a client.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, WellKnownPath, nil)
	set.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	cs, err := ParseKeySet(rec.Body)
	require.NoError(t, err)
	assert.Equal(t, "https://api.example.com", cs.Issuer())

	key, aead, err := cs.selectKey(time.Now(), AEADAES128GCM)
	require.NoError(t, err)
	assert.Equal(t, "2026-06", key.KID)
	assert.Equal(t, AEADAES128GCM, aead)
}

func TestServerKeySetValidation(t *testing.T) {
	k, _ := GenerateServerKey("k", time.Hour, 0)

	_, err := NewServerKeySet("", k)
	require.Error(t, err)

	_, err = NewServerKeySet("https://x")
	require.ErrorIs(t, err, ErrNoKeys)

	_, err = NewServerKeySet("https://x", k, k)
	require.Error(t, err, "duplicate kid rejected")
}

func TestHandlerMethodNotAllowed(t *testing.T) {
	k, _ := GenerateServerKey("k", time.Hour, 0)
	set, _ := NewServerKeySet("https://x", k)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, WellKnownPath, nil)
	set.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	assert.Equal(t, "GET, HEAD", rec.Header().Get("Allow"))
}

func TestHandlerHead(t *testing.T) {
	k, _ := GenerateServerKey("k", time.Hour, 0)
	set, _ := NewServerKeySet("https://x", k)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodHead, WellKnownPath, nil)
	set.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Body.Bytes())
}

func TestParseKeySetErrors(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"invalid json", `{`},
		{"unknown field", `{"issuer":"x","keys":[],"extra":1}`},
		{"empty issuer", `{"issuer":"","keys":[{"kid":"k","alg":"X25519","aeads":["AES-128-GCM"],"public_key":"AA","not_after":"2030-01-01T00:00:00Z","max_skew":300}]}`},
		{"no keys", `{"issuer":"x","keys":[]}`},
		{"bad alg", `{"issuer":"x","keys":[{"kid":"k","alg":"RSA","aeads":["AES-128-GCM"],"public_key":"AAAA","not_after":"2030-01-01T00:00:00Z","max_skew":300}]}`},
		{"bad kid", `{"issuer":"x","keys":[{"kid":"bad kid","alg":"X25519","aeads":["AES-128-GCM"],"public_key":"AAAA","not_after":"2030-01-01T00:00:00Z","max_skew":300}]}`},
		{"empty aeads", `{"issuer":"x","keys":[{"kid":"k","alg":"X25519","aeads":[],"public_key":"AAAA","not_after":"2030-01-01T00:00:00Z","max_skew":300}]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseKeySet(strings.NewReader(tt.body))
			require.Error(t, err)
		})
	}
}

func TestPublicKeyValidAt(t *testing.T) {
	nb := time.Unix(1000, 0)
	k := PublicKey{
		NotBefore: &nb,
		NotAfter:  time.Unix(2000, 0),
		MaxSkew:   10,
	}

	assert.False(t, k.validAt(time.Unix(980, 0)), "before not_before-skew")
	assert.True(t, k.validAt(time.Unix(995, 0)), "within skew of not_before")
	assert.True(t, k.validAt(time.Unix(1500, 0)))
	assert.True(t, k.validAt(time.Unix(2005, 0)), "within skew of not_after")
	assert.False(t, k.validAt(time.Unix(2020, 0)), "after not_after+skew")
}

func TestSelectKeySkipsExpired(t *testing.T) {
	expired, _ := GenerateServerKey("old", -time.Hour, 0)
	valid, _ := GenerateServerKey("new", time.Hour, 0)

	set, _ := NewServerKeySet("https://x", expired, valid)
	cs, err := ParseKeySet(bytes.NewReader(mustJSON(t, set.Document())))
	require.NoError(t, err)

	key, _, err := cs.selectKey(time.Now(), "")
	require.NoError(t, err)
	assert.Equal(t, "new", key.KID)
}

func TestSelectKeyPreferredUnavailable(t *testing.T) {
	k, _ := GenerateServerKey("k", time.Hour, 0, AEADAES128GCM)
	set, _ := NewServerKeySet("https://x", k)
	cs, _ := ParseKeySet(bytes.NewReader(mustJSON(t, set.Document())))

	_, _, err := cs.selectKey(time.Now(), AEADAES256GCM)
	require.ErrorIs(t, err, ErrNoKeys)
}

func TestFetchKeySet(t *testing.T) {
	k, _ := GenerateServerKey("k", time.Hour, DefaultMaxSkew)

	var issuer string
	srv := httptest.NewServer(nil)
	defer srv.Close()

	issuer = srv.URL
	set, _ := NewServerKeySet(issuer, k)
	srv.Config.Handler = set.Handler()

	cs, err := FetchKeySet(srv.Client(), issuer)
	require.NoError(t, err)
	assert.Equal(t, issuer, cs.Issuer())
}

func TestFetchKeySetIssuerMismatch(t *testing.T) {
	k, _ := GenerateServerKey("k", time.Hour, DefaultMaxSkew)
	set, _ := NewServerKeySet("https://wrong.example", k)

	srv := httptest.NewServer(set.Handler())
	defer srv.Close()

	_, err := FetchKeySet(srv.Client(), srv.URL)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrMalformed))
}

func mustJSON(t *testing.T, doc KeySetDocument) []byte {
	t.Helper()

	body, err := json.Marshal(doc)
	require.NoError(t, err)

	return body
}

// mustField serializes a sessionItem, failing the test on error.
func mustField(t *testing.T, item sessionItem) string {
	t.Helper()

	field, err := item.serialize()
	require.NoError(t, err)

	return field
}

// base64Std encodes b with standard base64 (used to hand-build sf-binary
// values in tests).
func base64Std(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

// newStatusServer starts a test server that always responds with status.
func newStatusServer(t *testing.T, status int) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	}))

	t.Cleanup(srv.Close)

	return srv
}

// newBodyServer starts a test server that responds with status and body.
func newBodyServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))

	t.Cleanup(srv.Close)

	return srv
}

func TestDecodeBadBase64PublicKey(t *testing.T) {
	k := PublicKey{
		KID:       "k",
		Alg:       "X25519",
		AEADs:     []AEAD{AEADAES128GCM},
		PublicKey: "!!!not-base64!!!",
		NotAfter:  time.Now().Add(time.Hour),
		MaxSkew:   300,
	}

	_, err := k.decode()
	require.ErrorIs(t, err, ErrMalformed)
}

func TestDecodeWrongKeyLength(t *testing.T) {
	k := PublicKey{
		KID:       "k",
		Alg:       "X25519",
		AEADs:     []AEAD{AEADAES128GCM},
		PublicKey: "AAAA", // valid base64 but wrong length
		NotAfter:  time.Now().Add(time.Hour),
		MaxSkew:   300,
	}

	_, err := k.decode()
	require.ErrorIs(t, err, ErrInvalidKey)
}

func TestDecodeInvalidAEADInList(t *testing.T) {
	good, err := GenerateServerKey("k", time.Hour, 300)
	require.NoError(t, err)

	entry := good.public()
	entry.AEADs = []AEAD{AEADAES128GCM, AEAD("bogus")}

	_, err = entry.decode()
	require.ErrorIs(t, err, ErrMalformed)
}

func TestDecodeNegativeMaxSkew(t *testing.T) {
	good, err := GenerateServerKey("k", time.Hour, 300)
	require.NoError(t, err)

	entry := good.public()
	entry.MaxSkew = -1

	_, err = entry.decode()
	require.ErrorIs(t, err, ErrMalformed)
}

func TestNewServerKeySetNilKey(t *testing.T) {
	_, err := NewServerKeySet("https://x", nil)
	require.ErrorIs(t, err, ErrInvalidKey)

	_, err = NewServerKeySet("https://x", &PrivateKey{KID: "k"})
	require.ErrorIs(t, err, ErrInvalidKey)
}

func TestNewClientKeySetDuplicateKID(t *testing.T) {
	good, _ := GenerateServerKey("dup", time.Hour, 300)
	entry := good.public()

	doc := KeySetDocument{
		Issuer: "https://x",
		Keys:   []PublicKey{entry, entry},
	}

	_, err := newClientKeySet(doc)
	require.ErrorIs(t, err, ErrMalformed)
}

func TestPrivateKeyValidAtNotBefore(t *testing.T) {
	nb := time.Unix(1000, 0)
	k := &PrivateKey{
		KID:       "k",
		NotBefore: &nb,
		NotAfter:  time.Unix(2000, 0),
		MaxSkew:   10,
	}

	assert.False(t, k.validAt(time.Unix(980, 0)))
	assert.True(t, k.validAt(time.Unix(1500, 0)))
}

func TestFetchKeySetBadURL(t *testing.T) {
	_, err := FetchKeySet(http.DefaultClient, "://bad-url")
	require.Error(t, err)
}

func TestFetchKeySetTransportError(t *testing.T) {
	// A client pointing at an unroutable address surfaces a transport error.
	client := &http.Client{Timeout: 100 * time.Millisecond}

	_, err := FetchKeySet(client, "http://127.0.0.1:1")
	require.Error(t, err)
}

func TestFetchKeySetNon200(t *testing.T) {
	srv := newStatusServer(t, http.StatusNotFound)

	_, err := FetchKeySet(srv.Client(), srv.URL)
	require.ErrorIs(t, err, ErrMalformed)
}

func TestFetchKeySetMalformedBody(t *testing.T) {
	srv := newBodyServer(t, http.StatusOK, "{not valid json")

	_, err := FetchKeySet(srv.Client(), srv.URL)
	require.ErrorIs(t, err, ErrMalformed)
}

func TestGenerateServerKeyKeyGenError(t *testing.T) {
	prev := x25519GenerateKey
	x25519GenerateKey = func(io.Reader) (*ecdh.PrivateKey, error) { return nil, forcedErr() }
	t.Cleanup(func() { x25519GenerateKey = prev })

	_, err := GenerateServerKey("k", time.Hour, DefaultMaxSkew)
	require.ErrorIs(t, err, ErrInvalidKey)
}

func TestHandlerGetHeaders(t *testing.T) {
	key, _ := GenerateServerKey("k", time.Hour, DefaultMaxSkew)
	set, _ := NewServerKeySet("https://x", key)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, WellKnownPath, nil)
	set.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, mux.ContentTypeApplicationJSON, rec.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache", rec.Header().Get("Cache-Control"))

	cs, err := ParseKeySet(rec.Body)
	require.NoError(t, err)
	assert.Equal(t, "https://x", cs.Issuer())
}

func TestNewServerKeyFromBytes(t *testing.T) {
	gen, err := GenerateServerKey("k", time.Hour, DefaultMaxSkew)
	require.NoError(t, err)

	raw := gen.Priv.Bytes()

	loaded, err := NewServerKey("k", raw, time.Hour, DefaultMaxSkew)
	require.NoError(t, err)

	assert.Equal(t, gen.Priv.PublicKey().Bytes(), loaded.Priv.PublicKey().Bytes())
}

func TestNewServerKeyInvalid(t *testing.T) {
	_, err := NewServerKey("k", []byte{1, 2, 3}, time.Hour, DefaultMaxSkew)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidKey)

	_, err = NewServerKey("bad kid", make([]byte, 32), time.Hour, DefaultMaxSkew)
	require.Error(t, err)
}

func TestServerKeySetIssuer(t *testing.T) {
	k, _ := GenerateServerKey("k", time.Hour, DefaultMaxSkew)
	set, _ := NewServerKeySet("https://api.example.com", k)
	assert.Equal(t, "https://api.example.com", set.Issuer())
}
