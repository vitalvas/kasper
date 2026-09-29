package sfv

import (
	"encoding/base64"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// String serializes the bare item per RFC 9651 Section 4.1.3. It returns an
// empty string for a value that cannot be represented (for example a
// non-finite Decimal); callers building fields from validated input do not hit
// that path.
func (b BareItem) String() string {
	if !b.valid() {
		return ""
	}
	var sb strings.Builder
	b.serialize(&sb)
	return sb.String()
}

func (b BareItem) serialize(sb *strings.Builder) {
	switch b.Kind {
	case KindInteger:
		sb.WriteString(strconv.FormatInt(b.Integer, 10))
	case KindDecimal:
		sb.WriteString(serializeDecimal(b.Decimal))
	case KindString:
		serializeString(sb, b.Str)
	case KindToken:
		sb.WriteString(b.Str)
	case KindByteSequence:
		sb.WriteByte(':')
		sb.WriteString(base64.StdEncoding.EncodeToString(b.Bytes))
		sb.WriteByte(':')
	case KindBoolean:
		if b.Boolean {
			sb.WriteString("?1")
		} else {
			sb.WriteString("?0")
		}
	case KindDate:
		sb.WriteByte('@')
		sb.WriteString(strconv.FormatInt(b.Integer, 10))
	case KindDisplayString:
		serializeDisplayString(sb, b.Str)
	}
}

// serializeDecimal formats a Decimal per RFC 9651 Section 4.1.5: at least one
// and at most three fractional digits, round-half-to-even.
func serializeDecimal(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) >= 1e12 {
		return ""
	}
	// Round to 3 fractional digits, half-to-even.
	scaled := math.RoundToEven(v * 1000)
	if math.Abs(scaled) > 999999999999999 {
		return ""
	}
	i := int64(scaled)
	neg := i < 0
	if neg {
		i = -i
	}
	intPart := i / 1000
	frac := i % 1000

	var sb strings.Builder
	if neg {
		sb.WriteByte('-')
	}
	sb.WriteString(strconv.FormatInt(intPart, 10))
	sb.WriteByte('.')
	// Trim trailing zeros but keep at least one fractional digit.
	switch {
	case frac%100 == 0:
		sb.WriteByte(byte('0' + frac/100))
	case frac%10 == 0:
		sb.WriteByte(byte('0' + frac/100))
		sb.WriteByte(byte('0' + (frac/10)%10))
	default:
		sb.WriteByte(byte('0' + frac/100))
		sb.WriteByte(byte('0' + (frac/10)%10))
		sb.WriteByte(byte('0' + frac%10))
	}
	return sb.String()
}

// serializeString writes an sf-string per RFC 9651 Section 4.1.6.
func serializeString(sb *strings.Builder, s string) {
	sb.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' || c == '"' {
			sb.WriteByte('\\')
		}
		sb.WriteByte(c)
	}
	sb.WriteByte('"')
}

// serializeDisplayString writes an sf-displaystring per RFC 9651 Section
// 4.1.10: %"..." with non-ASCII and % and " percent-encoded as UTF-8 bytes.
func serializeDisplayString(sb *strings.Builder, s string) {
	sb.WriteString(`%"`)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '%' || c == '"' || c < 0x20 || c > 0x7e {
			sb.WriteByte('%')
			const hex = "0123456789abcdef"
			sb.WriteByte(hex[c>>4])
			sb.WriteByte(hex[c&0x0f])
			continue
		}
		sb.WriteByte(c)
	}
	sb.WriteByte('"')
}

func (p Parameters) serialize(sb *strings.Builder) {
	for _, param := range p {
		sb.WriteByte(';')
		sb.WriteString(param.Key)
		// Boolean-true parameters serialize as the bare key.
		if param.Value.Kind == KindBoolean && param.Value.Boolean {
			continue
		}
		sb.WriteByte('=')
		param.Value.serialize(sb)
	}
}

// String serializes the Item per RFC 9651 Section 4.1.3.
func (it Item) String() string {
	if !it.valid() {
		return ""
	}
	var sb strings.Builder
	sb.Grow(16 + 8*len(it.Params))
	it.serialize(&sb)
	return sb.String()
}

func (it Item) serialize(sb *strings.Builder) {
	it.Value.serialize(sb)
	it.Params.serialize(sb)
}

// String serializes the Inner List per RFC 9651 Section 4.1.1.1.
func (il InnerList) String() string {
	if !il.valid() {
		return ""
	}
	var sb strings.Builder
	il.serialize(&sb)
	return sb.String()
}

func (il InnerList) serialize(sb *strings.Builder) {
	sb.WriteByte('(')
	for i := range il.Items {
		if i > 0 {
			sb.WriteByte(' ')
		}
		il.Items[i].serialize(sb)
	}
	sb.WriteByte(')')
	il.Params.serialize(sb)
}

func (m Member) serialize(sb *strings.Builder) {
	if m.IsInnerList {
		m.InnerList.serialize(sb)
		return
	}
	m.Item.serialize(sb)
}

// String serializes the List per RFC 9651 Section 4.1.1.
func (l List) String() string {
	if !validMembers(l) {
		return ""
	}
	var sb strings.Builder
	sb.Grow(16 * len(l))
	for i := range l {
		if i > 0 {
			sb.WriteString(", ")
		}
		l[i].serialize(&sb)
	}
	return sb.String()
}

// String serializes the Dictionary per RFC 9651 Section 4.1.2.
func (d Dictionary) String() string {
	if !d.valid() {
		return ""
	}
	var sb strings.Builder
	sb.Grow(24 * len(d))
	for i := range d {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(d[i].Key)
		m := d[i].Member
		// A key whose value is a boolean-true Item serializes as the bare key.
		if !m.IsInnerList && m.Item.Value.Kind == KindBoolean && m.Item.Value.Boolean {
			m.Item.Params.serialize(&sb)
			continue
		}
		sb.WriteByte('=')
		m.serialize(&sb)
	}
	return sb.String()
}

// Validation is performed before serialization so invalid nested values never
// produce a partial field that could be interpreted as a different value.
func (b BareItem) valid() bool {
	switch b.Kind {
	case KindInteger, KindDate:
		return b.Integer >= minInteger && b.Integer <= maxInteger
	case KindDecimal:
		return serializeDecimal(b.Decimal) != ""
	case KindString:
		for i := range len(b.Str) {
			if b.Str[i] < 0x20 || b.Str[i] > 0x7e {
				return false
			}
		}
		return true
	case KindToken:
		if len(b.Str) == 0 {
			return false
		}
		c := b.Str[0]
		if c != '*' && (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') {
			return false
		}
		for i := range len(b.Str) {
			if !isTokenChar(b.Str[i]) {
				return false
			}
		}
		return true
	case KindByteSequence, KindBoolean:
		return true
	case KindDisplayString:
		return utf8.ValidString(b.Str)
	default:
		return false
	}
}

func validKey(key string) bool {
	p := parser{s: key}
	_, err := p.parseKey()
	return err == nil && p.eof()
}

func (p Parameters) valid() bool {
	for _, param := range p {
		if !validKey(param.Key) || !param.Value.valid() {
			return false
		}
	}
	return true
}

func (it Item) valid() bool { return it.Value.valid() && it.Params.valid() }

func (il InnerList) valid() bool {
	if !il.Params.valid() {
		return false
	}
	for _, it := range il.Items {
		if !it.valid() {
			return false
		}
	}
	return true
}

func (m Member) valid() bool {
	if m.IsInnerList {
		return m.InnerList.valid()
	}
	return m.Item.valid()
}

func validMembers(l List) bool {
	for _, m := range l {
		if !m.valid() {
			return false
		}
	}
	return true
}

func (d Dictionary) valid() bool {
	for _, entry := range d {
		if !validKey(entry.Key) || !entry.Member.valid() {
			return false
		}
	}
	return true
}
