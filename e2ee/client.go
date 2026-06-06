package e2ee

import (
	"bytes"
	"crypto/ecdh"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// Session holds a reusable ephemeral X25519 key pair for a client. Reusing a
// session across requests reduces per-request key generation but widens the
// window of client-side forward-secrecy exposure. A session must be scoped to
// a single logical client instance, never persisted, and destroyed on process
// exit or user logout.
type Session struct {
	priv *ecdh.PrivateKey
}

// NewSession creates a Session with a fresh ephemeral key pair.
func NewSession() (*Session, error) {
	priv, err := generateKeyPair(nil)
	if err != nil {
		return nil, err
	}

	return &Session{priv: priv}, nil
}

// SessionHeader is the HTTP field carrying E2EE control metadata.
const SessionHeader = "E2EE-Session"

// MediaType is the media type of a protected (ciphertext) payload.
const MediaType = "application/e2ee"

// ClientConfig configures request encryption and response decryption.
type ClientConfig struct {
	// KeySet is the validated server key set used to select a recipient key.
	// Required.
	KeySet *ClientKeySet

	// AEAD requests a specific cipher suite. When empty, the first AEAD of the
	// selected key is used.
	AEAD AEAD

	// ContentType sets the inner plaintext media type advertised via the cty
	// parameter. When empty, cty is omitted.
	ContentType string

	// Session, when non-nil, reuses a single ephemeral key pair across
	// requests instead of generating a fresh pair per request. Sessions must
	// be scoped to a single logical client and never persisted; see the
	// package documentation. When nil, a fresh key pair is used per request
	// (the default), which minimizes key lifetime and replay-cache state.
	Session *Session

	// now overrides the clock (tests). Defaults to time.Now.
	now func() time.Time

	// randReader overrides randomness (tests). Defaults to crypto/rand.Reader.
	randReader io.Reader

	// newNID overrides replay-identifier generation (tests). Defaults to a
	// random UUIDv4 string.
	newNID func() string
}

// clientState holds the per-message key material produced while encrypting a
// request, needed later to decrypt the matching response.
type clientState struct {
	ekRes    []byte
	reqField string
	aead     AEAD
	kid      string
	nid      string
}

// encryptRequest encrypts plaintext into a protected request body and returns
// the serialized E2EE-Session field plus the state needed to decrypt the
// response. A fresh ephemeral key pair is generated for the message.
func (c *ClientConfig) encryptRequest(plaintext []byte) (body []byte, state *clientState, err error) {
	if c.KeySet == nil {
		return nil, nil, ErrNoKeySet
	}

	now := c.clock()

	key, aead, err := c.KeySet.selectKey(now, c.AEAD)
	if err != nil {
		return nil, nil, err
	}

	eph, err := c.ephemeralKey()
	if err != nil {
		return nil, nil, err
	}

	cpk := eph.PublicKey().Bytes()
	serverPub := key.pub.Bytes()

	ekReq, ekRes, err := deriveKeys(deriveParams{
		ourPriv:   eph,
		peerPub:   key.pub,
		cpk:       cpk,
		serverPub: serverPub,
		issuer:    c.KeySet.issuer,
		aead:      aead,
		kid:       key.KID,
	})
	if err != nil {
		return nil, nil, err
	}

	nid := c.nid()

	item := sessionItem{
		kid:    key.KID,
		aead:   string(aead),
		epk:    cpk,
		ts:     now.Unix(),
		nid:    nid,
		hasEPK: true,
	}

	if c.ContentType != "" {
		item.cty = c.ContentType
		item.hasCTY = true
	}

	reqField, err := serializeSession(item)
	if err != nil {
		return nil, nil, err
	}

	body, err = seal(ekReq, plaintext, requestAAD(reqField), c.randReader)
	if err != nil {
		return nil, nil, err
	}

	return body, &clientState{
		ekRes:    ekRes,
		reqField: reqField,
		aead:     aead,
		kid:      key.KID,
		nid:      nid,
	}, nil
}

// decryptResponse decrypts a protected response body using the state captured
// when the request was encrypted. It validates that the response echoes the
// request kid, aead, and nid before decryption. It returns the recovered
// plaintext and the inner content type from the response cty parameter (empty
// when absent).
func (c *ClientConfig) decryptResponse(resField string, body []byte, st *clientState) (plaintext []byte, cty string, err error) {
	item, err := parseSessionItem(resField)
	if err != nil {
		return nil, "", err
	}

	if item.hasEPK {
		return nil, "", fmt.Errorf("%w: epk prohibited in response", ErrMalformed)
	}

	if item.kid != st.kid || item.aead != string(st.aead) || item.nid != st.nid {
		return nil, "", fmt.Errorf("%w: response does not echo request", ErrMalformed)
	}

	aad := responseAAD(st.reqField, resField)

	plaintext, err = open(st.ekRes, body, aad)
	if err != nil {
		return nil, "", err
	}

	return plaintext, item.cty, nil
}

// ephemeralKey returns the ephemeral key pair to use for a request: the
// session key when a Session is configured, otherwise a fresh pair.
func (c *ClientConfig) ephemeralKey() (*ecdh.PrivateKey, error) {
	if c.Session != nil {
		return c.Session.priv, nil
	}

	return generateKeyPair(c.randReader)
}

func (c *ClientConfig) clock() time.Time {
	if c.now != nil {
		return c.now()
	}

	return time.Now()
}

func (c *ClientConfig) nid() string {
	if c.newNID != nil {
		return c.newNID()
	}

	return uuid.NewString()
}

// EncryptRequest encrypts r's body in place: it replaces the body with the
// protected envelope, sets the E2EE-Session and Content-Type headers, and
// returns a handle for decrypting the response. The request must have a
// readable body (possibly empty).
func EncryptRequest(r *http.Request, cfg ClientConfig) (*ResponseDecryptor, error) {
	var plaintext []byte

	if r.Body != nil {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}

		r.Body.Close()

		plaintext = b
	}

	body, state, err := cfg.encryptRequest(plaintext)
	if err != nil {
		return nil, err
	}

	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))
	r.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}

	r.Header.Set(SessionHeader, state.reqField)
	r.Header.Set("Content-Type", MediaType)

	return &ResponseDecryptor{cfg: cfg, state: state}, nil
}

// ResponseDecryptor decrypts the response that matches an encrypted request.
type ResponseDecryptor struct {
	cfg   ClientConfig
	state *clientState
}

// Decrypt reads and decrypts resp's protected body in place, replacing the
// body with the recovered plaintext and removing the E2EE Content-Type. It
// validates the E2EE-Session response field against the original request.
func (d *ResponseDecryptor) Decrypt(resp *http.Response) error {
	resField := resp.Header.Get(SessionHeader)
	if resField == "" {
		return fmt.Errorf("%w: missing %s response header", ErrMalformed, SessionHeader)
	}

	var body []byte

	if resp.Body != nil {
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		resp.Body.Close()

		body = b
	}

	plaintext, cty, err := d.cfg.decryptResponse(resField, body, d.state)
	if err != nil {
		return err
	}

	resp.Body = io.NopCloser(bytes.NewReader(plaintext))
	resp.ContentLength = int64(len(plaintext))
	resp.Header.Del(SessionHeader)

	if cty != "" {
		resp.Header.Set("Content-Type", cty)
	} else {
		resp.Header.Del("Content-Type")
	}

	return nil
}
