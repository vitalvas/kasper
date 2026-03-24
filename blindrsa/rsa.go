package blindrsa

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha512"
	"encoding/binary"
	"fmt"
	"io"
	"math/big"
)

const (
	// minRSAKeyBits is the minimum allowed RSA key size.
	minRSAKeyBits = 2048

	// minRSAPublicExponent is the minimum allowed RSA public exponent
	// per FIPS 186-5 as referenced by RFC 9474 Section 6.2.
	minRSAPublicExponent = 65537
)

var bigOne = big.NewInt(1)

// validatePublicKey checks that pub is non-nil, at least minRSAKeyBits,
// and has a public exponent of at least 65537.
func validatePublicKey(pub *rsa.PublicKey) error {
	if pub == nil {
		return fmt.Errorf("%w: rsa public key must not be nil", ErrInvalidKey)
	}

	if pub.N.BitLen() < minRSAKeyBits {
		return fmt.Errorf("%w: rsa key must be at least %d bits", ErrInvalidKey, minRSAKeyBits)
	}

	if pub.E < minRSAPublicExponent {
		return fmt.Errorf("%w: rsa public exponent must be at least %d", ErrInvalidKey, minRSAPublicExponent)
	}

	return nil
}

// validatePrivateKey checks that priv is non-nil, at least minRSAKeyBits,
// and has a public exponent of at least 65537.
func validatePrivateKey(priv *rsa.PrivateKey) error {
	if priv == nil {
		return fmt.Errorf("%w: rsa private key must not be nil", ErrInvalidKey)
	}

	if priv.N.BitLen() < minRSAKeyBits {
		return fmt.Errorf("%w: rsa key must be at least %d bits", ErrInvalidKey, minRSAKeyBits)
	}

	if priv.E < minRSAPublicExponent {
		return fmt.Errorf("%w: rsa public exponent must be at least %d", ErrInvalidKey, minRSAPublicExponent)
	}

	return nil
}

// keyLen returns the byte length of the RSA modulus (kLen in RFC 9474).
func keyLen(pub *rsa.PublicKey) int {
	return (pub.N.BitLen() + 7) / 8
}

// validateVariant checks that the variant is one of the four defined
// variants and returns its salt length.
func validateVariant(v Variant) (int, error) {
	sLen := v.saltLength()
	if sLen < 0 {
		return 0, fmt.Errorf("%w: %s", ErrUnsupportedVariant, v)
	}

	return sLen, nil
}

// generateBlindingFactor generates a random blinding factor r and returns
// r^e mod n (for blinding) and r^(-1) mod n (for unblinding).
func generateBlindingFactor(random io.Reader, pub *rsa.PublicKey) (rE, rInv *big.Int, err error) {
	n := pub.N
	e := big.NewInt(int64(pub.E))

	for {
		r, err := rand.Int(random, n)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %s", ErrBlindingFailed, err)
		}

		if r.Sign() == 0 {
			continue
		}

		gcd := new(big.Int).GCD(nil, nil, r, n)
		if gcd.Cmp(bigOne) != 0 {
			continue
		}

		inv := new(big.Int).ModInverse(r, n)
		if inv == nil {
			continue
		}

		rPowE := new(big.Int).Exp(r, e, n)

		return rPowE, inv, nil
	}
}

// emsaPSSEncode implements EMSA-PSS-ENCODE from RFC 8017 Section 9.1.1
// using SHA-384 as the hash function. The random reader is used for salt
// generation.
func emsaPSSEncode(random io.Reader, mHash []byte, emBits, sLen int) ([]byte, error) {
	salt := make([]byte, sLen)
	if sLen > 0 {
		if _, err := io.ReadFull(random, salt); err != nil {
			return nil, fmt.Errorf("%w: %s", ErrBlindingFailed, err)
		}
	}

	return emsaPSSEncodeWithSalt(mHash, emBits, salt)
}

// emsaPSSEncodeWithSalt implements EMSA-PSS-ENCODE from RFC 8017 Section 9.1.1
// using SHA-384 and a caller-provided salt. Used for RFC 9474 test vector
// validation via [fixedBlind].
func emsaPSSEncodeWithSalt(mHash []byte, emBits int, salt []byte) ([]byte, error) {
	hLen := sha384Size
	sLen := len(salt)

	if len(mHash) != hLen {
		return nil, fmt.Errorf("%w: hash must be %d bytes", ErrInvalidInput, hLen)
	}

	emLen := (emBits + 7) / 8
	if emLen < hLen+sLen+2 {
		return nil, ErrMessageTooLong
	}

	// M' = 0x00{8} || mHash || salt
	mPrime := make([]byte, 8+hLen+sLen)
	copy(mPrime[8:], mHash)
	copy(mPrime[8+hLen:], salt)

	// H = SHA-384(M')
	h := sha512.Sum384(mPrime)

	// DB = PS || 0x01 || salt
	dbLen := emLen - hLen - 1
	db := make([]byte, dbLen)
	db[dbLen-sLen-1] = 0x01
	copy(db[dbLen-sLen:], salt)

	// dbMask = MGF1(H, dbLen)
	dbMask := mgf1SHA384(h[:], dbLen)

	// maskedDB = DB XOR dbMask
	maskedDB := make([]byte, dbLen)
	for i := range maskedDB {
		maskedDB[i] = db[i] ^ dbMask[i]
	}

	// Zero the leftmost (8*emLen - emBits) bits
	zeroBits := 8*emLen - emBits
	if zeroBits > 0 {
		maskedDB[0] &= 0xFF >> zeroBits
	}

	// EM = maskedDB || H || 0xBC
	em := make([]byte, emLen)
	copy(em, maskedDB)
	copy(em[dbLen:], h[:])
	em[emLen-1] = 0xBC

	return em, nil
}

// mgf1SHA384 implements MGF1 from RFC 8017 Section B.2.1 using SHA-384.
func mgf1SHA384(seed []byte, maskLen int) []byte {
	var out []byte
	var counter [4]byte

	for i := 0; len(out) < maskLen; i++ {
		binary.BigEndian.PutUint32(counter[:], uint32(i))

		h := sha512.New384()
		h.Write(seed)
		h.Write(counter[:])
		out = append(out, h.Sum(nil)...)
	}

	return out[:maskLen]
}

// i2osp converts a non-negative big.Int to a byte slice of length xLen
// (Integer-to-Octet-String Primitive, RFC 8017 Section 4.1).
func i2osp(x *big.Int, xLen int) ([]byte, error) {
	if x.Sign() < 0 {
		return nil, fmt.Errorf("%w: negative integer", ErrInvalidInput)
	}

	b := x.Bytes()
	if len(b) > xLen {
		return nil, fmt.Errorf("%w: integer too large for %d bytes", ErrMessageTooLong, xLen)
	}

	result := make([]byte, xLen)
	copy(result[xLen-len(b):], b)

	return result, nil
}

// os2ip converts a byte slice to a non-negative big.Int
// (Octet-String-to-Integer Primitive, RFC 8017 Section 4.2).
func os2ip(x []byte) *big.Int {
	return new(big.Int).SetBytes(x)
}
