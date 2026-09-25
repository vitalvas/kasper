package sfv

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func mathInf() float64 { return math.Inf(1) }

func TestSerialize(t *testing.T) {
	tests := []struct {
		name string
		item BareItem
		want string
	}{
		{"integer", Integer(42), "42"},
		{"negative", Integer(-42), "-42"},
		{"decimal", Decimal(3.14), "3.14"},
		{"decimal round half even", Decimal(1.0005), "1.0"},
		{"decimal one digit", Decimal(1.5), "1.5"},
		{"decimal negative", Decimal(-2.75), "-2.75"},
		{"string", String(`a"b`), `"a\"b"`},
		{"token", Token("app/json"), "app/json"},
		{"bytes", ByteSequence([]byte("hi")), ":aGk=:"},
		{"bool true", Boolean(true), "?1"},
		{"bool false", Boolean(false), "?0"},
		{"date", Date(time.Unix(1659578233, 0)), "@1659578233"},
		{"display", DisplayString("café"), `%"caf%c3%a9"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.item.String())
		})
	}
}

func TestSerializeCompound(t *testing.T) {
	t.Run("item with params", func(t *testing.T) {
		params := Parameters{
			{Key: "a", Value: Integer(1)},
			{Key: "b", Value: Boolean(true)},
		}
		it := Item{Value: Integer(5), Params: params}
		assert.Equal(t, "5;a=1;b", it.String())
	})

	t.Run("list", func(t *testing.T) {
		l := List{
			{Item: Item{Value: Token("a")}},
			{Item: Item{Value: String("b")}},
		}
		assert.Equal(t, `a, "b"`, l.String())
	})

	t.Run("inner list", func(t *testing.T) {
		items := []Item{
			{Value: Token("x")},
			{Value: Token("y")},
		}
		il := InnerList{Items: items, Params: Parameters{{Key: "n", Value: Integer(2)}}}
		assert.Equal(t, "(x y);n=2", il.String())
	})

	t.Run("dictionary with bool shorthand", func(t *testing.T) {
		d := Dictionary{
			{Key: "a", Member: Member{Item: Item{Value: Boolean(true)}}},
			{Key: "b", Member: Member{Item: Item{Value: Integer(3)}}},
		}
		assert.Equal(t, "a, b=3", d.String())
	})

	t.Run("dictionary with inner list", func(t *testing.T) {
		inner := InnerList{Items: []Item{{Value: Token("apple")}}}
		d := Dictionary{
			{Key: "f", Member: Member{IsInnerList: true, InnerList: inner}},
		}
		assert.Equal(t, "f=(apple)", d.String())
	})
}

func BenchmarkSerializeItem(b *testing.B) {
	params := Parameters{
		{Key: "a", Value: Integer(1)},
		{Key: "b", Value: String("x")},
		{Key: "c", Value: Boolean(true)},
	}
	it := Item{Value: Integer(10), Params: params}
	b.ReportAllocs()
	for b.Loop() {
		_ = it.String()
	}
}

func BenchmarkSerializeList(b *testing.B) {
	innerItems := []Item{
		{Value: String("foo")},
		{Value: String("bar")},
	}
	inner := InnerList{Items: innerItems, Params: Parameters{{Key: "lvl", Value: Integer(5)}}}
	l := List{
		{IsInnerList: true, InnerList: inner},
		{Item: Item{Value: Token("sugar")}},
		{Item: Item{Value: Token("tea")}},
	}
	b.ReportAllocs()
	for b.Loop() {
		_ = l.String()
	}
}

func TestDecimalSerializeEdges(t *testing.T) {
	assert.Equal(t, "", serializeDecimal(mathInf()))
	assert.Equal(t, "0.0", Decimal(0).String())
	assert.Equal(t, "-0.5", Decimal(-0.5).String())
	assert.Equal(t, "123.456", Decimal(123.456).String())
}
