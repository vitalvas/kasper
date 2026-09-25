package muxhandlers

import "strings"

// ForwardedElement is one forwarded-element of an RFC 7239 Forwarded header:
// a set of parameters describing a single proxy hop. Values are the raw,
// unquoted parameter values as they appear in the header, so obfuscated
// identifiers ("_hidden") and the token "unknown" are preserved verbatim. A
// field is empty when its parameter is absent.
type ForwardedElement struct {
	For   string // "for" parameter: client or previous hop (IP, "unknown", or obfnode)
	By    string // "by" parameter: interface where the request came in
	Host  string // "host" parameter: original Host header
	Proto string // "proto" parameter: original scheme
}

// ParseForwarded parses an RFC 7239 Forwarded header value into its
// forwarded-elements, in header order (leftmost element first, i.e. closest to
// the original client). Parameter names are matched case-insensitively per
// RFC 7239 Section 4; quoted-string values are unquoted. Unknown parameters
// are ignored. It returns nil for an empty header.
func ParseForwarded(header string) []ForwardedElement {
	if strings.TrimSpace(header) == "" {
		return nil
	}

	var elements []ForwardedElement
	for _, elem := range splitForwarded(header, ',') {
		var fe ForwardedElement
		hasParam := false
		for _, param := range splitForwarded(elem, ';') {
			key, val, ok := strings.Cut(param, "=")
			if !ok {
				continue
			}
			key = strings.ToLower(strings.TrimSpace(key))
			val = unquoteForwarded(strings.TrimSpace(val))
			switch key {
			case "for":
				fe.For = val
				hasParam = true
			case "by":
				fe.By = val
				hasParam = true
			case "host":
				fe.Host = val
				hasParam = true
			case "proto":
				fe.Proto = val
				hasParam = true
			}
		}
		if hasParam {
			elements = append(elements, fe)
		}
	}
	return elements
}

// splitForwarded splits s on sep while respecting double-quoted regions, so a
// separator inside a quoted value does not split. Each part is returned
// trimmed of surrounding whitespace; empty parts are dropped.
func splitForwarded(s string, sep byte) []string {
	var parts []string
	var start int
	inQuote := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			if inQuote && i+1 < len(s) {
				i++ // skip the escaped character
			}
		case '"':
			inQuote = !inQuote
		case sep:
			if !inQuote {
				if p := strings.TrimSpace(s[start:i]); p != "" {
					parts = append(parts, p)
				}
				start = i + 1
			}
		}
	}
	if p := strings.TrimSpace(s[start:]); p != "" {
		parts = append(parts, p)
	}
	return parts
}

// unquoteForwarded removes surrounding double quotes and unescapes
// backslash-escaped characters within a quoted-string per RFC 7239.
func unquoteForwarded(s string) string {
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return s
	}
	inner := s[1 : len(s)-1]
	if !strings.Contains(inner, `\`) {
		return inner
	}
	var b strings.Builder
	b.Grow(len(inner))
	for i := 0; i < len(inner); i++ {
		if inner[i] == '\\' && i+1 < len(inner) {
			i++
		}
		b.WriteByte(inner[i])
	}
	return b.String()
}
