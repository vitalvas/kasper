package blindrsa

import (
	"crypto/rsa"
	"crypto/sha512"
	"fmt"
	"io"
	"math/big"
)

// randomPrefixLen is the length of the random prefix prepended to the
// message for randomized variants, as defined in RFC 9474 Section 4.1.
const randomPrefixLen = 32

// State holds the blinding state produced by [Blind] and consumed by
// [Finalize]. It is not safe for concurrent use.
type State struct {
	blindInv *big.Int
	inputMsg []byte
}

// InputMessage returns the prepared message used for blinding. For randomized
// variants this includes the random prefix prepended to the original message.
// The caller must pass this value (not the original message) to [Verify].
func (s *State) InputMessage() []byte {
	out := make([]byte, len(s.inputMsg))
	copy(out, s.inputMsg)

	return out
}

// Prepare prepares a message for blinding per RFC 9474 Section 4.1.
//
// For randomized variants, a 32-byte random prefix is prepended to the
// message. For deterministic variants, a copy of the message is returned.
// The random reader is used for prefix generation; use [crypto/rand.Reader]
// in production.
func Prepare(variant Variant, random io.Reader, msg []byte) ([]byte, error) {
	if _, err := validateVariant(variant); err != nil {
		return nil, err
	}

	if msg == nil {
		return nil, fmt.Errorf("%w: message must not be nil", ErrInvalidInput)
	}

	if variant.isRandomized() {
		if random == nil {
			return nil, fmt.Errorf("%w: random source must not be nil", ErrInvalidInput)
		}

		prefix := make([]byte, randomPrefixLen)
		if _, err := io.ReadFull(random, prefix); err != nil {
			return nil, fmt.Errorf("%w: %s", ErrBlindingFailed, err)
		}

		preparedMsg := make([]byte, randomPrefixLen+len(msg))
		copy(preparedMsg, prefix)
		copy(preparedMsg[randomPrefixLen:], msg)

		return preparedMsg, nil
	}

	preparedMsg := make([]byte, len(msg))
	copy(preparedMsg, msg)

	return preparedMsg, nil
}

// Blind blinds a prepared message per RFC 9474 Section 4.2.
//
// The preparedMsg must be the output of [Prepare]. The random reader is used
// for PSS salt and blinding factor generation; use [crypto/rand.Reader] in
// production. The returned [State] must be passed to [Finalize].
func Blind(variant Variant, pub *rsa.PublicKey, random io.Reader, preparedMsg []byte) ([]byte, *State, error) {
	if err := validatePublicKey(pub); err != nil {
		return nil, nil, err
	}

	sLen, err := validateVariant(variant)
	if err != nil {
		return nil, nil, err
	}

	if len(preparedMsg) == 0 {
		return nil, nil, fmt.Errorf("%w: prepared message must not be empty", ErrInvalidInput)
	}

	if random == nil {
		return nil, nil, fmt.Errorf("%w: random source must not be nil", ErrInvalidInput)
	}

	kLen := keyLen(pub)

	// Hash the prepared message.
	mHash := sha512.Sum384(preparedMsg)

	// PSS-encode the hash.
	emBits := pub.N.BitLen() - 1
	encoded, err := emsaPSSEncode(random, mHash[:], emBits, sLen)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %s", ErrBlindingFailed, err)
	}

	// Convert to integer.
	m := os2ip(encoded)

	// Check is_coprime(m, n) per RFC 9474 Section 4.2 Step 4.
	gcd := new(big.Int).GCD(nil, nil, m, pub.N)
	if gcd.Cmp(bigOne) != 0 {
		return nil, nil, fmt.Errorf("%w: encoded message not coprime to modulus", ErrBlindingFailed)
	}

	// Generate blinding factor.
	rE, rInv, err := generateBlindingFactor(random, pub)
	if err != nil {
		return nil, nil, err
	}

	// Blind: x = m * r^e mod n
	x := new(big.Int).Mul(m, rE)
	x.Mod(x, pub.N)

	blindedMsg, err := i2osp(x, kLen)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %s", ErrBlindingFailed, err)
	}

	state := &State{
		blindInv: rInv,
		inputMsg: preparedMsg,
	}

	return blindedMsg, state, nil
}

// fixedBlindParams holds predetermined cryptographic parameters for test
// vector validation per RFC 9474.
type fixedBlindParams struct {
	variant     Variant
	pub         *rsa.PublicKey
	preparedMsg []byte
	salt        []byte
	r           *big.Int
	rInv        *big.Int
}

// fixedBlind is like [Blind] but uses predetermined cryptographic parameters
// instead of random ones. It is used for RFC 9474 test vector validation.
//
// The salt is used directly in PSS encoding (instead of random generation).
// r and rInv are used as the blinding factor and its inverse (instead of
// random generation). The caller must ensure r * rInv ≡ 1 (mod n).
func fixedBlind(p fixedBlindParams) ([]byte, *State, error) {
	if err := validatePublicKey(p.pub); err != nil {
		return nil, nil, err
	}

	sLen, err := validateVariant(p.variant)
	if err != nil {
		return nil, nil, err
	}

	if len(p.preparedMsg) == 0 {
		return nil, nil, fmt.Errorf("%w: prepared message must not be empty", ErrInvalidInput)
	}

	if len(p.salt) != sLen {
		return nil, nil, fmt.Errorf("%w: salt must be %d bytes, got %d", ErrInvalidInput, sLen, len(p.salt))
	}

	kLen := keyLen(p.pub)

	mHash := sha512.Sum384(p.preparedMsg)

	emBits := p.pub.N.BitLen() - 1
	encoded, err := emsaPSSEncodeWithSalt(mHash[:], emBits, p.salt)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %s", ErrBlindingFailed, err)
	}

	m := os2ip(encoded)

	gcd := new(big.Int).GCD(nil, nil, m, p.pub.N)
	if gcd.Cmp(bigOne) != 0 {
		return nil, nil, fmt.Errorf("%w: encoded message not coprime to modulus", ErrBlindingFailed)
	}

	e := big.NewInt(int64(p.pub.E))
	rE := new(big.Int).Exp(p.r, e, p.pub.N)

	x := new(big.Int).Mul(m, rE)
	x.Mod(x, p.pub.N)

	blindedMsg, err := i2osp(x, kLen)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %s", ErrBlindingFailed, err)
	}

	state := &State{
		blindInv: p.rInv,
		inputMsg: p.preparedMsg,
	}

	return blindedMsg, state, nil
}
