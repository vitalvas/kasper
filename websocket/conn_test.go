package websocket

import (
	"bytes"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessageTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant int
		expected int
	}{
		{"TextMessage", TextMessage, 1},
		{"BinaryMessage", BinaryMessage, 2},
		{"CloseMessage", CloseMessage, 8},
		{"PingMessage", PingMessage, 9},
		{"PongMessage", PongMessage, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.constant)
		})
	}
}

func TestCloseCodeConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant int
		expected int
	}{
		{"CloseNormalClosure", CloseNormalClosure, 1000},
		{"CloseGoingAway", CloseGoingAway, 1001},
		{"CloseProtocolError", CloseProtocolError, 1002},
		{"CloseUnsupportedData", CloseUnsupportedData, 1003},
		{"CloseNoStatusReceived", CloseNoStatusReceived, 1005},
		{"CloseAbnormalClosure", CloseAbnormalClosure, 1006},
		{"CloseInvalidFramePayloadData", CloseInvalidFramePayloadData, 1007},
		{"ClosePolicyViolation", ClosePolicyViolation, 1008},
		{"CloseMessageTooBig", CloseMessageTooBig, 1009},
		{"CloseMandatoryExtension", CloseMandatoryExtension, 1010},
		{"CloseInternalServerErr", CloseInternalServerErr, 1011},
		{"CloseServiceRestart", CloseServiceRestart, 1012},
		{"CloseTryAgainLater", CloseTryAgainLater, 1013},
		{"CloseTLSHandshake", CloseTLSHandshake, 1015},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.constant)
		})
	}
}

func TestCloseError(t *testing.T) {
	t.Run("Error message format", func(t *testing.T) {
		err := &CloseError{Code: CloseNormalClosure, Text: "goodbye"}
		assert.Contains(t, err.Error(), "websocket: close")
		assert.Contains(t, err.Error(), "1000")
		assert.Contains(t, err.Error(), "goodbye")
	})

	t.Run("Unknown close code", func(t *testing.T) {
		err := &CloseError{Code: 4000, Text: "custom"}
		assert.Contains(t, err.Error(), "4000")
	})
}

func TestCloseCodeString(t *testing.T) {
	tests := []struct {
		code     int
		expected string
	}{
		{CloseNormalClosure, "1000 (normal)"},
		{CloseGoingAway, "1001 (going away)"},
		{CloseProtocolError, "1002 (protocol error)"},
		{CloseUnsupportedData, "1003 (unsupported data)"},
		{CloseNoStatusReceived, "1005 (no status)"},
		{CloseAbnormalClosure, "1006 (abnormal closure)"},
		{CloseInvalidFramePayloadData, "1007 (invalid payload)"},
		{ClosePolicyViolation, "1008 (policy violation)"},
		{CloseMessageTooBig, "1009 (message too big)"},
		{CloseMandatoryExtension, "1010 (mandatory extension)"},
		{CloseInternalServerErr, "1011 (internal server error)"},
		{CloseServiceRestart, "1012 (service restart)"},
		{CloseTryAgainLater, "1013 (try again later)"},
		{CloseTLSHandshake, "1015 (TLS handshake)"},
		{4000, "4000"},
		{4999, "4999"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := closeCodeString(tt.code)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMaskBytes(t *testing.T) {
	t.Run("Basic masking", func(t *testing.T) {
		data := []byte("hello")
		mask := []byte{0x12, 0x34, 0x56, 0x78}
		original := make([]byte, len(data))
		copy(original, data)

		maskBytes(mask, 0, data)
		assert.NotEqual(t, original, data)

		maskBytes(mask, 0, data)
		assert.Equal(t, original, data)
	})

	t.Run("With offset", func(t *testing.T) {
		data := []byte("test")
		mask := []byte{0xAA, 0xBB, 0xCC, 0xDD}

		pos := maskBytes(mask, 0, data)
		assert.Equal(t, 0, pos)
	})
}

func TestNewConn(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	t.Run("Default buffer sizes", func(t *testing.T) {
		conn := newConn(server, true, 0, 0)
		assert.NotNil(t, conn)
		assert.True(t, conn.isServer)
		assert.Equal(t, int64(0), conn.readLimit) // 0 means unlimited
	})

	t.Run("Custom buffer sizes", func(t *testing.T) {
		conn := newConn(client, false, 1024, 2048)
		assert.NotNil(t, conn)
		assert.False(t, conn.isServer)
	})
}

func TestConnBasicMethods(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	conn := newConn(server, true, 0, 0)

	t.Run("Subprotocol", func(t *testing.T) {
		conn.subprotocol = "graphql-ws"
		assert.Equal(t, "graphql-ws", conn.Subprotocol())
	})

	t.Run("LocalAddr", func(t *testing.T) {
		assert.NotNil(t, conn.LocalAddr())
	})

	t.Run("RemoteAddr", func(t *testing.T) {
		assert.NotNil(t, conn.RemoteAddr())
	})

	t.Run("SetReadLimit", func(t *testing.T) {
		conn.SetReadLimit(1024)
		assert.Equal(t, int64(1024), conn.readLimit)
	})

	t.Run("SetCompressionLevel valid", func(t *testing.T) {
		err := conn.SetCompressionLevel(5)
		assert.NoError(t, err)
		assert.Equal(t, 5, conn.compressionLevel)
	})

	t.Run("SetCompressionLevel invalid", func(t *testing.T) {
		err := conn.SetCompressionLevel(10)
		assert.Error(t, err)
	})

	t.Run("EnableWriteCompression", func(t *testing.T) {
		conn.EnableWriteCompression(true)
		assert.True(t, conn.writeCompress)
	})
}

func TestConnHandlers(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	conn := newConn(server, true, 0, 0)

	t.Run("SetPingHandler", func(t *testing.T) {
		called := false
		conn.SetPingHandler(func(_ string) error {
			called = true
			return nil
		})
		err := conn.pingHandler("test")
		assert.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("SetPingHandler nil resets to default", func(t *testing.T) {
		conn.SetPingHandler(nil)
		assert.NotNil(t, conn.pingHandler)
	})

	t.Run("SetPongHandler", func(t *testing.T) {
		called := false
		conn.SetPongHandler(func(_ string) error {
			called = true
			return nil
		})
		err := conn.pongHandler("test")
		assert.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("SetPongHandler nil resets to default", func(t *testing.T) {
		conn.SetPongHandler(nil)
		assert.NotNil(t, conn.pongHandler)
	})

	t.Run("SetCloseHandler", func(t *testing.T) {
		called := false
		conn.SetCloseHandler(func(_ int, _ string) error {
			called = true
			return nil
		})
		err := conn.closeHandler(1000, "bye")
		assert.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("SetCloseHandler nil resets to default", func(t *testing.T) {
		conn.SetCloseHandler(nil)
		assert.NotNil(t, conn.closeHandler)
	})

	t.Run("Default close handler sends close frame", func(t *testing.T) {
		mock := newMockConn()
		c := newConn(mock, true, 0, 0)
		c.SetCloseHandler(nil)

		err := c.closeHandler(CloseNormalClosure, "goodbye")
		assert.NoError(t, err)

		data := mock.writeBuf.Bytes()
		assert.True(t, len(data) > 0)
	})

	t.Run("Default ping handler sends pong", func(t *testing.T) {
		mock := newMockConn()
		c := newConn(mock, true, 0, 0)
		c.SetPingHandler(nil)

		err := c.pingHandler("ping-data")
		assert.NoError(t, err)

		data := mock.writeBuf.Bytes()
		assert.True(t, len(data) > 0)
		assert.Equal(t, byte(PongMessage)|finalBit, data[0])
	})

	t.Run("Default pong handler does nothing", func(t *testing.T) {
		mock := newMockConn()
		c := newConn(mock, true, 0, 0)
		c.SetPongHandler(nil)

		err := c.pongHandler("pong-data")
		assert.NoError(t, err)

		assert.Empty(t, mock.writeBuf.Bytes())
	})
}

func TestConnDeadlines(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	conn := newConn(server, true, 0, 0)
	deadline := time.Now().Add(time.Second)

	t.Run("SetReadDeadline", func(t *testing.T) {
		err := conn.SetReadDeadline(deadline)
		assert.NoError(t, err)
	})

	t.Run("SetWriteDeadline", func(t *testing.T) {
		err := conn.SetWriteDeadline(deadline)
		assert.NoError(t, err)
	})
}

func TestWriteControlValidation(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	conn := newConn(server, true, 0, 0)

	t.Run("Invalid message type", func(t *testing.T) {
		err := conn.WriteControl(TextMessage, []byte("test"), time.Now().Add(time.Second))
		assert.ErrorIs(t, err, ErrInvalidControlFrame)
	})

	t.Run("Payload too big", func(t *testing.T) {
		bigPayload := make([]byte, 126)
		err := conn.WriteControl(PingMessage, bigPayload, time.Now().Add(time.Second))
		assert.ErrorIs(t, err, ErrControlFramePayloadTooBig)
	})
}

func TestWriteMessageValidation(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	conn := newConn(server, true, 0, 0)

	t.Run("Invalid message type", func(t *testing.T) {
		err := conn.WriteMessage(PingMessage, []byte("test"))
		assert.ErrorIs(t, err, ErrInvalidMessageType)
	})
}

type mockConn struct {
	readBuf  *bytes.Buffer
	writeBuf *bytes.Buffer
	closed   bool
}

func newMockConn() *mockConn {
	return &mockConn{
		readBuf:  new(bytes.Buffer),
		writeBuf: new(bytes.Buffer),
	}
}

func (m *mockConn) Read(b []byte) (n int, err error) {
	return m.readBuf.Read(b)
}

func (m *mockConn) Write(b []byte) (n int, err error) {
	return m.writeBuf.Write(b)
}

func (m *mockConn) Close() error {
	m.closed = true
	return nil
}

func (m *mockConn) LocalAddr() net.Addr                { return &net.TCPAddr{} }
func (m *mockConn) RemoteAddr() net.Addr               { return &net.TCPAddr{} }
func (m *mockConn) SetDeadline(_ time.Time) error      { return nil }
func (m *mockConn) SetReadDeadline(_ time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(_ time.Time) error { return nil }

func TestWriteControlFrame(t *testing.T) {
	t.Run("Server writes ping", func(t *testing.T) {
		mock := newMockConn()
		conn := newConn(mock, true, 0, 0)

		err := conn.WriteControl(PingMessage, []byte("ping"), time.Now().Add(time.Second))
		require.NoError(t, err)

		data := mock.writeBuf.Bytes()
		assert.True(t, len(data) >= 2)
		assert.Equal(t, byte(PingMessage)|finalBit, data[0])
	})

	t.Run("Client writes ping with mask", func(t *testing.T) {
		mock := newMockConn()
		conn := newConn(mock, false, 0, 0)

		origRandReader := randReader
		randReader = bytes.NewReader([]byte{0x01, 0x02, 0x03, 0x04})
		defer func() { randReader = origRandReader }()

		err := conn.WriteControl(PingMessage, []byte("ping"), time.Now().Add(time.Second))
		require.NoError(t, err)

		data := mock.writeBuf.Bytes()
		assert.True(t, data[1]&maskBit != 0)
	})
}

func TestWriteDataFrame(t *testing.T) {
	t.Run("Server writes text message", func(t *testing.T) {
		mock := newMockConn()
		conn := newConn(mock, true, 0, 0)

		err := conn.WriteMessage(TextMessage, []byte("hello"))
		require.NoError(t, err)

		data := mock.writeBuf.Bytes()
		assert.True(t, len(data) >= 2)
	})

	t.Run("Server writes binary message", func(t *testing.T) {
		mock := newMockConn()
		conn := newConn(mock, true, 0, 0)

		err := conn.WriteMessage(BinaryMessage, []byte{0x01, 0x02, 0x03})
		require.NoError(t, err)

		data := mock.writeBuf.Bytes()
		assert.True(t, len(data) >= 2)
	})
}

func TestReadFrame(t *testing.T) {
	t.Run("Read text frame", func(t *testing.T) {
		mock := newMockConn()
		frame := []byte{byte(TextMessage) | finalBit, 5, 'h', 'e', 'l', 'l', 'o'}
		mock.readBuf.Write(frame)

		conn := newConn(mock, true, 0, 0)

		msgType, payload, final, compressed, err := conn.readFrame()
		require.NoError(t, err)
		assert.Equal(t, TextMessage, msgType)
		assert.Equal(t, []byte("hello"), payload)
		assert.True(t, final)
		assert.False(t, compressed)
	})

	t.Run("Read masked frame", func(t *testing.T) {
		mock := newMockConn()
		mask := []byte{0x12, 0x34, 0x56, 0x78}
		payload := []byte("hello")
		maskedPayload := make([]byte, len(payload))
		copy(maskedPayload, payload)
		maskBytes(mask, 0, maskedPayload)

		frame := make([]byte, 0, 2+len(mask)+len(maskedPayload))
		frame = append(frame, byte(TextMessage)|finalBit, byte(len(payload))|maskBit)
		frame = append(frame, mask...)
		frame = append(frame, maskedPayload...)
		mock.readBuf.Write(frame)

		conn := newConn(mock, true, 0, 0)

		msgType, data, final, _, err := conn.readFrame()
		require.NoError(t, err)
		assert.Equal(t, TextMessage, msgType)
		assert.Equal(t, payload, data)
		assert.True(t, final)
	})

	t.Run("Read 16-bit length frame", func(t *testing.T) {
		mock := newMockConn()
		payload := make([]byte, 200)
		for i := range payload {
			payload[i] = byte(i % 256)
		}

		frame := make([]byte, 0, 4+len(payload))
		frame = append(frame, byte(BinaryMessage)|finalBit, payloadLen16, 0, 200)
		frame = append(frame, payload...)
		mock.readBuf.Write(frame)

		conn := newConn(mock, true, 0, 0)

		msgType, data, final, _, err := conn.readFrame()
		require.NoError(t, err)
		assert.Equal(t, BinaryMessage, msgType)
		assert.Equal(t, payload, data)
		assert.True(t, final)
	})

	t.Run("Reserved bits set", func(t *testing.T) {
		mock := newMockConn()
		frame := []byte{byte(TextMessage) | finalBit | rsv2Bit, 5, 'h', 'e', 'l', 'l', 'o'}
		mock.readBuf.Write(frame)

		conn := newConn(mock, true, 0, 0)

		_, _, _, _, err := conn.readFrame()
		assert.ErrorIs(t, err, ErrReservedBits)
	})

	t.Run("Control frame too big", func(t *testing.T) {
		mock := newMockConn()
		frame := []byte{byte(PingMessage) | finalBit, payloadLen16, 0, 200}
		mock.readBuf.Write(frame)

		conn := newConn(mock, true, 0, 0)

		_, _, _, _, err := conn.readFrame()
		assert.ErrorIs(t, err, ErrControlFramePayloadTooBig)
	})

	t.Run("Fragmented control frame", func(t *testing.T) {
		mock := newMockConn()
		frame := []byte{byte(PingMessage), 5, 'h', 'e', 'l', 'l', 'o'}
		mock.readBuf.Write(frame)

		conn := newConn(mock, true, 0, 0)

		_, _, _, _, err := conn.readFrame()
		assert.ErrorIs(t, err, ErrFragmentedControlFrame)
	})

	t.Run("Read limit exceeded", func(t *testing.T) {
		mock := newMockConn()
		frame := make([]byte, 0, 102)
		frame = append(frame, byte(TextMessage)|finalBit, 100)
		frame = append(frame, make([]byte, 100)...)
		mock.readBuf.Write(frame)

		conn := newConn(mock, true, 0, 0)
		conn.SetReadLimit(50)

		_, _, _, _, err := conn.readFrame()
		assert.ErrorIs(t, err, ErrReadLimit)
	})
}

func TestNextReader(t *testing.T) {
	t.Run("Read text message", func(t *testing.T) {
		mock := newMockConn()
		frame := []byte{byte(TextMessage) | finalBit, 5, 'h', 'e', 'l', 'l', 'o'}
		mock.readBuf.Write(frame)

		conn := newConn(mock, true, 0, 0)

		msgType, reader, err := conn.NextReader()
		require.NoError(t, err)
		assert.Equal(t, TextMessage, msgType)

		data, err := io.ReadAll(reader)
		require.NoError(t, err)
		assert.Equal(t, []byte("hello"), data)
	})

}

func TestReadMessage(t *testing.T) {
	mock := newMockConn()
	frame := []byte{byte(TextMessage) | finalBit, 5, 'h', 'e', 'l', 'l', 'o'}
	mock.readBuf.Write(frame)

	conn := newConn(mock, true, 0, 0)

	msgType, data, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, TextMessage, msgType)
	assert.Equal(t, []byte("hello"), data)
}

func TestConnClose(t *testing.T) {
	mock := newMockConn()
	conn := newConn(mock, true, 0, 0)

	err := conn.Close()
	require.NoError(t, err)
	assert.True(t, mock.closed)
}

func TestMessageWriter(t *testing.T) {
	t.Run("Write to closed writer", func(t *testing.T) {
		mock := newMockConn()
		conn := newConn(mock, true, 0, 0)

		w, err := conn.NextWriter(TextMessage)
		require.NoError(t, err)

		err = w.Close()
		require.NoError(t, err)

		_, err = w.Write([]byte("test"))
		assert.ErrorIs(t, err, ErrWriteToClosedConnection)
	})

	t.Run("Double close is safe", func(t *testing.T) {
		mock := newMockConn()
		conn := newConn(mock, true, 0, 0)

		w, err := conn.NextWriter(TextMessage)
		require.NoError(t, err)

		err = w.Close()
		require.NoError(t, err)

		err = w.Close()
		require.NoError(t, err)
	})
}

func TestNextReaderEdgeCases(t *testing.T) {
	t.Run("Handle ping during read", func(t *testing.T) {
		mock := newMockConn()
		pingFrame := []byte{byte(PingMessage) | finalBit, 4, 'p', 'i', 'n', 'g'}
		textFrame := []byte{byte(TextMessage) | finalBit, 5, 'h', 'e', 'l', 'l', 'o'}
		mock.readBuf.Write(pingFrame)
		mock.readBuf.Write(textFrame)

		conn := newConn(mock, true, 0, 0)
		pingHandled := false
		conn.SetPingHandler(func(_ string) error {
			pingHandled = true
			return nil
		})

		msgType, reader, err := conn.NextReader()
		require.NoError(t, err)
		assert.True(t, pingHandled)
		assert.Equal(t, TextMessage, msgType)

		data, err := io.ReadAll(reader)
		require.NoError(t, err)
		assert.Equal(t, []byte("hello"), data)
	})

	t.Run("Ping handler error", func(t *testing.T) {
		mock := newMockConn()
		pingFrame := []byte{byte(PingMessage) | finalBit, 4, 'p', 'i', 'n', 'g'}
		mock.readBuf.Write(pingFrame)

		conn := newConn(mock, true, 0, 0)
		testErr := errors.New("ping handler error")
		conn.SetPingHandler(func(_ string) error {
			return testErr
		})

		_, _, err := conn.NextReader()
		assert.ErrorIs(t, err, testErr)
	})

	t.Run("Pong handler error", func(t *testing.T) {
		mock := newMockConn()
		pongFrame := []byte{byte(PongMessage) | finalBit, 4, 'p', 'o', 'n', 'g'}
		mock.readBuf.Write(pongFrame)

		conn := newConn(mock, true, 0, 0)
		testErr := errors.New("pong handler error")
		conn.SetPongHandler(func(_ string) error {
			return testErr
		})

		_, _, err := conn.NextReader()
		assert.ErrorIs(t, err, testErr)
	})

	t.Run("Close handler error", func(t *testing.T) {
		mock := newMockConn()
		closeFrame := []byte{byte(CloseMessage) | finalBit, 2, 0x03, 0xe8}
		mock.readBuf.Write(closeFrame)

		conn := newConn(mock, true, 0, 0)
		testErr := errors.New("close handler error")
		conn.SetCloseHandler(func(_ int, _ string) error {
			return testErr
		})

		_, _, err := conn.NextReader()
		assert.ErrorIs(t, err, testErr)
	})

	t.Run("Handle close frame", func(t *testing.T) {
		mock := newMockConn()
		closeFrame := []byte{byte(CloseMessage) | finalBit, 2, 0x03, 0xe8}
		mock.readBuf.Write(closeFrame)

		conn := newConn(mock, true, 0, 0)
		closeHandled := false
		conn.SetCloseHandler(func(code int, _ string) error {
			closeHandled = true
			assert.Equal(t, CloseNormalClosure, code)
			return nil
		})

		_, _, err := conn.NextReader()
		assert.Error(t, err)
		assert.True(t, closeHandled)
		var closeErr *CloseError
		assert.ErrorAs(t, err, &closeErr)
		assert.Equal(t, CloseNormalClosure, closeErr.Code)
	})

	t.Run("Handle close frame with text", func(t *testing.T) {
		mock := newMockConn()
		closeFrame := []byte{byte(CloseMessage) | finalBit, 5, 0x03, 0xe8, 'b', 'y', 'e'}
		mock.readBuf.Write(closeFrame)

		conn := newConn(mock, true, 0, 0)

		_, _, err := conn.NextReader()
		assert.Error(t, err)
		var closeErr *CloseError
		assert.ErrorAs(t, err, &closeErr)
		assert.Equal(t, "bye", closeErr.Text)
	})

	t.Run("Unexpected continuation frame", func(t *testing.T) {
		mock := newMockConn()
		frame := []byte{byte(continuationFrame) | finalBit, 5, 'h', 'e', 'l', 'l', 'o'}
		mock.readBuf.Write(frame)

		conn := newConn(mock, true, 0, 0)

		_, _, err := conn.NextReader()
		assert.ErrorIs(t, err, ErrUnexpectedContinuation)
	})

	t.Run("Invalid opcode", func(t *testing.T) {
		mock := newMockConn()
		frame := []byte{byte(3) | finalBit, 5, 'h', 'e', 'l', 'l', 'o'}
		mock.readBuf.Write(frame)

		conn := newConn(mock, true, 0, 0)

		_, _, err := conn.NextReader()
		assert.ErrorIs(t, err, ErrInvalidOpcode)
	})

	t.Run("NextReader with existing read error", func(t *testing.T) {
		mock := newMockConn()
		conn := newConn(mock, true, 0, 0)
		conn.readErr = io.EOF

		_, _, err := conn.NextReader()
		assert.ErrorIs(t, err, io.EOF)
	})

	t.Run("NextWriter with existing write error", func(t *testing.T) {
		mock := newMockConn()
		conn := newConn(mock, true, 0, 0)
		conn.writeErr = io.EOF

		_, err := conn.NextWriter(TextMessage)
		assert.ErrorIs(t, err, io.EOF)
	})
}

func TestMessageReaderRead(t *testing.T) {
	t.Run("Read in chunks", func(t *testing.T) {
		mock := newMockConn()
		frame := []byte{byte(TextMessage) | finalBit, 10, 'h', 'e', 'l', 'l', 'o', 'w', 'o', 'r', 'l', 'd'}
		mock.readBuf.Write(frame)

		conn := newConn(mock, true, 0, 0)

		_, reader, err := conn.NextReader()
		require.NoError(t, err)

		buf := make([]byte, 5)
		n, err := reader.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, 5, n)
		assert.Equal(t, []byte("hello"), buf)

		n, err = reader.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, 5, n)
		assert.Equal(t, []byte("world"), buf)

		_, err = reader.Read(buf)
		assert.ErrorIs(t, err, io.EOF)
	})

	t.Run("Read fragmented message with continuation", func(t *testing.T) {
		mock := newMockConn()
		firstFrame := []byte{byte(TextMessage), 5, 'h', 'e', 'l', 'l', 'o'}
		contFrame := []byte{byte(continuationFrame) | finalBit, 5, 'w', 'o', 'r', 'l', 'd'}
		mock.readBuf.Write(firstFrame)
		mock.readBuf.Write(contFrame)

		conn := newConn(mock, true, 0, 0)

		_, reader, err := conn.NextReader()
		require.NoError(t, err)

		data, err := io.ReadAll(reader)
		require.NoError(t, err)
		assert.Equal(t, []byte("helloworld"), data)
	})

	t.Run("Read error on expected continuation", func(t *testing.T) {
		mock := newMockConn()
		firstFrame := []byte{byte(TextMessage), 5, 'h', 'e', 'l', 'l', 'o'}
		badFrame := []byte{byte(TextMessage) | finalBit, 5, 'w', 'o', 'r', 'l', 'd'}
		mock.readBuf.Write(firstFrame)
		mock.readBuf.Write(badFrame)

		conn := newConn(mock, true, 0, 0)

		_, reader, err := conn.NextReader()
		require.NoError(t, err)

		buf := make([]byte, 10)
		_, err = reader.Read(buf)
		require.NoError(t, err)

		_, err = reader.Read(buf)
		assert.ErrorIs(t, err, ErrExpectedContinuation)
	})

	t.Run("Read error on continuation frame", func(t *testing.T) {
		mock := newMockConn()
		firstFrame := []byte{byte(TextMessage), 5, 'h', 'e', 'l', 'l', 'o'}
		mock.readBuf.Write(firstFrame)

		conn := newConn(mock, true, 0, 0)

		_, reader, err := conn.NextReader()
		require.NoError(t, err)

		buf := make([]byte, 10)
		_, err = reader.Read(buf)
		require.NoError(t, err)

		_, err = reader.Read(buf)
		assert.Error(t, err)
	})
}

func TestLargeFrames(t *testing.T) {
	t.Run("Read 64-bit length frame", func(t *testing.T) {
		mock := newMockConn()
		payload := make([]byte, 70000)
		for i := range payload {
			payload[i] = byte(i % 256)
		}

		frame := make([]byte, 0, 10+len(payload))
		frame = append(frame, byte(BinaryMessage)|finalBit, payloadLen64)
		lenBytes := make([]byte, 8)
		lenBytes[4] = byte(len(payload) >> 24)
		lenBytes[5] = byte(len(payload) >> 16)
		lenBytes[6] = byte(len(payload) >> 8)
		lenBytes[7] = byte(len(payload))
		frame = append(frame, lenBytes...)
		frame = append(frame, payload...)
		mock.readBuf.Write(frame)

		conn := newConn(mock, true, 0, 0)
		conn.SetReadLimit(100000)

		msgType, data, err := conn.ReadMessage()
		require.NoError(t, err)
		assert.Equal(t, BinaryMessage, msgType)
		assert.Equal(t, len(payload), len(data))
	})

	t.Run("Write large message client", func(t *testing.T) {
		mock := newMockConn()
		conn := newConn(mock, false, 0, 0)

		origRandReader := randReader
		randReader = bytes.NewReader(make([]byte, 4))
		defer func() { randReader = origRandReader }()

		largePayload := make([]byte, 70000)
		err := conn.WriteMessage(BinaryMessage, largePayload)
		require.NoError(t, err)

		data := mock.writeBuf.Bytes()
		assert.True(t, len(data) > 70000)
		assert.Equal(t, byte(payloadLen64), data[1]&payloadLenMask)
	})

	t.Run("Write medium message (16-bit length)", func(t *testing.T) {
		mock := newMockConn()
		conn := newConn(mock, true, 0, 0)

		mediumPayload := make([]byte, 200)
		err := conn.WriteMessage(BinaryMessage, mediumPayload)
		require.NoError(t, err)

		data := mock.writeBuf.Bytes()
		assert.Equal(t, byte(payloadLen16), data[1]&payloadLenMask)
	})
}

func TestRsv3Reserved(t *testing.T) {
	mock := newMockConn()
	frame := []byte{byte(TextMessage) | finalBit | rsv3Bit, 5, 'h', 'e', 'l', 'l', 'o'}
	mock.readBuf.Write(frame)

	conn := newConn(mock, true, 0, 0)

	_, _, err := conn.ReadMessage()
	assert.ErrorIs(t, err, ErrReservedBits)
}

func TestWriteControlToClosedConn(t *testing.T) {
	mock := newMockConn()
	conn := newConn(mock, true, 0, 0)
	conn.writeErr = io.EOF

	err := conn.WriteControl(PingMessage, []byte("ping"), time.Now().Add(time.Second))
	assert.Error(t, err)
}

func TestWriteMessageToClosedConn(t *testing.T) {
	mock := newMockConn()
	conn := newConn(mock, true, 0, 0)
	conn.writeErr = io.EOF

	err := conn.WriteMessage(TextMessage, []byte("test"))
	assert.Error(t, err)
}

func BenchmarkWriteMessage(b *testing.B) {
	sizes := []struct {
		name string
		size int
	}{
		{"Small_64B", 64},
		{"Medium_1KB", 1024},
		{"Large_64KB", 64 * 1024},
		{"XLarge_1MB", 1024 * 1024},
	}

	for _, size := range sizes {
		data := make([]byte, size.size)
		for i := range data {
			data[i] = byte(i % 256)
		}

		b.Run("Text_"+size.name, func(b *testing.B) {
			mock := &benchMockConn{buf: make([]byte, 0, size.size*2)}
			conn := newConn(mock, true, 0, 0)

			b.ResetTimer()
			b.SetBytes(int64(size.size))

			for b.Loop() {
				mock.Reset()
				_ = conn.WriteMessage(TextMessage, data)
			}
		})

		b.Run("Binary_"+size.name, func(b *testing.B) {
			mock := &benchMockConn{buf: make([]byte, 0, size.size*2)}
			conn := newConn(mock, true, 0, 0)

			b.ResetTimer()
			b.SetBytes(int64(size.size))

			for b.Loop() {
				mock.Reset()
				_ = conn.WriteMessage(BinaryMessage, data)
			}
		})
	}
}

func BenchmarkWriteMessageClient(b *testing.B) {
	data := make([]byte, 1024)
	mock := &benchMockConn{buf: make([]byte, 0, 2048)}
	conn := newConn(mock, false, 0, 0)

	b.ResetTimer()
	b.SetBytes(1024)

	for b.Loop() {
		mock.Reset()
		_ = conn.WriteMessage(TextMessage, data)
	}
}

func BenchmarkReadMessage(b *testing.B) {
	sizes := []struct {
		name string
		size int
	}{
		{"Small_64B", 64},
		{"Medium_1KB", 1024},
		{"Large_64KB", 64 * 1024},
	}

	for _, size := range sizes {
		payload := make([]byte, size.size)
		for i := range payload {
			payload[i] = byte(i % 256)
		}

		frame := buildFrame(BinaryMessage, payload, false, false)

		b.Run(size.name, func(b *testing.B) {
			b.SetBytes(int64(size.size))

			for b.Loop() {
				mock := &benchMockConn{
					readBuf: bytes.NewBuffer(frame),
					buf:     make([]byte, 0, 1024),
				}
				conn := newConn(mock, true, 0, 0)
				conn.SetReadLimit(int64(size.size + 1024))

				_, _, _ = conn.ReadMessage()
			}
		})
	}
}

func BenchmarkWriteControl(b *testing.B) {
	mock := &benchMockConn{buf: make([]byte, 0, 256)}
	conn := newConn(mock, true, 0, 0)
	pingData := []byte("ping")

	b.ResetTimer()

	for b.Loop() {
		mock.Reset()
		_ = conn.WriteControl(PingMessage, pingData, time.Time{})
	}
}

func BenchmarkBuildFrame(b *testing.B) {
	data := make([]byte, 1024)

	b.Run("Server", func(b *testing.B) {
		for b.Loop() {
			_ = buildFrame(TextMessage, data, false, false)
		}
	})

	b.Run("Client", func(b *testing.B) {
		for b.Loop() {
			_ = buildFrame(TextMessage, data, true, false)
		}
	})

	b.Run("Compressed", func(b *testing.B) {
		for b.Loop() {
			_ = buildFrame(TextMessage, data, false, true)
		}
	})
}

func BenchmarkMaskBytes(b *testing.B) {
	sizes := []int{64, 1024, 64 * 1024}
	mask := []byte{0x12, 0x34, 0x56, 0x78}

	for _, size := range sizes {
		data := make([]byte, size)

		b.Run(byteCountSI(size), func(b *testing.B) {
			b.SetBytes(int64(size))

			for b.Loop() {
				maskBytes(mask, 0, data)
			}
		})
	}
}

func BenchmarkFormatCloseMessage(b *testing.B) {
	for b.Loop() {
		_ = FormatCloseMessage(CloseNormalClosure, "goodbye")
	}
}

type benchMockConn struct {
	buf     []byte
	readBuf *bytes.Buffer
}

func (m *benchMockConn) Read(b []byte) (int, error) {
	if m.readBuf != nil {
		return m.readBuf.Read(b)
	}
	return 0, nil
}

func (m *benchMockConn) Write(b []byte) (int, error) {
	m.buf = append(m.buf, b...)
	return len(b), nil
}

func (m *benchMockConn) Close() error                       { return nil }
func (m *benchMockConn) LocalAddr() net.Addr                { return &net.TCPAddr{} }
func (m *benchMockConn) RemoteAddr() net.Addr               { return &net.TCPAddr{} }
func (m *benchMockConn) SetDeadline(_ time.Time) error      { return nil }
func (m *benchMockConn) SetReadDeadline(_ time.Time) error  { return nil }
func (m *benchMockConn) SetWriteDeadline(_ time.Time) error { return nil }
func (m *benchMockConn) Reset()                             { m.buf = m.buf[:0] }

func byteCountSI(b int) string {
	const unit = 1024
	if b < unit {
		return string(rune('0'+b/100)) + string(rune('0'+(b/10)%10)) + string(rune('0'+b%10)) + "B"
	}
	div, exp := unit, 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return string(rune('0'+b/div)) + string([]rune{'K', 'M', 'G', 'T', 'P'}[exp]) + "B"
}

func FuzzReadFrame(f *testing.F) {
	f.Add([]byte{byte(TextMessage) | finalBit, 5, 'h', 'e', 'l', 'l', 'o'})
	f.Add([]byte{byte(BinaryMessage) | finalBit, 0})
	f.Add([]byte{byte(PingMessage) | finalBit, 4, 'p', 'i', 'n', 'g'})
	f.Add([]byte{byte(PongMessage) | finalBit, 4, 'p', 'o', 'n', 'g'})
	f.Add([]byte{byte(CloseMessage) | finalBit, 2, 0x03, 0xe8})
	f.Add([]byte{byte(TextMessage) | finalBit, payloadLen16, 0, 200})
	f.Add([]byte{byte(TextMessage) | finalBit | maskBit, 5, 0x12, 0x34, 0x56, 0x78, 'h', 'e', 'l', 'l', 'o'})

	f.Fuzz(func(_ *testing.T, data []byte) {
		if len(data) < 2 {
			return
		}

		mock := &fuzzMockConn{readBuf: bytes.NewBuffer(data)}
		conn := newConn(mock, true, 0, 0)
		conn.SetReadLimit(1024 * 1024)

		_, _, _ = conn.ReadMessage()
	})
}

func FuzzFormatCloseMessage(f *testing.F) {
	f.Add(1000, "normal closure")
	f.Add(1001, "going away")
	f.Add(1002, "protocol error")
	f.Add(1003, "")
	f.Add(4000, "custom code")
	f.Add(0, "zero code")

	f.Fuzz(func(t *testing.T, code int, text string) {
		if code < 0 || code > 65535 {
			return
		}
		if len(text) > 123 {
			text = text[:123]
		}

		result := FormatCloseMessage(code, text)

		if code == CloseNoStatusReceived {
			if len(result) != 0 {
				t.Errorf("expected empty result for CloseNoStatusReceived")
			}
			return
		}

		if len(result) < 2 {
			t.Errorf("result too short: %d", len(result))
			return
		}

		gotCode := int(result[0])<<8 | int(result[1])
		if gotCode != code {
			t.Errorf("code mismatch: got %d, want %d", gotCode, code)
		}

		if len(result) > 2 {
			gotText := string(result[2:])
			if gotText != text {
				t.Errorf("text mismatch: got %q, want %q", gotText, text)
			}
		}
	})
}

func FuzzIsCloseError(f *testing.F) {
	f.Add(1000, "bye", 1000, 1001)
	f.Add(1001, "", 1000, 1001)
	f.Add(1002, "error", 1000, 1001)
	f.Add(4000, "custom", 4000, 4001)

	f.Fuzz(func(t *testing.T, code int, text string, check1, check2 int) {
		if code < 0 || code > 65535 {
			return
		}

		err := &CloseError{Code: code, Text: text}

		result := IsCloseError(err, check1, check2)
		expected := code == check1 || code == check2

		if result != expected {
			t.Errorf("IsCloseError(%d, %d, %d) = %v, want %v", code, check1, check2, result, expected)
		}
	})
}

func FuzzIsUnexpectedCloseError(f *testing.F) {
	f.Add(1000, "bye", 1000, 1001)
	f.Add(1002, "error", 1000, 1001)
	f.Add(4000, "custom", 1000, 1001)

	f.Fuzz(func(t *testing.T, code int, text string, expected1, expected2 int) {
		if code < 0 || code > 65535 {
			return
		}

		err := &CloseError{Code: code, Text: text}

		result := IsUnexpectedCloseError(err, expected1, expected2)
		isExpected := code == expected1 || code == expected2

		if result == isExpected {
			t.Errorf("IsUnexpectedCloseError(%d, %d, %d) = %v, want %v", code, expected1, expected2, result, !isExpected)
		}
	})
}

func FuzzMaskBytes(f *testing.F) {
	f.Add([]byte{0x12, 0x34, 0x56, 0x78}, []byte("hello"))
	f.Add([]byte{0x00, 0x00, 0x00, 0x00}, []byte("test"))
	f.Add([]byte{0xff, 0xff, 0xff, 0xff}, []byte("data"))
	f.Add([]byte{0xaa, 0xbb, 0xcc, 0xdd}, []byte{})

	f.Fuzz(func(t *testing.T, mask, data []byte) {
		if len(mask) != 4 {
			return
		}

		original := make([]byte, len(data))
		copy(original, data)

		maskBytes(mask, 0, data)
		maskBytes(mask, 0, data)

		if !bytes.Equal(original, data) {
			t.Errorf("double mask did not restore original data")
		}
	})
}

func FuzzBuildFrame(f *testing.F) {
	f.Add(TextMessage, []byte("hello"), false, false)
	f.Add(BinaryMessage, []byte{0x01, 0x02, 0x03}, true, false)
	f.Add(TextMessage, []byte("compressed"), false, true)

	f.Fuzz(func(t *testing.T, msgType int, data []byte, masked, compressed bool) {
		if msgType != TextMessage && msgType != BinaryMessage {
			return
		}
		if len(data) > 10000 {
			data = data[:10000]
		}

		frame := buildFrame(msgType, data, masked, compressed)

		if len(frame) < 2 {
			t.Errorf("frame too short: %d", len(frame))
			return
		}

		opcode := int(frame[0] & opcodeMask)
		if opcode != msgType {
			t.Errorf("opcode mismatch: got %d, want %d", opcode, msgType)
		}

		if frame[0]&finalBit == 0 {
			t.Errorf("final bit not set")
		}

		if compressed && frame[0]&rsv1Bit == 0 {
			t.Errorf("rsv1 bit not set for compressed frame")
		}

		if masked && frame[1]&maskBit == 0 {
			t.Errorf("mask bit not set for masked frame")
		}
	})
}

type fuzzMockConn struct {
	readBuf *bytes.Buffer
}

func (m *fuzzMockConn) Read(b []byte) (int, error) {
	if m.readBuf != nil {
		return m.readBuf.Read(b)
	}
	return 0, nil
}

func (m *fuzzMockConn) Write(b []byte) (int, error)        { return len(b), nil }
func (m *fuzzMockConn) Close() error                       { return nil }
func (m *fuzzMockConn) LocalAddr() net.Addr                { return &net.TCPAddr{} }
func (m *fuzzMockConn) RemoteAddr() net.Addr               { return &net.TCPAddr{} }
func (m *fuzzMockConn) SetDeadline(_ time.Time) error      { return nil }
func (m *fuzzMockConn) SetReadDeadline(_ time.Time) error  { return nil }
func (m *fuzzMockConn) SetWriteDeadline(_ time.Time) error { return nil }

func TestConnWithNilNetConn(t *testing.T) {
	// Create a connection with nil netConn (simulates HTTP/2 or http.Client path).
	rwc := &mockRWC{}
	conn := newConnFromRWC(rwc, nil, false, 1024, 1024, nil)

	t.Run("LocalAddr returns nil", func(t *testing.T) {
		assert.Nil(t, conn.LocalAddr())
	})

	t.Run("RemoteAddr returns nil", func(t *testing.T) {
		assert.Nil(t, conn.RemoteAddr())
	})

	t.Run("SetReadDeadline returns nil", func(t *testing.T) {
		err := conn.SetReadDeadline(time.Now().Add(time.Second))
		assert.NoError(t, err)
	})

	t.Run("SetWriteDeadline returns nil", func(t *testing.T) {
		err := conn.SetWriteDeadline(time.Now().Add(time.Second))
		assert.NoError(t, err)
	})

	t.Run("UnderlyingConn returns nil", func(t *testing.T) {
		assert.Nil(t, conn.UnderlyingConn())
	})
}

type mockRWC struct {
	readBuf  bytes.Buffer
	writeBuf bytes.Buffer
	closed   bool
}

func (m *mockRWC) Read(p []byte) (int, error) {
	if m.closed {
		return 0, io.EOF
	}
	return m.readBuf.Read(p)
}

func (m *mockRWC) Write(p []byte) (int, error) {
	if m.closed {
		return 0, errors.New("closed")
	}
	return m.writeBuf.Write(p)
}

func (m *mockRWC) Close() error {
	m.closed = true
	return nil
}

func TestNextReaderControlFrames(t *testing.T) {
	t.Run("Pong message handled", func(t *testing.T) {
		// Build a pong frame followed by a text message.
		var buf bytes.Buffer

		// Pong frame (opcode 10, FIN, no mask, 4 bytes payload).
		buf.Write([]byte{0x8a, 0x04, 'p', 'o', 'n', 'g'})

		// Text message frame.
		buf.Write([]byte{0x81, 0x05, 'h', 'e', 'l', 'l', 'o'})

		mockConn := &mockConn{readBuf: &buf, writeBuf: &bytes.Buffer{}}
		conn := newConn(mockConn, true, 1024, 1024)

		pongReceived := false
		conn.SetPongHandler(func(appData string) error {
			pongReceived = true
			assert.Equal(t, "pong", appData)
			return nil
		})

		msgType, reader, err := conn.NextReader()
		require.NoError(t, err)
		assert.True(t, pongReceived)
		assert.Equal(t, TextMessage, msgType)

		data, _ := io.ReadAll(reader)
		assert.Equal(t, []byte("hello"), data)
	})

	t.Run("Ping message triggers pong", func(t *testing.T) {
		var buf bytes.Buffer

		// Ping frame (opcode 9, FIN, no mask, 4 bytes payload).
		buf.Write([]byte{0x89, 0x04, 'p', 'i', 'n', 'g'})

		// Text message frame.
		buf.Write([]byte{0x81, 0x05, 'h', 'e', 'l', 'l', 'o'})

		mockConn := &mockConn{readBuf: &buf, writeBuf: &bytes.Buffer{}}
		conn := newConn(mockConn, true, 1024, 1024)

		msgType, reader, err := conn.NextReader()
		require.NoError(t, err)
		assert.Equal(t, TextMessage, msgType)

		data, _ := io.ReadAll(reader)
		assert.Equal(t, []byte("hello"), data)
	})
}

func TestNextReaderInvalidFrames(t *testing.T) {
	t.Run("Invalid opcode", func(t *testing.T) {
		var buf bytes.Buffer

		// Invalid opcode 3 (reserved).
		buf.Write([]byte{0x83, 0x05, 'h', 'e', 'l', 'l', 'o'})

		mockConn := &mockConn{readBuf: &buf, writeBuf: &bytes.Buffer{}}
		conn := newConn(mockConn, true, 1024, 1024)

		_, _, err := conn.NextReader()
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidOpcode)
	})

	t.Run("Unexpected continuation frame", func(t *testing.T) {
		var buf bytes.Buffer

		// Continuation frame without a data frame first.
		buf.Write([]byte{0x80, 0x05, 'h', 'e', 'l', 'l', 'o'})

		mockConn := &mockConn{readBuf: &buf, writeBuf: &bytes.Buffer{}}
		conn := newConn(mockConn, true, 1024, 1024)

		_, _, err := conn.NextReader()
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrUnexpectedContinuation)
	})
}

func TestMessageWriterContinuation(t *testing.T) {
	t.Run("Multiple writes create continuation frames", func(t *testing.T) {
		mock := newMockConn()
		conn := newConn(mock, true, 0, 0)

		w, err := conn.NextWriter(TextMessage)
		require.NoError(t, err)

		// First write.
		_, err = w.Write([]byte("hello"))
		require.NoError(t, err)

		// Second write should use continuation frame.
		_, err = w.Write([]byte("world"))
		require.NoError(t, err)

		err = w.Close()
		require.NoError(t, err)

		data := mock.writeBuf.Bytes()
		assert.True(t, len(data) > 0)
	})
}

func TestCompressedMessages(t *testing.T) {
	t.Run("Compressed single frame message", func(t *testing.T) {
		// Create a compressed payload using deflate.
		original := []byte("hello world, this is a test message for compression testing")
		compressed, err := compressData(original, -1)
		require.NoError(t, err)

		var buf bytes.Buffer
		// Text frame with RSV1 bit set (compressed), FIN set.
		buf.WriteByte(byte(TextMessage) | finalBit | rsv1Bit)
		buf.WriteByte(byte(len(compressed)))
		buf.Write(compressed)

		mockConn := &mockConn{readBuf: &buf, writeBuf: &bytes.Buffer{}}
		conn := newConn(mockConn, true, 1024, 1024)
		conn.compressionEnabled = true

		msgType, reader, err := conn.NextReader()
		require.NoError(t, err)
		assert.Equal(t, TextMessage, msgType)

		data, err := io.ReadAll(reader)
		require.NoError(t, err)
		assert.Equal(t, original, data)
	})

	t.Run("Compressed fragmented message", func(t *testing.T) {
		// Create a compressed payload.
		original := []byte("hello world, this is a fragmented test message")
		compressed, err := compressData(original, -1)
		require.NoError(t, err)

		// Split compressed data into two parts.
		half := len(compressed) / 2
		part1 := compressed[:half]
		part2 := compressed[half:]

		var buf bytes.Buffer
		// First frame: Text with RSV1 (compressed), no FIN.
		buf.WriteByte(byte(TextMessage) | rsv1Bit)
		buf.WriteByte(byte(len(part1)))
		buf.Write(part1)

		// Continuation frame with FIN.
		buf.WriteByte(byte(continuationFrame) | finalBit)
		buf.WriteByte(byte(len(part2)))
		buf.Write(part2)

		mockConn := &mockConn{readBuf: &buf, writeBuf: &bytes.Buffer{}}
		conn := newConn(mockConn, true, 1024, 1024)
		conn.compressionEnabled = true

		msgType, reader, err := conn.NextReader()
		require.NoError(t, err)
		assert.Equal(t, TextMessage, msgType)

		data, err := io.ReadAll(reader)
		require.NoError(t, err)
		assert.Equal(t, original, data)
	})

	t.Run("Compressed fragmented message read error", func(t *testing.T) {
		compressed, err := compressData([]byte("test"), -1)
		require.NoError(t, err)

		var buf bytes.Buffer
		// First frame: Text with RSV1 (compressed), no FIN.
		buf.WriteByte(byte(TextMessage) | rsv1Bit)
		buf.WriteByte(byte(len(compressed)))
		buf.Write(compressed)
		// No continuation frame - will cause read error.

		mockConn := &mockConn{readBuf: &buf, writeBuf: &bytes.Buffer{}}
		conn := newConn(mockConn, true, 1024, 1024)
		conn.compressionEnabled = true

		_, _, err = conn.NextReader()
		require.Error(t, err)
	})

	t.Run("Compressed fragmented message wrong frame type", func(t *testing.T) {
		compressed, err := compressData([]byte("test"), -1)
		require.NoError(t, err)

		var buf bytes.Buffer
		// First frame: Text with RSV1 (compressed), no FIN.
		buf.WriteByte(byte(TextMessage) | rsv1Bit)
		buf.WriteByte(byte(len(compressed)))
		buf.Write(compressed)
		// Wrong frame type (Text instead of continuation).
		buf.WriteByte(byte(TextMessage) | finalBit)
		buf.WriteByte(5)
		buf.Write([]byte("hello"))

		mockConn := &mockConn{readBuf: &buf, writeBuf: &bytes.Buffer{}}
		conn := newConn(mockConn, true, 1024, 1024)
		conn.compressionEnabled = true

		_, _, err = conn.NextReader()
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrExpectedContinuation)
	})

	t.Run("Invalid compressed data", func(t *testing.T) {
		var buf bytes.Buffer
		// Text frame with RSV1 bit set (compressed) but invalid compressed data.
		buf.WriteByte(byte(TextMessage) | finalBit | rsv1Bit)
		buf.WriteByte(5)
		buf.Write([]byte("hello")) // Invalid deflate data.

		mockConn := &mockConn{readBuf: &buf, writeBuf: &bytes.Buffer{}}
		conn := newConn(mockConn, true, 1024, 1024)
		conn.compressionEnabled = true

		_, _, err := conn.NextReader()
		require.Error(t, err)
	})
}

func TestWriteCompressedMessage(t *testing.T) {
	t.Run("Write compressed text message", func(t *testing.T) {
		mock := newMockConn()
		conn := newConn(mock, true, 0, 0)
		conn.compressionEnabled = true
		conn.EnableWriteCompression(true)

		err := conn.WriteMessage(TextMessage, []byte("hello world, this is a test for compression"))
		require.NoError(t, err)

		data := mock.writeBuf.Bytes()
		assert.True(t, len(data) > 0)
		assert.True(t, data[0]&rsv1Bit != 0)
	})
}

func TestCloseNoStatusReceived(t *testing.T) {
	t.Run("Close frame without status code", func(t *testing.T) {
		var buf bytes.Buffer
		// Close frame with no payload.
		buf.WriteByte(byte(CloseMessage) | finalBit)
		buf.WriteByte(0)

		mockConn := &mockConn{readBuf: &buf, writeBuf: &bytes.Buffer{}}
		conn := newConn(mockConn, true, 1024, 1024)

		_, _, err := conn.NextReader()
		require.Error(t, err)
		var closeErr *CloseError
		require.ErrorAs(t, err, &closeErr)
		assert.Equal(t, CloseNoStatusReceived, closeErr.Code)
	})
}
