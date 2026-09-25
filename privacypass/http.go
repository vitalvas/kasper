package privacypass

import (
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"strings"
)

// authScheme is the RFC 9577 HTTP authentication scheme name.
const authScheme = "PrivateToken"

// HTTP header errors.
var (
	// ErrNoToken is returned when an Authorization header carries no
	// PrivateToken credential.
	ErrNoToken = errors.New("privacypass: no token in authorization header")
)

// b64 is base64url WITH padding, as required by RFC 9577 for the challenge,
// token-key, and token header parameters.
var b64 = base64.URLEncoding

// BuildChallengeHeader returns the WWW-Authenticate header value advertising a
// PrivateToken challenge for the given challenge and issuer public key, per
// RFC 9577 Section 2.1. Both the challenge and token-key are base64url encoded
// with padding.
func BuildChallengeHeader(challenge *TokenChallenge, pub *rsa.PublicKey) (string, error) {
	enc, err := challenge.Marshal()
	if err != nil {
		return "", err
	}
	keyDER, err := MarshalPublicKey(pub)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString(authScheme)
	b.WriteString(` challenge="`)
	b.WriteString(b64.EncodeToString(enc))
	b.WriteString(`", token-key="`)
	b.WriteString(b64.EncodeToString(keyDER))
	b.WriteString(`"`)
	return b.String(), nil
}

// BuildAuthorizationHeader returns the Authorization header value that redeems
// a token, per RFC 9577 Section 2.2: PrivateToken token="<base64url token>".
func BuildAuthorizationHeader(tok *Token) (string, error) {
	enc, err := tok.Marshal()
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString(authScheme)
	b.WriteString(` token="`)
	b.WriteString(b64.EncodeToString(enc))
	b.WriteByte('"')
	return b.String(), nil
}

// ParseAuthorizationHeader extracts and decodes a Token from an Authorization
// header value of the form: PrivateToken token="<base64url token>". It returns
// ErrNoToken when the header does not carry a PrivateToken credential and
// ErrMalformed when the token cannot be decoded.
func ParseAuthorizationHeader(header string) (*Token, error) {
	scheme, rest, ok := strings.Cut(strings.TrimSpace(header), " ")
	if !ok || scheme != authScheme {
		return nil, ErrNoToken
	}
	value, ok := authParam(rest, "token")
	if !ok {
		return nil, ErrNoToken
	}
	raw, err := b64.DecodeString(value)
	if err != nil {
		return nil, ErrMalformed
	}
	return UnmarshalToken(raw)
}

// authParam extracts the value of key from a comma-separated list of
// key="value" auth parameters. It returns the unquoted value and whether the
// key was found.
func authParam(params, key string) (string, bool) {
	for part := range strings.SplitSeq(params, ",") {
		part = strings.TrimSpace(part)
		name, value, ok := strings.Cut(part, "=")
		if !ok || strings.TrimSpace(name) != key {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
			return value[1 : len(value)-1], true
		}
		return value, true
	}
	return "", false
}
