package websocket

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatCloseMessage(t *testing.T) {
	tests := []struct {
		name     string
		code     int
		text     string
		expected []byte
	}{
		{
			name:     "Normal closure with text",
			code:     CloseNormalClosure,
			text:     "goodbye",
			expected: []byte{0x03, 0xe8, 'g', 'o', 'o', 'd', 'b', 'y', 'e'},
		},
		{
			name:     "Normal closure without text",
			code:     CloseNormalClosure,
			text:     "",
			expected: []byte{0x03, 0xe8},
		},
		{
			name:     "No status received returns empty",
			code:     CloseNoStatusReceived,
			text:     "ignored",
			expected: []byte{},
		},
		{
			name:     "Going away",
			code:     CloseGoingAway,
			text:     "bye",
			expected: []byte{0x03, 0xe9, 'b', 'y', 'e'},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatCloseMessage(tt.code, tt.text)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsCloseError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		codes    []int
		expected bool
	}{
		{
			name:     "Matching close error",
			err:      &CloseError{Code: CloseNormalClosure, Text: "bye"},
			codes:    []int{CloseNormalClosure, CloseGoingAway},
			expected: true,
		},
		{
			name:     "Non-matching close error",
			err:      &CloseError{Code: CloseProtocolError, Text: "error"},
			codes:    []int{CloseNormalClosure, CloseGoingAway},
			expected: false,
		},
		{
			name:     "Not a close error",
			err:      errors.New("some error"),
			codes:    []int{CloseNormalClosure},
			expected: false,
		},
		{
			name:     "Nil error",
			err:      nil,
			codes:    []int{CloseNormalClosure},
			expected: false,
		},
		{
			name:     "Single matching code",
			err:      &CloseError{Code: CloseGoingAway, Text: ""},
			codes:    []int{CloseGoingAway},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsCloseError(tt.err, tt.codes...)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsUnexpectedCloseError(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		expectedCodes []int
		expected      bool
	}{
		{
			name:          "Expected close code",
			err:           &CloseError{Code: CloseNormalClosure, Text: "bye"},
			expectedCodes: []int{CloseNormalClosure, CloseGoingAway},
			expected:      false,
		},
		{
			name:          "Unexpected close code",
			err:           &CloseError{Code: CloseProtocolError, Text: "error"},
			expectedCodes: []int{CloseNormalClosure, CloseGoingAway},
			expected:      true,
		},
		{
			name:          "Not a close error",
			err:           errors.New("some error"),
			expectedCodes: []int{CloseNormalClosure},
			expected:      false,
		},
		{
			name:          "Nil error",
			err:           nil,
			expectedCodes: []int{CloseNormalClosure},
			expected:      false,
		},
		{
			name:          "Empty expected codes with close error",
			err:           &CloseError{Code: CloseNormalClosure, Text: ""},
			expectedCodes: []int{},
			expected:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsUnexpectedCloseError(tt.err, tt.expectedCodes...)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBufferPoolInterface(t *testing.T) {
	t.Run("Interface compliance", func(_ *testing.T) {
		var _ BufferPool = (*testBufferPool)(nil)
	})
}

type testBufferPool struct {
	buffers []any
}

func (p *testBufferPool) Get() any {
	if len(p.buffers) == 0 {
		return make([]byte, 1024)
	}
	buf := p.buffers[len(p.buffers)-1]
	p.buffers = p.buffers[:len(p.buffers)-1]
	return buf
}

func (p *testBufferPool) Put(buf any) {
	p.buffers = append(p.buffers, buf)
}

func BenchmarkComputeAcceptKey(b *testing.B) {
	key := "dGhlIHNhbXBsZSBub25jZQ=="

	for b.Loop() {
		_ = computeAcceptKey(key)
	}
}

func FuzzEqualASCIIFold(f *testing.F) {
	f.Add("abc", "abc")
	f.Add("ABC", "abc")
	f.Add("abc", "ABC")
	f.Add("AbC", "aBc")
	f.Add("abc", "abcd")
	f.Add("", "")
	f.Add("websocket", "WebSocket")

	f.Fuzz(func(t *testing.T, s1, s2 string) {
		if len(s1) > 1000 || len(s2) > 1000 {
			return
		}

		result := equalASCIIFold(s1, s2)

		if len(s1) != len(s2) && result {
			t.Errorf("equalASCIIFold returned true for strings of different length")
		}
	})
}

func FuzzComputeAcceptKey(f *testing.F) {
	f.Add("dGhlIHNhbXBsZSBub25jZQ==")
	f.Add("xqBt3ImNzJbYqRINxEFlkg==")
	f.Add("")
	f.Add("short")

	f.Fuzz(func(t *testing.T, key string) {
		result := computeAcceptKey(key)

		if result == "" {
			t.Errorf("computeAcceptKey returned empty string")
		}

		result2 := computeAcceptKey(key)
		if result != result2 {
			t.Errorf("computeAcceptKey not deterministic")
		}
	})
}
