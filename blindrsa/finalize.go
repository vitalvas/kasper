package blindrsa

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha512"
	"fmt"
	"math/big"
)

// Finalize unblinds a blind signature and verifies the result per
// RFC 9474 Section 4.4.
//
// It returns the final RSA-PSS signature that can be verified by anyone
// using [Verify] with the prepared message from [State.InputMessage].
func Finalize(variant Variant, pub *rsa.PublicKey, blindSig []byte, state *State) ([]byte, error) {
	if err := validatePublicKey(pub); err != nil {
		return nil, err
	}

	sLen, err := validateVariant(variant)
	if err != nil {
		return nil, err
	}

	if state == nil {
		return nil, fmt.Errorf("%w: state must not be nil", ErrInvalidInput)
	}

	kLen := keyLen(pub)
	if len(blindSig) != kLen {
		return nil, fmt.Errorf("%w: blind signature must be %d bytes, got %d", ErrInvalidInput, kLen, len(blindSig))
	}

	// Unblind: s = z * r_inv mod n.
	z := os2ip(blindSig)
	s := new(big.Int).Mul(z, state.blindInv)
	s.Mod(s, pub.N)

	sig, err := i2osp(s, kLen)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrFinalizeFailed, err)
	}

	// Verify the unblinded signature against the prepared message.
	mHash := sha512.Sum384(state.inputMsg)

	verifyErr := rsa.VerifyPSS(pub, crypto.SHA384, mHash[:], sig, &rsa.PSSOptions{
		SaltLength: sLen,
	})
	if verifyErr != nil {
		return nil, fmt.Errorf("%w: signature verification after unblinding failed", ErrFinalizeFailed)
	}

	return sig, nil
}
