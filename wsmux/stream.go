package wsmux

import (
	"io"
	"sync"
)

// Stream is one logical channel multiplexed over a shared websocket.Conn.
// Read and Write may each be called from one goroutine concurrently with the
// other, matching the single-reader/single-writer model of the underlying
// connection.
type Stream struct {
	id  uint32
	mux *Mux

	recv chan []byte // inbound payloads; the Mux read loop is the sole sender

	writeMu sync.Mutex // orders outgoing DATA and CLOSE frames
	mu      sync.Mutex
	closed  bool
}

func newStream(id uint32, mux *Mux, bufSize int) *Stream {
	return &Stream{
		id:   id,
		mux:  mux,
		recv: make(chan []byte, bufSize),
	}
}

// ID returns the stream's identifier.
func (s *Stream) ID() uint32 { return s.id }

// Read returns the next inbound payload, blocking until one arrives. It
// returns io.EOF once the stream is closed and all buffered payloads have
// been drained.
func (s *Stream) Read() ([]byte, error) {
	data, ok := <-s.recv
	if !ok {
		return nil, io.EOF
	}
	return data, nil
}

// Write sends a payload to the peer as a DATA frame on this stream.
func (s *Stream) Write(data []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return io.ErrClosedPipe
	}
	return s.mux.writeFrame(frameData, s.id, data)
}

// Close sends a CLOSE frame to the peer and tears down the local stream.
// It is idempotent.
func (s *Stream) Close() error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if !s.kill() {
		return nil
	}
	s.mux.removeStream(s.id)
	// Best-effort CLOSE notification; the tunnel may already be gone.
	_ = s.mux.writeFrame(frameClose, s.id, nil)
	return nil
}

// deliver hands an inbound payload to the stream, holding s.mu so it cannot
// race kill closing recv. Returns false if the stream is closed or its buffer
// is full; the Mux uses that to avoid blocking the shared read loop on one
// slow stream (no head-of-line blocking across streams).
func (s *Stream) deliver(payload []byte) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	select {
	case s.recv <- payload:
		return true
	default:
		return false
	}
}

// kill marks the stream closed and closes its receive channel so a blocked
// Read returns io.EOF. Holding s.mu makes close(recv) mutually exclusive with
// deliver's send, so there is no send-on-closed-channel race. It is
// idempotent.
func (s *Stream) kill() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.closed = true
	close(s.recv)
	return true
}
