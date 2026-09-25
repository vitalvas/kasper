package sfv

import (
	"encoding/base64"
	"math"
	"strconv"
	"strings"
)

// String serializes the bare item per RFC 9651 Section 4.1.3. It returns an
// empty string for a value that cannot be represented (for example a
// non-finite Decimal); callers building fields from validated input do not hit
// that path.
func (b BareItem) String() string {
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
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return ""
	}
	// Round to 3 fractional digits, half-to-even.
	scaled := math.RoundToEven(v * 1000)
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
