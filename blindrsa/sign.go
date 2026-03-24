package blindrsa

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"math/big"
)

// BlindSign produces a blind signature over a blinded message per
// RFC 9474 Section 4.3. The server does not learn the original message.
//
// The private key operation uses randomized RSA blinding internally to
// protect against timing side-channel attacks on the private exponent.
func BlindSign(variant Variant, priv *rsa.PrivateKey, blindedMsg []byte) ([]byte, error) {
	if err := validatePrivateKey(priv); err != nil {
		return nil, err
	}

	if _, err := validateVariant(variant); err != nil {
		return nil, err
	}

	if len(blindedMsg) == 0 {
		return nil, fmt.Errorf("%w: blinded message must not be empty", ErrInvalidInput)
	}

	kLen := keyLen(&priv.PublicKey)
	if len(blindedMsg) != kLen {
		return nil, fmt.Errorf("%w: blinded message must be %d bytes, got %d", ErrInvalidInput, kLen, len(blindedMsg))
	}

	// Convert to integer and check range.
	m := os2ip(blindedMsg)
	if m.Cmp(priv.N) >= 0 {
		return nil, fmt.Errorf("%w: blinded message representative out of range", ErrSignatureFailed)
	}

	s, err := rsaPrivateOp(priv, m)
	if err != nil {
		return nil, err
	}

	blindSig, err := i2osp(s, kLen)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrSignatureFailed, err)
	}

	return blindSig, nil
}

// rsaPrivateOp computes m^d mod n with side-channel protection and fault
// checking. It randomizes the exponentiation input to prevent timing
// analysis of the private exponent d, then verifies the result.
func rsaPrivateOp(priv *rsa.PrivateKey, m *big.Int) (*big.Int, error) {
	// Side-channel protection: randomize the exponentiation input.
	// Generate random r, compute m' = m * r^e mod n, then s' = m'^d mod n,
	// then s = s' * r^(-1) mod n. The exponentiation operates on the
	// randomized m', hiding timing correlation with the actual input.
	rE, rInv, err := generateBlindingFactor(rand.Reader, &priv.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrSignatureFailed, err)
	}

	mBlinded := new(big.Int).Mul(m, rE)
	mBlinded.Mod(mBlinded, priv.N)

	sBlinded := new(big.Int).Exp(mBlinded, priv.D, priv.N)

	s := new(big.Int).Mul(sBlinded, rInv)
	s.Mod(s, priv.N)

	// Fault attack protection per RFC 9474 Section 4.3:
	// verify s^e mod n == m before returning.
	e := big.NewInt(int64(priv.E))
	check := new(big.Int).Exp(s, e, priv.N)
	if check.Cmp(m) != 0 {
		return nil, fmt.Errorf("%w: blind signature fault check failed", ErrSignatureFailed)
	}

	return s, nil
}
