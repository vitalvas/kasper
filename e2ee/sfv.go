package e2ee

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

// E2EE-Session parameters, per draft-vasylenko-e2ee-http Section 8.1 (Syntax).
// RFC 9651 preserves parameter insertion order, so the canonical form used for
// AAD must serialize parameters in a fixed, well-known order. Both peers
// serialize in this order regardless of receive order.
const (
	paramAEAD = "aead" // String, required (request and response)
	paramEPK  = "epk"  // Byte Sequence, required in request, prohibited in response
	paramTS   = "ts"   // Integer, required (Unix timestamp)
	paramNID  = "nid"  // String, required (per-message replay identifier)
	paramCTY  = "cty"  // String, optional (inner plaintext media type)
)

// canonicalParamOrder is the fixed ordering used when serializing the
// E2EE-Session field deterministically for AAD and transmission.
var canonicalParamOrder = []string{paramAEAD, paramEPK, paramTS, paramNID, paramCTY}

// serializeSession seams sessionItem.serialize so the defensive error branches
// at its call sites are reachable in tests; for items built from validated
// key-set and request data, serialization does not fail.
var serializeSession = func(item sessionItem) (string, error) { return item.serialize() }

// sessionItem is a parsed RFC 9651 Structured Field Item representing the
// E2EE-Session header (draft-vasylenko-e2ee-http Section 8). The bare item
// value is the key identifier (kid), carried as an sf-string. Parameters carry
// the AEAD identifier, ephemeral public key, timestamp, replay identifier, and
// optional inner content type.
type sessionItem struct {
	kid    string
	aead   string
	epk    []byte // nil/absent in responses
	ts     int64
	nid    string
	cty    string // optional; empty means absent
	hasEPK bool
	hasCTY bool
}

// serialize renders the item using RFC 9651 deterministic serialization with
// the fixed canonical parameter order. The result is the field value only
// (no field name, colon, or surrounding whitespace) and is used both on the
// wire and as the basis for AAD (draft-vasylenko-e2ee-http Sections 8.1, 7.4).
func (s sessionItem) serialize() (string, error) {
	var b strings.Builder

	val, err := serializeString(s.kid)
	if err != nil {
		return "", fmt.Errorf("%w: kid: %s", ErrMalformed, err)
	}

	b.WriteString(val)

	for _, name := range canonicalParamOrder {
		switch name {
		case paramAEAD:
			sv, err := serializeString(s.aead)
			if err != nil {
				return "", fmt.Errorf("%w: aead: %s", ErrMalformed, err)
			}

			b.WriteString(";")
			b.WriteString(name)
			b.WriteString("=")
			b.WriteString(sv)

		case paramEPK:
			if !s.hasEPK {
				continue
			}

			b.WriteString(";")
			b.WriteString(name)
			b.WriteString("=")
			b.WriteString(serializeByteSequence(s.epk))

		case paramTS:
			b.WriteString(";")
			b.WriteString(name)
			b.WriteString("=")
			b.WriteString(strconv.FormatInt(s.ts, 10))

		case paramNID:
			sv, err := serializeString(s.nid)
			if err != nil {
				return "", fmt.Errorf("%w: nid: %s", ErrMalformed, err)
			}

			b.WriteString(";")
			b.WriteString(name)
			b.WriteString("=")
			b.WriteString(sv)

		case paramCTY:
			if !s.hasCTY {
				continue
			}

			sv, err := serializeString(s.cty)
			if err != nil {
				return "", fmt.Errorf("%w: cty: %s", ErrMalformed, err)
			}

			b.WriteString(";")
			b.WriteString(name)
			b.WriteString("=")
			b.WriteString(sv)
		}
	}

	return b.String(), nil
}

// serializeString renders an RFC 9651 sf-string. The input must consist of
// printable ASCII (0x20-0x7E); backslash and double quote are escaped.
func serializeString(in string) (string, error) {
	var b strings.Builder

	b.WriteByte('"')

	for i := 0; i < len(in); i++ {
		c := in[i]
		if c < 0x20 || c > 0x7e {
			return "", fmt.Errorf("invalid character 0x%02x", c)
		}

		if c == '\\' || c == '"' {
			b.WriteByte('\\')
		}

		b.WriteByte(c)
	}

	b.WriteByte('"')

	return b.String(), nil
}

// serializeByteSequence renders an RFC 9651 sf-binary value: base64 (standard
// alphabet, with padding) delimited by colons.
func serializeByteSequence(in []byte) string {
	return fmt.Sprintf(":%s:", base64.StdEncoding.EncodeToString(in))
}

// parseSessionItem parses an E2EE-Session field value into a sessionItem.
// It accepts parameters in any order; serialization re-imposes the canonical
// order. Unknown parameters are rejected to keep the field tightly bound.
func parseSessionItem(value string) (sessionItem, error) {
	var item sessionItem

	value = strings.TrimSpace(value)
	if value == "" {
		return item, fmt.Errorf("%w: empty field", ErrMalformed)
	}

	bare, params, err := splitItem(value)
	if err != nil {
		return item, err
	}

	kid, err := parseString(bare)
	if err != nil {
		return item, fmt.Errorf("%w: kid: %s", ErrMalformed, err)
	}

	item.kid = kid

	seen := make(map[string]bool, len(params))

	for _, p := range params {
		name, raw := p.name, p.value
		if seen[name] {
			return item, fmt.Errorf("%w: duplicate parameter %q", ErrMalformed, name)
		}

		seen[name] = true

		switch name {
		case paramAEAD:
			s, err := parseString(raw)
			if err != nil {
				return item, fmt.Errorf("%w: aead: %s", ErrMalformed, err)
			}

			item.aead = s

		case paramEPK:
			b, err := parseByteSequence(raw)
			if err != nil {
				return item, fmt.Errorf("%w: epk: %s", ErrMalformed, err)
			}

			item.epk = b
			item.hasEPK = true

		case paramTS:
			n, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				return item, fmt.Errorf("%w: ts: %s", ErrMalformed, err)
			}

			item.ts = n

		case paramNID:
			s, err := parseString(raw)
			if err != nil {
				return item, fmt.Errorf("%w: nid: %s", ErrMalformed, err)
			}

			item.nid = s

		case paramCTY:
			s, err := parseString(raw)
			if err != nil {
				return item, fmt.Errorf("%w: cty: %s", ErrMalformed, err)
			}

			item.cty = s
			item.hasCTY = true

		default:
			return item, fmt.Errorf("%w: unknown parameter %q", ErrMalformed, name)
		}
	}

	return item, nil
}

// rawParam is a single parsed parameter name and its unparsed value text.
type rawParam struct {
	name  string
	value string
}

// splitItem splits an Item value into its bare value and its ordered list of
// parameters. It is a focused splitter for the E2EE-Session grammar: it is
// aware of quoted strings and colon-delimited byte sequences so that ';'
// characters inside those values do not terminate a parameter.
func splitItem(value string) (string, []rawParam, error) {
	bare, rest, err := scanValue(value)
	if err != nil {
		return "", nil, err
	}

	var params []rawParam

	for rest != "" {
		if rest[0] != ';' {
			return "", nil, fmt.Errorf("%w: expected ';' before parameter", ErrMalformed)
		}

		rest = strings.TrimLeft(rest[1:], " ")

		name, after, ok := scanParamName(rest)
		if !ok {
			return "", nil, fmt.Errorf("%w: invalid parameter name", ErrMalformed)
		}

		if after == "" || after[0] != '=' {
			return "", nil, fmt.Errorf("%w: parameter %q missing value", ErrMalformed, name)
		}

		pval, remaining, err := scanValue(after[1:])
		if err != nil {
			return "", nil, err
		}

		params = append(params, rawParam{name: name, value: pval})
		rest = remaining
	}

	return bare, params, nil
}

// scanValue consumes a single bare value (string, byte sequence, integer, or
// token) from the front of s and returns it along with the remaining text.
func scanValue(s string) (string, string, error) {
	if s == "" {
		return "", "", fmt.Errorf("%w: empty value", ErrMalformed)
	}

	switch s[0] {
	case '"':
		end := indexClosingQuote(s)
		if end < 0 {
			return "", "", fmt.Errorf("%w: unterminated string", ErrMalformed)
		}

		return s[:end+1], s[end+1:], nil

	case ':':
		end := strings.IndexByte(s[1:], ':')
		if end < 0 {
			return "", "", fmt.Errorf("%w: unterminated byte sequence", ErrMalformed)
		}

		return s[:end+2], s[end+2:], nil

	default:
		end := strings.IndexByte(s, ';')
		if end < 0 {
			return s, "", nil
		}

		return s[:end], s[end:], nil
	}
}

// indexClosingQuote returns the index of the closing quote of a string that
// starts at s[0]=='"', accounting for backslash escapes, or -1 if none.
func indexClosingQuote(s string) int {
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++ // skip escaped character
		case '"':
			return i
		}
	}

	return -1
}

// scanParamName consumes a parameter name (lcalpha / DIGIT / "_" / "-" / "."
// / "*"), starting with lcalpha or "*", and returns it with the remainder.
func scanParamName(s string) (string, string, bool) {
	if s == "" {
		return "", "", false
	}

	if (s[0] < 'a' || s[0] > 'z') && s[0] != '*' {
		return "", "", false
	}

	i := 0
	for i < len(s) {
		c := s[i]

		ok := (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == '_' || c == '-' || c == '.' || c == '*'
		if !ok {
			break
		}

		i++
	}

	return s[:i], s[i:], true
}

// parseString parses an RFC 9651 sf-string token, returning the unescaped
// value. Input must be a fully-quoted string of printable ASCII.
func parseString(in string) (string, error) {
	if len(in) < 2 || in[0] != '"' {
		return "", fmt.Errorf("expected quoted string")
	}

	var b strings.Builder

	i := 1
	for i < len(in) {
		c := in[i]

		switch {
		case c == '\\':
			if i+1 >= len(in) {
				return "", fmt.Errorf("trailing backslash")
			}

			next := in[i+1]
			if next != '\\' && next != '"' {
				return "", fmt.Errorf("invalid escape")
			}

			b.WriteByte(next)
			i += 2

		case c == '"':
			if i != len(in)-1 {
				return "", fmt.Errorf("trailing data after string")
			}

			return b.String(), nil

		case c < 0x20 || c > 0x7e:
			return "", fmt.Errorf("invalid character 0x%02x", c)

		default:
			b.WriteByte(c)
			i++
		}
	}

	return "", fmt.Errorf("unterminated string")
}

// parseByteSequence parses an RFC 9651 sf-binary value (colon-delimited
// standard base64) into raw bytes.
func parseByteSequence(in string) ([]byte, error) {
	if len(in) < 2 || in[0] != ':' || in[len(in)-1] != ':' {
		return nil, fmt.Errorf("expected colon-delimited byte sequence")
	}

	decoded, err := base64.StdEncoding.DecodeString(in[1 : len(in)-1])
	if err != nil {
		return nil, fmt.Errorf("invalid base64: %s", err)
	}

	return decoded, nil
}
