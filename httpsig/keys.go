package httpsig

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"math/big"
)

// JWKThumbprintEd25519 returns the JWK SHA-256 thumbprint (RFC 7638) of an
// Ed25519 public key, base64url-encoded without padding.
//
// Web Bot Auth (draft-meunier-web-bot-auth-architecture, used by Cloudflare)
// requires this value as the signature keyid and as the kid of the JWK
// published in the key directory.
func JWKThumbprintEd25519(pub ed25519.PublicKey) (string, error) {
	if len(pub) != ed25519.PublicKeySize {
		return "", fmt.Errorf("%w: ed25519 public key must be %d bytes", ErrInvalidKey, ed25519.PublicKeySize)
	}

	// RFC 7638 Section 3.2: hash the canonical JSON of the required JWK
	// members in lexicographic order, with no whitespace.
	canonical := fmt.Sprintf(`{"crv":"Ed25519","kty":"OKP","x":"%s"}`, base64.RawURLEncoding.EncodeToString(pub))
	sum := sha256.Sum256([]byte(canonical))

	return base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

// Minimum RSA key size in bits.
const minRSAKeyBits = 2048

// ECDSA signature element sizes in bytes per RFC 9421 Sections 3.3.3 and
// 3.3.4: r and s are each zero-padded to the curve's field size.
const (
	ecdsaP256ElementBytes = 32
	ecdsaP384ElementBytes = 48
)

// ecdsaRawSign signs digest and encodes the signature as the concatenation
// of r and s, each as a big-endian unsigned integer zero-padded to size
// bytes, per RFC 9421 Sections 3.3.3 and 3.3.4.
func ecdsaRawSign(key *ecdsa.PrivateKey, digest []byte, size int) ([]byte, error) {
	r, s, err := ecdsa.Sign(rand.Reader, key, digest)
	if err != nil {
		return nil, err
	}

	sig := make([]byte, 2*size)
	r.FillBytes(sig[:size])
	s.FillBytes(sig[size:])

	return sig, nil
}

// ecdsaRawVerify verifies a raw r||s signature per RFC 9421 Sections 3.3.3
// and 3.3.4. ASN.1 DER signatures produced by earlier kasper releases are
// also accepted for backward compatibility.
func ecdsaRawVerify(key *ecdsa.PublicKey, digest, signature []byte, size int) error {
	if len(signature) == 2*size {
		r := new(big.Int).SetBytes(signature[:size])
		s := new(big.Int).SetBytes(signature[size:])

		if ecdsa.Verify(key, digest, r, s) {
			return nil
		}
	}

	// Legacy ASN.1 DER encoding from earlier kasper releases.
	if ecdsa.VerifyASN1(key, digest, signature) {
		return nil
	}

	return ErrSignatureInvalid
}

// --- Ed25519 ---

type ed25519Signer struct {
	key   ed25519.PrivateKey
	keyID string
}

// NewEd25519Signer creates a Signer using Ed25519.
func NewEd25519Signer(keyID string, key ed25519.PrivateKey) (Signer, error) {
	if len(key) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("%w: ed25519 private key must be %d bytes", ErrInvalidKey, ed25519.PrivateKeySize)
	}

	return &ed25519Signer{
		key:   key,
		keyID: keyID,
	}, nil
}

func (s *ed25519Signer) Sign(message []byte) ([]byte, error) {
	return ed25519.Sign(s.key, message), nil
}

func (s *ed25519Signer) Algorithm() Algorithm { return AlgorithmEd25519 }
func (s *ed25519Signer) KeyID() string        { return s.keyID }

type ed25519Verifier struct {
	key   ed25519.PublicKey
	keyID string
}

// NewEd25519Verifier creates a Verifier using Ed25519.
func NewEd25519Verifier(keyID string, key ed25519.PublicKey) (Verifier, error) {
	if len(key) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("%w: ed25519 public key must be %d bytes", ErrInvalidKey, ed25519.PublicKeySize)
	}

	return &ed25519Verifier{
		key:   key,
		keyID: keyID,
	}, nil
}

func (v *ed25519Verifier) Verify(message, signature []byte) error {
	if !ed25519.Verify(v.key, message, signature) {
		return ErrSignatureInvalid
	}

	return nil
}

func (v *ed25519Verifier) Algorithm() Algorithm { return AlgorithmEd25519 }
func (v *ed25519Verifier) KeyID() string        { return v.keyID }

// --- ECDSA P-256 ---

type ecdsaP256Signer struct {
	key   *ecdsa.PrivateKey
	keyID string
}

// NewECDSAP256Signer creates a Signer using ECDSA with curve P-256 and SHA-256.
func NewECDSAP256Signer(keyID string, key *ecdsa.PrivateKey) (Signer, error) {
	if key == nil {
		return nil, fmt.Errorf("%w: ecdsa private key must not be nil", ErrInvalidKey)
	}

	if key.Curve != elliptic.P256() {
		return nil, fmt.Errorf("%w: key curve must be P-256", ErrInvalidKey)
	}

	return &ecdsaP256Signer{
		key:   key,
		keyID: keyID,
	}, nil
}

func (s *ecdsaP256Signer) Sign(message []byte) ([]byte, error) {
	digest := sha256.Sum256(message)

	return ecdsaRawSign(s.key, digest[:], ecdsaP256ElementBytes)
}

func (s *ecdsaP256Signer) Algorithm() Algorithm { return AlgorithmECDSAP256SHA256 }
func (s *ecdsaP256Signer) KeyID() string        { return s.keyID }

type ecdsaP256Verifier struct {
	key   *ecdsa.PublicKey
	keyID string
}

// NewECDSAP256Verifier creates a Verifier using ECDSA with curve P-256 and SHA-256.
func NewECDSAP256Verifier(keyID string, key *ecdsa.PublicKey) (Verifier, error) {
	if key == nil {
		return nil, fmt.Errorf("%w: ecdsa public key must not be nil", ErrInvalidKey)
	}

	if key.Curve != elliptic.P256() {
		return nil, fmt.Errorf("%w: key curve must be P-256", ErrInvalidKey)
	}

	return &ecdsaP256Verifier{
		key:   key,
		keyID: keyID,
	}, nil
}

func (v *ecdsaP256Verifier) Verify(message, signature []byte) error {
	digest := sha256.Sum256(message)

	return ecdsaRawVerify(v.key, digest[:], signature, ecdsaP256ElementBytes)
}

func (v *ecdsaP256Verifier) Algorithm() Algorithm { return AlgorithmECDSAP256SHA256 }
func (v *ecdsaP256Verifier) KeyID() string        { return v.keyID }

// --- ECDSA P-384 ---

type ecdsaP384Signer struct {
	key   *ecdsa.PrivateKey
	keyID string
}

// NewECDSAP384Signer creates a Signer using ECDSA with curve P-384 and SHA-384.
func NewECDSAP384Signer(keyID string, key *ecdsa.PrivateKey) (Signer, error) {
	if key == nil {
		return nil, fmt.Errorf("%w: ecdsa private key must not be nil", ErrInvalidKey)
	}

	if key.Curve != elliptic.P384() {
		return nil, fmt.Errorf("%w: key curve must be P-384", ErrInvalidKey)
	}

	return &ecdsaP384Signer{
		key:   key,
		keyID: keyID,
	}, nil
}

func (s *ecdsaP384Signer) Sign(message []byte) ([]byte, error) {
	digest := sha512.Sum384(message)

	return ecdsaRawSign(s.key, digest[:], ecdsaP384ElementBytes)
}

func (s *ecdsaP384Signer) Algorithm() Algorithm { return AlgorithmECDSAP384SHA384 }
func (s *ecdsaP384Signer) KeyID() string        { return s.keyID }

type ecdsaP384Verifier struct {
	key   *ecdsa.PublicKey
	keyID string
}

// NewECDSAP384Verifier creates a Verifier using ECDSA with curve P-384 and SHA-384.
func NewECDSAP384Verifier(keyID string, key *ecdsa.PublicKey) (Verifier, error) {
	if key == nil {
		return nil, fmt.Errorf("%w: ecdsa public key must not be nil", ErrInvalidKey)
	}

	if key.Curve != elliptic.P384() {
		return nil, fmt.Errorf("%w: key curve must be P-384", ErrInvalidKey)
	}

	return &ecdsaP384Verifier{
		key:   key,
		keyID: keyID,
	}, nil
}

func (v *ecdsaP384Verifier) Verify(message, signature []byte) error {
	digest := sha512.Sum384(message)

	return ecdsaRawVerify(v.key, digest[:], signature, ecdsaP384ElementBytes)
}

func (v *ecdsaP384Verifier) Algorithm() Algorithm { return AlgorithmECDSAP384SHA384 }
func (v *ecdsaP384Verifier) KeyID() string        { return v.keyID }

// --- RSA-PSS SHA-512 ---

type rsaPSSSigner struct {
	key   *rsa.PrivateKey
	keyID string
}

// NewRSAPSSSigner creates a Signer using RSASSA-PSS with SHA-512.
func NewRSAPSSSigner(keyID string, key *rsa.PrivateKey) (Signer, error) {
	if key == nil {
		return nil, fmt.Errorf("%w: rsa private key must not be nil", ErrInvalidKey)
	}

	if key.N.BitLen() < minRSAKeyBits {
		return nil, fmt.Errorf("%w: rsa key must be at least %d bits", ErrInvalidKey, minRSAKeyBits)
	}

	return &rsaPSSSigner{
		key:   key,
		keyID: keyID,
	}, nil
}

func (s *rsaPSSSigner) Sign(message []byte) ([]byte, error) {
	digest := sha512.Sum512(message)

	return rsa.SignPSS(rand.Reader, s.key, crypto.SHA512, digest[:], &rsa.PSSOptions{
		SaltLength: rsa.PSSSaltLengthEqualsHash,
	})
}

func (s *rsaPSSSigner) Algorithm() Algorithm { return AlgorithmRSAPSSSHA512 }
func (s *rsaPSSSigner) KeyID() string        { return s.keyID }

type rsaPSSVerifier struct {
	key   *rsa.PublicKey
	keyID string
}

// NewRSAPSSVerifier creates a Verifier using RSASSA-PSS with SHA-512.
func NewRSAPSSVerifier(keyID string, key *rsa.PublicKey) (Verifier, error) {
	if key == nil {
		return nil, fmt.Errorf("%w: rsa public key must not be nil", ErrInvalidKey)
	}

	if key.N.BitLen() < minRSAKeyBits {
		return nil, fmt.Errorf("%w: rsa key must be at least %d bits", ErrInvalidKey, minRSAKeyBits)
	}

	return &rsaPSSVerifier{
		key:   key,
		keyID: keyID,
	}, nil
}

func (v *rsaPSSVerifier) Verify(message, signature []byte) error {
	digest := sha512.Sum512(message)

	err := rsa.VerifyPSS(v.key, crypto.SHA512, digest[:], signature, &rsa.PSSOptions{
		SaltLength: rsa.PSSSaltLengthEqualsHash,
	})
	if err != nil {
		return ErrSignatureInvalid
	}

	return nil
}

func (v *rsaPSSVerifier) Algorithm() Algorithm { return AlgorithmRSAPSSSHA512 }
func (v *rsaPSSVerifier) KeyID() string        { return v.keyID }

// --- RSA v1.5 SHA-256 ---

type rsaV15Signer struct {
	key   *rsa.PrivateKey
	keyID string
}

// NewRSAv15Signer creates a Signer using RSASSA-PKCS1-v1_5 with SHA-256.
func NewRSAv15Signer(keyID string, key *rsa.PrivateKey) (Signer, error) {
	if key == nil {
		return nil, fmt.Errorf("%w: rsa private key must not be nil", ErrInvalidKey)
	}

	if key.N.BitLen() < minRSAKeyBits {
		return nil, fmt.Errorf("%w: rsa key must be at least %d bits", ErrInvalidKey, minRSAKeyBits)
	}

	return &rsaV15Signer{
		key:   key,
		keyID: keyID,
	}, nil
}

func (s *rsaV15Signer) Sign(message []byte) ([]byte, error) {
	digest := sha256.Sum256(message)

	return rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, digest[:])
}

func (s *rsaV15Signer) Algorithm() Algorithm { return AlgorithmRSAv15SHA256 }
func (s *rsaV15Signer) KeyID() string        { return s.keyID }

type rsaV15Verifier struct {
	key   *rsa.PublicKey
	keyID string
}

// NewRSAv15Verifier creates a Verifier using RSASSA-PKCS1-v1_5 with SHA-256.
func NewRSAv15Verifier(keyID string, key *rsa.PublicKey) (Verifier, error) {
	if key == nil {
		return nil, fmt.Errorf("%w: rsa public key must not be nil", ErrInvalidKey)
	}

	if key.N.BitLen() < minRSAKeyBits {
		return nil, fmt.Errorf("%w: rsa key must be at least %d bits", ErrInvalidKey, minRSAKeyBits)
	}

	return &rsaV15Verifier{
		key:   key,
		keyID: keyID,
	}, nil
}

func (v *rsaV15Verifier) Verify(message, signature []byte) error {
	digest := sha256.Sum256(message)

	err := rsa.VerifyPKCS1v15(v.key, crypto.SHA256, digest[:], signature)
	if err != nil {
		return ErrSignatureInvalid
	}

	return nil
}

func (v *rsaV15Verifier) Algorithm() Algorithm { return AlgorithmRSAv15SHA256 }
func (v *rsaV15Verifier) KeyID() string        { return v.keyID }

// --- HMAC SHA-256 ---

const minHMACKeyBytes = 32

type hmacSHA256Signer struct {
	key   []byte
	keyID string
}

// NewHMACSHA256Signer creates a Signer using HMAC-SHA256.
// The key must be at least 32 bytes.
func NewHMACSHA256Signer(keyID string, key []byte) (Signer, error) {
	if len(key) < minHMACKeyBytes {
		return nil, fmt.Errorf("%w: hmac key must be at least %d bytes", ErrInvalidKey, minHMACKeyBytes)
	}

	keyCopy := make([]byte, len(key))
	copy(keyCopy, key)

	return &hmacSHA256Signer{
		key:   keyCopy,
		keyID: keyID,
	}, nil
}

func (s *hmacSHA256Signer) Sign(message []byte) ([]byte, error) {
	return computeHMAC(s.key, message), nil
}

func (s *hmacSHA256Signer) Algorithm() Algorithm { return AlgorithmHMACSHA256 }
func (s *hmacSHA256Signer) KeyID() string        { return s.keyID }

type hmacSHA256Verifier struct {
	key   []byte
	keyID string
}

// NewHMACSHA256Verifier creates a Verifier using HMAC-SHA256.
// The key must be at least 32 bytes.
func NewHMACSHA256Verifier(keyID string, key []byte) (Verifier, error) {
	if len(key) < minHMACKeyBytes {
		return nil, fmt.Errorf("%w: hmac key must be at least %d bytes", ErrInvalidKey, minHMACKeyBytes)
	}

	keyCopy := make([]byte, len(key))
	copy(keyCopy, key)

	return &hmacSHA256Verifier{
		key:   keyCopy,
		keyID: keyID,
	}, nil
}

func (v *hmacSHA256Verifier) Verify(message, signature []byte) error {
	expected := computeHMAC(v.key, message)
	if !hmac.Equal(expected, signature) {
		return ErrSignatureInvalid
	}

	return nil
}

func (v *hmacSHA256Verifier) Algorithm() Algorithm { return AlgorithmHMACSHA256 }
func (v *hmacSHA256Verifier) KeyID() string        { return v.keyID }

func computeHMAC(key, message []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(message)

	return h.Sum(nil)
}
