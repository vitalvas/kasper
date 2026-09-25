package sfv

import (
	"errors"
	"time"
)

// Sentinel errors returned by the parser.
var (
	// ErrParse is returned when input does not conform to the RFC 9651
	// grammar. It is the base error for all parse failures; use errors.Is.
	ErrParse = errors.New("sfv: parse error")
	// ErrTrailing is returned when characters remain after a complete
	// structured field value has been parsed.
	ErrTrailing = errors.New("sfv: unexpected trailing characters")
)

// Kind identifies the type of a bare item.
type Kind int

// Bare item kinds per RFC 9651 Section 3.3.
const (
	KindInteger Kind = iota
	KindDecimal
	KindString
	KindToken
	KindByteSequence
	KindBoolean
	KindDate
	KindDisplayString
)

// Limits from RFC 9651 Section 3.3.1 and 3.3.2.
const (
	maxIntegerDigits = 15
	maxDecimalIntDig = 12
	minInteger       = -999999999999999
	maxInteger       = 999999999999999
)

// BareItem is a single typed value: Integer, Decimal, String, Token, Byte
// Sequence, Boolean, Date, or Display String. Exactly one field is meaningful,
// selected by Kind.
type BareItem struct {
	Kind    Kind
	Integer int64     // KindInteger, KindDate (Unix seconds)
	Decimal float64   // KindDecimal
	Str     string    // KindString, KindToken, KindDisplayString
	Bytes   []byte    // KindByteSequence
	Boolean bool      // KindBoolean
	Date    time.Time // KindDate (mirror of Integer for convenience)
}

// Parameter is a single key/value pair. Boolean-true parameters carry a
// BareItem of KindBoolean with Boolean == true and serialize as the bare key.
type Parameter struct {
	Key   string
	Value BareItem
}

// Parameters is an ordered list of parameters. Order is preserved as parsed or
// appended; when a key repeats during parsing the last value wins but the
// original position is kept.
type Parameters []Parameter

// Get returns the value for key and whether it was present.
func (p Parameters) Get(key string) (BareItem, bool) {
	for i := range p {
		if p[i].Key == key {
			return p[i].Value, true
		}
	}
	return BareItem{}, false
}

// Item is a bare item with optional parameters.
type Item struct {
	Value  BareItem
	Params Parameters
}

// InnerList is a sequence of Items with optional parameters, per RFC 9651
// Section 3.1.1.
type InnerList struct {
	Items  []Item
	Params Parameters
}

// Member is one element of a List or a Dictionary value: either a single Item
// or an Inner List. IsInnerList selects which field is meaningful.
type Member struct {
	IsInnerList bool
	Item        Item
	InnerList   InnerList
}

// List is a sequence of Members, per RFC 9651 Section 3.1.
type List []Member

// DictEntry is one key/Member pair in a Dictionary.
type DictEntry struct {
	Key    string
	Member Member
}

// Dictionary is an ordered mapping of keys to Members, per RFC 9651
// Section 3.2. Order is preserved; on a repeated key during parsing the last
// value wins while the key keeps its earliest position.
type Dictionary []DictEntry

// Get returns the Member for key and whether it was present.
func (d Dictionary) Get(key string) (Member, bool) {
	for i := range d {
		if d[i].Key == key {
			return d[i].Member, true
		}
	}
	return Member{}, false
}

// Constructor helpers for building values programmatically.

// Integer returns an Integer bare item.
func Integer(v int64) BareItem { return BareItem{Kind: KindInteger, Integer: v} }

// Decimal returns a Decimal bare item.
func Decimal(v float64) BareItem { return BareItem{Kind: KindDecimal, Decimal: v} }

// String returns a String bare item.
func String(v string) BareItem { return BareItem{Kind: KindString, Str: v} }

// Token returns a Token bare item.
func Token(v string) BareItem { return BareItem{Kind: KindToken, Str: v} }

// ByteSequence returns a Byte Sequence bare item.
func ByteSequence(v []byte) BareItem { return BareItem{Kind: KindByteSequence, Bytes: v} }

// Boolean returns a Boolean bare item.
func Boolean(v bool) BareItem { return BareItem{Kind: KindBoolean, Boolean: v} }

// Date returns a Date bare item from t, truncated to whole seconds.
func Date(t time.Time) BareItem {
	s := t.Unix()
	return BareItem{Kind: KindDate, Integer: s, Date: time.Unix(s, 0).UTC()}
}

// DisplayString returns a Display String bare item holding a Unicode string.
func DisplayString(v string) BareItem { return BareItem{Kind: KindDisplayString, Str: v} }
