package wsmux

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamDeliverBackpressure(t *testing.T) {
	s := newStream(1, nil, 1) // buffer of 1

	assert.True(t, s.deliver([]byte("a")), "first delivery fits the buffer")
	assert.False(t, s.deliver([]byte("b")), "second delivery overflows and is refused")

	got, err := s.Read()
	require.NoError(t, err)
	assert.Equal(t, []byte("a"), got)
}

func TestStreamKillIdempotentAndEOF(t *testing.T) {
	s := newStream(1, nil, 4)
	s.kill()
	s.kill() // must not panic on double close

	_, err := s.Read()
	assert.ErrorIs(t, err, io.EOF)
}

func TestStreamDeliverAfterKillRefused(t *testing.T) {
	s := newStream(1, nil, 4)
	s.kill()
	assert.False(t, s.deliver([]byte("x")), "delivery to a killed stream is refused, not a panic")
}
