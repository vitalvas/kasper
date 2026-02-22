package httpsig

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildSignatureBase(t *testing.T) {
	t.Run("basic request with method authority path", func(t *testing.T) {
		req := httptest.NewRequest("POST", "https://example.com/api/items", nil)
		req.Host = "example.com"

		params := signatureParams{
			components: []string{"@method", "@authority", "@path"},
			created:    time.Unix(1618884473, 0),
			alg:        AlgorithmEd25519,
			keyID:      "test-key-ed25519",
		}

		base, sigParams, err := buildSignatureBase(req, params)
		require.NoError(t, err)

		expected := "\"@method\": POST\n" +
			"\"@authority\": example.com\n" +
			"\"@path\": /api/items\n" +
			"\"@signature-params\": " + sigParams

		assert.Equal(t, expected, string(base))
		assert.Contains(t, sigParams, "(\"@method\" \"@authority\" \"@path\")")
		assert.Contains(t, sigParams, "created=1618884473")
		assert.Contains(t, sigParams, "keyid=\"test-key-ed25519\"")
		assert.Contains(t, sigParams, "alg=\"ed25519\"")
	})

	t.Run("with header components", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)
		req.Host = "example.com"
		req.Header.Set("Content-Type", "application/json")

		params := signatureParams{
			components: []string{"@method", "content-type"},
			created:    time.Unix(1000000, 0),
			alg:        AlgorithmHMACSHA256,
			keyID:      "shared-key",
		}

		base, _, err := buildSignatureBase(req, params)
		require.NoError(t, err)

		assert.Contains(t, string(base), "\"@method\": GET\n")
		assert.Contains(t, string(base), "\"content-type\": application/json\n")
	})

	t.Run("missing component returns error", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)

		params := signatureParams{
			components: []string{"@method", "x-missing-header"},
			alg:        AlgorithmEd25519,
			keyID:      "k",
		}

		_, _, err := buildSignatureBase(req, params)
		assert.ErrorIs(t, err, ErrUnknownComponent)
	})

	t.Run("unknown derived component returns error", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)

		params := signatureParams{
			components: []string{"@unknown"},
			alg:        AlgorithmEd25519,
			keyID:      "k",
		}

		_, _, err := buildSignatureBase(req, params)
		assert.ErrorIs(t, err, ErrUnknownComponent)
	})
}

func TestSerializeSignatureParams(t *testing.T) {
	t.Run("full parameters", func(t *testing.T) {
		params := signatureParams{
			components: []string{"@method", "@authority", "@path"},
			created:    time.Unix(1618884473, 0),
			expires:    time.Unix(1618884773, 0),
			nonce:      "abc123",
			alg:        AlgorithmECDSAP256SHA256,
			keyID:      "my-key",
			tag:        "my-app",
		}

		result := serializeSignatureParams(params)

		assert.Contains(t, result, "(\"@method\" \"@authority\" \"@path\")")
		assert.Contains(t, result, "created=1618884473")
		assert.Contains(t, result, "expires=1618884773")
		assert.Contains(t, result, "nonce=\"abc123\"")
		assert.Contains(t, result, "alg=\"ecdsa-p256-sha256\"")
		assert.Contains(t, result, "keyid=\"my-key\"")
		assert.Contains(t, result, "tag=\"my-app\"")
	})

	t.Run("zero created time is omitted", func(t *testing.T) {
		params := signatureParams{
			components: []string{"@method"},
			alg:        AlgorithmEd25519,
			keyID:      "k",
		}

		result := serializeSignatureParams(params)
		assert.NotContains(t, result, "created=")
	})

	t.Run("empty nonce is omitted", func(t *testing.T) {
		params := signatureParams{
			components: []string{"@method"},
			alg:        AlgorithmEd25519,
			keyID:      "k",
		}

		result := serializeSignatureParams(params)
		assert.NotContains(t, result, "nonce=")
	})

	t.Run("empty tag is omitted", func(t *testing.T) {
		params := signatureParams{
			components: []string{"@method"},
			alg:        AlgorithmEd25519,
			keyID:      "k",
		}

		result := serializeSignatureParams(params)
		assert.NotContains(t, result, "tag=")
	})

	t.Run("empty components", func(t *testing.T) {
		params := signatureParams{
			alg:   AlgorithmEd25519,
			keyID: "k",
		}

		result := serializeSignatureParams(params)
		assert.True(t, strings.HasPrefix(result, "()"), "expected params to start with (), got: %s", result)
	})

	t.Run("values with backslash", func(t *testing.T) {
		params := signatureParams{
			components: []string{"@method"},
			nonce:      `a\b`,
			alg:        AlgorithmEd25519,
			keyID:      "k",
		}

		result := serializeSignatureParams(params)
		assert.Contains(t, result, `nonce="a\\b"`)
	})

	t.Run("values with embedded quote", func(t *testing.T) {
		params := signatureParams{
			components: []string{"@method"},
			alg:        AlgorithmEd25519,
			keyID:      `k"ey`,
		}

		result := serializeSignatureParams(params)
		assert.Contains(t, result, `keyid="k\"ey"`)
	})
}

func TestParseSignatureParams(t *testing.T) {
	t.Run("round trip", func(t *testing.T) {
		original := signatureParams{
			components: []string{"@method", "@authority", "@path"},
			created:    time.Unix(1618884473, 0),
			expires:    time.Unix(1618884773, 0),
			nonce:      "abc123",
			alg:        AlgorithmEd25519,
			keyID:      "test-key",
			tag:        "my-tag",
		}

		serialized := serializeSignatureParams(original)
		parsed, err := parseSignatureParams(serialized)
		require.NoError(t, err)

		assert.Equal(t, original.components, parsed.components)
		assert.Equal(t, original.created, parsed.created)
		assert.Equal(t, original.expires, parsed.expires)
		assert.Equal(t, original.nonce, parsed.nonce)
		assert.Equal(t, original.alg, parsed.alg)
		assert.Equal(t, original.keyID, parsed.keyID)
		assert.Equal(t, original.tag, parsed.tag)
	})

	t.Run("minimal params", func(t *testing.T) {
		input := `("@method");alg="ed25519";keyid="k"`
		parsed, err := parseSignatureParams(input)
		require.NoError(t, err)

		assert.Equal(t, []string{"@method"}, parsed.components)
		assert.Equal(t, AlgorithmEd25519, parsed.alg)
		assert.Equal(t, "k", parsed.keyID)
		assert.True(t, parsed.created.IsZero())
		assert.True(t, parsed.expires.IsZero())
		assert.Empty(t, parsed.nonce)
		assert.Empty(t, parsed.tag)
	})

	t.Run("invalid format no parens", func(t *testing.T) {
		_, err := parseSignatureParams("invalid")
		assert.ErrorIs(t, err, ErrMalformedHeader)
	})

	t.Run("invalid created timestamp", func(t *testing.T) {
		_, err := parseSignatureParams(`("@method");created=notanumber;alg="ed25519";keyid="k"`)
		assert.ErrorIs(t, err, ErrMalformedHeader)
	})

	t.Run("invalid expires timestamp", func(t *testing.T) {
		_, err := parseSignatureParams(`("@method");expires=notanumber;alg="ed25519";keyid="k"`)
		assert.ErrorIs(t, err, ErrMalformedHeader)
	})

	t.Run("empty inner list", func(t *testing.T) {
		input := `();alg="ed25519";keyid="k"`
		parsed, err := parseSignatureParams(input)
		require.NoError(t, err)
		assert.Empty(t, parsed.components)
	})

	t.Run("param without equals is ignored", func(t *testing.T) {
		input := `("@method");noparam;alg="ed25519";keyid="k"`
		parsed, err := parseSignatureParams(input)
		require.NoError(t, err)
		assert.Equal(t, []string{"@method"}, parsed.components)
		assert.Equal(t, "k", parsed.keyID)
	})

	t.Run("missing alg returns error", func(t *testing.T) {
		input := `("@method");keyid="k"`
		_, err := parseSignatureParams(input)
		assert.ErrorIs(t, err, ErrMalformedHeader)
	})

	t.Run("missing keyid returns error", func(t *testing.T) {
		input := `("@method");alg="ed25519"`
		_, err := parseSignatureParams(input)
		assert.ErrorIs(t, err, ErrMalformedHeader)
	})

	t.Run("missing both alg and keyid returns error", func(t *testing.T) {
		input := `("@method");created=123`
		_, err := parseSignatureParams(input)
		assert.ErrorIs(t, err, ErrMalformedHeader)
	})

	t.Run("unquoted param values", func(t *testing.T) {
		input := `("@method");alg=ed25519;keyid=mykey`
		parsed, err := parseSignatureParams(input)
		require.NoError(t, err)
		assert.Equal(t, Algorithm("ed25519"), parsed.alg)
		assert.Equal(t, "mykey", parsed.keyID)
	})

	t.Run("round trip with escaped backslash in nonce", func(t *testing.T) {
		original := signatureParams{
			components: []string{"@method"},
			nonce:      `a\b`,
			alg:        AlgorithmEd25519,
			keyID:      "test-key",
		}

		serialized := serializeSignatureParams(original)
		parsed, err := parseSignatureParams(serialized)
		require.NoError(t, err)

		assert.Equal(t, original.nonce, parsed.nonce)
		assert.Equal(t, original.alg, parsed.alg)
		assert.Equal(t, original.keyID, parsed.keyID)
	})

	t.Run("round trip with escaped quote in keyid", func(t *testing.T) {
		original := signatureParams{
			components: []string{"@method"},
			alg:        AlgorithmEd25519,
			keyID:      `k"ey`,
		}

		serialized := serializeSignatureParams(original)
		parsed, err := parseSignatureParams(serialized)
		require.NoError(t, err)

		assert.Equal(t, original.keyID, parsed.keyID)
	})

	t.Run("round trip with both escapes in tag", func(t *testing.T) {
		original := signatureParams{
			components: []string{"@method"},
			alg:        AlgorithmEd25519,
			keyID:      "k",
			tag:        `my\ta"g`,
		}

		serialized := serializeSignatureParams(original)
		parsed, err := parseSignatureParams(serialized)
		require.NoError(t, err)

		assert.Equal(t, original.tag, parsed.tag)
	})
}

func TestParseInnerList(t *testing.T) {
	t.Run("quoted items", func(t *testing.T) {
		items := parseInnerList(`"@method" "@authority" "@path"`)
		assert.Equal(t, []string{"@method", "@authority", "@path"}, items)
	})

	t.Run("unquoted items", func(t *testing.T) {
		items := parseInnerList(`foo bar baz`)
		assert.Equal(t, []string{"foo", "bar", "baz"}, items)
	})

	t.Run("single unquoted item", func(t *testing.T) {
		items := parseInnerList(`single`)
		assert.Equal(t, []string{"single"}, items)
	})

	t.Run("malformed unclosed quote", func(t *testing.T) {
		items := parseInnerList(`"unclosed`)
		assert.Equal(t, []string{"unclosed"}, items)
	})

	t.Run("empty string", func(t *testing.T) {
		items := parseInnerList("")
		assert.Nil(t, items)
	})

	t.Run("whitespace only", func(t *testing.T) {
		items := parseInnerList("   ")
		assert.Nil(t, items)
	})

	t.Run("mixed quoted and unquoted", func(t *testing.T) {
		items := parseInnerList(`"@method" plain "@path"`)
		assert.Equal(t, []string{"@method", "plain", "@path"}, items)
	})

	t.Run("trailing spaces after last item", func(t *testing.T) {
		items := parseInnerList(`"@method"   `)
		assert.Equal(t, []string{"@method"}, items)
	})
}

func TestUnquote(t *testing.T) {
	t.Run("quoted string", func(t *testing.T) {
		assert.Equal(t, "value", unquote(`"value"`))
	})

	t.Run("unquoted string", func(t *testing.T) {
		assert.Equal(t, "value", unquote("value"))
	})

	t.Run("empty string", func(t *testing.T) {
		assert.Equal(t, "", unquote(""))
	})

	t.Run("single char", func(t *testing.T) {
		assert.Equal(t, "x", unquote("x"))
	})

	t.Run("empty quotes", func(t *testing.T) {
		assert.Equal(t, "", unquote(`""`))
	})

	t.Run("escaped backslash", func(t *testing.T) {
		assert.Equal(t, `a\b`, unquote(`"a\\b"`))
	})

	t.Run("escaped quote", func(t *testing.T) {
		assert.Equal(t, `k"ey`, unquote(`"k\"ey"`))
	})

	t.Run("multiple escapes", func(t *testing.T) {
		assert.Equal(t, `a\b"c`, unquote(`"a\\b\"c"`))
	})

	t.Run("trailing backslash without pair", func(t *testing.T) {
		assert.Equal(t, `trail\`, unquote(`"trail\"`))
	})
}

func TestQuoteRFC8941(t *testing.T) {
	t.Run("simple string", func(t *testing.T) {
		assert.Equal(t, `"hello"`, quoteRFC8941("hello"))
	})

	t.Run("empty string", func(t *testing.T) {
		assert.Equal(t, `""`, quoteRFC8941(""))
	})

	t.Run("backslash escaped", func(t *testing.T) {
		assert.Equal(t, `"a\\b"`, quoteRFC8941(`a\b`))
	})

	t.Run("quote escaped", func(t *testing.T) {
		assert.Equal(t, `"k\"ey"`, quoteRFC8941(`k"ey`))
	})

	t.Run("both escapes", func(t *testing.T) {
		assert.Equal(t, `"a\\b\"c"`, quoteRFC8941(`a\b"c`))
	})

	t.Run("no other escapes", func(t *testing.T) {
		// Newline and tab are passed through literally, not Go-escaped.
		assert.Equal(t, "\"\n\t\"", quoteRFC8941("\n\t"))
	})
}

func TestSplitQuoteAware(t *testing.T) {
	t.Run("simple comma split", func(t *testing.T) {
		result := splitQuoteAware("a, b, c", ',')
		assert.Equal(t, []string{"a", "b", "c"}, result)
	})

	t.Run("comma without space", func(t *testing.T) {
		result := splitQuoteAware("a,b,c", ',')
		assert.Equal(t, []string{"a", "b", "c"}, result)
	})

	t.Run("semicolons inside quoted value", func(t *testing.T) {
		result := splitQuoteAware(`nonce="val;ue";alg="ed25519"`, ';')
		assert.Equal(t, []string{`nonce="val;ue"`, `alg="ed25519"`}, result)
	})

	t.Run("commas inside quoted value", func(t *testing.T) {
		result := splitQuoteAware(`key="a,b", other="c"`, ',')
		assert.Equal(t, []string{`key="a,b"`, `other="c"`}, result)
	})

	t.Run("escaped quote inside quoted value", func(t *testing.T) {
		result := splitQuoteAware(`key="val\"ue";next=1`, ';')
		assert.Equal(t, []string{`key="val\"ue"`, "next=1"}, result)
	})

	t.Run("empty input", func(t *testing.T) {
		result := splitQuoteAware("", ',')
		assert.Nil(t, result)
	})

	t.Run("no delimiter present", func(t *testing.T) {
		result := splitQuoteAware("single", ',')
		assert.Equal(t, []string{"single"}, result)
	})

	t.Run("empty parts skipped", func(t *testing.T) {
		result := splitQuoteAware("a,,b", ',')
		assert.Equal(t, []string{"a", "b"}, result)
	})

	t.Run("whitespace-only parts skipped", func(t *testing.T) {
		result := splitQuoteAware("a, , b", ',')
		assert.Equal(t, []string{"a", "b"}, result)
	})

	t.Run("byte-sequence values with colons", func(t *testing.T) {
		result := splitQuoteAware("sig1=:dGVzdA==:, sig2=:YWJj:", ',')
		assert.Equal(t, []string{"sig1=:dGVzdA==:", "sig2=:YWJj:"}, result)
	})
}

func TestSplitParams(t *testing.T) {
	t.Run("normal params", func(t *testing.T) {
		result := splitParams(`;created=123;alg="ed25519"`)
		assert.Equal(t, []string{"created=123", `alg="ed25519"`}, result)
	})

	t.Run("empty string", func(t *testing.T) {
		result := splitParams("")
		assert.Nil(t, result)
	})

	t.Run("whitespace only", func(t *testing.T) {
		result := splitParams("   ")
		assert.Nil(t, result)
	})

	t.Run("semicolon inside quoted nonce", func(t *testing.T) {
		result := splitParams(`;nonce="val;ue";alg="ed25519";keyid="k"`)
		assert.Equal(t, []string{`nonce="val;ue"`, `alg="ed25519"`, `keyid="k"`}, result)
	})
}
