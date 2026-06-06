package e2ee

import (
	"bytes"
	"crypto/ecdh"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSessionUsesFreshKey(t *testing.T) {
	s1, err := NewSession()
	require.NoError(t, err)

	s2, err := NewSession()
	require.NoError(t, err)

	assert.NotEqual(t, s1.priv.Bytes(), s2.priv.Bytes())
}

func TestNewSessionKeyGenError(t *testing.T) {
	prev := x25519GenerateKey
	x25519GenerateKey = func(io.Reader) (*ecdh.PrivateKey, error) { return nil, forcedErr() }
	t.Cleanup(func() { x25519GenerateKey = prev })

	_, err := NewSession()
	require.ErrorIs(t, err, ErrInvalidKey)
}

func TestEncryptRequestNoKeySet(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "https://x/api", strings.NewReader("x"))

	_, err := EncryptRequest(req, ClientConfig{})
	require.ErrorIs(t, err, ErrNoKeySet)
}

func TestEncryptRequestNoUsableKey(t *testing.T) {
	// A key set with only an expired key yields ErrNoKeys at selection.
	expired, _ := GenerateServerKey("old", -time.Hour, 0)
	set, _ := NewServerKeySet("https://x", expired)
	cs, _ := ParseKeySet(strings.NewReader(string(mustJSON(t, set.Document()))))

	cfg := ClientConfig{KeySet: cs}
	_, _, err := cfg.encryptRequest([]byte("x"))
	require.ErrorIs(t, err, ErrNoKeys)
}

func TestEncryptRequestKeyGenError(t *testing.T) {
	cs := clientKeySetFor(t)

	prev := x25519GenerateKey
	x25519GenerateKey = func(io.Reader) (*ecdh.PrivateKey, error) { return nil, forcedErr() }
	t.Cleanup(func() { x25519GenerateKey = prev })

	cfg := ClientConfig{KeySet: cs}
	_, _, err := cfg.encryptRequest([]byte("x"))
	require.ErrorIs(t, err, ErrInvalidKey)
}

func TestEncryptRequestDeriveError(t *testing.T) {
	cs := clientKeySetFor(t)

	prev := hkdfExtract
	hkdfExtract = func([]byte, []byte) ([]byte, error) { return nil, forcedErr() }
	t.Cleanup(func() { hkdfExtract = prev })

	cfg := ClientConfig{KeySet: cs}
	_, _, err := cfg.encryptRequest([]byte("x"))
	require.ErrorIs(t, err, ErrInvalidKey)
}

func TestEncryptRequestSerializeError(t *testing.T) {
	cs := clientKeySetFor(t)

	prev := serializeSession
	serializeSession = func(sessionItem) (string, error) { return "", forcedErr() }
	t.Cleanup(func() { serializeSession = prev })

	cfg := ClientConfig{KeySet: cs}
	_, _, err := cfg.encryptRequest([]byte("x"))
	require.Error(t, err)
}

func TestEncryptRequestSealFailure(t *testing.T) {
	cs := clientKeySetFor(t)

	withFailingGCM(t)

	cfg := ClientConfig{KeySet: cs}
	_, _, err := cfg.encryptRequest([]byte("x"))
	require.Error(t, err)
}

func TestEncryptRequestNonceFailure(t *testing.T) {
	cs := clientKeySetFor(t)

	cfg := ClientConfig{KeySet: cs, randReader: errReader{}}
	_, _, err := cfg.encryptRequest([]byte("x"))
	require.Error(t, err)
}

func TestEncryptRequestBodyReadError(t *testing.T) {
	cs := clientKeySetFor(t)

	req, _ := http.NewRequest(http.MethodPost, "https://x/api", errReadCloser{})

	_, err := EncryptRequest(req, ClientConfig{KeySet: cs})
	require.Error(t, err)
}

func TestDecryptResponseMissingHeader(t *testing.T) {
	d := &ResponseDecryptor{state: &clientState{}}

	resp := &http.Response{Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}

	err := d.Decrypt(resp)
	require.ErrorIs(t, err, ErrMalformed)
}

func TestDecryptResponseBodyReadError(t *testing.T) {
	d := &ResponseDecryptor{cfg: ClientConfig{}, state: &clientState{}}

	resp := &http.Response{Header: make(http.Header), Body: errReadCloser{}}
	resp.Header.Set(SessionHeader, `"k";aead="AES-256-GCM";ts=1;nid="n"`)

	err := d.Decrypt(resp)
	require.Error(t, err)
}

func TestDecryptResponseMalformedField(t *testing.T) {
	d := &ResponseDecryptor{cfg: ClientConfig{}, state: &clientState{}}

	resp := &http.Response{Header: make(http.Header), Body: io.NopCloser(strings.NewReader("body"))}
	resp.Header.Set(SessionHeader, "not-valid")

	err := d.Decrypt(resp)
	require.ErrorIs(t, err, ErrMalformed)
}

func TestDecryptResponseEPKProhibited(t *testing.T) {
	cc := &ClientConfig{}
	st := &clientState{ekRes: make([]byte, 32), reqField: "x", aead: AEADAES256GCM, kid: "k", nid: "n"}

	field := mustField(t, sessionItem{
		kid:    "k",
		aead:   "AES-256-GCM",
		epk:    make([]byte, keySize),
		hasEPK: true,
		ts:     1,
		nid:    "n",
	})

	_, _, err := cc.decryptResponse(field, make([]byte, minBodySize), st)
	require.ErrorIs(t, err, ErrMalformed)
}

func TestDecryptResponseEchoMismatch(t *testing.T) {
	cc := &ClientConfig{}
	st := &clientState{ekRes: make([]byte, 32), reqField: "x", aead: AEADAES256GCM, kid: "k", nid: "n"}

	// Response echoes a different nid.
	bad := sessionItem{kid: "k", aead: string(AEADAES256GCM), ts: 1, nid: "other"}
	field, err := bad.serialize()
	require.NoError(t, err)

	_, _, err = cc.decryptResponse(field, make([]byte, minBodySize), st)
	require.ErrorIs(t, err, ErrMalformed)
}

func TestDecryptResponseOpenFailureViaGCM(t *testing.T) {
	set, cs := clientServerPair(t)

	cc := ClientConfig{KeySet: cs}
	sc := ServerConfig{KeySet: set, Replay: NewMemoryReplayCache()}

	body, cstate, err := cc.encryptRequest([]byte("ping"))
	require.NoError(t, err)

	_, sstate, err := sc.decryptRequest(cstate.reqField, body)
	require.NoError(t, err)

	resField, resBody, err := sc.encryptResponse([]byte("pong"), sstate)
	require.NoError(t, err)

	withFailingGCM(t)

	_, _, err = cc.decryptResponse(resField, resBody, cstate)
	require.Error(t, err)
}

func TestClientConfigEncryptDecryptUnit(t *testing.T) {
	key := serverKeyForKID(t, "kid")
	set, err := NewServerKeySet("https://api.example.com", key)
	require.NoError(t, err)

	cs, err := ParseKeySet(strings.NewReader(string(mustJSON(t, set.Document()))))
	require.NoError(t, err)

	cc := &ClientConfig{KeySet: cs, ContentType: "application/json"}
	sc := &ServerConfig{KeySet: set, Replay: NewMemoryReplayCache()}

	body, cstate, err := cc.encryptRequest([]byte("ping"))
	require.NoError(t, err)

	plaintext, sstate, err := sc.decryptRequest(cstate.reqField, body)
	require.NoError(t, err)
	assert.Equal(t, "ping", string(plaintext))
	assert.Equal(t, "application/json", sstate.reqCTY)

	resField, resBody, err := sc.encryptResponse([]byte("pong"), sstate)
	require.NoError(t, err)

	respPlain, cty, err := cc.decryptResponse(resField, resBody, cstate)
	require.NoError(t, err)
	assert.Equal(t, "pong", string(respPlain))
	assert.Equal(t, "application/json", cty)
}

func TestDecryptResponseClearsContentType(t *testing.T) {
	// With no inner cty, a successful decrypt clears the response Content-Type.
	set, cs := clientServerPair(t)

	cc := ClientConfig{KeySet: cs} // no ContentType
	sc := ServerConfig{KeySet: set, Replay: NewMemoryReplayCache()}

	body, cstate, err := cc.encryptRequest([]byte("ping"))
	require.NoError(t, err)

	_, sstate, err := sc.decryptRequest(cstate.reqField, body)
	require.NoError(t, err)

	resField, resBody, err := sc.encryptResponse([]byte("pong"), sstate)
	require.NoError(t, err)

	resp := &http.Response{Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(resBody))}
	resp.Header.Set(SessionHeader, resField)
	resp.Header.Set("Content-Type", MediaType)

	dec := &ResponseDecryptor{cfg: cc, state: cstate}
	require.NoError(t, dec.Decrypt(resp))

	out, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "pong", string(out))
	assert.Empty(t, resp.Header.Get("Content-Type"), "empty cty clears content type")
}

// serverKeyFor returns a server key with the default "kid" identifier.
func serverKeyFor(t *testing.T) *PrivateKey {
	t.Helper()

	return serverKeyForKID(t, "kid")
}

// serverKeyForKID returns a server key with the given kid.
func serverKeyForKID(t *testing.T, kid string) *PrivateKey {
	t.Helper()

	key, err := GenerateServerKey(kid, time.Hour, DefaultMaxSkew, AEADAES256GCM, AEADAES128GCM)
	require.NoError(t, err)

	return key
}

// clientKeySetFor returns a client key set for a fresh server key.
func clientKeySetFor(t *testing.T) *ClientKeySet {
	t.Helper()

	_, cs := clientServerPair(t)

	return cs
}

// clientServerPair returns a matched server key set and the client key set
// derived from it, so client-encrypted requests decrypt on the server.
func clientServerPair(t *testing.T) (*ServerKeySet, *ClientKeySet) {
	t.Helper()

	set, err := NewServerKeySet("https://api.example.com", serverKeyFor(t))
	require.NoError(t, err)

	cs, err := ParseKeySet(strings.NewReader(string(mustJSON(t, set.Document()))))
	require.NoError(t, err)

	return set, cs
}
