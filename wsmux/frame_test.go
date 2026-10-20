package wsmux

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPackParseRoundTrip(t *testing.T) {
	payload := []byte("hello")
	frame := pack(frameData, 0x01020304, payload)

	typ, id, got, ok := parse(frame)
	assert.True(t, ok)
	assert.Equal(t, frameData, typ)
	assert.Equal(t, uint32(0x01020304), id)
	assert.Equal(t, payload, got)
}

func TestPackParseEmptyPayload(t *testing.T) {
	frame := pack(frameClose, 7, nil)
	typ, id, payload, ok := parse(frame)
	assert.True(t, ok)
	assert.Equal(t, frameClose, typ)
	assert.Equal(t, uint32(7), id)
	assert.Empty(t, payload)
}

func TestParseTooShort(t *testing.T) {
	typ, id, payload, ok := parse([]byte{1, 2, 3})
	assert.False(t, ok)
	assert.Zero(t, typ)
	assert.Zero(t, id)
	assert.Nil(t, payload)
}
