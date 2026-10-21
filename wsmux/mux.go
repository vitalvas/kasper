package wsmux

import (
	"errors"
	"sync"

	"go.vitalvas.com/kasper/websocket"
)

// defaultStreamBuffer is the inbound payload buffer per stream. A full buffer
// causes the read loop to drop and reset that stream rather than block the
// shared tunnel.
const defaultStreamBuffer = 64

// ErrClosed is returned once the Mux (and its underlying connection) is closed.
var ErrClosed = errors.New("wsmux: closed")

// ErrStreamIDExhausted is returned when all local stream IDs have been used.
var ErrStreamIDExhausted = errors.New("wsmux: stream IDs exhausted")

// messageConn is the subset of *websocket.Conn the Mux depends on, so tests
// can substitute an in-memory tunnel.
type messageConn interface {
	ReadMessage() (int, []byte, error)
	WriteMessage(messageType int, data []byte) error
	CloseNow() error
}

// *websocket.Conn must satisfy messageConn.
var _ messageConn = (*websocket.Conn)(nil)

// Mux multiplexes many logical Streams over one WebSocket connection.
//
// A single internal goroutine owns all reads and delivers stream payloads.
// Stream locks serialize delivery with teardown. Writes from multiple streams
// are serialized on a mutex, matching the one-writer rule of the connection.
type Mux struct {
	conn      messageConn
	bufSize   int
	clientOdd bool // this side allocates odd stream IDs

	writeMu sync.Mutex

	mu      sync.Mutex
	streams map[uint32]*Stream
	retired retiredIDs // bounded history; closed peer IDs must never be reopened
	nextID  uint32
	closed  bool

	accept chan *Stream
	resets chan uint32
	done   chan struct{}
}

// Options configures a Mux.
type Options struct {
	// StreamBuffer is the per-stream inbound payload buffer. Zero uses a
	// sensible default.
	StreamBuffer int
}

// NewClient wraps conn as the stream-initiating (odd-ID) side and starts the
// read loop.
func NewClient(conn *websocket.Conn, opts Options) *Mux {
	return newMux(conn, opts, true)
}

// NewServer wraps conn as the accepting (even-ID) side and starts the read
// loop. Peer-initiated streams arrive via Accept.
func NewServer(conn *websocket.Conn, opts Options) *Mux {
	return newMux(conn, opts, false)
}

func newMux(conn messageConn, opts Options, clientOdd bool) *Mux {
	bufSize := opts.StreamBuffer
	if bufSize <= 0 {
		bufSize = defaultStreamBuffer
	}
	first := uint32(2)
	if clientOdd {
		first = 1
	}
	m := &Mux{
		conn:      conn,
		bufSize:   bufSize,
		clientOdd: clientOdd,
		streams:   make(map[uint32]*Stream),
		nextID:    first,
		accept:    make(chan *Stream, 16),
		resets:    make(chan uint32, 16),
		done:      make(chan struct{}),
	}
	go m.readLoop()
	go m.resetLoop()
	return m
}

// Open creates a new locally-initiated stream. The peer sees it on the first
// DATA frame.
func (m *Mux) Open() (*Stream, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, ErrClosed
	}
	if m.nextID == 0 {
		return nil, ErrStreamIDExhausted
	}
	id := m.nextID
	m.nextID += 2 // preserve odd/even parity for this side
	if m.nextID < id {
		m.nextID = 0 // exhausted; never reuse a live or retired ID
	}
	s := newStream(id, m, m.bufSize)
	m.streams[id] = s
	return s, nil
}

// Accept returns the next peer-initiated stream, blocking until one arrives.
// It returns ErrClosed once the Mux is closed.
func (m *Mux) Accept() (*Stream, error) {
	select {
	case s := <-m.accept:
		m.mu.Lock()
		closed := m.closed
		m.mu.Unlock()
		if closed {
			return nil, ErrClosed
		}
		return s, nil
	case <-m.done:
		return nil, ErrClosed
	}
}

// Close tears down every stream and closes the underlying connection.
func (m *Mux) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	streams := make([]*Stream, 0, len(m.streams))
	for _, s := range m.streams {
		streams = append(streams, s)
	}
	m.streams = make(map[uint32]*Stream)
	m.retired = retiredIDs{}
	close(m.done)
	m.mu.Unlock()

	for _, s := range streams {
		s.kill()
	}
	return m.conn.CloseNow()
}

// writeFrame serializes a framed write onto the shared connection.
func (m *Mux) writeFrame(typ uint8, id uint32, payload []byte) error {
	m.writeMu.Lock()
	defer m.writeMu.Unlock()
	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()
	if closed {
		return ErrClosed
	}
	if err := m.conn.WriteMessage(websocket.BinaryMessage, pack(typ, id, payload)); err != nil {
		_ = m.Close()
		return ErrClosed
	}
	return nil
}

// removeStream drops a stream from the map without closing its channel (the
// caller handles that), so a later inbound frame for the id is ignored.
func (m *Mux) removeStream(id uint32) {
	m.mu.Lock()
	delete(m.streams, id)
	withinLimit := true
	if !m.closed && m.peerInitiated(id) {
		withinLimit = m.retired.add(id)
	}
	m.mu.Unlock()
	if !withinLimit {
		_ = m.Close()
	}
}

// peerInitiated reports whether an unseen id was opened by the peer, i.e. its
// parity is opposite to the ids this side allocates.
func (m *Mux) peerInitiated(id uint32) bool {
	idOdd := id%2 == 1
	return idOdd != m.clientOdd
}

// readLoop is the sole reader of the connection and deliverer to stream channels.
func (m *Mux) readLoop() {
	for {
		messageType, msg, err := m.conn.ReadMessage()
		if err != nil {
			_ = m.Close()
			return
		}
		if messageType != websocket.BinaryMessage {
			continue
		}
		typ, id, payload, ok := parse(msg)
		if !ok {
			continue // too short to be a valid frame; ignore
		}
		m.dispatch(typ, id, payload)
	}
}

func (m *Mux) dispatch(typ uint8, id uint32, payload []byte) {
	if id == 0 || (typ != frameData && typ != frameClose) {
		return
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	s, known := m.streams[id]
	if !known {
		if m.retired.contains(id) {
			m.mu.Unlock()
			return
		}
		if !m.peerInitiated(id) {
			// A local-parity ID we never opened cannot create a stream.
			m.mu.Unlock()
			return
		}
		if typ == frameClose {
			// The peer may close a lazy stream before sending its first DATA.
			withinLimit := m.retired.add(id)
			m.mu.Unlock()
			if !withinLimit {
				_ = m.Close()
			}
			return
		}
		s = newStream(id, m, m.bufSize)
		m.streams[id] = s
		m.mu.Unlock()
		// Offer to Accept; drop the stream if nobody is accepting.
		select {
		case m.accept <- s:
		case <-m.done:
			return
		default:
			m.removeStream(id)
			s.kill()
			m.queueReset(id)
			return
		}
	} else {
		m.mu.Unlock()
	}

	if typ == frameClose {
		m.removeStream(id)
		s.kill()
		return
	}

	// Copy the payload: it aliases the read buffer, which the next
	// ReadMessage will overwrite.
	buf := make([]byte, len(payload))
	copy(buf, payload)
	if !s.deliver(buf) {
		// Buffer full or stream closed: reset just this stream so one slow
		// consumer never blocks the shared read loop.
		m.removeStream(id)
		s.kill()
		m.queueReset(id)
	}
}

// queueReset never waits for network I/O on the shared read loop. If the peer
// stops reading enough resets to fill the queue, close the tunnel so every
// stream is notified rather than leaving the peers with inconsistent state.
func (m *Mux) queueReset(id uint32) {
	select {
	case <-m.done:
	case m.resets <- id:
	default:
		_ = m.Close()
	}
}

func (m *Mux) resetLoop() {
	for {
		select {
		case <-m.done:
			return
		case id := <-m.resets:
			if err := m.writeFrame(frameClose, id, nil); err != nil {
				return
			}
		}
	}
}
