package websocket

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"slices"
)

var randReader io.Reader = rand.Reader

// BufferPool represents a pool of buffers for reuse.
type BufferPool interface {
	Get() any
	Put(any)
}

// FormatCloseMessage formats closeCode and text as a WebSocket close message
// per RFC 6455, section 5.5.1. The close frame body consists of a 2-byte
// status code followed by optional UTF-8 encoded reason text.
func FormatCloseMessage(closeCode int, text string) []byte {
	if closeCode == CloseNoStatusReceived {
		return []byte{}
	}
	buf := make([]byte, 2+len(text))
	binary.BigEndian.PutUint16(buf, uint16(closeCode))
	copy(buf[2:], text)
	return buf
}

// IsCloseError returns true if the error is a CloseError with one of the specified codes.
// Close codes are defined in RFC 6455, section 7.4.1.
func IsCloseError(err error, codes ...int) bool {
	var closeErr *CloseError
	if !errors.As(err, &closeErr) {
		return false
	}
	return slices.Contains(codes, closeErr.Code)
}

// IsUnexpectedCloseError returns true if the error is a CloseError with a code
// NOT in the expected codes list. Close codes are defined in RFC 6455, section 7.4.1.
func IsUnexpectedCloseError(err error, expectedCodes ...int) bool {
	var closeErr *CloseError
	if !errors.As(err, &closeErr) {
		return false
	}
	return !slices.Contains(expectedCodes, closeErr.Code)
}
