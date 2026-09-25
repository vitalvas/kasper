package privacypass

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildChallengeHeader(t *testing.T) {
	priv := testRSAKey(t)
	challenge := testChallenge()

	header, err := BuildChallengeHeader(challenge, &priv.PublicKey)
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(header, "PrivateToken "))
	assert.Contains(t, header, `challenge="`)
	assert.Contains(t, header, `token-key="`)

	// The challenge parameter decodes back to the original structure.
	value, ok := authParam(strings.TrimPrefix(header, "PrivateToken "), "challenge")
	require.True(t, ok)
	raw, err := b64.DecodeString(value)
	require.NoError(t, err)
	got, err := UnmarshalTokenChallenge(raw)
	require.NoError(t, err)
	assert.Equal(t, challenge.IssuerName, got.IssuerName)
	assert.Equal(t, challenge.OriginInfo, got.OriginInfo)
}

func TestAuthorizationHeaderRoundTrip(t *testing.T) {
	priv := testRSAKey(t)
	tok := issueToken(t, priv, testChallenge())

	header, err := BuildAuthorizationHeader(tok)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(header, `PrivateToken token="`))

	got, err := ParseAuthorizationHeader(header)
	require.NoError(t, err)
	assert.Equal(t, tok.Nonce, got.Nonce)
	assert.Equal(t, tok.Authenticator, got.Authenticator)
}

func TestBuildAuthorizationHeaderMalformed(t *testing.T) {
	// A Token with wrong field sizes fails to marshal.
	_, err := BuildAuthorizationHeader(&Token{TokenType: TokenType, Nonce: []byte{1}})
	require.ErrorIs(t, err, ErrMalformed)
}

func TestBuildChallengeHeaderMalformed(t *testing.T) {
	priv := testRSAKey(t)
	bad := &TokenChallenge{TokenType: TokenType, RedemptionContext: []byte{1, 2, 3}}
	_, err := BuildChallengeHeader(bad, &priv.PublicKey)
	require.ErrorIs(t, err, ErrMalformed)
}

func TestParseAuthorizationHeaderErrors(t *testing.T) {
	tests := []struct {
		name    string
		header  string
		wantErr error
	}{
		{"empty", "", ErrNoToken},
		{"wrong scheme", `Bearer abc`, ErrNoToken},
		{"no token param", `PrivateToken foo="bar"`, ErrNoToken},
		{"bad base64", `PrivateToken token="!!!not base64!!!"`, ErrMalformed},
		{"wrong length", `PrivateToken token="YWJj"`, ErrMalformed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseAuthorizationHeader(tt.header)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}
