package privacypass

import (
	"encoding/binary"
	"errors"
)

// TokenType is the Privacy Pass token type implemented by this package:
// 0x0002, Blind RSA (RFC 9578).
const TokenType uint16 = 0x0002

// Fixed field sizes per RFC 9578.
const (
	nonceSize    = 32  // Token.nonce
	digestSize   = 32  // challenge_digest (SHA-256)
	keyIDSize    = 32  // token_key_id (SHA-256)
	authSize     = 256 // authenticator / blinded_msg / blind_sig (Nk, RSA-2048)
	tokenInputSz = 2 + nonceSize + digestSize + keyIDSize
	tokenSize    = 2 + nonceSize + digestSize + keyIDSize + authSize // 354
	requestSize  = 2 + 1 + authSize                                  // 259
)

// Sentinel errors for malformed wire data.
var (
	// ErrMalformed is returned when a structure cannot be decoded because its
	// length or fields do not match the RFC 9578 / RFC 9577 layout.
	ErrMalformed = errors.New("privacypass: malformed structure")
	// ErrWrongTokenType is returned when a decoded structure carries a token
	// type other than 0x0002.
	ErrWrongTokenType = errors.New("privacypass: unsupported token type")
)

// TokenChallenge is the WWW-Authenticate challenge structure of RFC 9577
// Section 2.1.
type TokenChallenge struct {
	// TokenType identifies the token type; for this package it is TokenType.
	TokenType uint16
	// IssuerName is the issuer host name (opaque, up to 2^16-1 bytes).
	IssuerName string
	// RedemptionContext is empty or exactly 32 bytes.
	RedemptionContext []byte
	// OriginInfo is optional origin scoping (opaque, up to 2^16-1 bytes).
	OriginInfo string
}

// Marshal encodes the TokenChallenge to its byte form per RFC 9577 Section 2.1.
// It returns ErrMalformed if RedemptionContext is neither empty nor 32 bytes,
// or if a variable-length field exceeds 2^16-1 bytes.
func (c *TokenChallenge) Marshal() ([]byte, error) {
	if len(c.RedemptionContext) != 0 && len(c.RedemptionContext) != 32 {
		return nil, ErrMalformed
	}
	if len(c.IssuerName) > 0xffff || len(c.OriginInfo) > 0xffff {
		return nil, ErrMalformed
	}

	out := make([]byte, 0, 2+2+len(c.IssuerName)+1+len(c.RedemptionContext)+2+len(c.OriginInfo))
	out = binary.BigEndian.AppendUint16(out, c.TokenType)
	out = appendVec16(out, []byte(c.IssuerName))
	out = append(out, byte(len(c.RedemptionContext)))
	out = append(out, c.RedemptionContext...)
	out = appendVec16(out, []byte(c.OriginInfo))
	return out, nil
}

// UnmarshalTokenChallenge decodes a TokenChallenge from its byte form.
func UnmarshalTokenChallenge(b []byte) (*TokenChallenge, error) {
	r := &reader{b: b}
	var c TokenChallenge
	c.TokenType = r.uint16()
	c.IssuerName = string(r.vec16())
	rc := r.vec8()
	c.RedemptionContext = rc
	c.OriginInfo = string(r.vec16())
	if r.err || !r.done() {
		return nil, ErrMalformed
	}
	if len(rc) != 0 && len(rc) != 32 {
		return nil, ErrMalformed
	}
	return &c, nil
}

// Token is the redeemed token structure of RFC 9578 Section 2 (354 bytes).
type Token struct {
	TokenType       uint16
	Nonce           []byte // 32 bytes
	ChallengeDigest []byte // 32 bytes, SHA-256 of the TokenChallenge
	TokenKeyID      []byte // 32 bytes
	Authenticator   []byte // 256 bytes, RSA-PSS blind signature over token_input
}

// Marshal encodes the Token to its 354-byte form.
func (t *Token) Marshal() ([]byte, error) {
	if len(t.Nonce) != nonceSize || len(t.ChallengeDigest) != digestSize ||
		len(t.TokenKeyID) != keyIDSize || len(t.Authenticator) != authSize {
		return nil, ErrMalformed
	}
	out := make([]byte, 0, tokenSize)
	out = binary.BigEndian.AppendUint16(out, t.TokenType)
	out = append(out, t.Nonce...)
	out = append(out, t.ChallengeDigest...)
	out = append(out, t.TokenKeyID...)
	out = append(out, t.Authenticator...)
	return out, nil
}

// UnmarshalToken decodes a Token from its 354-byte form. It returns
// ErrWrongTokenType when the encoded token type is not 0x0002.
func UnmarshalToken(b []byte) (*Token, error) {
	if len(b) != tokenSize {
		return nil, ErrMalformed
	}
	t := &Token{
		TokenType:       binary.BigEndian.Uint16(b[0:2]),
		Nonce:           b[2:34],
		ChallengeDigest: b[34:66],
		TokenKeyID:      b[66:98],
		Authenticator:   b[98:354],
	}
	if t.TokenType != TokenType {
		return nil, ErrWrongTokenType
	}
	return t, nil
}

// tokenInput returns the 98-byte message that is blind-signed and later
// verified: token_type || nonce || challenge_digest || token_key_id.
func tokenInput(nonce, challengeDigest, keyID []byte) []byte {
	out := make([]byte, 0, tokenInputSz)
	out = binary.BigEndian.AppendUint16(out, TokenType)
	out = append(out, nonce...)
	out = append(out, challengeDigest...)
	out = append(out, keyID...)
	return out
}

// TokenRequest is sent to the issuer per RFC 9578 (259 bytes).
type TokenRequest struct {
	TokenType           uint16
	TruncatedTokenKeyID byte
	BlindedMsg          []byte // 256 bytes
}

// Marshal encodes the TokenRequest to its 259-byte form.
func (tr *TokenRequest) Marshal() ([]byte, error) {
	if len(tr.BlindedMsg) != authSize {
		return nil, ErrMalformed
	}
	out := make([]byte, 0, requestSize)
	out = binary.BigEndian.AppendUint16(out, tr.TokenType)
	out = append(out, tr.TruncatedTokenKeyID)
	out = append(out, tr.BlindedMsg...)
	return out, nil
}

// UnmarshalTokenRequest decodes a TokenRequest from its 259-byte form.
func UnmarshalTokenRequest(b []byte) (*TokenRequest, error) {
	if len(b) != requestSize {
		return nil, ErrMalformed
	}
	tr := &TokenRequest{
		TokenType:           binary.BigEndian.Uint16(b[0:2]),
		TruncatedTokenKeyID: b[2],
		BlindedMsg:          b[3:259],
	}
	if tr.TokenType != TokenType {
		return nil, ErrWrongTokenType
	}
	return tr, nil
}

// --- variable-length vector helpers ---

func appendVec16(dst, v []byte) []byte {
	dst = binary.BigEndian.AppendUint16(dst, uint16(len(v)))
	return append(dst, v...)
}

// reader is a minimal cursor over a byte slice for decoding. On any
// out-of-bounds read it sets err and returns zero values.
type reader struct {
	b   []byte
	pos int
	err bool
}

func (r *reader) done() bool { return r.pos == len(r.b) }

func (r *reader) take(n int) []byte {
	if r.err || r.pos+n > len(r.b) {
		r.err = true
		return nil
	}
	out := r.b[r.pos : r.pos+n]
	r.pos += n
	return out
}

func (r *reader) uint16() uint16 {
	b := r.take(2)
	if b == nil {
		return 0
	}
	return binary.BigEndian.Uint16(b)
}

func (r *reader) vec8() []byte {
	b := r.take(1)
	if b == nil {
		return nil
	}
	return r.take(int(b[0]))
}

func (r *reader) vec16() []byte {
	b := r.take(2)
	if b == nil {
		return nil
	}
	return r.take(int(binary.BigEndian.Uint16(b)))
}
