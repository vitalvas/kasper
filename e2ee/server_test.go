package e2ee

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecryptRequestValidationBranches(t *testing.T) {
	key := serverKeyForKID(t, "kid")

	set, err := NewServerKeySet("https://api.example.com", key)
	require.NoError(t, err)

	sc := ServerConfig{KeySet: set, Replay: NewMemoryReplayCache()}

	now := time.Now().Unix()

	t.Run("no key set", func(t *testing.T) {
		empty := ServerConfig{}
		_, _, err := empty.decryptRequest("", nil)
		require.ErrorIs(t, err, ErrNoKeySet)
	})

	t.Run("malformed field", func(t *testing.T) {
		_, _, err := sc.decryptRequest("not-a-valid-field", nil)
		require.ErrorIs(t, err, ErrMalformed)
	})

	t.Run("missing epk", func(t *testing.T) {
		field := mustField(t, sessionItem{kid: "kid", aead: "AES-256-GCM", ts: now, nid: "n"})
		_, _, err := sc.decryptRequest(field, make([]byte, minBodySize))
		require.ErrorIs(t, err, ErrMalformed)
	})

	t.Run("unknown kid", func(t *testing.T) {
		field := mustField(t, sessionItem{
			kid:    "other",
			aead:   "AES-256-GCM",
			epk:    make([]byte, keySize),
			hasEPK: true,
			ts:     now,
			nid:    "n",
		})
		_, _, err := sc.decryptRequest(field, make([]byte, minBodySize))
		require.ErrorIs(t, err, ErrKeyUnknown)
	})

	t.Run("aead not advertised", func(t *testing.T) {
		field := mustField(t, sessionItem{
			kid:    "kid",
			aead:   "AES-192-GCM",
			epk:    make([]byte, keySize),
			hasEPK: true,
			ts:     now,
			nid:    "n",
		})
		_, _, err := sc.decryptRequest(field, make([]byte, minBodySize))
		require.ErrorIs(t, err, ErrAEADUnsupported)
	})

	t.Run("bad epk length", func(t *testing.T) {
		field := mustField(t, sessionItem{
			kid:    "kid",
			aead:   "AES-256-GCM",
			epk:    make([]byte, 8),
			hasEPK: true,
			ts:     now,
			nid:    "n",
		})
		_, _, err := sc.decryptRequest(field, make([]byte, minBodySize))
		require.ErrorIs(t, err, ErrMalformed)
	})

	t.Run("short body", func(t *testing.T) {
		field := mustField(t, sessionItem{
			kid:    "kid",
			aead:   "AES-256-GCM",
			epk:    make([]byte, keySize),
			hasEPK: true,
			ts:     now,
			nid:    "n",
		})
		_, _, err := sc.decryptRequest(field, []byte("short"))
		require.ErrorIs(t, err, ErrMalformed)
	})

	t.Run("empty aead param", func(t *testing.T) {
		// A field whose aead is absent triggers the missing-required branch.
		field := fmt.Sprintf(`"kid";epk=:%s:;ts=1;nid="n"`, base64Std(make([]byte, keySize)))
		_, _, err := sc.decryptRequest(field, make([]byte, minBodySize))
		require.ErrorIs(t, err, ErrMalformed)
	})
}

func TestDecryptRequestExpiredKey(t *testing.T) {
	key, err := GenerateServerKey("kid", -time.Hour, DefaultMaxSkew, AEADAES256GCM)
	require.NoError(t, err)

	set, err := NewServerKeySet("https://api.example.com", key)
	require.NoError(t, err)

	sc := ServerConfig{KeySet: set, Replay: NewMemoryReplayCache()}

	field := mustField(t, sessionItem{
		kid:    "kid",
		aead:   "AES-256-GCM",
		epk:    make([]byte, keySize),
		hasEPK: true,
		ts:     time.Now().Unix(),
		nid:    "n",
	})

	_, _, err = sc.decryptRequest(field, make([]byte, minBodySize))
	require.ErrorIs(t, err, ErrKeyExpired)
}

func TestDecryptRequestDeriveError(t *testing.T) {
	key := serverKeyForKID(t, "kid")
	set, _ := NewServerKeySet("https://api.example.com", key)
	cs, _ := ParseKeySet(strings.NewReader(string(mustJSON(t, set.Document()))))

	cc := ClientConfig{KeySet: cs}
	body, st, err := cc.encryptRequest([]byte("hi"))
	require.NoError(t, err)

	prev := hkdfExtract
	hkdfExtract = func([]byte, []byte) ([]byte, error) { return nil, forcedErr() }
	t.Cleanup(func() { hkdfExtract = prev })

	sc := ServerConfig{KeySet: set, Replay: NewMemoryReplayCache()}
	_, _, err = sc.decryptRequest(st.reqField, body)
	require.ErrorIs(t, err, ErrInvalidKey)
}

func TestDecryptRequestOpenFailureViaGCM(t *testing.T) {
	key := serverKeyForKID(t, "kid")
	set, _ := NewServerKeySet("https://api.example.com", key)
	cs, _ := ParseKeySet(strings.NewReader(string(mustJSON(t, set.Document()))))

	cc := ClientConfig{KeySet: cs}
	body, st, err := cc.encryptRequest([]byte("hi"))
	require.NoError(t, err)

	withFailingGCM(t)

	sc := ServerConfig{KeySet: set, Replay: NewMemoryReplayCache()}
	_, _, err = sc.decryptRequest(st.reqField, body)
	require.Error(t, err)
}

func TestEncryptResponseSealFailure(t *testing.T) {
	withFailingGCM(t)

	sc := ServerConfig{}
	st := &serverState{ekRes: make([]byte, 32), reqField: "x", aead: AEADAES256GCM, kid: "k", nid: "n"}

	_, _, err := sc.encryptResponse([]byte("data"), st)
	require.Error(t, err)
}

func TestEncryptResponseSerializeError(t *testing.T) {
	prev := serializeSession
	serializeSession = func(sessionItem) (string, error) { return "", forcedErr() }
	t.Cleanup(func() { serializeSession = prev })

	sc := ServerConfig{}
	st := &serverState{ekRes: make([]byte, 32), reqField: "x", aead: AEADAES256GCM, kid: "k", nid: "n"}

	_, _, err := sc.encryptResponse([]byte("data"), st)
	require.Error(t, err)
}

func TestEncryptResponseUsesResponseContentType(t *testing.T) {
	sc := ServerConfig{ResponseContentType: "application/cbor"}
	st := &serverState{
		ekRes:    make([]byte, 32),
		reqField: "x",
		aead:     AEADAES256GCM,
		kid:      "k",
		nid:      "n",
		reqCTY:   "application/json",
	}

	resField, _, err := sc.encryptResponse([]byte("data"), st)
	require.NoError(t, err)

	item, err := parseSessionItem(resField)
	require.NoError(t, err)
	assert.Equal(t, "application/cbor", item.cty, "ResponseContentType overrides request cty")
}

func TestServerClockOverride(t *testing.T) {
	fixed := time.Unix(12345, 0)
	sc := ServerConfig{now: func() time.Time { return fixed }}
	assert.Equal(t, fixed, sc.clock())
}

func TestDecryptRequestBodyReadError(t *testing.T) {
	key := serverKeyForKID(t, "kid")
	set, _ := NewServerKeySet("https://x", key)

	req, _ := http.NewRequest(http.MethodPost, "https://x/api", errReadCloser{})
	req.Header.Set(SessionHeader, mustField(t, sessionItem{
		kid:    "kid",
		aead:   "AES-256-GCM",
		epk:    make([]byte, keySize),
		hasEPK: true,
		ts:     time.Now().Unix(),
		nid:    "n",
	}))

	_, err := DecryptRequest(req, ServerConfig{KeySet: set})
	require.Error(t, err)
}

func TestDecryptRequestMissingHeader(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "https://x/api", strings.NewReader("x"))

	_, err := DecryptRequest(req, ServerConfig{KeySet: &ServerKeySet{}})
	require.ErrorIs(t, err, ErrMalformed)
}

func TestDecryptRequestWithoutReplayCacheAllows(t *testing.T) {
	// A ServerConfig with no Replay uses the noReplay fallback, which accepts
	// every nid. DecryptRequest (used directly, not via Middleware) therefore
	// does not enforce replay protection.
	key := serverKeyForKID(t, "kid")

	set, err := NewServerKeySet("https://api.example.com", key)
	require.NoError(t, err)

	cs, err := ParseKeySet(strings.NewReader(string(mustJSON(t, set.Document()))))
	require.NoError(t, err)

	session, err := NewSession()
	require.NoError(t, err)

	cc := ClientConfig{
		KeySet:  cs,
		Session: session,
		newNID:  func() string { return "same" },
	}
	sc := ServerConfig{KeySet: set} // no Replay configured

	for range 2 {
		body, st, err := cc.encryptRequest([]byte("hi"))
		require.NoError(t, err)

		plain, _, err := sc.decryptRequest(st.reqField, body)
		require.NoError(t, err)
		assert.Equal(t, "hi", string(plain))
	}
}
