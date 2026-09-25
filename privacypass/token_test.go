package privacypass

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenChallengeRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		c    TokenChallenge
	}{
		{
			name: "full",
			c: TokenChallenge{
				TokenType:         TokenType,
				IssuerName:        "issuer.example",
				RedemptionContext: bytes.Repeat([]byte{0xab}, 32),
				OriginInfo:        "origin.example",
			},
		},
		{
			name: "empty redemption context and origin",
			c: TokenChallenge{
				TokenType:  TokenType,
				IssuerName: "issuer.example",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc, err := tt.c.Marshal()
			require.NoError(t, err)

			got, err := UnmarshalTokenChallenge(enc)
			require.NoError(t, err)
			assert.Equal(t, tt.c.TokenType, got.TokenType)
			assert.Equal(t, tt.c.IssuerName, got.IssuerName)
			assert.Equal(t, tt.c.OriginInfo, got.OriginInfo)
			assert.Equal(t, len(tt.c.RedemptionContext), len(got.RedemptionContext))
			if len(tt.c.RedemptionContext) > 0 {
				assert.Equal(t, tt.c.RedemptionContext, got.RedemptionContext)
			}
		})
	}
}

func TestTokenChallengeMarshalErrors(t *testing.T) {
	t.Run("invalid redemption context length", func(t *testing.T) {
		c := TokenChallenge{TokenType: TokenType, RedemptionContext: []byte{1, 2, 3}}
		_, err := c.Marshal()
		require.ErrorIs(t, err, ErrMalformed)
	})
}

func TestUnmarshalTokenChallengeErrors(t *testing.T) {
	t.Run("truncated", func(t *testing.T) {
		_, err := UnmarshalTokenChallenge([]byte{0x00})
		require.ErrorIs(t, err, ErrMalformed)
	})
	t.Run("trailing bytes", func(t *testing.T) {
		c := TokenChallenge{TokenType: TokenType, IssuerName: "x"}
		enc, err := c.Marshal()
		require.NoError(t, err)
		_, err = UnmarshalTokenChallenge(append(enc, 0x00))
		require.ErrorIs(t, err, ErrMalformed)
	})
	t.Run("bad redemption context length", func(t *testing.T) {
		// token_type(2) + issuer vec16 empty + rc len=3 + 3 bytes + origin vec16 empty
		b := []byte{0x00, 0x02, 0x00, 0x00, 0x03, 0x01, 0x02, 0x03, 0x00, 0x00}
		_, err := UnmarshalTokenChallenge(b)
		require.ErrorIs(t, err, ErrMalformed)
	})
}

func TestTokenRoundTrip(t *testing.T) {
	tok := Token{
		TokenType:       TokenType,
		Nonce:           bytes.Repeat([]byte{0x01}, nonceSize),
		ChallengeDigest: bytes.Repeat([]byte{0x02}, digestSize),
		TokenKeyID:      bytes.Repeat([]byte{0x03}, keyIDSize),
		Authenticator:   bytes.Repeat([]byte{0x04}, authSize),
	}
	enc, err := tok.Marshal()
	require.NoError(t, err)
	assert.Len(t, enc, tokenSize)

	got, err := UnmarshalToken(enc)
	require.NoError(t, err)
	assert.Equal(t, tok.Nonce, got.Nonce)
	assert.Equal(t, tok.ChallengeDigest, got.ChallengeDigest)
	assert.Equal(t, tok.TokenKeyID, got.TokenKeyID)
	assert.Equal(t, tok.Authenticator, got.Authenticator)
}

func TestTokenMarshalErrors(t *testing.T) {
	tok := Token{TokenType: TokenType, Nonce: []byte{1}}
	_, err := tok.Marshal()
	require.ErrorIs(t, err, ErrMalformed)
}

func TestUnmarshalTokenErrors(t *testing.T) {
	t.Run("wrong length", func(t *testing.T) {
		_, err := UnmarshalToken(make([]byte, 10))
		require.ErrorIs(t, err, ErrMalformed)
	})
	t.Run("wrong token type", func(t *testing.T) {
		b := make([]byte, tokenSize)
		b[1] = 0x03 // token_type = 0x0003
		_, err := UnmarshalToken(b)
		require.ErrorIs(t, err, ErrWrongTokenType)
	})
}

func TestTokenRequestRoundTrip(t *testing.T) {
	tr := TokenRequest{
		TokenType:           TokenType,
		TruncatedTokenKeyID: 0x7f,
		BlindedMsg:          bytes.Repeat([]byte{0x05}, authSize),
	}
	enc, err := tr.Marshal()
	require.NoError(t, err)
	assert.Len(t, enc, requestSize)

	got, err := UnmarshalTokenRequest(enc)
	require.NoError(t, err)
	assert.Equal(t, tr.TruncatedTokenKeyID, got.TruncatedTokenKeyID)
	assert.Equal(t, tr.BlindedMsg, got.BlindedMsg)
}

func TestUnmarshalTokenRequestErrors(t *testing.T) {
	t.Run("wrong length", func(t *testing.T) {
		_, err := UnmarshalTokenRequest(make([]byte, 5))
		require.ErrorIs(t, err, ErrMalformed)
	})
	t.Run("wrong token type", func(t *testing.T) {
		b := make([]byte, requestSize)
		b[1] = 0x03
		_, err := UnmarshalTokenRequest(b)
		require.ErrorIs(t, err, ErrWrongTokenType)
	})
	t.Run("marshal bad blinded length", func(t *testing.T) {
		tr := TokenRequest{TokenType: TokenType, BlindedMsg: []byte{1, 2}}
		_, err := tr.Marshal()
		require.ErrorIs(t, err, ErrMalformed)
	})
}

func TestTokenInputLayout(t *testing.T) {
	nonce := bytes.Repeat([]byte{0x01}, nonceSize)
	digest := bytes.Repeat([]byte{0x02}, digestSize)
	keyID := bytes.Repeat([]byte{0x03}, keyIDSize)
	in := tokenInput(nonce, digest, keyID)
	require.Len(t, in, tokenInputSz)
	assert.Equal(t, byte(0x00), in[0])
	assert.Equal(t, byte(0x02), in[1]) // token_type 0x0002
	assert.Equal(t, nonce, in[2:34])
	assert.Equal(t, digest, in[34:66])
	assert.Equal(t, keyID, in[66:98])
}
