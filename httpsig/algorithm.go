package httpsig

// Algorithm identifies the HTTP message signature algorithm per RFC 9421
// Section 3.3.
type Algorithm string

const (
	// AlgorithmRSAPSSSHA512 is RSASSA-PSS using SHA-512.
	AlgorithmRSAPSSSHA512 Algorithm = "rsa-pss-sha512"

	// AlgorithmRSAv15SHA256 is RSASSA-PKCS1-v1_5 using SHA-256.
	AlgorithmRSAv15SHA256 Algorithm = "rsa-v1_5-sha256"

	// AlgorithmHMACSHA256 is HMAC using SHA-256.
	AlgorithmHMACSHA256 Algorithm = "hmac-sha256"

	// AlgorithmECDSAP256SHA256 is ECDSA using curve P-256 and SHA-256.
	AlgorithmECDSAP256SHA256 Algorithm = "ecdsa-p256-sha256"

	// AlgorithmECDSAP384SHA384 is ECDSA using curve P-384 and SHA-384.
	AlgorithmECDSAP384SHA384 Algorithm = "ecdsa-p384-sha384"

	// AlgorithmEd25519 is Edwards-Curve Digital Signature Algorithm
	// using curve 25519.
	AlgorithmEd25519 Algorithm = "ed25519"
)

// String returns the string representation of the algorithm as registered
// in the HTTP Signature Algorithms Registry.
func (a Algorithm) String() string {
	return string(a)
}

// Signer creates signatures over HTTP message signature base strings.
type Signer interface {
	// Sign produces a signature over the given message bytes.
	Sign(message []byte) ([]byte, error)

	// Algorithm returns the algorithm identifier for this signer.
	Algorithm() Algorithm

	// KeyID returns the key identifier included in signature parameters.
	KeyID() string
}

// Verifier validates signatures over HTTP message signature base strings.
type Verifier interface {
	// Verify checks that signature is valid for the given message bytes.
	// Returns nil on success, non-nil on failure.
	Verify(message, signature []byte) error

	// Algorithm returns the algorithm identifier for this verifier.
	Algorithm() Algorithm

	// KeyID returns the key identifier for this verifier.
	KeyID() string
}
