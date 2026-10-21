package wsmux

import "encoding/binary"

// Frame types for the multiplexing protocol.
const (
	frameData  uint8 = 1 // stream payload
	frameClose uint8 = 2 // full stream close
)

// headerSize is the fixed wsmux frame header: 1 type byte + 4 stream-id bytes.
// No length field is carried: the underlying websocket.Conn frames each
// message and reports its exact length, so the payload is everything after
// the header.
const headerSize = 5

// pack builds a frame: [type][4-byte big-endian stream id][payload].
func pack(typ uint8, id uint32, payload []byte) []byte {
	frame := make([]byte, headerSize+len(payload))
	frame[0] = typ
	binary.BigEndian.PutUint32(frame[1:headerSize], id)
	copy(frame[headerSize:], payload)
	return frame
}

// parse splits a received message into its type, stream id, and payload.
// The returned payload aliases msg; callers that retain it must copy. ok is
// false when msg is shorter than a header.
func parse(msg []byte) (typ uint8, id uint32, payload []byte, ok bool) {
	if len(msg) < headerSize {
		return 0, 0, nil, false
	}
	return msg[0], binary.BigEndian.Uint32(msg[1:headerSize]), msg[headerSize:], true
}
