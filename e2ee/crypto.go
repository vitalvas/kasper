package e2ee

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"strings"
)

// AEAD identifies an AES-GCM cipher suite per the draft. The associated key
// length is fixed by the suite.
type AEAD string

const (
	// AEADAES128GCM is AES-128-GCM (16-byte key). Mandatory to implement.
	AEADAES128GCM AEAD = "AES-128-GCM"

	// AEADAES192GCM is AES-192-GCM (24-byte key). Optional.
	AEADAES192GCM AEAD = "AES-192-GCM"

	// AEADAES256GCM is AES-256-GCM (32-byte key). Mandatory to implement.
	AEADAES256GCM AEAD = "AES-256-GCM"
)

// Fixed sizes defined by the draft.
const (
	// keySize is the X25519 public/private key length in bytes.
	keySize = 32

	// nonceSize is the AES-GCM nonce length in bytes.
	nonceSize = 12

	// tagSize is the AES-GCM authentication tag length in bytes.
	tagSize = 16

	// minBodySize is the minimum protected body length: nonce + empty
	// ciphertext + tag.
	minBodySize = nonceSize + tagSize

	// fingerprintSize is the number of leading SHA-256 bytes used as a key
	// fingerprint.
	fingerprintSize = 16
)

// keyLength returns the AEAD key length in bytes, or ok=false for an unknown
// suite.
func (a AEAD) keyLength() (int, bool) {
	switch a {
	case AEADAES128GCM:
		return 16, true
	case AEADAES192GCM:
		return 24, true
	case AEADAES256GCM:
		return 32, true
	default:
		return 0, false
	}
}

// valid reports whether a is a known AEAD suite.
func (a AEAD) valid() bool {
	_, ok := a.keyLength()

	return ok
}

// direction selects which directional key is derived.
type direction string

const (
	dirRequest  direction = "req"
	dirResponse direction = "res"
)

// Crypto constructor seams. These wrap stdlib calls that, for valid-length
// inputs, cannot currently fail; they are indirected through variables so
// that tests can exercise the defensive error-handling branches. Production
// code always uses the stdlib implementations.
var (
	x25519GenerateKey = func(r io.Reader) (*ecdh.PrivateKey, error) { return ecdh.X25519().GenerateKey(r) }
	x25519NewPublic   = func(b []byte) (*ecdh.PublicKey, error) { return ecdh.X25519().NewPublicKey(b) }
	x25519NewPrivate  = func(b []byte) (*ecdh.PrivateKey, error) { return ecdh.X25519().NewPrivateKey(b) }
	ecdhShared        = func(priv *ecdh.PrivateKey, pub *ecdh.PublicKey) ([]byte, error) { return priv.ECDH(pub) }
)

// generateKeyPair returns a fresh X25519 key pair using the provided random
// source. When randReader is nil, crypto/rand.Reader is used.
func generateKeyPair(randReader io.Reader) (*ecdh.PrivateKey, error) {
	if randReader == nil {
		randReader = rand.Reader
	}

	priv, err := x25519GenerateKey(randReader)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidKey, err)
	}

	return priv, nil
}

// publicKeyFromBytes parses a 32-byte X25519 public key.
func publicKeyFromBytes(b []byte) (*ecdh.PublicKey, error) {
	if len(b) != keySize {
		return nil, fmt.Errorf("%w: public key must be %d bytes", ErrInvalidKey, keySize)
	}

	pub, err := x25519NewPublic(b)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidKey, err)
	}

	return pub, nil
}

// privateKeyFromBytes parses a 32-byte X25519 private key (scalar).
func privateKeyFromBytes(b []byte) (*ecdh.PrivateKey, error) {
	if len(b) != keySize {
		return nil, fmt.Errorf("%w: private key must be %d bytes", ErrInvalidKey, keySize)
	}

	priv, err := x25519NewPrivate(b)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidKey, err)
	}

	return priv, nil
}

// deriveParams carries the inputs to deriveKeys. ourPriv and peerPub provide
// the X25519 scalar-multiplication inputs; cpk and serverPub are the canonical
// client and server public keys used for the HKDF salt regardless of role;
// issuer, aead, and kid bind the derived keys to their context.
type deriveParams struct {
	ourPriv   *ecdh.PrivateKey
	peerPub   *ecdh.PublicKey
	cpk       []byte
	serverPub []byte
	issuer    string
	aead      AEAD
	kid       string
}

// deriveKeys computes the shared secret via X25519 and derives both
// directional encryption keys with HKDF-SHA256 per the draft.
//
// The HKDF-Extract salt is cpk || serverPub (raw 32-byte keys). The Expand
// info string binds direction, issuer, AEAD, and kid.
func deriveKeys(p deriveParams) (ekReq, ekRes []byte, err error) {
	nk, ok := p.aead.keyLength()
	if !ok {
		return nil, nil, fmt.Errorf("%w: %s", ErrAEADUnsupported, p.aead)
	}

	z, err := ecdhShared(p.ourPriv, p.peerPub)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %s", ErrInvalidKey, err)
	}

	// X25519 rejects all-zero output internally; defend explicitly as well.
	if allZero(z) {
		return nil, nil, fmt.Errorf("%w: zero shared secret", ErrInvalidKey)
	}

	salt := make([]byte, 0, len(p.cpk)+len(p.serverPub))
	salt = append(salt, p.cpk...)
	salt = append(salt, p.serverPub...)

	prk, err := hkdfExtract(z, salt)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: hkdf extract: %s", ErrInvalidKey, err)
	}

	ekReq, err = hkdfExpand(prk, infoString(dirRequest, p.issuer, p.aead, p.kid), nk)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: hkdf expand req: %s", ErrInvalidKey, err)
	}

	ekRes, err = hkdfExpand(prk, infoString(dirResponse, p.issuer, p.aead, p.kid), nk)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: hkdf expand res: %s", ErrInvalidKey, err)
	}

	return ekReq, ekRes, nil
}

// HKDF seams. These wrap crypto/hkdf so tests can exercise the defensive
// error-handling branches; for SHA-256 and the fixed AEAD key lengths the
// stdlib calls do not fail in practice.
var (
	hkdfExtract = func(secret, salt []byte) ([]byte, error) {
		return hkdf.Extract(sha256.New, secret, salt)
	}
	hkdfExpand = func(prk []byte, info string, length int) ([]byte, error) {
		return hkdf.Expand(sha256.New, prk, info, length)
	}
)

// infoString builds the HKDF-Expand info per the draft:
//
//	"e2ee/v1:<dir> " || issuer || " " || aead || " " || kid
func infoString(dir direction, issuer string, aead AEAD, kid string) string {
	var b strings.Builder

	b.WriteString("e2ee/v1:")
	b.WriteString(string(dir))
	b.WriteByte(' ')
	b.WriteString(issuer)
	b.WriteByte(' ')
	b.WriteString(string(aead))
	b.WriteByte(' ')
	b.WriteString(kid)

	return b.String()
}

// seal encrypts plaintext with the directional key and AAD, producing the
// wire body nonce || ciphertext || tag. A fresh random nonce is drawn from
// randReader (crypto/rand.Reader when nil).
func seal(ek []byte, plaintext, aad []byte, randReader io.Reader) ([]byte, error) {
	if randReader == nil {
		randReader = rand.Reader
	}

	gcm, err := newGCM(ek)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(randReader, nonce); err != nil {
		return nil, fmt.Errorf("%w: nonce generation: %s", ErrDecryptFailed, err)
	}

	// Seal prepends nonce to the output; ciphertext includes the tag.
	return gcm.Seal(nonce, nonce, plaintext, aad), nil
}

// open decrypts a wire body (nonce || ciphertext || tag) with the directional
// key and AAD. It returns ErrMalformed for short bodies and ErrDecryptFailed
// for authentication failures.
func open(ek []byte, body, aad []byte) ([]byte, error) {
	if len(body) < minBodySize {
		return nil, fmt.Errorf("%w: body shorter than %d bytes", ErrMalformed, minBodySize)
	}

	gcm, err := newGCM(ek)
	if err != nil {
		return nil, err
	}

	nonce := body[:nonceSize]
	ciphertext := body[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, ErrDecryptFailed
	}

	return plaintext, nil
}

// newGCM constructs an AES-GCM AEAD from a derived key. Tests may override it.
var newGCM = func(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidKey, err)
	}

	gcm, err := newCipherGCM(block)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidKey, err)
	}

	return gcm, nil
}

// newCipherGCM seams cipher.NewGCM so the defensive error branch in newGCM is
// reachable in tests; for an AES block it does not fail in practice.
var newCipherGCM = cipher.NewGCM

// fingerprint returns the first 16 bytes of SHA-256 over the public key.
func fingerprint(pub []byte) []byte {
	sum := sha256.Sum256(pub)

	out := make([]byte, fingerprintSize)
	copy(out, sum[:fingerprintSize])

	return out
}

// allZero reports whether b consists entirely of zero bytes.
func allZero(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}

	return true
}
