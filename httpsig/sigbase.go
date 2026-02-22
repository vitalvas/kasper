package httpsig

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// signatureParams holds the parameters that appear in the @signature-params
// component of the signature base.
type signatureParams struct {
	components []string
	created    time.Time
	expires    time.Time
	nonce      string
	alg        Algorithm
	keyID      string
	tag        string
}

// buildSignatureBase constructs the signature base string per RFC 9421
// Section 2.5. Each covered component produces a line
// "<component-id>": <value>\n and the final line is
// "@signature-params": <params>.
func buildSignatureBase(r *http.Request, params signatureParams) ([]byte, string, error) {
	var base strings.Builder

	for _, id := range params.components {
		val, err := componentValue(id, r)
		if err != nil {
			return nil, "", err
		}

		fmt.Fprintf(&base, "%q: %s\n", id, val)
	}

	sigParamsStr := serializeSignatureParams(params)
	fmt.Fprintf(&base, "\"@signature-params\": %s", sigParamsStr)

	return []byte(base.String()), sigParamsStr, nil
}

// serializeSignatureParams produces the inner-list representation of the
// signature parameters per RFC 9421 Section 2.3 and RFC 8941 Section 3.1.1.
//
// Format: (<component-ids>);<key>=<value>;...
func serializeSignatureParams(params signatureParams) string {
	var b strings.Builder

	// Inner list of component identifiers.
	b.WriteByte('(')
	for i, id := range params.components {
		if i > 0 {
			b.WriteByte(' ')
		}

		b.WriteString(strconv.Quote(id))
	}
	b.WriteByte(')')

	// Parameters.
	if !params.created.IsZero() {
		fmt.Fprintf(&b, ";created=%d", params.created.Unix())
	}

	if !params.expires.IsZero() {
		fmt.Fprintf(&b, ";expires=%d", params.expires.Unix())
	}

	if params.nonce != "" {
		b.WriteString(";nonce=")
		b.WriteString(quoteRFC8941(params.nonce))
	}

	b.WriteString(";alg=")
	b.WriteString(quoteRFC8941(params.alg.String()))
	b.WriteString(";keyid=")
	b.WriteString(quoteRFC8941(params.keyID))

	if params.tag != "" {
		b.WriteString(";tag=")
		b.WriteString(quoteRFC8941(params.tag))
	}

	return b.String()
}

// parseSignatureParams parses a signature parameters string as produced by
// serializeSignatureParams. It extracts the inner list of component
// identifiers and the key-value parameters.
//
// Expected format: ("@method" "@authority" "@path");created=...;keyid="..."
func parseSignatureParams(raw string) (signatureParams, error) {
	var params signatureParams

	// Find the inner list boundaries.
	openParen := strings.IndexByte(raw, '(')
	closeParen := strings.IndexByte(raw, ')')

	if openParen < 0 || closeParen < 0 || closeParen <= openParen {
		return params, fmt.Errorf("%w: invalid signature params format", ErrMalformedHeader)
	}

	inner := raw[openParen+1 : closeParen]
	params.components = parseInnerList(inner)

	// Parse parameters after the closing paren.
	rest := raw[closeParen+1:]
	paramParts := splitParams(rest)

	for _, part := range paramParts {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}

		switch key {
		case "created":
			ts, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return params, fmt.Errorf("%w: invalid created timestamp", ErrMalformedHeader)
			}
			t := time.Unix(ts, 0)
			params.created = t

		case "expires":
			ts, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return params, fmt.Errorf("%w: invalid expires timestamp", ErrMalformedHeader)
			}
			t := time.Unix(ts, 0)
			params.expires = t

		case "nonce":
			params.nonce = unquote(value)

		case "alg":
			params.alg = Algorithm(unquote(value))

		case "keyid":
			params.keyID = unquote(value)

		case "tag":
			params.tag = unquote(value)
		}
	}

	if params.alg == "" {
		return params, fmt.Errorf("%w: missing alg parameter", ErrMalformedHeader)
	}

	if params.keyID == "" {
		return params, fmt.Errorf("%w: missing keyid parameter", ErrMalformedHeader)
	}

	return params, nil
}

// parseInnerList parses a space-separated list of quoted strings inside
// parentheses.
func parseInnerList(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}

	var items []string
	for len(s) > 0 {
		s = strings.TrimLeft(s, " ")
		if len(s) == 0 {
			break
		}

		if s[0] == '"' {
			end := strings.IndexByte(s[1:], '"')
			if end < 0 {
				// Malformed, take the rest.
				items = append(items, s[1:])
				break
			}

			items = append(items, s[1:end+1])
			s = s[end+2:]
		} else {
			end := strings.IndexByte(s, ' ')
			if end < 0 {
				items = append(items, s)
				break
			}

			items = append(items, s[:end])
			s = s[end+1:]
		}
	}

	return items
}

// splitQuoteAware splits s on delim while respecting "..." quoted regions.
// Backslash-escaped quotes (\") inside quoted strings are handled. Each
// resulting part is trimmed of whitespace and empty parts are skipped.
func splitQuoteAware(s string, delim byte) []string {
	var result []string
	var part strings.Builder
	inQuote := false

	for i := 0; i < len(s); i++ {
		ch := s[i]

		if inQuote {
			if ch == '\\' && i+1 < len(s) {
				part.WriteByte(ch)
				i++
				part.WriteByte(s[i])
				continue
			}

			if ch == '"' {
				inQuote = false
			}

			part.WriteByte(ch)
			continue
		}

		if ch == '"' {
			inQuote = true
			part.WriteByte(ch)
			continue
		}

		if ch == delim {
			p := strings.TrimSpace(part.String())
			if p != "" {
				result = append(result, p)
			}

			part.Reset()
			continue
		}

		part.WriteByte(ch)
	}

	if p := strings.TrimSpace(part.String()); p != "" {
		result = append(result, p)
	}

	return result
}

// splitParams splits ";key=value" parameter pairs.
func splitParams(s string) []string {
	s = strings.TrimLeft(s, " ")
	if s == "" {
		return nil
	}

	return splitQuoteAware(s, ';')
}

// quoteRFC8941 produces an RFC 8941 quoted-string. Only backslash and
// double-quote are escaped (Section 3.3.3); no other escape sequences
// are permitted.
func quoteRFC8941(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')

	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '\\' || ch == '"' {
			b.WriteByte('\\')
		}

		b.WriteByte(ch)
	}

	b.WriteByte('"')

	return b.String()
}

// unquote removes surrounding double quotes and unescapes RFC 8941
// escape sequences (\\ → \ and \" → ").
func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}

	if !strings.Contains(s, `\`) {
		return s
	}

	var b strings.Builder
	b.Grow(len(s))

	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			b.WriteByte(s[i])

			continue
		}

		b.WriteByte(s[i])
	}

	return b.String()
}
