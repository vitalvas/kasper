package blindrsa

// Variant identifies an RSABSSA variant defined in RFC 9474 Section 5.
type Variant string

const (
	// VariantSHA384PSSRandomized is RSABSSA-SHA384-PSS-Randomized.
	// Uses SHA-384, salt length equal to hash length (48), and a random
	// message prefix.
	VariantSHA384PSSRandomized Variant = "RSABSSA-SHA384-PSS-Randomized"

	// VariantSHA384PSSZERORandomized is RSABSSA-SHA384-PSSZERO-Randomized.
	// Uses SHA-384, zero salt length, and a random message prefix.
	VariantSHA384PSSZERORandomized Variant = "RSABSSA-SHA384-PSSZERO-Randomized"

	// VariantSHA384PSSDeterministic is RSABSSA-SHA384-PSS-Deterministic.
	// Uses SHA-384, salt length equal to hash length (48), and no message
	// prefix.
	VariantSHA384PSSDeterministic Variant = "RSABSSA-SHA384-PSS-Deterministic"

	// VariantSHA384PSSZERODeterministic is RSABSSA-SHA384-PSSZERO-Deterministic.
	// Uses SHA-384, zero salt length, and no message prefix.
	VariantSHA384PSSZERODeterministic Variant = "RSABSSA-SHA384-PSSZERO-Deterministic"
)

// sha384Size is the output size of SHA-384 in bytes.
const sha384Size = 48

// String returns the variant name as defined in RFC 9474.
func (v Variant) String() string {
	return string(v)
}

// saltLength returns the PSS salt length in bytes for this variant.
func (v Variant) saltLength() int {
	switch v {
	case VariantSHA384PSSRandomized, VariantSHA384PSSDeterministic:
		return sha384Size
	case VariantSHA384PSSZERORandomized, VariantSHA384PSSZERODeterministic:
		return 0
	default:
		return -1
	}
}

// isRandomized reports whether the variant uses a random message prefix.
func (v Variant) isRandomized() bool {
	return v == VariantSHA384PSSRandomized || v == VariantSHA384PSSZERORandomized
}
