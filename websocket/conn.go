package websocket

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

// Connection state constants for atomic state tracking.
const (
	stateOpen    int32 = 0
	stateClosing int32 = 1
	stateClosed  int32 = 2
)

// Message types defined in RFC 6455, section 11.8.
const (
	TextMessage   = 1
	BinaryMessage = 2
	CloseMessage  = 8
	PingMessage   = 9
	PongMessage   = 10
)

// Close codes defined in RFC 6455, section 7.4.1.
const (
	CloseNormalClosure           = 1000
	CloseGoingAway               = 1001
	CloseProtocolError           = 1002
	CloseUnsupportedData         = 1003
	CloseNoStatusReceived        = 1005
	CloseAbnormalClosure         = 1006
	CloseInvalidFramePayloadData = 1007
	ClosePolicyViolation         = 1008
	CloseMessageTooBig           = 1009
	CloseMandatoryExtension      = 1010
	CloseInternalServerErr       = 1011
	CloseServiceRestart          = 1012
	CloseTryAgainLater           = 1013
	CloseTLSHandshake            = 1015
)

// Errors returned by the websocket package.
var (
	ErrCloseSent                 = errors.New("websocket: close sent")
	ErrReadLimit                 = errors.New("websocket: read limit exceeded")
	ErrBadHandshake              = errors.New("websocket: bad handshake")
	ErrInvalidControlFrame       = errors.New("websocket: invalid control frame")
	ErrInvalidMessageType        = errors.New("websocket: invalid message type")
	ErrWriteToClosedConnection   = errors.New("websocket: write to closed connection")
	ErrInvalidCloseCode          = errors.New("websocket: invalid close code")
	ErrReservedBits              = errors.New("websocket: reserved bits set")
	ErrInvalidOpcode             = errors.New("websocket: invalid opcode")
	ErrFragmentedControlFrame    = errors.New("websocket: fragmented control frame")
	ErrControlFramePayloadTooBig = errors.New("websocket: control frame payload too big")
	ErrUnexpectedContinuation    = errors.New("websocket: unexpected continuation frame")
	ErrExpectedContinuation      = errors.New("websocket: expected continuation frame")
	ErrMaskViolation             = errors.New("websocket: mask bit violation")
	ErrInvalidClosePayload       = errors.New("websocket: invalid close frame payload length")
	ErrPayloadLengthOverflow     = errors.New("websocket: payload length overflow")
	ErrInvalidUTF8               = errors.New("websocket: invalid UTF-8 in text message")
	ErrDeadlineNotSupported      = errors.New("websocket: deadline not supported on this connection")
)

// CloseError represents a WebSocket close error.
type CloseError struct {
	Code int
	Text string
}

func (e *CloseError) Error() string {
	return "websocket: close " + closeCodeString(e.Code) + " " + e.Text
}

func closeCodeString(code int) string {
	switch code {
	case CloseNormalClosure:
		return "1000 (normal)"
	case CloseGoingAway:
		return "1001 (going away)"
	case CloseProtocolError:
		return "1002 (protocol error)"
	case CloseUnsupportedData:
		return "1003 (unsupported data)"
	case CloseNoStatusReceived:
		return "1005 (no status)"
	case CloseAbnormalClosure:
		return "1006 (abnormal closure)"
	case CloseInvalidFramePayloadData:
		return "1007 (invalid payload)"
	case ClosePolicyViolation:
		return "1008 (policy violation)"
	case CloseMessageTooBig:
		return "1009 (message too big)"
	case CloseMandatoryExtension:
		return "1010 (mandatory extension)"
	case CloseInternalServerErr:
		return "1011 (internal server error)"
	case CloseServiceRestart:
		return "1012 (service restart)"
	case CloseTryAgainLater:
		return "1013 (try again later)"
	case CloseTLSHandshake:
		return "1015 (TLS handshake)"
	default:
		return strconv.Itoa(code)
	}
}

// isValidCloseCode reports whether the close code is valid on the wire
// per RFC 6455, section 7.4.1. Codes 1004, 1005, 1006, and 1015 must
// never appear in a close frame sent by an endpoint.
func isValidCloseCode(code int) bool {
	switch {
	case code >= 1000 && code <= 1003:
		return true
	case code >= 1007 && code <= 1014:
		return true
	case code >= 3000 && code <= 4999:
		return true
	default:
		return false
	}
}

// Frame header constants per RFC 6455, section 5.2.
const (
	maxFrameHeaderSize         = 14  // 2 bytes base + 8 bytes extended length + 4 bytes mask
	maxControlFramePayloadSize = 125 // RFC 6455, section 5.5: control frame payload <= 125 bytes
	defaultWriteBufferSize     = 4096
	defaultReadBufferSize      = 4096

	// First byte bits (RFC 6455, section 5.2).
	finalBit = 1 << 7 // FIN bit indicates final fragment
	rsv1Bit  = 1 << 6 // RSV1 bit used for permessage-deflate (RFC 7692)
	rsv2Bit  = 1 << 5 // RSV2 bit reserved
	rsv3Bit  = 1 << 4 // RSV3 bit reserved

	// Second byte bits (RFC 6455, section 5.2).
	maskBit = 1 << 7 // MASK bit indicates payload is masked

	// Masks and length indicators (RFC 6455, section 5.2).
	opcodeMask     = 0x0f // Opcode occupies bits 0-3
	payloadLenMask = 0x7f // Payload length occupies bits 0-6
	payloadLen16   = 126  // Indicates 16-bit extended payload length follows
	payloadLen64   = 127  // Indicates 64-bit extended payload length follows

	// Opcode for continuation frame (RFC 6455, section 5.4).
	continuationFrame = 0
)

// Conn represents a WebSocket connection.
type Conn struct {
	rwc         io.ReadWriteCloser // underlying connection
	netConn     net.Conn           // optional, for net.Conn-specific methods
	br          io.Reader          // buffered reader for reading frames
	isServer    bool
	subprotocol string
	state       int32 // atomic: stateOpen, stateClosing, stateClosed

	readMu       sync.Mutex
	readLimit    int64
	readMsgSize  int64 // accumulated message size across fragments
	readErr      error
	reader       io.Reader
	readBuf      []byte
	readMsgType  int
	readFinal    bool
	readCompress bool

	writeMu         sync.Mutex
	writeErr        error
	writeBuf        []byte
	writePos        int
	writeFrameType  int
	writeCompress   bool
	writeBufferPool BufferPool
	pingHandler     func(appData string) error
	pongHandler     func(appData string) error
	closeHandler    func(code int, text string) error

	compressionEnabled bool
	compressionLevel   int
}

func newConn(conn net.Conn, isServer bool, readBufferSize, writeBufferSize int) *Conn {
	return newConnWithPool(conn, isServer, readBufferSize, writeBufferSize, nil)
}

func newConnWithPool(conn net.Conn, isServer bool, readBufferSize, writeBufferSize int, writeBufferPool BufferPool) *Conn {
	return newConnFromRWC(conn, conn, isServer, readBufferSize, writeBufferSize, writeBufferPool)
}

func newConnFromRWC(rwc io.ReadWriteCloser, netConn net.Conn, isServer bool, readBufferSize, writeBufferSize int, writeBufferPool BufferPool) *Conn {
	if readBufferSize <= 0 {
		readBufferSize = defaultReadBufferSize
	}
	if writeBufferSize <= 0 {
		writeBufferSize = defaultWriteBufferSize
	}

	var writeBuf []byte
	if writeBufferPool != nil {
		if buf, ok := writeBufferPool.Get().([]byte); ok && len(buf) >= writeBufferSize+maxFrameHeaderSize {
			writeBuf = buf[:writeBufferSize+maxFrameHeaderSize]
		}
	}
	if writeBuf == nil {
		writeBuf = make([]byte, writeBufferSize+maxFrameHeaderSize)
	}

	var br io.Reader = rwc
	if netConn != nil {
		br = netConn
	}

	// Wrap with a buffered reader if not already buffered.
	// This reduces system call overhead for small reads during frame parsing.
	if _, ok := br.(*bufio.Reader); !ok {
		br = bufio.NewReaderSize(br, readBufferSize)
	}

	c := &Conn{
		rwc:              rwc,
		netConn:          netConn,
		br:               br,
		isServer:         isServer,
		readBuf:          make([]byte, readBufferSize),
		writeBuf:         writeBuf,
		writePos:         maxFrameHeaderSize,
		writeBufferPool:  writeBufferPool,
		compressionLevel: 1,
	}

	c.pingHandler = func(appData string) error {
		return c.WriteControl(PongMessage, []byte(appData), time.Now().Add(5*time.Second))
	}
	c.pongHandler = func(_ string) error { return nil }
	c.closeHandler = func(code int, text string) error {
		msg := FormatCloseMessage(code, text)
		_ = c.WriteControl(CloseMessage, msg, time.Now().Add(5*time.Second))
		return nil
	}

	return c
}

// Subprotocol returns the negotiated subprotocol for the connection.
func (c *Conn) Subprotocol() string {
	return c.subprotocol
}

// Close closes the underlying connection with a normal close handshake.
func (c *Conn) Close() error {
	return c.CloseWithMessage(CloseNormalClosure, "")
}

// CloseWithMessage sends a close frame with the given code and text,
// then closes the underlying connection. It is safe to call concurrently.
func (c *Conn) CloseWithMessage(code int, text string) error {
	if !atomic.CompareAndSwapInt32(&c.state, stateOpen, stateClosing) {
		// Already closing or closed.
		return nil
	}

	// Best-effort send close frame.
	msg := FormatCloseMessage(code, text)
	_ = c.WriteControl(CloseMessage, msg, time.Now().Add(5*time.Second))

	// Return write buffer to pool under lock.
	c.writeMu.Lock()
	if c.writeBufferPool != nil && c.writeBuf != nil {
		c.writeBufferPool.Put(c.writeBuf)
		c.writeBuf = nil
	}
	c.writeMu.Unlock()

	atomic.StoreInt32(&c.state, stateClosed)
	return c.rwc.Close()
}

// IsClosed reports whether the connection has been closed.
func (c *Conn) IsClosed() bool {
	return atomic.LoadInt32(&c.state) == stateClosed
}

// LocalAddr returns the local network address, or nil if not available.
func (c *Conn) LocalAddr() net.Addr {
	if c.netConn != nil {
		return c.netConn.LocalAddr()
	}
	return nil
}

// RemoteAddr returns the remote network address, or nil if not available.
func (c *Conn) RemoteAddr() net.Addr {
	if c.netConn != nil {
		return c.netConn.RemoteAddr()
	}
	return nil
}

// UnderlyingConn returns the underlying net.Conn, or nil for HTTP/2 connections.
func (c *Conn) UnderlyingConn() net.Conn {
	return c.netConn
}

// SetReadDeadline sets the read deadline on the underlying network connection.
// Returns ErrDeadlineNotSupported if the underlying connection does not support deadlines (e.g., HTTP/2).
func (c *Conn) SetReadDeadline(t time.Time) error {
	if c.netConn != nil {
		return c.netConn.SetReadDeadline(t)
	}
	return ErrDeadlineNotSupported
}

// SetWriteDeadline sets the write deadline on the underlying network connection.
// Returns ErrDeadlineNotSupported if the underlying connection does not support deadlines (e.g., HTTP/2).
func (c *Conn) SetWriteDeadline(t time.Time) error {
	if c.netConn != nil {
		return c.netConn.SetWriteDeadline(t)
	}
	return ErrDeadlineNotSupported
}

// SetReadLimit sets the maximum size in bytes for a message read from the peer.
func (c *Conn) SetReadLimit(limit int64) {
	c.readLimit = limit
}

// StartKeepalive sends periodic ping frames to detect dead connections.
// The interval specifies how often pings are sent. The pongTimeout specifies
// how long to wait for a pong response before considering the connection dead.
// The goroutine stops when the connection is closed.
func (c *Conn) StartKeepalive(interval, pongTimeout time.Duration) {
	var lastPong atomic.Int64
	lastPong.Store(time.Now().UnixNano())
	c.SetPongHandler(func(_ string) error {
		lastPong.Store(time.Now().UnixNano())
		return nil
	})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			if c.IsClosed() {
				return
			}

			elapsed := time.Since(time.Unix(0, lastPong.Load()))
			if elapsed > interval+pongTimeout {
				_ = c.Close()
				return
			}

			if err := c.WriteControl(PingMessage, nil, time.Now().Add(pongTimeout)); err != nil {
				return
			}
		}
	}()
}

// SetPingHandler sets the handler for ping messages received from the peer.
func (c *Conn) SetPingHandler(h func(appData string) error) {
	if h == nil {
		h = func(appData string) error {
			return c.WriteControl(PongMessage, []byte(appData), time.Now().Add(5*time.Second))
		}
	}
	c.pingHandler = h
}

// SetPongHandler sets the handler for pong messages received from the peer.
func (c *Conn) SetPongHandler(h func(appData string) error) {
	if h == nil {
		h = func(_ string) error { return nil }
	}
	c.pongHandler = h
}

// SetCloseHandler sets the handler for close messages received from the peer.
func (c *Conn) SetCloseHandler(h func(code int, text string) error) {
	if h == nil {
		h = func(code int, text string) error {
			msg := FormatCloseMessage(code, text)
			_ = c.WriteControl(CloseMessage, msg, time.Now().Add(5*time.Second))
			return nil
		}
	}
	c.closeHandler = h
}

// EnableWriteCompression enables or disables write compression for the connection.
// When enabled and compression is negotiated (RFC 7692), outgoing messages will
// be compressed using the permessage-deflate extension.
func (c *Conn) EnableWriteCompression(enable bool) {
	c.writeCompress = enable
}

// SetCompressionLevel sets the compression level for the connection.
// Valid levels are -2 to 9 (flate package constants).
// Per RFC 7692, compression uses the DEFLATE algorithm.
func (c *Conn) SetCompressionLevel(level int) error {
	if level < -2 || level > 9 {
		return errors.New("websocket: invalid compression level")
	}
	c.compressionLevel = level
	return nil
}

// WriteControl writes a control message with the given deadline.
func (c *Conn) WriteControl(messageType int, data []byte, deadline time.Time) error {
	if messageType != CloseMessage && messageType != PingMessage && messageType != PongMessage {
		return ErrInvalidControlFrame
	}
	if len(data) > maxControlFramePayloadSize {
		return ErrControlFramePayloadTooBig
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if c.writeErr != nil {
		return c.writeErr
	}

	if c.netConn != nil {
		_ = c.netConn.SetWriteDeadline(deadline)
	}
	frame := make([]byte, 2+len(data))
	frame[0] = byte(messageType) | finalBit
	frame[1] = byte(len(data))
	if !c.isServer {
		frame[1] |= maskBit
		mask := make([]byte, 4)
		if _, err := io.ReadFull(randReader, mask); err != nil {
			return err
		}
		frame = append(frame[:2], mask...)
		frame = append(frame, data...)
		maskBytes(mask, 0, frame[6:])
	} else {
		copy(frame[2:], data)
	}

	_, err := c.rwc.Write(frame)

	// Clear the deadline so subsequent data writes are not affected.
	if c.netConn != nil {
		_ = c.netConn.SetWriteDeadline(time.Time{})
	}

	if messageType == CloseMessage {
		c.writeErr = ErrCloseSent
	}
	return err
}

// WriteMessage writes a message with the given message type and payload.
func (c *Conn) WriteMessage(messageType int, data []byte) error {
	if messageType != TextMessage && messageType != BinaryMessage {
		return ErrInvalidMessageType
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if c.writeErr != nil {
		return c.writeErr
	}

	compress := c.writeCompress && c.compressionEnabled
	_, err := c.writeFrameWithCompress(messageType, data, true, compress)
	return err
}

// NextWriter returns a writer for the next message to send.
// Only TextMessage and BinaryMessage are valid message types.
func (c *Conn) NextWriter(messageType int) (io.WriteCloser, error) {
	if messageType != TextMessage && messageType != BinaryMessage {
		return nil, ErrInvalidMessageType
	}

	c.writeMu.Lock()

	if c.writeErr != nil {
		c.writeMu.Unlock()
		return nil, c.writeErr
	}

	c.writeFrameType = messageType
	return &messageWriter{c: c, compress: c.writeCompress && c.compressionEnabled}, nil
}

// ReadMessage reads the next message from the connection.
// For text messages, validates UTF-8 encoding per RFC 6455, section 5.6.
func (c *Conn) ReadMessage() (messageType int, p []byte, err error) {
	var r io.Reader
	messageType, r, err = c.NextReader()
	if err != nil {
		return 0, nil, err
	}
	p, err = io.ReadAll(r)
	if err != nil {
		return messageType, p, err
	}
	if messageType == TextMessage && !utf8.Valid(p) {
		return 0, nil, ErrInvalidUTF8
	}
	return messageType, p, err
}

// ReadMessageContext reads a message with context cancellation support.
// When the context is cancelled, the read deadline is set to the current time
// to unblock any pending read operation.
func (c *Conn) ReadMessageContext(ctx context.Context) (messageType int, p []byte, err error) {
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		select {
		case <-ctx.Done():
			_ = c.SetReadDeadline(time.Now())
		case <-done:
		}
	}()

	messageType, p, err = c.ReadMessage()
	close(done)
	<-finished // wait for goroutine to complete before resetting deadline

	if ctx.Err() != nil {
		// Reset the deadline so subsequent reads are not affected.
		_ = c.SetReadDeadline(time.Time{})
		if err != nil {
			return 0, nil, ctx.Err()
		}
	}

	return messageType, p, err
}

// NextReader returns the next message reader from the connection.
func (c *Conn) NextReader() (messageType int, r io.Reader, err error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()

	if c.readErr != nil {
		return 0, nil, c.readErr
	}

	for {
		frameType, payload, final, compressed, err := c.readFrame()
		if err != nil {
			c.readErr = err
			return 0, nil, err
		}

		switch frameType {
		case PingMessage:
			if err := c.pingHandler(string(payload)); err != nil {
				return 0, nil, err
			}
			continue
		case PongMessage:
			if err := c.pongHandler(string(payload)); err != nil {
				return 0, nil, err
			}
			continue
		case CloseMessage:
			code := CloseNoStatusReceived
			text := ""
			if len(payload) >= 2 {
				code = int(payload[0])<<8 | int(payload[1])
				text = string(payload[2:])
				if !isValidCloseCode(code) {
					c.readErr = ErrInvalidCloseCode
					return 0, nil, ErrInvalidCloseCode
				}
			}
			if err := c.closeHandler(code, text); err != nil {
				return 0, nil, err
			}
			c.readErr = &CloseError{Code: code, Text: text}
			return 0, nil, c.readErr
		case TextMessage, BinaryMessage:
			c.readMsgType = frameType
			c.readFinal = final
			c.readCompress = compressed
			c.readMsgSize = int64(len(payload))

			// Per RFC 7692, compression applies to the entire message.
			// For fragmented compressed messages, we must read all frames,
			// concatenate the compressed data, then decompress once.
			switch {
			case compressed && !final:
				// Read all continuation frames and accumulate compressed data.
				compressedData := payload
				for !final {
					ft, p, f, _, readErr := c.readFrame()
					if readErr != nil {
						c.readErr = readErr
						return 0, nil, readErr
					}
					// Handle control frames inline per RFC 6455, section 5.4.
					switch ft {
					case PingMessage:
						if err := c.pingHandler(string(p)); err != nil {
							return 0, nil, err
						}
						continue
					case PongMessage:
						if err := c.pongHandler(string(p)); err != nil {
							return 0, nil, err
						}
						continue
					case CloseMessage:
						code := CloseNoStatusReceived
						text := ""
						if len(p) >= 2 {
							code = int(p[0])<<8 | int(p[1])
							text = string(p[2:])
							if !isValidCloseCode(code) {
								c.readErr = ErrInvalidCloseCode
								return 0, nil, ErrInvalidCloseCode
							}
						}
						if err := c.closeHandler(code, text); err != nil {
							return 0, nil, err
						}
						c.readErr = &CloseError{Code: code, Text: text}
						return 0, nil, c.readErr
					case continuationFrame:
						// Expected continuation frame.
					default:
						return 0, nil, ErrExpectedContinuation
					}
					c.readMsgSize += int64(len(p))
					if c.readLimit > 0 && c.readMsgSize > c.readLimit {
						c.readErr = ErrReadLimit
						return 0, nil, ErrReadLimit
					}
					compressedData = append(compressedData, p...)
					final = f
				}
				// Decompress the complete message, enforcing read limit
				// to prevent decompression bombs.
				var decErr error
				payload, decErr = decompressDataLimited(compressedData, c.readLimit)
				if decErr != nil {
					if decErr == ErrReadLimit {
						c.readErr = ErrReadLimit
					}
					return 0, nil, decErr
				}
				c.reader = &messageReader{c: c, buf: payload, final: true, compressed: false}
			case compressed:
				// Single frame compressed message, enforcing read limit
				// to prevent decompression bombs.
				var decErr error
				payload, decErr = decompressDataLimited(payload, c.readLimit)
				if decErr != nil {
					if decErr == ErrReadLimit {
						c.readErr = ErrReadLimit
					}
					return 0, nil, decErr
				}
				c.reader = &messageReader{c: c, buf: payload, final: final, compressed: false}
			default:
				// Uncompressed message.
				c.reader = &messageReader{c: c, buf: payload, final: final, compressed: false}
			}
			return frameType, c.reader, nil
		case continuationFrame:
			return 0, nil, ErrUnexpectedContinuation
		default:
			return 0, nil, ErrInvalidOpcode
		}
	}
}

// readFrame reads a single WebSocket frame per RFC 6455, section 5.2.
// Returns the frame opcode, payload, final flag, and compression flag.
// The compressed flag is set when RSV1 is set (RFC 7692 permessage-deflate).
func (c *Conn) readFrame() (frameType int, payload []byte, final bool, compressed bool, err error) {
	// Use readBuf for header reading to reduce allocations.
	// readBuf layout: [0:2] header, [2:10] extended length, [10:14] mask
	if len(c.readBuf) < maxFrameHeaderSize {
		c.readBuf = make([]byte, maxFrameHeaderSize)
	}

	// Read the first two bytes of the frame header (RFC 6455, section 5.2).
	if _, err := io.ReadFull(c.br, c.readBuf[:2]); err != nil {
		return 0, nil, false, false, err
	}

	// Parse first byte: FIN, RSV1-3, opcode (RFC 6455, section 5.2).
	final = c.readBuf[0]&finalBit != 0
	compressed = c.readBuf[0]&rsv1Bit != 0 // RSV1 indicates compressed per RFC 7692
	rsv2 := c.readBuf[0]&rsv2Bit != 0
	rsv3 := c.readBuf[0]&rsv3Bit != 0

	// RSV2 and RSV3 must be 0 unless an extension defines them (RFC 6455, section 5.2).
	// RSV1 is only valid if permessage-deflate was negotiated (RFC 7692).
	if rsv2 || rsv3 || (compressed && !c.compressionEnabled) {
		return 0, nil, false, false, ErrReservedBits
	}

	frameType = int(c.readBuf[0] & opcodeMask)
	masked := c.readBuf[1]&maskBit != 0

	// RFC 6455, section 5.1: client-to-server frames MUST be masked,
	// server-to-client frames MUST NOT be masked.
	if c.isServer && !masked {
		return 0, nil, false, false, ErrMaskViolation
	}

	if !c.isServer && masked {
		return 0, nil, false, false, ErrMaskViolation
	}

	// RFC 6455, section 5.5: RSV1 must be 0 on control frames, even
	// when permessage-deflate is negotiated (RFC 7692, section 6.1).
	if compressed && frameType >= CloseMessage {
		return 0, nil, false, false, ErrReservedBits
	}

	payloadLen := int64(c.readBuf[1] & payloadLenMask)

	switch payloadLen {
	case payloadLen16:
		if _, err := io.ReadFull(c.br, c.readBuf[2:4]); err != nil {
			return 0, nil, false, false, err
		}
		payloadLen = int64(c.readBuf[2])<<8 | int64(c.readBuf[3])
	case payloadLen64:
		if _, err := io.ReadFull(c.br, c.readBuf[2:10]); err != nil {
			return 0, nil, false, false, err
		}
		// RFC 6455, section 5.2: the most significant bit MUST be 0.
		if c.readBuf[2]&0x80 != 0 {
			return 0, nil, false, false, ErrPayloadLengthOverflow
		}
		payloadLen = int64(c.readBuf[2])<<56 | int64(c.readBuf[3])<<48 |
			int64(c.readBuf[4])<<40 | int64(c.readBuf[5])<<32 |
			int64(c.readBuf[6])<<24 | int64(c.readBuf[7])<<16 |
			int64(c.readBuf[8])<<8 | int64(c.readBuf[9])
	}

	if frameType >= CloseMessage && payloadLen > maxControlFramePayloadSize {
		return 0, nil, false, false, ErrControlFramePayloadTooBig
	}

	if frameType >= CloseMessage && !final {
		return 0, nil, false, false, ErrFragmentedControlFrame
	}

	// RFC 6455, section 5.5.1: close frame payload must be 0 or >= 2 bytes.
	if frameType == CloseMessage && payloadLen == 1 {
		return 0, nil, false, false, ErrInvalidClosePayload
	}

	// Check read limit (0 means unlimited).
	if c.readLimit > 0 && payloadLen > c.readLimit {
		return 0, nil, false, false, ErrReadLimit
	}

	var mask []byte
	if masked {
		if _, err := io.ReadFull(c.br, c.readBuf[10:14]); err != nil {
			return 0, nil, false, false, err
		}
		mask = c.readBuf[10:14]
	}

	payload = make([]byte, payloadLen)
	if _, err := io.ReadFull(c.br, payload); err != nil {
		return 0, nil, false, false, err
	}

	if masked {
		maskBytes(mask, 0, payload)
	}

	return frameType, payload, final, compressed, nil
}

// maskBytes applies XOR masking to data per RFC 6455, section 5.3.
// Client-to-server frames must be masked; server-to-client frames must not.
// The mask is a 4-byte value, applied cyclically to each byte of the payload.
// maskBytes applies the XOR mask to data per RFC 6455, section 5.3.
// Uses word-at-a-time XOR for performance on larger payloads.
func maskBytes(mask []byte, pos int, data []byte) int {
	n := len(data)
	if n == 0 {
		return pos % 4
	}

	i := 0

	// Handle alignment: XOR byte-by-byte until pos is aligned to mask boundary.
	for i < n && pos%4 != 0 {
		data[i] ^= mask[pos%4]
		pos++
		i++
	}

	// XOR 4 bytes at a time using uint32 operations.
	if n-i >= 4 {
		maskWord := binary.BigEndian.Uint32(mask)
		for i+4 <= n {
			v := binary.BigEndian.Uint32(data[i:])
			binary.BigEndian.PutUint32(data[i:], v^maskWord)
			i += 4
			pos += 4
		}
	}

	// Handle remaining bytes.
	for i < n {
		data[i] ^= mask[pos%4]
		pos++
		i++
	}

	return pos % 4
}

type messageWriter struct {
	c          *Conn
	compress   bool
	closed     bool
	firstWrite bool
	buf        []byte // buffer for compressed message assembly (RFC 7692)
}

func (w *messageWriter) Write(p []byte) (int, error) {
	if w.closed {
		return 0, ErrWriteToClosedConnection
	}

	// Per RFC 7692, compression applies to the entire message before fragmentation.
	// Buffer all data until Close() to compress as a single unit.
	if w.compress {
		w.buf = append(w.buf, p...)
		return len(p), nil
	}

	frameType := w.c.writeFrameType
	if w.firstWrite {
		frameType = continuationFrame
	} else {
		w.firstWrite = true
	}

	_, err := w.c.writeFrameWithCompress(frameType, p, false, false)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (w *messageWriter) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true

	if w.compress {
		// Compress the entire buffered message and send as a single frame.
		_, err := w.c.writeFrameWithCompress(w.c.writeFrameType, w.buf, true, true)
		w.buf = nil
		w.c.writeFrameType = 0
		w.c.writeMu.Unlock()
		return err
	}

	frameType := w.c.writeFrameType
	if w.firstWrite {
		frameType = continuationFrame
	}

	_, err := w.c.writeFrameWithCompress(frameType, nil, true, false)
	w.c.writeFrameType = 0
	w.c.writeMu.Unlock()
	return err
}

// writeFrameWithCompress writes a WebSocket frame per RFC 6455, section 5.2.
// If compress is true, the payload is compressed using DEFLATE (RFC 7692)
// and RSV1 bit is set to indicate permessage-deflate compression.
func (c *Conn) writeFrameWithCompress(frameType int, data []byte, final bool, compress bool) (int, error) {
	if c.writeErr != nil {
		return 0, c.writeErr
	}

	originalLen := len(data)

	// Compress payload if requested (RFC 7692 permessage-deflate).
	if compress {
		var err error
		data, err = compressData(data, c.compressionLevel)
		if err != nil {
			return 0, err
		}
	}

	// Use writeBuf for header to reduce allocations.
	// writeBuf has maxFrameHeaderSize bytes at the beginning for the header.
	headerLen := 2

	// First byte: FIN, RSV1, opcode.
	b0 := byte(frameType)
	if final {
		b0 |= finalBit // Set FIN bit for final fragment
	}
	if compress {
		b0 |= rsv1Bit // Set RSV1 for compressed frame (RFC 7692)
	}
	c.writeBuf[0] = b0

	payloadLen := len(data)
	switch {
	case payloadLen <= 125:
		c.writeBuf[1] = byte(payloadLen)
	case payloadLen <= 65535:
		c.writeBuf[1] = payloadLen16
		c.writeBuf[2] = byte(payloadLen >> 8)
		c.writeBuf[3] = byte(payloadLen)
		headerLen = 4
	default:
		c.writeBuf[1] = payloadLen64
		c.writeBuf[2] = byte(payloadLen >> 56)
		c.writeBuf[3] = byte(payloadLen >> 48)
		c.writeBuf[4] = byte(payloadLen >> 40)
		c.writeBuf[5] = byte(payloadLen >> 32)
		c.writeBuf[6] = byte(payloadLen >> 24)
		c.writeBuf[7] = byte(payloadLen >> 16)
		c.writeBuf[8] = byte(payloadLen >> 8)
		c.writeBuf[9] = byte(payloadLen)
		headerLen = 10
	}

	if !c.isServer {
		c.writeBuf[1] |= maskBit
		if _, err := io.ReadFull(randReader, c.writeBuf[headerLen:headerLen+4]); err != nil {
			return 0, err
		}
		mask := c.writeBuf[headerLen : headerLen+4]
		headerLen += 4

		maskedData := make([]byte, len(data))
		copy(maskedData, data)
		maskBytes(mask, 0, maskedData)
		data = maskedData
	}

	// If payload fits in writeBuf after header, use single write.
	if headerLen+payloadLen <= len(c.writeBuf) {
		copy(c.writeBuf[headerLen:], data)
		_, err := c.rwc.Write(c.writeBuf[:headerLen+payloadLen])
		if err != nil {
			c.writeErr = err
		}
		return originalLen, err
	}

	// For large payloads, write header and data separately.
	if _, err := c.rwc.Write(c.writeBuf[:headerLen]); err != nil {
		c.writeErr = err
		return 0, err
	}
	_, err := c.rwc.Write(data)
	if err != nil {
		c.writeErr = err
	}
	return originalLen, err
}

type messageReader struct {
	c          *Conn
	buf        []byte
	pos        int
	final      bool
	compressed bool
}

func (r *messageReader) Read(p []byte) (int, error) {
	for r.pos >= len(r.buf) {
		if r.final {
			return 0, io.EOF
		}
		// Read next frame for uncompressed fragmented messages.
		// Compressed fragmented messages are fully read in NextReader.
		frameType, payload, final, _, err := r.c.readFrame()
		if err != nil {
			r.c.readErr = err
			return 0, err
		}
		// Handle control frames inline per RFC 6455, section 5.4.
		switch frameType {
		case PingMessage:
			if err := r.c.pingHandler(string(payload)); err != nil {
				return 0, err
			}
			continue
		case PongMessage:
			if err := r.c.pongHandler(string(payload)); err != nil {
				return 0, err
			}
			continue
		case CloseMessage:
			code := CloseNoStatusReceived
			text := ""
			if len(payload) >= 2 {
				code = int(payload[0])<<8 | int(payload[1])
				text = string(payload[2:])
				if !isValidCloseCode(code) {
					r.c.readErr = ErrInvalidCloseCode
					return 0, ErrInvalidCloseCode
				}
			}
			if err := r.c.closeHandler(code, text); err != nil {
				return 0, err
			}
			closeErr := &CloseError{Code: code, Text: text}
			r.c.readErr = closeErr
			return 0, closeErr
		case continuationFrame:
			// Expected continuation frame.
		default:
			return 0, ErrExpectedContinuation
		}
		r.c.readMsgSize += int64(len(payload))
		if r.c.readLimit > 0 && r.c.readMsgSize > r.c.readLimit {
			r.c.readErr = ErrReadLimit
			return 0, ErrReadLimit
		}
		r.buf = payload
		r.pos = 0
		r.final = final
	}

	n := copy(p, r.buf[r.pos:])
	r.pos += n
	return n, nil
}
