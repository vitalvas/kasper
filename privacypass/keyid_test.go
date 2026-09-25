package privacypass

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return k
}

func TestMarshalPublicKeyStructure(t *testing.T) {
	priv := testRSAKey(t)
	spki, err := MarshalPublicKey(&priv.PublicKey)
	require.NoError(t, err)

	// Go's x509 does not parse the id-RSASSA-PSS OID for RSA keys, so verify
	// the structure directly: decode the SPKI, then the inner RSAPublicKey,
	// and confirm the recovered modulus and exponent match.
	var info subjectPublicKeyInfo
	rest, err := asn1.Unmarshal(spki, &info)
	require.NoError(t, err)
	assert.Empty(t, rest)
	assert.True(t, info.Algorithm.Algorithm.Equal(oidRSASSAPSS))

	var params pssParams
	_, err = asn1.Unmarshal(info.Algorithm.Parameters.FullBytes, &params)
	require.NoError(t, err)
	assert.True(t, params.Hash.Algorithm.Equal(oidSHA384))
	assert.True(t, params.MaskGen.Algorithm.Equal(oidMGF1))
	assert.True(t, params.MaskGen.Hash.Algorithm.Equal(oidSHA384))
	assert.Equal(t, saltLength, params.SaltLength)

	var pub rsaPublicKeyDER
	_, err = asn1.Unmarshal(info.SubjectPublicKey.Bytes, &pub)
	require.NoError(t, err)
	assert.Equal(t, priv.N, pub.N)
	assert.Equal(t, priv.E, pub.E)
}

func TestMarshalPublicKeyUsesRSAPSSOID(t *testing.T) {
	priv := testRSAKey(t)
	spki, err := MarshalPublicKey(&priv.PublicKey)
	require.NoError(t, err)

	var info subjectPublicKeyInfo
	_, err = asn1.Unmarshal(spki, &info)
	require.NoError(t, err)
	// Must be id-RSASSA-PSS, not the generic rsaEncryption OID that
	// x509.MarshalPKIXPublicKey would emit.
	assert.True(t, info.Algorithm.Algorithm.Equal(oidRSASSAPSS))

	// x509's default encoding must differ (different OID), proving we are not
	// accidentally producing the generic form.
	generic, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	require.NoError(t, err)
	assert.NotEqual(t, generic, spki)
}

func TestTokenKeyIDDeterministic(t *testing.T) {
	priv := testRSAKey(t)
	id1, err := TokenKeyID(&priv.PublicKey)
	require.NoError(t, err)
	id2, err := TokenKeyID(&priv.PublicKey)
	require.NoError(t, err)
	assert.Len(t, id1, keyIDSize)
	assert.Equal(t, id1, id2)

	// It is the SHA-256 of the id-RSASSA-PSS SPKI.
	spki, err := MarshalPublicKey(&priv.PublicKey)
	require.NoError(t, err)
	want := sha256.Sum256(spki)
	assert.Equal(t, want[:], id1)
}

func TestTokenKeyIDDistinctKeys(t *testing.T) {
	a := testRSAKey(t)
	b := testRSAKey(t)
	idA, err := TokenKeyID(&a.PublicKey)
	require.NoError(t, err)
	idB, err := TokenKeyID(&b.PublicKey)
	require.NoError(t, err)
	assert.NotEqual(t, idA, idB)
}

func TestTruncatedKeyID(t *testing.T) {
	id := make([]byte, keyIDSize)
	id[keyIDSize-1] = 0x9c
	assert.Equal(t, byte(0x9c), truncatedKeyID(id))
}
