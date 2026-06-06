package e2ee

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ServerConfig configures request decryption and response encryption.
type ServerConfig struct {
	// KeySet holds the server's private keys. Required.
	KeySet *ServerKeySet

	// Replay is the replay cache used to reject duplicate nids. When nil, a
	// fresh MemoryReplayCache is used.
	Replay ReplayCache

	// ResponseContentType sets the inner plaintext media type advertised via
	// the response cty parameter. When empty, the request cty is echoed.
	ResponseContentType string

	// now overrides the clock (tests). Defaults to time.Now.
	now func() time.Time

	// randReader overrides randomness (tests). Defaults to crypto/rand.Reader.
	randReader io.Reader
}

// serverState holds the per-message key material produced while decrypting a
// request, needed to encrypt the matching response.
type serverState struct {
	ekRes    []byte
	reqField string
	aead     AEAD
	kid      string
	nid      string
	reqCTY   string
}

// decryptRequest validates and decrypts a protected request. It performs the
// validation steps from the draft in order and returns the recovered
// plaintext plus the state needed to encrypt the response.
func (c *ServerConfig) decryptRequest(reqField string, body []byte) (plaintext []byte, st *serverState, err error) {
	if c.KeySet == nil {
		return nil, nil, ErrNoKeySet
	}

	// 1-2. Parse and structurally validate the E2EE-Session field.
	item, err := parseSessionItem(reqField)
	if err != nil {
		return nil, nil, err
	}

	if !item.hasEPK {
		return nil, nil, fmt.Errorf("%w: epk required in request", ErrMalformed)
	}

	if item.aead == "" || item.nid == "" {
		return nil, nil, fmt.Errorf("%w: missing required parameter", ErrMalformed)
	}

	aead := AEAD(item.aead)

	// 4. Resolve kid against the key set.
	key := c.KeySet.lookup(item.kid)
	if key == nil {
		return nil, nil, ErrKeyUnknown
	}

	now := c.clock()

	// 8. Validate timestamp against the key validity window and max_skew.
	if !key.validAt(time.Unix(item.ts, 0)) {
		return nil, nil, ErrKeyExpired
	}

	if !withinSkew(item.ts, now.Unix(), key.MaxSkew) {
		return nil, nil, ErrTimestampSkew
	}

	// 5. Verify the AEAD is advertised for the kid.
	if !key.supportsAEAD(aead) {
		return nil, nil, ErrAEADUnsupported
	}

	// 6. Decode and length-check the ephemeral public key.
	clientPub, err := publicKeyFromBytes(item.epk)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %s", ErrMalformed, err)
	}

	// 7. Check body length.
	if len(body) < minBodySize {
		return nil, nil, fmt.Errorf("%w: body shorter than %d bytes", ErrMalformed, minBodySize)
	}

	cpk := item.epk
	serverPub := key.Priv.PublicKey().Bytes()

	// 10. Perform X25519 and HKDF-SHA256 derivation.
	ekReq, ekRes, err := deriveKeys(deriveParams{
		ourPriv:   key.Priv,
		peerPub:   clientPub,
		cpk:       cpk,
		serverPub: serverPub,
		issuer:    c.KeySet.issuer,
		aead:      aead,
		kid:       key.KID,
	})
	if err != nil {
		return nil, nil, err
	}

	// 9. Check nid against the replay cache (pre-decryption read), then 12.
	// insert atomically only after successful authentication. StoreUnique
	// performs both in one atomic step, called after Open succeeds below.

	// 11. Attempt AES-GCM decryption with AAD.
	plaintext, err = open(ekReq, body, requestAAD(reqField))
	if err != nil {
		return nil, nil, err
	}

	// 12. Insert nid into the replay cache atomically after authentication.
	ttl := time.Duration(key.MaxSkew)*time.Second + skewTolerance
	if !c.replay().StoreUnique(item.kid, item.epk, item.nid, ttl) {
		return nil, nil, ErrReplayDetected
	}

	return plaintext, &serverState{
		ekRes:    ekRes,
		reqField: reqField,
		aead:     aead,
		kid:      item.kid,
		nid:      item.nid,
		reqCTY:   item.cty,
	}, nil
}

// encryptResponse encrypts response plaintext using the state captured during
// request decryption. It builds the response E2EE-Session field (echoing kid,
// aead, nid with a fresh timestamp and no epk) and returns the field and body.
func (c *ServerConfig) encryptResponse(plaintext []byte, st *serverState) (resField string, body []byte, err error) {
	now := c.clock()

	cty := st.reqCTY
	if c.ResponseContentType != "" {
		cty = c.ResponseContentType
	}

	// The response echoes the request kid, aead, and nid with a fresh
	// timestamp and no epk, per the draft.
	item := sessionItem{
		kid:  st.kid,
		aead: string(st.aead),
		ts:   now.Unix(),
		nid:  st.nid,
	}

	if cty != "" {
		item.cty = cty
		item.hasCTY = true
	}

	resField, err = serializeSession(item)
	if err != nil {
		return "", nil, err
	}

	body, err = seal(st.ekRes, plaintext, responseAAD(st.reqField, resField), c.randReader)
	if err != nil {
		return "", nil, err
	}

	return resField, body, nil
}

func (c *ServerConfig) clock() time.Time {
	if c.now != nil {
		return c.now()
	}

	return time.Now()
}

func (c *ServerConfig) replay() ReplayCache {
	if c.Replay != nil {
		return c.Replay
	}

	// A nil Replay means the caller did not configure one. Allocating a fresh
	// cache per call would defeat replay protection, so callers should set
	// Replay; Middleware ensures a shared instance is installed.
	return noReplay{}
}

// noReplay is a ReplayCache that accepts every nid. It is a safe fallback that
// disables replay protection; Middleware always installs a real cache.
type noReplay struct{}

func (noReplay) StoreUnique(string, []byte, string, time.Duration) bool { return true }

// skewTolerance is added to a key's max_skew when computing replay-cache TTL,
// ensuring entries outlive the window in which a replay could be accepted.
const skewTolerance = 60 * time.Second

// withinSkew reports whether ts is within maxSkew seconds of now.
func withinSkew(ts, now int64, maxSkew int) bool {
	diff := ts - now
	if diff < 0 {
		diff = -diff
	}

	return diff <= int64(maxSkew)
}

// DecryptRequest validates and decrypts r's protected body in place. It
// replaces the body with the recovered plaintext, restores the inner
// Content-Type from the cty parameter, and returns a handle for encrypting
// the response. On protocol error it returns an error that maps to an RFC
// 9457 Problem Details response via WriteProblem.
func DecryptRequest(r *http.Request, cfg ServerConfig) (*ResponseEncryptor, error) {
	reqField := r.Header.Get(SessionHeader)
	if reqField == "" {
		return nil, fmt.Errorf("%w: missing %s header", ErrMalformed, SessionHeader)
	}

	var body []byte

	if r.Body != nil {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}

		r.Body.Close()

		body = b
	}

	plaintext, state, err := cfg.decryptRequest(reqField, body)
	if err != nil {
		return nil, err
	}

	r.Body = io.NopCloser(bytes.NewReader(plaintext))
	r.ContentLength = int64(len(plaintext))
	r.Header.Del(SessionHeader)

	if state.reqCTY != "" {
		r.Header.Set("Content-Type", state.reqCTY)
	} else {
		r.Header.Del("Content-Type")
	}

	return &ResponseEncryptor{cfg: cfg, state: state}, nil
}

// ResponseEncryptor encrypts handler responses for an E2EE request.
type ResponseEncryptor struct {
	cfg   ServerConfig
	state *serverState
}

// Encrypt encrypts plaintext into a protected response body and writes the
// E2EE-Session and Content-Type headers to w along with status. It is the
// low-level primitive used by the middleware's buffering ResponseWriter.
func (e *ResponseEncryptor) Encrypt(plaintext []byte) (resField string, body []byte, err error) {
	return e.cfg.encryptResponse(plaintext, e.state)
}
