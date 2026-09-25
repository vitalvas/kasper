package sfv

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoundTrip(t *testing.T) {
	items := []string{
		"42",
		"-17",
		"3.14",
		`"hello world"`,
		"foo",
		"?1",
		":aGVsbG8=:",
		"@1659578233",
		`%"caf%c3%a9"`,
		`10;a=1;b="x";c`,
	}
	for _, in := range items {
		t.Run(in, func(t *testing.T) {
			it, err := ParseItem(in)
			require.NoError(t, err)
			assert.Equal(t, in, it.String())
		})
	}

	lists := []string{
		`sugar, tea, "rum"`,
		`("foo" "bar");lvl=5, ("baz")`,
		"a, b, c",
	}
	for _, in := range lists {
		t.Run("list", func(t *testing.T) {
			l, err := ParseList(in)
			require.NoError(t, err)
			assert.Equal(t, in, l.String())
		})
	}

	dicts := []string{
		`en="Applepie", da=:aGVsbG8=:`,
		`a, b=?0, c`,
		`rating=1.5, fruits=(apple orange)`,
	}
	for _, in := range dicts {
		t.Run("dict", func(t *testing.T) {
			d, err := ParseDictionary(in)
			require.NoError(t, err)
			assert.Equal(t, in, d.String())
		})
	}
}

func TestParametersGet(t *testing.T) {
	p := Parameters{
		{Key: "a", Value: Integer(1)},
		{Key: "b", Value: Boolean(true)},
	}
	v, ok := p.Get("a")
	require.True(t, ok)
	assert.Equal(t, int64(1), v.Integer)
	_, ok = p.Get("missing")
	assert.False(t, ok)
}

func FuzzParseSerialize(f *testing.F) {
	seeds := []string{
		"42", "3.14", `"x"`, "tok", "?1", ":aGk=:", "@1", `%"a"`,
		`a, b=2, c=(1 2)`, `10;p=1`,
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		// Whichever parser succeeds must round-trip stably: parsing the
		// serialization yields the same serialization.
		if it, err := ParseItem(in); err == nil {
			s1 := it.String()
			it2, err := ParseItem(s1)
			require.NoError(t, err)
			assert.Equal(t, s1, it2.String())
		}
		if l, err := ParseList(in); err == nil {
			s1 := l.String()
			l2, err := ParseList(s1)
			require.NoError(t, err)
			assert.Equal(t, s1, l2.String())
		}
		if d, err := ParseDictionary(in); err == nil {
			s1 := d.String()
			d2, err := ParseDictionary(s1)
			require.NoError(t, err)
			assert.Equal(t, s1, d2.String())
		}
	})
}
