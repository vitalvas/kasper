package websocket

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPreparedMessage(t *testing.T) {
	tests := []struct {
		name            string
		messageType     int
		data            []byte
		expectErr       bool
		expectedErrIs   error
		wantMessageType int
		wantData        []byte
	}{
		{
			name:            "Valid text message",
			messageType:     TextMessage,
			data:            []byte("hello"),
			wantMessageType: TextMessage,
			wantData:        []byte("hello"),
		},
		{
			name:            "Valid binary message",
			messageType:     BinaryMessage,
			data:            []byte{0x01, 0x02, 0x03},
			wantMessageType: BinaryMessage,
		},
		{
			name:          "Invalid message type",
			messageType:   PingMessage,
			data:          []byte("ping"),
			expectErr:     true,
			expectedErrIs: ErrInvalidMessageType,
		},
		{
			name:        "Empty data",
			messageType: TextMessage,
			data:        []byte{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm, err := NewPreparedMessage(tt.messageType, tt.data)

			if tt.expectErr {
				assert.Nil(t, pm)
				assert.ErrorIs(t, err, tt.expectedErrIs)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, pm)

			if tt.wantMessageType != 0 {
				assert.Equal(t, tt.wantMessageType, pm.messageType)
			}

			if tt.wantData != nil {
				assert.Equal(t, tt.wantData, pm.data)
			}
		})
	}
}

func TestBuildFrame(t *testing.T) {
	t.Run("Server frame not masked", func(t *testing.T) {
		frame := buildFrame(TextMessage, []byte("hello"), false, false)

		assert.True(t, len(frame) >= 2)
		assert.Equal(t, byte(TextMessage)|finalBit, frame[0])
		assert.Equal(t, byte(5), frame[1])
		assert.Equal(t, []byte("hello"), frame[2:])
	})

	t.Run("Client frame masked", func(t *testing.T) {
		frame := buildFrame(TextMessage, []byte("hello"), true, false)

		assert.True(t, len(frame) >= 7)
		assert.Equal(t, byte(TextMessage)|finalBit, frame[0])
		assert.Equal(t, byte(5)|maskBit, frame[1])
	})

	t.Run("Compressed frame", func(t *testing.T) {
		frame := buildFrame(TextMessage, []byte("hello"), false, true)

		assert.Equal(t, byte(TextMessage)|finalBit|rsv1Bit, frame[0])
	})

	t.Run("16-bit length", func(t *testing.T) {
		data := make([]byte, 200)
		frame := buildFrame(BinaryMessage, data, false, false)

		assert.Equal(t, byte(payloadLen16), frame[1])
		assert.Equal(t, byte(0), frame[2])
		assert.Equal(t, byte(200), frame[3])
	})

	t.Run("64-bit length", func(t *testing.T) {
		data := make([]byte, 70000)
		frame := buildFrame(BinaryMessage, data, false, false)

		assert.Equal(t, byte(payloadLen64), frame[1])
	})
}

func TestPreparedMessageFrame(t *testing.T) {
	t.Run("Cache frames", func(t *testing.T) {
		pm, err := NewPreparedMessage(TextMessage, []byte("hello"))
		require.NoError(t, err)

		key := prepareKey{isServer: true, compress: false, compressNo: true}

		frame1, err := pm.frame(key)
		require.NoError(t, err)

		frame2, err := pm.frame(key)
		require.NoError(t, err)

		assert.Equal(t, frame1, frame2)
		assert.Len(t, pm.frames, 1)
	})

	t.Run("Different keys different frames", func(t *testing.T) {
		pm, err := NewPreparedMessage(TextMessage, []byte("hello"))
		require.NoError(t, err)

		serverKey := prepareKey{isServer: true, compress: false, compressNo: true}
		clientKey := prepareKey{isServer: false, compress: false, compressNo: true}

		serverFrame, err := pm.frame(serverKey)
		require.NoError(t, err)

		clientFrame, err := pm.frame(clientKey)
		require.NoError(t, err)

		assert.NotEqual(t, serverFrame, clientFrame)
		assert.Len(t, pm.frames, 2)
	})
}

func TestWritePreparedMessage(t *testing.T) {
	tests := []struct {
		name         string
		isServer     bool
		checkMaskBit bool
	}{
		{
			name:     "Server writes prepared message",
			isServer: true,
		},
		{
			name:         "Client writes prepared message",
			isServer:     false,
			checkMaskBit: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm, err := NewPreparedMessage(TextMessage, []byte("prepared hello"))
			require.NoError(t, err)

			mock := newMockConn()
			conn := newConn(mock, tt.isServer, 0, 0)

			err = conn.WritePreparedMessage(pm)
			require.NoError(t, err)

			data := mock.writeBuf.Bytes()
			assert.Equal(t, byte(TextMessage)|finalBit, data[0])

			if tt.checkMaskBit {
				assert.True(t, data[1]&maskBit != 0)
			}
		})
	}
}

func TestWritePreparedMessageMultiple(t *testing.T) {
	t.Run("Same message to multiple connections", func(t *testing.T) {
		pm, err := NewPreparedMessage(TextMessage, []byte("shared message"))
		require.NoError(t, err)

		mock1 := newMockConn()
		conn1 := newConn(mock1, true, 0, 0)

		mock2 := newMockConn()
		conn2 := newConn(mock2, true, 0, 0)

		err = conn1.WritePreparedMessage(pm)
		require.NoError(t, err)

		err = conn2.WritePreparedMessage(pm)
		require.NoError(t, err)

		assert.Equal(t, mock1.writeBuf.Bytes(), mock2.writeBuf.Bytes())
	})
}

func TestWritePreparedMessageError(t *testing.T) {
	t.Run("Write to closed connection", func(t *testing.T) {
		mock := newMockConn()
		conn := newConn(mock, true, 0, 0)
		conn.writeErr = ErrCloseSent

		pm, err := NewPreparedMessage(TextMessage, []byte("test"))
		require.NoError(t, err)

		err = conn.WritePreparedMessage(pm)
		assert.ErrorIs(t, err, ErrCloseSent)
	})
}

func TestPreparedMessageWithCompression(t *testing.T) {
	t.Run("Compressed frame", func(t *testing.T) {
		pm, err := NewPreparedMessage(TextMessage, []byte("compressible data"))
		require.NoError(t, err)

		key := prepareKey{isServer: true, compress: true, compressNo: false}
		frame, err := pm.frame(key)
		require.NoError(t, err)

		assert.Equal(t, byte(TextMessage)|finalBit|rsv1Bit, frame[0])
	})

	t.Run("Compressed and uncompressed different", func(t *testing.T) {
		pm, err := NewPreparedMessage(TextMessage, []byte("data to compress"))
		require.NoError(t, err)

		compressedKey := prepareKey{isServer: true, compress: true, compressNo: false}
		uncompressedKey := prepareKey{isServer: true, compress: false, compressNo: true}

		compFrame, err := pm.frame(compressedKey)
		require.NoError(t, err)

		uncomFrame, err := pm.frame(uncompressedKey)
		require.NoError(t, err)

		assert.NotEqual(t, compFrame, uncomFrame)
	})

	t.Run("Client compressed frame masked", func(t *testing.T) {
		pm, err := NewPreparedMessage(TextMessage, []byte("compressed client"))
		require.NoError(t, err)

		key := prepareKey{isServer: false, compress: true, compressNo: false}
		frame, err := pm.frame(key)
		require.NoError(t, err)

		assert.Equal(t, byte(TextMessage)|finalBit|rsv1Bit, frame[0])
		assert.True(t, frame[1]&maskBit != 0)
	})
}

func TestWritePreparedMessageCompressed(t *testing.T) {
	t.Run("Write compressed prepared message", func(t *testing.T) {
		pm, err := NewPreparedMessage(TextMessage, []byte("compress me"))
		require.NoError(t, err)

		mock := newMockConn()
		conn := newConn(mock, true, 0, 0)
		conn.compressionEnabled = true
		conn.writeCompress = true

		err = conn.WritePreparedMessage(pm)
		require.NoError(t, err)

		data := mock.writeBuf.Bytes()
		assert.True(t, len(data) > 0)
		assert.Equal(t, byte(TextMessage)|finalBit|rsv1Bit, data[0])
	})
}

func BenchmarkPreparedMessage(b *testing.B) {
	data := []byte("prepared message data prepared message data prepared message data ")
	pm, _ := NewPreparedMessage(TextMessage, data)

	b.Run("Create", func(b *testing.B) {
		for b.Loop() {
			_, _ = NewPreparedMessage(TextMessage, data)
		}
	})

	b.Run("Write", func(b *testing.B) {
		mock := newMockConn()
		conn := newConn(mock, true, 0, 0)

		b.ResetTimer()

		for b.Loop() {
			mock.writeBuf.Reset()
			_ = conn.WritePreparedMessage(pm)
		}
	})

	b.Run("WriteMultiple", func(b *testing.B) {
		mocks := make([]*mockConn, 10)
		conns := make([]*Conn, 10)
		for i := range mocks {
			mocks[i] = newMockConn()
			conns[i] = newConn(mocks[i], true, 0, 0)
		}

		b.ResetTimer()

		for b.Loop() {
			for i := range conns {
				mocks[i].writeBuf.Reset()
				_ = conns[i].WritePreparedMessage(pm)
			}
		}
	})
}
