package blindrsa

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVariant(t *testing.T) {
	t.Run("String", func(t *testing.T) {
		tests := []struct {
			variant  Variant
			expected string
		}{
			{VariantSHA384PSSRandomized, "RSABSSA-SHA384-PSS-Randomized"},
			{VariantSHA384PSSZERORandomized, "RSABSSA-SHA384-PSSZERO-Randomized"},
			{VariantSHA384PSSDeterministic, "RSABSSA-SHA384-PSS-Deterministic"},
			{VariantSHA384PSSZERODeterministic, "RSABSSA-SHA384-PSSZERO-Deterministic"},
		}

		for _, tt := range tests {
			t.Run(tt.expected, func(t *testing.T) {
				assert.Equal(t, tt.expected, tt.variant.String())
			})
		}
	})

	t.Run("saltLength", func(t *testing.T) {
		tests := []struct {
			variant  Variant
			expected int
		}{
			{VariantSHA384PSSRandomized, 48},
			{VariantSHA384PSSZERORandomized, 0},
			{VariantSHA384PSSDeterministic, 48},
			{VariantSHA384PSSZERODeterministic, 0},
			{Variant("unknown"), -1},
		}

		for _, tt := range tests {
			t.Run(tt.variant.String(), func(t *testing.T) {
				assert.Equal(t, tt.expected, tt.variant.saltLength())
			})
		}
	})

	t.Run("isRandomized", func(t *testing.T) {
		tests := []struct {
			variant  Variant
			expected bool
		}{
			{VariantSHA384PSSRandomized, true},
			{VariantSHA384PSSZERORandomized, true},
			{VariantSHA384PSSDeterministic, false},
			{VariantSHA384PSSZERODeterministic, false},
			{Variant("unknown"), false},
		}

		for _, tt := range tests {
			t.Run(tt.variant.String(), func(t *testing.T) {
				assert.Equal(t, tt.expected, tt.variant.isRandomized())
			})
		}
	})
}
