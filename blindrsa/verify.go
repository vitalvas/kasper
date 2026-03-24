package blindrsa

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha512"
	"fmt"
)

// Verify checks an RSA-PSS signature produced by the blind signature
// protocol per RFC 9474 Section 4.5.
//
// For randomized variants, inputMsg must be the prepared message (including
// the random prefix) obtained from [State.InputMessage], not the original
// message.
func Verify(variant Variant, pub *rsa.PublicKey, inputMsg, sig []byte) error {
	if err := validatePublicKey(pub); err != nil {
		return err
	}

	sLen, err := validateVariant(variant)
	if err != nil {
		return err
	}

	if len(inputMsg) == 0 {
		return fmt.Errorf("%w: input message must not be empty", ErrInvalidInput)
	}

	if len(sig) == 0 {
		return fmt.Errorf("%w: signature must not be empty", ErrInvalidInput)
	}

	mHash := sha512.Sum384(inputMsg)

	verifyErr := rsa.VerifyPSS(pub, crypto.SHA384, mHash[:], sig, &rsa.PSSOptions{
		SaltLength: sLen,
	})
	if verifyErr != nil {
		return ErrVerifyFailed
	}

	return nil
}
