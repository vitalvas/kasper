package sfv

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseItem(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    BareItem
		wantErr bool
	}{
		{"integer", "42", Integer(42), false},
		{"negative integer", "-42", Integer(-42), false},
		{"max integer", "999999999999999", Integer(maxInteger), false},
		{"integer too long", "1000000000000000", BareItem{}, true},
		{"decimal", "3.14", Decimal(3.14), false},
		{"decimal one frac", "3.1", Decimal(3.1), false},
		{"decimal no frac", "3.", BareItem{}, true},
		{"decimal four frac", "1.2345", BareItem{}, true},
		{"decimal int too long", "1234567890123.0", BareItem{}, true},
		{"string", `"hello"`, String("hello"), false},
		{"string escape", `"a\"b\\c"`, String(`a"b\c`), false},
		{"string bad escape", `"a\nb"`, BareItem{}, true},
		{"string unterminated", `"abc`, BareItem{}, true},
		{"token", "foo", Token("foo"), false},
		{"token with colon", "application/json", Token("application/json"), false},
		{"token star", "*", Token("*"), false},
		{"boolean true", "?1", Boolean(true), false},
		{"boolean false", "?0", Boolean(false), false},
		{"boolean bad", "?2", BareItem{}, true},
		{"byte sequence", ":aGVsbG8=:", ByteSequence([]byte("hello")), false},
		{"byte sequence bad", ":not base64!:", BareItem{}, true},
		{"byte sequence unterminated", ":aGVsbG8=", BareItem{}, true},
		{"date", "@1659578233", Date(time.Unix(1659578233, 0).UTC()), false},
		{"date non-integer", "@3.5", BareItem{}, true},
		{"date empty", "@", BareItem{}, true},
		{"display string", `%"caf%c3%a9"`, DisplayString("café"), false},
		{"display string plain", `%"hello"`, DisplayString("hello"), false},
		{"display string bad escape", `%"a%zz"`, BareItem{}, true},
		{"display string no quote", `%caf`, BareItem{}, true},
		{"empty", "", BareItem{}, true},
		{"trailing", "42 43", BareItem{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			it, err := ParseItem(tt.in)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want.Kind, it.Value.Kind)
			switch tt.want.Kind {
			case KindInteger, KindDate:
				assert.Equal(t, tt.want.Integer, it.Value.Integer)
			case KindDecimal:
				assert.InDelta(t, tt.want.Decimal, it.Value.Decimal, 1e-9)
			case KindString, KindToken, KindDisplayString:
				assert.Equal(t, tt.want.Str, it.Value.Str)
			case KindByteSequence:
				assert.Equal(t, tt.want.Bytes, it.Value.Bytes)
			case KindBoolean:
				assert.Equal(t, tt.want.Boolean, it.Value.Boolean)
			}
		})
	}
}

func TestParseItemParameters(t *testing.T) {
	t.Run("item with parameters", func(t *testing.T) {
		it, err := ParseItem(`10;a=1;b="x";c`)
		require.NoError(t, err)
		assert.Equal(t, int64(10), it.Value.Integer)
		require.Len(t, it.Params, 3)
		a, ok := it.Params.Get("a")
		require.True(t, ok)
		assert.Equal(t, int64(1), a.Integer)
		c, ok := it.Params.Get("c")
		require.True(t, ok)
		assert.True(t, c.Boolean)
		_, ok = it.Params.Get("missing")
		assert.False(t, ok)
	})

	t.Run("duplicate parameter last wins keeps position", func(t *testing.T) {
		it, err := ParseItem(`1;a=1;b=2;a=3`)
		require.NoError(t, err)
		require.Len(t, it.Params, 2)
		assert.Equal(t, "a", it.Params[0].Key)
		assert.Equal(t, int64(3), it.Params[0].Value.Integer)
	})
}

func TestParseList(t *testing.T) {
	t.Run("mixed members", func(t *testing.T) {
		l, err := ParseList(`sugar, tea, "rum"`)
		require.NoError(t, err)
		require.Len(t, l, 3)
		assert.Equal(t, "sugar", l[0].Item.Value.Str)
		assert.Equal(t, "rum", l[2].Item.Value.Str)
	})

	t.Run("inner lists", func(t *testing.T) {
		l, err := ParseList(`("foo" "bar");lvl=5, ("baz")`)
		require.NoError(t, err)
		require.Len(t, l, 2)
		require.True(t, l[0].IsInnerList)
		require.Len(t, l[0].InnerList.Items, 2)
		lvl, ok := l[0].InnerList.Params.Get("lvl")
		require.True(t, ok)
		assert.Equal(t, int64(5), lvl.Integer)
		require.True(t, l[1].IsInnerList)
		require.Len(t, l[1].InnerList.Items, 1)
	})

	t.Run("empty list", func(t *testing.T) {
		l, err := ParseList("")
		require.NoError(t, err)
		assert.Empty(t, l)
	})

	t.Run("trailing comma", func(t *testing.T) {
		_, err := ParseList("a, b,")
		require.Error(t, err)
	})

	t.Run("unterminated inner list", func(t *testing.T) {
		_, err := ParseList("(a b")
		require.Error(t, err)
	})
}

func TestInnerListParams(t *testing.T) {
	l, err := ParseList(`(a b);key="val";flag`)
	require.NoError(t, err)
	require.Len(t, l, 1)
	require.True(t, l[0].IsInnerList)
	v, ok := l[0].InnerList.Params.Get("key")
	require.True(t, ok)
	assert.Equal(t, "val", v.Str)
	f, ok := l[0].InnerList.Params.Get("flag")
	require.True(t, ok)
	assert.True(t, f.Boolean)
	assert.Equal(t, `(a b);key="val";flag`, l.String())
}

func TestParseDictionary(t *testing.T) {
	t.Run("basic dictionary", func(t *testing.T) {
		d, err := ParseDictionary(`en="Applepie", da=:aGVsbG8=:`)
		require.NoError(t, err)
		require.Len(t, d, 2)
		en, ok := d.Get("en")
		require.True(t, ok)
		assert.Equal(t, "Applepie", en.Item.Value.Str)
	})

	t.Run("boolean true shorthand", func(t *testing.T) {
		d, err := ParseDictionary(`a, b=?0, c`)
		require.NoError(t, err)
		require.Len(t, d, 3)
		a, _ := d.Get("a")
		assert.True(t, a.Item.Value.Boolean)
		b, _ := d.Get("b")
		assert.False(t, b.Item.Value.Boolean)
	})

	t.Run("inner list value", func(t *testing.T) {
		d, err := ParseDictionary(`rating=1.5, fruits=(apple orange)`)
		require.NoError(t, err)
		fruits, ok := d.Get("fruits")
		require.True(t, ok)
		require.True(t, fruits.IsInnerList)
		assert.Len(t, fruits.InnerList.Items, 2)
	})

	t.Run("duplicate key last wins keeps position", func(t *testing.T) {
		d, err := ParseDictionary(`a=1, b=2, a=3`)
		require.NoError(t, err)
		require.Len(t, d, 2)
		assert.Equal(t, "a", d[0].Key)
		assert.Equal(t, int64(3), d[0].Member.Item.Value.Integer)
	})

	t.Run("missing key", func(t *testing.T) {
		d, err := ParseDictionary("a=1")
		require.NoError(t, err)
		_, ok := d.Get("z")
		assert.False(t, ok)
	})
}

func TestParseErrors(t *testing.T) {
	t.Run("list expected comma", func(t *testing.T) {
		_, err := ParseList("a b")
		require.Error(t, err)
	})
	t.Run("dict expected comma", func(t *testing.T) {
		_, err := ParseDictionary("a=1 b=2")
		require.Error(t, err)
	})
	t.Run("dict bad key", func(t *testing.T) {
		_, err := ParseDictionary("A=1")
		require.Error(t, err)
	})
	t.Run("dict trailing comma", func(t *testing.T) {
		_, err := ParseDictionary("a=1,")
		require.Error(t, err)
	})
	t.Run("list trailing content", func(t *testing.T) {
		_, err := ParseList("(a))")
		require.Error(t, err)
	})
	t.Run("dict trailing content", func(t *testing.T) {
		_, err := ParseDictionary("a=1)")
		require.Error(t, err)
	})
	t.Run("integer negative in range", func(t *testing.T) {
		_, err := ParseItem("-999999999999999")
		require.NoError(t, err)
	})
	t.Run("boolean eof after question", func(t *testing.T) {
		_, err := ParseItem("?")
		require.Error(t, err)
	})
}

func BenchmarkParseItem(b *testing.B) {
	const in = `10;a=1;b="x";c`
	b.ReportAllocs()
	for b.Loop() {
		if _, err := ParseItem(in); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseList(b *testing.B) {
	const in = `("foo" "bar");lvl=5, ("baz"), sugar, tea`
	b.ReportAllocs()
	for b.Loop() {
		if _, err := ParseList(in); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseDictionary(b *testing.B) {
	const in = `en="Applepie", da=:aGVsbG8=:, rating=1.5, fruits=(apple orange)`
	b.ReportAllocs()
	for b.Loop() {
		if _, err := ParseDictionary(in); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseBareInteger(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := ParseItem("123456789"); err != nil {
			b.Fatal(err)
		}
	}
}

func TestParseErrorPropagation(t *testing.T) {
	cases := []struct {
		name string
		fn   func(string) error
		in   string
	}{
		{"list bad item", func(s string) error { _, e := ParseList(s); return e }, `"unterminated`},
		{"list bad inner item", func(s string) error { _, e := ParseList(s); return e }, `(?9)`},
		{"list inner bad separator", func(s string) error { _, e := ParseList(s); return e }, `(a,b)`},
		{"list bad param", func(s string) error { _, e := ParseList(s); return e }, `a;=1`},
		{"dict bad value", func(s string) error { _, e := ParseDictionary(s); return e }, `a=?9`},
		{"dict bad inner value", func(s string) error { _, e := ParseDictionary(s); return e }, `a=(?9)`},
		{"dict value eof after eq", func(s string) error { _, e := ParseDictionary(s); return e }, `a=`},
		{"item bad param key", func(s string) error { _, e := ParseItem(s); return e }, `1;9`},
		{"integer bare minus", func(s string) error { _, e := ParseItem(s); return e }, `-`},
		{"display truncated escape", func(s string) error { _, e := ParseItem(s); return e }, `%"a%c`},
		{"display unterminated", func(s string) error { _, e := ParseItem(s); return e }, `%"abc`},
		{"display control char", func(s string) error { _, e := ParseItem(s); return e }, "%\"a\x01\""},
		{"string control char", func(s string) error { _, e := ParseItem(s); return e }, "\"a\x01\""},
		{"param on inner list value", func(s string) error { _, e := ParseList(s); return e }, `(a);9`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Error(t, tc.fn(tc.in))
		})
	}
}
