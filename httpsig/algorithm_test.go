package httpsig

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAlgorithmString(t *testing.T) {
	tests := []struct {
		name string
		alg  Algorithm
		want string
	}{
		{name: "rsa-pss-sha512", alg: AlgorithmRSAPSSSHA512, want: "rsa-pss-sha512"},
		{name: "rsa-v1_5-sha256", alg: AlgorithmRSAv15SHA256, want: "rsa-v1_5-sha256"},
		{name: "hmac-sha256", alg: AlgorithmHMACSHA256, want: "hmac-sha256"},
		{name: "ecdsa-p256-sha256", alg: AlgorithmECDSAP256SHA256, want: "ecdsa-p256-sha256"},
		{name: "ecdsa-p384-sha384", alg: AlgorithmECDSAP384SHA384, want: "ecdsa-p384-sha384"},
		{name: "ed25519", alg: AlgorithmEd25519, want: "ed25519"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.alg.String())
		})
	}
}

func TestAlgorithmConstants(t *testing.T) {
	t.Run("all algorithms are distinct", func(t *testing.T) {
		algorithms := []Algorithm{
			AlgorithmRSAPSSSHA512,
			AlgorithmRSAv15SHA256,
			AlgorithmHMACSHA256,
			AlgorithmECDSAP256SHA256,
			AlgorithmECDSAP384SHA384,
			AlgorithmEd25519,
		}

		seen := make(map[Algorithm]bool, len(algorithms))
		for _, alg := range algorithms {
			assert.False(t, seen[alg], "duplicate algorithm: %s", alg)
			seen[alg] = true
		}
	})

	t.Run("algorithms are not empty", func(t *testing.T) {
		algorithms := []Algorithm{
			AlgorithmRSAPSSSHA512,
			AlgorithmRSAv15SHA256,
			AlgorithmHMACSHA256,
			AlgorithmECDSAP256SHA256,
			AlgorithmECDSAP384SHA384,
			AlgorithmEd25519,
		}

		for _, alg := range algorithms {
			assert.NotEmpty(t, alg.String())
		}
	})
}
