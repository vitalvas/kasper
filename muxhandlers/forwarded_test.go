package muxhandlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseForwardedElements(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   []ForwardedElement
	}{
		{
			name:   "empty",
			header: "",
			want:   nil,
		},
		{
			name:   "single for",
			header: "for=192.0.2.60",
			want:   []ForwardedElement{{For: "192.0.2.60"}},
		},
		{
			name:   "all params",
			header: `for=192.0.2.60;proto=http;by=203.0.113.43;host=example.com`,
			want: []ForwardedElement{
				{For: "192.0.2.60", Proto: "http", By: "203.0.113.43", Host: "example.com"},
			},
		},
		{
			name:   "multiple elements in order",
			header: `for=192.0.2.43, for=198.51.100.17`,
			want: []ForwardedElement{
				{For: "192.0.2.43"},
				{For: "198.51.100.17"},
			},
		},
		{
			name:   "obfuscated and unknown identifiers preserved",
			header: `for=_hidden, for=unknown`,
			want: []ForwardedElement{
				{For: "_hidden"},
				{For: "unknown"},
			},
		},
		{
			name:   "quoted ipv6 with port",
			header: `for="[2001:db8:cafe::17]:4711"`,
			want:   []ForwardedElement{{For: "[2001:db8:cafe::17]:4711"}},
		},
		{
			name:   "case-insensitive keys",
			header: `For=192.0.2.60;Proto=HTTPS`,
			want:   []ForwardedElement{{For: "192.0.2.60", Proto: "HTTPS"}},
		},
		{
			name:   "quoted value containing separators",
			header: `host="a;b,c";for=192.0.2.60`,
			want:   []ForwardedElement{{Host: "a;b,c", For: "192.0.2.60"}},
		},
		{
			name:   "unknown params ignored",
			header: `for=192.0.2.60;secret=abc`,
			want:   []ForwardedElement{{For: "192.0.2.60"}},
		},
		{
			name:   "param without equals skipped",
			header: `for=192.0.2.60;garbage`,
			want:   []ForwardedElement{{For: "192.0.2.60"}},
		},
		{
			name:   "whitespace only",
			header: "   ",
			want:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseForwarded(tt.header)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseForwardedEscapedQuotes(t *testing.T) {
	got := ParseForwarded(`host="a\"b"`)
	require.Len(t, got, 1)
	assert.Equal(t, `a"b`, got[0].Host)
}

func TestParseForwardedDropsEmptyElements(t *testing.T) {
	// A stray comma produces no empty element.
	got := ParseForwarded(`for=192.0.2.60, , for=198.51.100.17`)
	require.Len(t, got, 2)
	assert.Equal(t, "192.0.2.60", got[0].For)
	assert.Equal(t, "198.51.100.17", got[1].For)
}
