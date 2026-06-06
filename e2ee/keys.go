package e2ee

import (
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"time"

	"github.com/vitalvas/kasper/mux"
)

// WellKnownPath is the well-known URI suffix where servers publish their
// encryption key set.
const WellKnownPath = "/.well-known/encryption-keys"

// algX25519 is the only key agreement algorithm defined by this version.
const algX25519 = "X25519"

// kidPattern constrains key identifiers per the draft.
var kidPattern = regexp.MustCompile(`^[A-Za-z0-9._~-]{1,128}$`)

// DefaultMaxSkew is the recommended maximum clock skew in seconds.
const DefaultMaxSkew = 300

// PublicKey is a single published key entry in a key set document. Byte
// fields are exposed as base64url strings on the wire and decoded on parse.
type PublicKey struct {
	KID         string     `json:"kid"`
	Alg         string     `json:"alg"`
	AEADs       []AEAD     `json:"aeads"`
	PublicKey   string     `json:"public_key"`
	Fingerprint string     `json:"fingerprint,omitempty"`
	NotBefore   *time.Time `json:"not_before,omitempty"`
	NotAfter    time.Time  `json:"not_after"`
	MaxSkew     int        `json:"max_skew"`

	pub *ecdh.PublicKey // decoded lazily by parse/validate
}

// KeySetDocument is the JSON document published at WellKnownPath.
type KeySetDocument struct {
	Issuer string      `json:"issuer"`
	Keys   []PublicKey `json:"keys"`
}

// decodeKey parses and validates a single PublicKey entry, decoding the
// X25519 public key. It returns the decoded key for caching on the entry.
func (k *PublicKey) decode() (*ecdh.PublicKey, error) {
	if !kidPattern.MatchString(k.KID) {
		return nil, fmt.Errorf("%w: invalid kid", ErrMalformed)
	}

	if k.Alg != algX25519 {
		return nil, fmt.Errorf("%w: unsupported alg %q", ErrMalformed, k.Alg)
	}

	if len(k.AEADs) == 0 {
		return nil, fmt.Errorf("%w: aeads must not be empty", ErrMalformed)
	}

	for _, a := range k.AEADs {
		if !a.valid() {
			return nil, fmt.Errorf("%w: unknown aead %q", ErrMalformed, a)
		}
	}

	raw, err := base64.RawURLEncoding.DecodeString(k.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("%w: public_key base64: %s", ErrMalformed, err)
	}

	pub, err := publicKeyFromBytes(raw)
	if err != nil {
		return nil, err
	}

	if k.MaxSkew < 0 {
		return nil, fmt.Errorf("%w: max_skew must be non-negative", ErrMalformed)
	}

	return pub, nil
}

// supportsAEAD reports whether the key advertises the given AEAD suite.
func (k *PublicKey) supportsAEAD(a AEAD) bool {
	return slices.Contains(k.AEADs, a)
}

// validAt reports whether the key is within its validity window at t,
// allowing the key's own max_skew tolerance on both bounds.
func (k *PublicKey) validAt(t time.Time) bool {
	skew := time.Duration(k.MaxSkew) * time.Second

	if k.NotBefore != nil && t.Before(k.NotBefore.Add(-skew)) {
		return false
	}

	return !t.After(k.NotAfter.Add(skew))
}

// PrivateKey is a server-side signing key: an X25519 private key plus the
// metadata published in the key set.
type PrivateKey struct {
	KID       string
	Priv      *ecdh.PrivateKey
	AEADs     []AEAD
	NotBefore *time.Time
	NotAfter  time.Time
	MaxSkew   int
}

// GenerateServerKey creates a server PrivateKey with a fresh X25519 key pair.
// validFor sets not_after relative to now; maxSkew is in seconds. When aeads
// is empty, the mandatory suites (AES-256-GCM, AES-128-GCM) are advertised.
func GenerateServerKey(kid string, validFor time.Duration, maxSkew int, aeads ...AEAD) (*PrivateKey, error) {
	aeads, err := validateKeyParams(kid, maxSkew, aeads)
	if err != nil {
		return nil, err
	}

	priv, err := generateKeyPair(nil)
	if err != nil {
		return nil, err
	}

	return &PrivateKey{
		KID:      kid,
		Priv:     priv,
		AEADs:    aeads,
		NotAfter: time.Now().Add(validFor),
		MaxSkew:  maxSkew,
	}, nil
}

// NewServerKey constructs a server PrivateKey from a raw 32-byte X25519
// private scalar, allowing keys to be loaded from persisted material. validFor
// sets not_after relative to now; maxSkew is in seconds. When aeads is empty,
// the mandatory suites are advertised.
func NewServerKey(kid string, priv []byte, validFor time.Duration, maxSkew int, aeads ...AEAD) (*PrivateKey, error) {
	aeads, err := validateKeyParams(kid, maxSkew, aeads)
	if err != nil {
		return nil, err
	}

	key, err := privateKeyFromBytes(priv)
	if err != nil {
		return nil, err
	}

	return &PrivateKey{
		KID:      kid,
		Priv:     key,
		AEADs:    aeads,
		NotAfter: time.Now().Add(validFor),
		MaxSkew:  maxSkew,
	}, nil
}

// validateKeyParams validates kid, maxSkew, and aeads, returning the effective
// aead list (filled with mandatory suites when empty).
func validateKeyParams(kid string, maxSkew int, aeads []AEAD) ([]AEAD, error) {
	if !kidPattern.MatchString(kid) {
		return nil, fmt.Errorf("%w: invalid kid", ErrInvalidKey)
	}

	if maxSkew < 0 {
		return nil, fmt.Errorf("%w: max_skew must be non-negative", ErrInvalidKey)
	}

	if len(aeads) == 0 {
		return []AEAD{AEADAES256GCM, AEADAES128GCM}, nil
	}

	for _, a := range aeads {
		if !a.valid() {
			return nil, fmt.Errorf("%w: unknown aead %q", ErrInvalidKey, a)
		}
	}

	return aeads, nil
}

// public renders the server key as a published PublicKey entry.
func (k *PrivateKey) public() PublicKey {
	pubBytes := k.Priv.PublicKey().Bytes()

	return PublicKey{
		KID:         k.KID,
		Alg:         algX25519,
		AEADs:       k.AEADs,
		PublicKey:   base64.RawURLEncoding.EncodeToString(pubBytes),
		Fingerprint: base64.RawURLEncoding.EncodeToString(fingerprint(pubBytes)),
		NotBefore:   k.NotBefore,
		NotAfter:    k.NotAfter,
		MaxSkew:     k.MaxSkew,
		pub:         k.Priv.PublicKey(),
	}
}

// supportsAEAD reports whether the server key advertises the given AEAD.
func (k *PrivateKey) supportsAEAD(a AEAD) bool {
	return slices.Contains(k.AEADs, a)
}

// validAt reports whether the server key is valid at t (with max_skew).
func (k *PrivateKey) validAt(t time.Time) bool {
	skew := time.Duration(k.MaxSkew) * time.Second

	if k.NotBefore != nil && t.Before(k.NotBefore.Add(-skew)) {
		return false
	}

	return !t.After(k.NotAfter.Add(skew))
}

// ServerKeySet is a collection of server private keys published under a single
// issuer. It is safe for concurrent reads; rotate by constructing a new set.
type ServerKeySet struct {
	issuer string
	keys   map[string]*PrivateKey
	order  []string // publication order, most preferred first
}

// NewServerKeySet creates a key set for issuer with the given keys, in
// preference order (most preferred first). The issuer must be an HTTPS origin.
func NewServerKeySet(issuer string, keys ...*PrivateKey) (*ServerKeySet, error) {
	if issuer == "" {
		return nil, fmt.Errorf("%w: issuer must not be empty", ErrInvalidKey)
	}

	if len(keys) == 0 {
		return nil, ErrNoKeys
	}

	set := &ServerKeySet{
		issuer: issuer,
		keys:   make(map[string]*PrivateKey, len(keys)),
		order:  make([]string, 0, len(keys)),
	}

	for _, k := range keys {
		if k == nil || k.Priv == nil {
			return nil, fmt.Errorf("%w: nil key", ErrInvalidKey)
		}

		if _, dup := set.keys[k.KID]; dup {
			return nil, fmt.Errorf("%w: duplicate kid %q", ErrInvalidKey, k.KID)
		}

		set.keys[k.KID] = k
		set.order = append(set.order, k.KID)
	}

	return set, nil
}

// Issuer returns the issuer origin for the key set.
func (s *ServerKeySet) Issuer() string { return s.issuer }

// lookup returns the private key for kid, or nil if absent.
func (s *ServerKeySet) lookup(kid string) *PrivateKey {
	return s.keys[kid]
}

// Document renders the key set as a publishable KeySetDocument.
func (s *ServerKeySet) Document() KeySetDocument {
	doc := KeySetDocument{
		Issuer: s.issuer,
		Keys:   make([]PublicKey, 0, len(s.order)),
	}

	for _, kid := range s.order {
		doc.Keys = append(doc.Keys, s.keys[kid].public())
	}

	return doc
}

// Handler returns an http.Handler serving the key set document as JSON at
// WellKnownPath. It responds only to GET and HEAD.
func (s *ServerKeySet) Handler() http.Handler {
	doc := s.Document()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			w.WriteHeader(http.StatusMethodNotAllowed)

			return
		}

		if r.Method == http.MethodHead {
			w.Header().Set("Content-Type", mux.ContentTypeApplicationJSON)
			w.Header().Set("Cache-Control", "no-cache")
			w.WriteHeader(http.StatusOK)

			return
		}

		mux.ResponseJSON(w, http.StatusOK, doc, mux.ResponseConfig{
			Headers: map[string]string{"Cache-Control": "no-cache"},
		})
	})
}

// ClientKeySet is a parsed, validated key set obtained from a server. It is
// immutable after construction and safe for concurrent reads.
type ClientKeySet struct {
	issuer string
	keys   []PublicKey
	byKID  map[string]*PublicKey
}

// ParseKeySet parses and validates a key set document read from r.
func ParseKeySet(r io.Reader) (*ClientKeySet, error) {
	var doc KeySetDocument

	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()

	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("%w: key set json: %s", ErrMalformed, err)
	}

	return newClientKeySet(doc)
}

// newClientKeySet validates a decoded document into a ClientKeySet.
func newClientKeySet(doc KeySetDocument) (*ClientKeySet, error) {
	if doc.Issuer == "" {
		return nil, fmt.Errorf("%w: issuer must not be empty", ErrMalformed)
	}

	if len(doc.Keys) == 0 {
		return nil, ErrNoKeys
	}

	set := &ClientKeySet{
		issuer: doc.Issuer,
		keys:   make([]PublicKey, 0, len(doc.Keys)),
		byKID:  make(map[string]*PublicKey, len(doc.Keys)),
	}

	for i := range doc.Keys {
		k := doc.Keys[i]

		pub, err := k.decode()
		if err != nil {
			return nil, err
		}

		k.pub = pub

		set.keys = append(set.keys, k)
	}

	for i := range set.keys {
		k := &set.keys[i]
		if _, dup := set.byKID[k.KID]; dup {
			return nil, fmt.Errorf("%w: duplicate kid %q", ErrMalformed, k.KID)
		}

		set.byKID[k.KID] = k
	}

	return set, nil
}

// Issuer returns the issuer origin from the key set document.
func (s *ClientKeySet) Issuer() string { return s.issuer }

// selectKey chooses a usable key valid at t and the AEAD to use. When
// preferred is non-empty, the client requests that AEAD if any selected key
// advertises it. Otherwise the first AEAD of the first valid key is used.
func (s *ClientKeySet) selectKey(t time.Time, preferred AEAD) (*PublicKey, AEAD, error) {
	for i := range s.keys {
		k := &s.keys[i]
		if !k.validAt(t) {
			continue
		}

		if preferred != "" {
			if k.supportsAEAD(preferred) {
				return k, preferred, nil
			}

			continue
		}

		return k, k.AEADs[0], nil
	}

	return nil, "", ErrNoKeys
}

// FetchKeySet retrieves and validates the key set published at the issuer's
// well-known URI using client. The issuer must be an https origin (scheme +
// host, no path). client must not be nil.
func FetchKeySet(client *http.Client, issuer string) (*ClientKeySet, error) {
	req, err := http.NewRequest(http.MethodGet, issuer+WellKnownPath, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrMalformed, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: key set fetch status %d", ErrMalformed, resp.StatusCode)
	}

	set, err := ParseKeySet(resp.Body)
	if err != nil {
		return nil, err
	}

	if set.issuer != issuer {
		return nil, fmt.Errorf("%w: issuer mismatch", ErrMalformed)
	}

	return set, nil
}
