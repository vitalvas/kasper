package sfv

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// parser holds the input and cursor for the RFC 9651 parsing algorithm
// (Section 4.2). The input must already have leading/trailing OWS removed by
// the entry points.
type parser struct {
	s   string
	pos int
}

func (p *parser) eof() bool  { return p.pos >= len(p.s) }
func (p *parser) peek() byte { return p.s[p.pos] }

func (p *parser) errf(format string, args ...any) error {
	return fmt.Errorf("%w: %s at offset %d", ErrParse, fmt.Sprintf(format, args...), p.pos)
}

// ParseItem parses an sf-item (bare item with parameters) from a field value.
func ParseItem(s string) (Item, error) {
	p := &parser{s: strings.TrimSpace(s)}
	it, err := p.parseItem()
	if err != nil {
		return Item{}, err
	}
	if !p.eof() {
		return Item{}, ErrTrailing
	}
	return it, nil
}

// ParseList parses an sf-list from a field value.
func ParseList(s string) (List, error) {
	p := &parser{s: strings.TrimSpace(s)}
	l, err := p.parseList()
	if err != nil {
		return nil, err
	}
	if !p.eof() {
		return nil, ErrTrailing
	}
	return l, nil
}

// ParseDictionary parses an sf-dictionary from a field value.
func ParseDictionary(s string) (Dictionary, error) {
	p := &parser{s: strings.TrimSpace(s)}
	d, err := p.parseDictionary()
	if err != nil {
		return nil, err
	}
	if !p.eof() {
		return nil, ErrTrailing
	}
	return d, nil
}

func (p *parser) parseList() (List, error) {
	var l List
	for !p.eof() {
		m, err := p.parseMember()
		if err != nil {
			return nil, err
		}
		l = append(l, m)
		p.skipOWS()
		if p.eof() {
			break
		}
		if p.peek() != ',' {
			return nil, p.errf("expected comma in list")
		}
		p.pos++
		p.skipOWS()
		if p.eof() {
			return nil, p.errf("trailing comma in list")
		}
	}
	return l, nil
}

func (p *parser) parseDictionary() (Dictionary, error) {
	var d Dictionary
	for !p.eof() {
		key, err := p.parseKey()
		if err != nil {
			return nil, err
		}
		var m Member
		if !p.eof() && p.peek() == '=' {
			p.pos++
			m, err = p.parseMember()
			if err != nil {
				return nil, err
			}
		} else {
			params, err := p.parseParams()
			if err != nil {
				return nil, err
			}
			m = Member{Item: Item{Value: Boolean(true), Params: params}}
		}
		d = d.set(key, m)
		p.skipOWS()
		if p.eof() {
			break
		}
		if p.peek() != ',' {
			return nil, p.errf("expected comma in dictionary")
		}
		p.pos++
		p.skipOWS()
		if p.eof() {
			return nil, p.errf("trailing comma in dictionary")
		}
	}
	return d, nil
}

// set applies last-wins semantics while preserving the earliest position.
func (d Dictionary) set(key string, m Member) Dictionary {
	for i := range d {
		if d[i].Key == key {
			d[i].Member = m
			return d
		}
	}
	return append(d, DictEntry{Key: key, Member: m})
}

// set applies last-wins semantics for parameters, preserving position.
func (p Parameters) set(key string, v BareItem) Parameters {
	for i := range p {
		if p[i].Key == key {
			p[i].Value = v
			return p
		}
	}
	return append(p, Parameter{Key: key, Value: v})
}

func (p *parser) parseMember() (Member, error) {
	if !p.eof() && p.peek() == '(' {
		il, err := p.parseInnerList()
		if err != nil {
			return Member{}, err
		}
		return Member{IsInnerList: true, InnerList: il}, nil
	}
	it, err := p.parseItem()
	if err != nil {
		return Member{}, err
	}
	return Member{Item: it}, nil
}

func (p *parser) parseInnerList() (InnerList, error) {
	p.pos++ // consume '('
	var il InnerList
	for {
		p.skipSP()
		if p.eof() {
			return InnerList{}, p.errf("unterminated inner list")
		}
		if p.peek() == ')' {
			p.pos++
			break
		}
		it, err := p.parseItem()
		if err != nil {
			return InnerList{}, err
		}
		il.Items = append(il.Items, it)
		if p.eof() {
			return InnerList{}, p.errf("unterminated inner list")
		}
		if c := p.peek(); c != ' ' && c != ')' {
			return InnerList{}, p.errf("expected SP or close paren in inner list")
		}
	}
	params, err := p.parseParams()
	if err != nil {
		return InnerList{}, err
	}
	il.Params = params
	return il, nil
}

func (p *parser) parseItem() (Item, error) {
	bi, err := p.parseBareItem()
	if err != nil {
		return Item{}, err
	}
	params, err := p.parseParams()
	if err != nil {
		return Item{}, err
	}
	return Item{Value: bi, Params: params}, nil
}

func (p *parser) parseParams() (Parameters, error) {
	var params Parameters
	for !p.eof() && p.peek() == ';' {
		p.pos++
		p.skipSP()
		key, err := p.parseKey()
		if err != nil {
			return nil, err
		}
		val := Boolean(true)
		if !p.eof() && p.peek() == '=' {
			p.pos++
			val, err = p.parseBareItem()
			if err != nil {
				return nil, err
			}
		}
		params = params.set(key, val)
	}
	return params, nil
}

func (p *parser) parseKey() (string, error) {
	if p.eof() {
		return "", p.errf("expected key")
	}
	c := p.peek()
	if (c < 'a' || c > 'z') && c != '*' {
		return "", p.errf("key must start with lcalpha or *")
	}
	start := p.pos
	for !p.eof() {
		c := p.peek()
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == '_' || c == '-' || c == '.' || c == '*' {
			p.pos++
			continue
		}
		break
	}
	return p.s[start:p.pos], nil
}

func (p *parser) parseBareItem() (BareItem, error) {
	if p.eof() {
		return BareItem{}, p.errf("expected bare item")
	}
	c := p.peek()
	switch {
	case c == '-' || (c >= '0' && c <= '9'):
		return p.parseNumber()
	case c == '"':
		return p.parseString()
	case c == '?':
		return p.parseBoolean()
	case c == ':':
		return p.parseByteSequence()
	case c == '@':
		return p.parseDate()
	case c == '%':
		return p.parseDisplayString()
	case c == '*' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
		return p.parseToken()
	default:
		return BareItem{}, p.errf("unrecognized bare item")
	}
}

func (p *parser) parseNumber() (BareItem, error) {
	if p.eof() {
		return BareItem{}, p.errf("expected number")
	}
	start := p.pos
	if p.peek() == '-' {
		p.pos++
	}
	if p.eof() || !isDigit(p.peek()) {
		return BareItem{}, p.errf("expected digit in number")
	}
	isDecimal := false
	for !p.eof() {
		c := p.peek()
		if isDigit(c) {
			p.pos++
			continue
		}
		if c == '.' && !isDecimal {
			isDecimal = true
			p.pos++
			continue
		}
		break
	}
	text := p.s[start:p.pos]
	if isDecimal {
		return parseDecimalText(p, text)
	}
	return parseIntegerText(p, text)
}

func parseIntegerText(p *parser, text string) (BareItem, error) {
	digits := strings.TrimPrefix(text, "-")
	if len(digits) > maxIntegerDigits {
		return BareItem{}, p.errf("integer exceeds %d digits", maxIntegerDigits)
	}
	v, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return BareItem{}, p.errf("invalid integer")
	}
	if v < minInteger || v > maxInteger {
		return BareItem{}, p.errf("integer out of range")
	}
	return Integer(v), nil
}

func parseDecimalText(p *parser, text string) (BareItem, error) {
	intPart, fracDigits, _ := strings.Cut(text, ".")
	intDigits := strings.TrimPrefix(intPart, "-")
	if len(intDigits) > maxDecimalIntDig {
		return BareItem{}, p.errf("decimal integer part exceeds %d digits", maxDecimalIntDig)
	}
	if len(fracDigits) == 0 {
		return BareItem{}, p.errf("decimal must have fractional digits")
	}
	if len(fracDigits) > 3 {
		return BareItem{}, p.errf("decimal fractional part exceeds 3 digits")
	}
	v, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return BareItem{}, p.errf("invalid decimal")
	}
	return Decimal(v), nil
}

func (p *parser) parseString() (BareItem, error) {
	p.pos++ // consume opening quote
	// Fast path: scan for the closing quote assuming no escapes. The common
	// case has none, so the value is a slice of the input with no allocation.
	start := p.pos
	for !p.eof() {
		c := p.peek()
		switch {
		case c == '\\':
			return p.parseStringEscaped(start)
		case c == '"':
			s := p.s[start:p.pos]
			p.pos++
			return String(s), nil
		case c < 0x20 || c > 0x7e:
			return BareItem{}, p.errf("invalid character in string")
		}
		p.pos++
	}
	return BareItem{}, p.errf("unterminated string")
}

// parseStringEscaped handles the slow path once a backslash is seen. start is
// the index just after the opening quote; the cursor is at the first
// backslash.
func (p *parser) parseStringEscaped(start int) (BareItem, error) {
	var sb strings.Builder
	sb.WriteString(p.s[start:p.pos])
	for !p.eof() {
		c := p.peek()
		p.pos++
		switch {
		case c == '\\':
			if p.eof() {
				return BareItem{}, p.errf("trailing backslash in string")
			}
			n := p.peek()
			p.pos++
			if n != '\\' && n != '"' {
				return BareItem{}, p.errf("invalid escape in string")
			}
			sb.WriteByte(n)
		case c == '"':
			return String(sb.String()), nil
		case c < 0x20 || c > 0x7e:
			return BareItem{}, p.errf("invalid character in string")
		default:
			sb.WriteByte(c)
		}
	}
	return BareItem{}, p.errf("unterminated string")
}

func (p *parser) parseToken() (BareItem, error) {
	start := p.pos
	p.pos++ // first char already validated as ALPHA or *
	for !p.eof() {
		if isTokenChar(p.peek()) {
			p.pos++
			continue
		}
		break
	}
	return Token(p.s[start:p.pos]), nil
}

func (p *parser) parseBoolean() (BareItem, error) {
	p.pos++ // consume '?'
	if p.eof() {
		return BareItem{}, p.errf("expected 0 or 1 after ?")
	}
	c := p.peek()
	p.pos++
	switch c {
	case '0':
		return Boolean(false), nil
	case '1':
		return Boolean(true), nil
	default:
		return BareItem{}, p.errf("boolean must be ?0 or ?1")
	}
}

func (p *parser) parseByteSequence() (BareItem, error) {
	p.pos++ // consume opening colon
	start := p.pos
	for !p.eof() && p.peek() != ':' {
		p.pos++
	}
	if p.eof() {
		return BareItem{}, p.errf("unterminated byte sequence")
	}
	enc := p.s[start:p.pos]
	p.pos++ // consume closing colon
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return BareItem{}, p.errf("invalid base64 in byte sequence")
	}
	return ByteSequence(raw), nil
}

func (p *parser) parseDate() (BareItem, error) {
	p.pos++ // consume '@'
	bi, err := p.parseNumber()
	if err != nil {
		return BareItem{}, err
	}
	if bi.Kind != KindInteger {
		return BareItem{}, p.errf("date must be an integer")
	}
	return Date(time.Unix(bi.Integer, 0).UTC()), nil
}

func (p *parser) parseDisplayString() (BareItem, error) {
	p.pos++ // consume '%'
	if p.eof() || p.peek() != '"' {
		return BareItem{}, p.errf("display string must begin with %%\"")
	}
	p.pos++ // consume opening quote
	var raw []byte
	for !p.eof() {
		c := p.peek()
		p.pos++
		switch {
		case c == '"':
			if !utf8.Valid(raw) {
				return BareItem{}, p.errf("display string is not valid UTF-8")
			}
			return DisplayString(string(raw)), nil
		case c == '%':
			if p.pos+2 > len(p.s) {
				return BareItem{}, p.errf("truncated percent-escape")
			}
			hi, lo := lowerHexNibble(p.s[p.pos]), lowerHexNibble(p.s[p.pos+1])
			if hi < 0 || lo < 0 {
				return BareItem{}, p.errf("invalid percent-escape")
			}
			raw = append(raw, byte(hi<<4|lo))
			p.pos += 2
		case c < 0x20 || c > 0x7e:
			return BareItem{}, p.errf("invalid character in display string")
		default:
			raw = append(raw, c)
		}
	}
	return BareItem{}, p.errf("unterminated display string")
}

func (p *parser) skipOWS() {
	for !p.eof() && (p.peek() == ' ' || p.peek() == '\t') {
		p.pos++
	}
}

func (p *parser) skipSP() {
	for !p.eof() && p.peek() == ' ' {
		p.pos++
	}
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isTokenChar(c byte) bool {
	// tchar (RFC 9110) plus ':' and '/' per RFC 9651 sf-token.
	switch c {
	case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~', ':', '/':
		return true
	}
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

func lowerHexNibble(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	default:
		return -1
	}
}
