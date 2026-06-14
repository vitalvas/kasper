package e2ee

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionItemSerialize(t *testing.T) {
	tests := []struct {
		name string
		item sessionItem
		want string
	}{
		{
			name: "request with all params",
			item: sessionItem{
				kid:    "2026-06",
				aead:   "AES-256-GCM",
				epk:    []byte{0x01, 0x02, 0x03},
				ts:     1781006400,
				nid:    "abc",
				cty:    "application/json",
				hasEPK: true,
				hasCTY: true,
			},
			want: `"2026-06";aead="AES-256-GCM";epk=:AQID:;ts=1781006400;nid="abc";cty="application/json"`,
		},
		{
			name: "response without epk and cty",
			item: sessionItem{
				kid:  "2026-06",
				aead: "AES-128-GCM",
				ts:   42,
				nid:  "n",
			},
			want: `"2026-06";aead="AES-128-GCM";ts=42;nid="n"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.item.serialize()
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSessionItemRoundTrip(t *testing.T) {
	orig := sessionItem{
		kid:    "key.id~1",
		aead:   "AES-256-GCM",
		epk:    []byte{0xff, 0x00, 0xaa, 0x55},
		ts:     1781006400,
		nid:    "3b1c1c2e-2b6a-4a0d-9b6c-2a9f1b6a0e21",
		cty:    "application/json",
		hasEPK: true,
		hasCTY: true,
	}

	wire, err := orig.serialize()
	require.NoError(t, err)

	parsed, err := parseSessionItem(wire)
	require.NoError(t, err)

	assert.Equal(t, orig.kid, parsed.kid)
	assert.Equal(t, orig.aead, parsed.aead)
	assert.Equal(t, orig.epk, parsed.epk)
	assert.Equal(t, orig.ts, parsed.ts)
	assert.Equal(t, orig.nid, parsed.nid)
	assert.Equal(t, orig.cty, parsed.cty)
	assert.True(t, parsed.hasEPK)
	assert.True(t, parsed.hasCTY)

	// Re-serialization must be deterministic regardless of parse.
	wire2, err := parsed.serialize()
	require.NoError(t, err)
	assert.Equal(t, wire, wire2)
}

func TestParseSessionItemReorderingCanonical(t *testing.T) {
	// Parameters provided out of canonical order must serialize canonically.
	in := `"k";nid="n";ts=5;aead="AES-128-GCM";epk=:AQID:`

	parsed, err := parseSessionItem(in)
	require.NoError(t, err)

	out, err := parsed.serialize()
	require.NoError(t, err)

	assert.Equal(t, `"k";aead="AES-128-GCM";epk=:AQID:;ts=5;nid="n"`, out)
}

func TestParseSessionItemEscapes(t *testing.T) {
	item := sessionItem{
		kid:  `a"b\c`,
		aead: "AES-128-GCM",
		ts:   1,
		nid:  "n",
	}

	wire, err := item.serialize()
	require.NoError(t, err)

	parsed, err := parseSessionItem(wire)
	require.NoError(t, err)
	assert.Equal(t, `a"b\c`, parsed.kid)
}

func TestParseSessionItemErrors(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"unquoted kid", `k;aead="AES-128-GCM"`},
		{"unterminated string", `"k`},
		{"unterminated byte seq", `"k";epk=:AQID`},
		{"unknown param", `"k";foo="x"`},
		{"duplicate param", `"k";ts=1;ts=2`},
		{"bad integer", `"k";ts=notnum`},
		{"bad base64", `"k";epk=:!!!:`},
		{"missing value", `"k";aead`},
		{"bad param after semicolon", `"k" ;`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseSessionItem(tt.input)
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrMalformed))
		})
	}
}

func TestSerializeStringRejectsNonASCII(t *testing.T) {
	_, err := serializeString("héllo")
	require.Error(t, err)
}

func TestSerializeInvalidParams(t *testing.T) {
	tests := []struct {
		name string
		item sessionItem
	}{
		{"bad kid", sessionItem{kid: "bad\x01", aead: "AES-128-GCM", ts: 1, nid: "n"}},
		{"bad aead", sessionItem{kid: "k", aead: "bad\x01", ts: 1, nid: "n"}},
		{"bad nid", sessionItem{kid: "k", aead: "AES-128-GCM", ts: 1, nid: "bad\x01"}},
		{
			name: "bad cty",
			item: sessionItem{
				kid:    "k",
				aead:   "AES-128-GCM",
				ts:     1,
				nid:    "n",
				cty:    "bad\x01",
				hasCTY: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.item.serialize()
			require.ErrorIs(t, err, ErrMalformed)
		})
	}
}

func TestParseStringErrors(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"not quoted", `abc`},
		{"too short", `"`},
		{"trailing backslash", `"abc\`},
		{"invalid escape", `"a\nb"`},
		{"trailing data after close", `"ab"cd`},
		{"control char", "\"a\x01b\""},
		{"unterminated", `"abc`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseString(tt.input)
			require.Error(t, err)
		})
	}
}

func TestParseByteSequenceErrors(t *testing.T) {
	_, err := parseByteSequence("noclons")
	require.Error(t, err)

	_, err = parseByteSequence(":!!!:")
	require.Error(t, err)
}

func TestScanParamNameRejectsLeading(t *testing.T) {
	_, _, ok := scanParamName("9bad")
	assert.False(t, ok)

	_, _, ok = scanParamName("")
	assert.False(t, ok)
}

func TestScanValueEmpty(t *testing.T) {
	_, _, err := scanValue("")
	require.ErrorIs(t, err, ErrMalformed)
}

func TestParseSessionItemEmptyAfterTrim(t *testing.T) {
	_, err := parseSessionItem("   ")
	require.ErrorIs(t, err, ErrMalformed)
}

func TestParseSessionItemUnquotedStringParams(t *testing.T) {
	// aead, nid, and cty must be quoted strings; an unquoted token value scans
	// successfully as a bare value but fails string parsing.
	tests := []struct {
		name  string
		input string
	}{
		{"unquoted aead", `"k";aead=123;ts=1;nid="n"`},
		{"unquoted nid", `"k";aead="AES-128-GCM";ts=1;nid=123`},
		{"unquoted cty", `"k";aead="AES-128-GCM";ts=1;nid="n";cty=123`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseSessionItem(tt.input)
			require.ErrorIs(t, err, ErrMalformed)
		})
	}
}

func TestParseSessionItemInvalidParamName(t *testing.T) {
	// A parameter name starting with a digit is rejected by scanParamName.
	_, err := parseSessionItem(`"k";9bad="x"`)
	require.ErrorIs(t, err, ErrMalformed)
}

func TestSerializeByteSequenceValue(t *testing.T) {
	assert.Equal(t, ":AQID:", serializeByteSequence([]byte{1, 2, 3}))
	assert.Equal(t, "::", serializeByteSequence(nil))
}
