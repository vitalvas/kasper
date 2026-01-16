package websocket

import (
	"sync"
)

// PreparedMessage caches on-the-wire representations of a message payload.
// Use PreparedMessage to efficiently send a message payload to multiple connections.
type PreparedMessage struct {
	messageType int
	data        []byte
	mu          sync.Mutex
	frames      map[prepareKey]*preparedFrame
}

type prepareKey struct {
	isServer   bool
	compress   bool
	compressNo bool
}

type preparedFrame struct {
	data []byte
}

// NewPreparedMessage returns an initialized PreparedMessage.
func NewPreparedMessage(messageType int, data []byte) (*PreparedMessage, error) {
	if messageType != TextMessage && messageType != BinaryMessage {
		return nil, ErrInvalidMessageType
	}

	pm := &PreparedMessage{
		messageType: messageType,
		data:        data,
		frames:      make(map[prepareKey]*preparedFrame),
	}

	return pm, nil
}

func (pm *PreparedMessage) frame(key prepareKey) ([]byte, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pf, ok := pm.frames[key]; ok {
		return pf.data, nil
	}

	data := pm.data
	if key.compress && !key.compressNo {
		compressed, err := compressData(data, defaultCompressionLevel)
		if err != nil {
			return nil, err
		}
		data = compressed
	}

	frameData := buildFrame(pm.messageType, data, !key.isServer, key.compress && !key.compressNo)

	pm.frames[key] = &preparedFrame{data: frameData}
	return frameData, nil
}

func buildFrame(messageType int, data []byte, masked bool, compressed bool) []byte {
	header := make([]byte, maxFrameHeaderSize)
	headerLen := 2

	b0 := byte(messageType) | finalBit
	if compressed {
		b0 |= rsv1Bit
	}
	header[0] = b0

	payloadLen := len(data)
	switch {
	case payloadLen <= 125:
		header[1] = byte(payloadLen)
	case payloadLen <= 65535:
		header[1] = payloadLen16
		header[2] = byte(payloadLen >> 8)
		header[3] = byte(payloadLen)
		headerLen = 4
	default:
		header[1] = payloadLen64
		header[2] = byte(payloadLen >> 56)
		header[3] = byte(payloadLen >> 48)
		header[4] = byte(payloadLen >> 40)
		header[5] = byte(payloadLen >> 32)
		header[6] = byte(payloadLen >> 24)
		header[7] = byte(payloadLen >> 16)
		header[8] = byte(payloadLen >> 8)
		header[9] = byte(payloadLen)
		headerLen = 10
	}

	if masked {
		header[1] |= maskBit
		mask := make([]byte, 4)
		_, _ = randReader.Read(mask)
		copy(header[headerLen:], mask)
		headerLen += 4

		maskedData := make([]byte, len(data))
		copy(maskedData, data)
		maskBytes(mask, 0, maskedData)
		data = maskedData
	}

	frame := make([]byte, headerLen+len(data))
	copy(frame, header[:headerLen])
	copy(frame[headerLen:], data)
	return frame
}

// WritePreparedMessage writes pm to the connection.
func (c *Conn) WritePreparedMessage(pm *PreparedMessage) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if c.writeErr != nil {
		return c.writeErr
	}

	key := prepareKey{
		isServer:   c.isServer,
		compress:   c.compressionEnabled && c.writeCompress,
		compressNo: !c.compressionEnabled,
	}

	frameData, err := pm.frame(key)
	if err != nil {
		return err
	}

	_, err = c.conn.Write(frameData)
	return err
}
