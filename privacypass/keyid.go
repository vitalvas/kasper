package privacypass

import (
	"crypto/rsa"
	"crypto/sha256"
	"encoding/asn1"
	"math/big"
)

// OIDs used in the id-RSASSA-PSS SubjectPublicKeyInfo required by RFC 9578 for
// token type 0x0002 (see RFC 4055 and RFC 8017).
var (
	oidRSASSAPSS = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 10}
	oidSHA384    = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 2}
	oidMGF1      = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 8}
)

// saltLength is the PSS salt length in bytes for token type 0x0002.
const saltLength = 48

// pssParams is the DER structure of RSASSA-PSS-params (RFC 4055 Section 3.1)
// with explicit hash, mask generation, and salt-length fields.
type pssParams struct {
	Hash       algorithmIdentifier `asn1:"explicit,tag:0"`
	MaskGen    mgfIdentifier       `asn1:"explicit,tag:1"`
	SaltLength int                 `asn1:"explicit,tag:2"`
	// TrailerField is omitted; its default (1) is the only allowed value.
}

type algorithmIdentifier struct {
	Algorithm  asn1.ObjectIdentifier
	Parameters asn1.RawValue `asn1:"optional"`
}

// mgfIdentifier is an AlgorithmIdentifier whose parameters are themselves an
// AlgorithmIdentifier (the MGF1 hash), per RFC 4055.
type mgfIdentifier struct {
	Algorithm asn1.ObjectIdentifier
	Hash      algorithmIdentifier
}

type rsaPublicKeyDER struct {
	N *big.Int
	E int
}

type subjectPublicKeyInfo struct {
	Algorithm        algorithmIdentifier
	SubjectPublicKey asn1.BitString
}

// nullRawValue is the DER encoding of ASN.1 NULL, used as the parameters of the
// SHA-384 and MGF1-inner hash algorithm identifiers.
var nullRawValue = asn1.RawValue{Tag: asn1.TagNull}

// MarshalPublicKey encodes an RSA public key as the id-RSASSA-PSS
// SubjectPublicKeyInfo (DER) that RFC 9578 uses for token type 0x0002. This is
// the encoding advertised in the token-key challenge parameter and hashed to
// produce the token_key_id.
//
// Go's x509.MarshalPKIXPublicKey emits the generic rsaEncryption OID, which
// would yield a different key id, so this package encodes the RSA-PSS form
// explicitly.
func MarshalPublicKey(pub *rsa.PublicKey) ([]byte, error) {
	sha384 := algorithmIdentifier{Algorithm: oidSHA384, Parameters: nullRawValue}
	params := pssParams{
		Hash: sha384,
		MaskGen: mgfIdentifier{
			Algorithm: oidMGF1,
			Hash:      sha384,
		},
		SaltLength: saltLength,
	}
	paramsDER, err := asn1.Marshal(params)
	if err != nil {
		return nil, err
	}

	pubDER, err := asn1.Marshal(rsaPublicKeyDER{N: pub.N, E: pub.E})
	if err != nil {
		return nil, err
	}

	spki := subjectPublicKeyInfo{
		Algorithm: algorithmIdentifier{
			Algorithm:  oidRSASSAPSS,
			Parameters: asn1.RawValue{FullBytes: paramsDER},
		},
		SubjectPublicKey: asn1.BitString{Bytes: pubDER, BitLength: len(pubDER) * 8},
	}
	return asn1.Marshal(spki)
}

// TokenKeyID returns the 32-byte token_key_id for an issuer public key:
// SHA-256 of the id-RSASSA-PSS SubjectPublicKeyInfo (RFC 9578).
func TokenKeyID(pub *rsa.PublicKey) ([]byte, error) {
	spki, err := MarshalPublicKey(pub)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(spki)
	return sum[:], nil
}

// truncatedKeyID returns the least significant byte of the token_key_id, used
// in the TokenRequest sent to the issuer.
func truncatedKeyID(keyID []byte) byte {
	return keyID[len(keyID)-1]
}
