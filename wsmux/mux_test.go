package wsmux

import (
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.vitalvas.com/kasper/websocket"
)

// pipeConn is an in-memory messageConn. Messages written on one end are read
// on the other. Closing either end makes both ends' reads fail.
type pipeConn struct {
	out chan []byte // this end writes here
	in  chan []byte // this end reads here

	once sync.Once
	done chan struct{} // this end's close signal
	peer chan struct{} // the other end's close signal
}

func newPipe() (*pipeConn, *pipeConn) {
	a2b := make(chan []byte, 64)
	b2a := make(chan []byte, 64)
	aDone := make(chan struct{})
	bDone := make(chan struct{})
	a := &pipeConn{out: a2b, in: b2a, done: aDone, peer: bDone}
	b := &pipeConn{out: b2a, in: a2b, done: bDone, peer: aDone}
	return a, b
}

func (p *pipeConn) ReadMessage() (int, []byte, error) {
	select {
	case msg := <-p.in:
		return 2, msg, nil // 2 == websocket.BinaryMessage
	case <-p.done:
		return 0, nil, errors.New("pipe closed")
	case <-p.peer:
		return 0, nil, errors.New("pipe closed")
	}
}

func (p *pipeConn) WriteMessage(_ int, data []byte) error {
	// Copy: callers may reuse the slice.
	buf := make([]byte, len(data))
	copy(buf, data)
	select {
	case p.out <- buf:
		return nil
	case <-p.done:
		return errors.New("pipe closed")
	case <-p.peer:
		return errors.New("pipe closed")
	}
}

func (p *pipeConn) CloseNow() error {
	p.once.Do(func() { close(p.done) })
	return nil
}

// newMuxPair wires two muxes over an in-memory pipe.
func newMuxPair(t *testing.T) (client, server *Mux) {
	t.Helper()
	ca, cb := newPipe()
	client = newMux(ca, Options{}, true)
	server = newMux(cb, Options{}, false)
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})
	return client, server
}

func TestOpenWriteAcceptRead(t *testing.T) {
	client, server := newMuxPair(t)

	cs, err := client.Open()
	require.NoError(t, err)
	assert.Equal(t, uint32(1), cs.ID()) // client ids are odd

	require.NoError(t, cs.Write([]byte("ping")))

	ss, err := server.Accept()
	require.NoError(t, err)
	assert.Equal(t, uint32(1), ss.ID())

	got, err := ss.Read()
	require.NoError(t, err)
	assert.Equal(t, []byte("ping"), got)

	// Echo back on the same stream.
	require.NoError(t, ss.Write([]byte("pong")))
	resp, err := cs.Read()
	require.NoError(t, err)
	assert.Equal(t, []byte("pong"), resp)
}

func TestStreamIsolation(t *testing.T) {
	client, server := newMuxPair(t)

	s1, _ := client.Open()
	s2, _ := client.Open()
	assert.Equal(t, uint32(1), s1.ID())
	assert.Equal(t, uint32(3), s2.ID())

	require.NoError(t, s1.Write([]byte("one")))
	require.NoError(t, s2.Write([]byte("two")))

	a, err := server.Accept()
	require.NoError(t, err)
	b, err := server.Accept()
	require.NoError(t, err)

	// Accept order follows first-DATA order: s1 then s2.
	byID := map[uint32]*Stream{a.ID(): a, b.ID(): b}
	d1, err := byID[1].Read()
	require.NoError(t, err)
	assert.Equal(t, []byte("one"), d1)
	d2, err := byID[3].Read()
	require.NoError(t, err)
	assert.Equal(t, []byte("two"), d2)
}

func TestCloseSendsEOF(t *testing.T) {
	client, server := newMuxPair(t)

	cs, _ := client.Open()
	require.NoError(t, cs.Write([]byte("x")))

	ss, err := server.Accept()
	require.NoError(t, err)
	_, err = ss.Read()
	require.NoError(t, err)

	require.NoError(t, cs.Close())

	// The peer's Read drains then returns io.EOF after the CLOSE frame.
	_, err = ss.Read()
	assert.Error(t, err)
}

func TestWriteAfterStreamCloseFails(t *testing.T) {
	client, _ := newMuxPair(t)
	cs, _ := client.Open()
	require.NoError(t, cs.Close())
	assert.Error(t, cs.Write([]byte("x")))
}

func TestMuxCloseUnblocksAccept(t *testing.T) {
	_, server := newMuxPair(t)

	errc := make(chan error, 1)
	go func() {
		_, err := server.Accept()
		errc <- err
	}()

	// Give Accept a moment to block, then close.
	require.NoError(t, server.Close())

	select {
	case err := <-errc:
		assert.ErrorIs(t, err, ErrClosed)
	case <-time.After(time.Second):
		t.Fatal("Accept did not unblock on Close")
	}
}

func TestConnDropClosesStreams(t *testing.T) {
	client, server := newMuxPair(t)

	cs, _ := client.Open()
	require.NoError(t, cs.Write([]byte("x")))
	ss, err := server.Accept()
	require.NoError(t, err)
	_, err = ss.Read()
	require.NoError(t, err)

	// Drop the client's connection; the server's read loop should tear down.
	require.NoError(t, client.Close())

	_, err = ss.Read()
	assert.Error(t, err) // EOF once the server observes the dead tunnel
}

func TestOpenAfterCloseFails(t *testing.T) {
	client, _ := newMuxPair(t)
	require.NoError(t, client.Close())
	_, err := client.Open()
	assert.ErrorIs(t, err, ErrClosed)
}

func newTestMux(t *testing.T, opts Options) (*Mux, *pipeConn) {
	t.Helper()
	local, peer := newPipe()
	m := newMux(local, opts, false)
	t.Cleanup(func() { _ = m.Close() })
	return m, peer
}

func TestClosedStreamIsNotRecreated(t *testing.T) {
	for _, localClose := range []bool{false, true} {
		name := "peer close"
		if localClose {
			name = "local close"
		}
		t.Run(name, func(t *testing.T) {
			m, _ := newTestMux(t, Options{})
			m.dispatch(frameData, 1, []byte("first"))
			s, err := m.Accept()
			require.NoError(t, err)
			if localClose {
				require.NoError(t, s.Close())
			} else {
				m.dispatch(frameClose, 1, nil)
			}
			m.dispatch(frameData, 1, []byte("late"))
			select {
			case s := <-m.accept:
				t.Fatalf("closed stream %d was offered to Accept again", s.ID())
			default:
			}
		})
	}
}

func TestPeerClosesBeforeFirstData(t *testing.T) {
	m, _ := newTestMux(t, Options{})
	m.dispatch(frameClose, 1, nil)
	m.dispatch(frameData, 1, []byte("late"))
	assert.Empty(t, m.accept)
}

func TestPeerStreamsCanStartOutOfOrder(t *testing.T) {
	m, _ := newTestMux(t, Options{})
	m.dispatch(frameData, 3, []byte("first"))
	m.dispatch(frameClose, 3, nil)
	m.dispatch(frameData, 1, []byte("second"))
	first, err := m.Accept()
	require.NoError(t, err)
	assert.Equal(t, uint32(3), first.ID())
	second, err := m.Accept()
	require.NoError(t, err)
	assert.Equal(t, uint32(1), second.ID())
	data, err := second.Read()
	require.NoError(t, err)
	assert.Equal(t, []byte("second"), data)
}

func TestClosedStreamHistoryCompacts(t *testing.T) {
	m, _ := newTestMux(t, Options{StreamBuffer: 1})
	for id := uint32(1); id < 20000; id += 2 {
		m.dispatch(frameData, id, nil)
		s, err := m.Accept()
		require.NoError(t, err)
		_, err = s.Read()
		require.NoError(t, err)
		m.dispatch(frameClose, id, nil)
	}
	assert.Empty(t, m.streams)
	require.Len(t, m.retired.ranges, 1, "history must not grow with sequential stream churn")
	assert.Equal(t, streamIDRange{first: 1, last: 19999}, m.retired.ranges[0])
	// Even the oldest closed ID must still be rejected after heavy churn.
	m.dispatch(frameData, 1, []byte("late"))
	assert.Empty(t, m.accept)
	select {
	case <-m.done:
		t.Fatal("sequential stream churn closed the mux")
	default:
	}
}

func TestRetiredHistoryOverflowClosesMux(t *testing.T) {
	for _, known := range []bool{false, true} {
		name := "close before data"
		if known {
			name = "close active stream"
		}
		t.Run(name, func(t *testing.T) {
			m, peer := newTestMux(t, Options{StreamBuffer: 1})
			for i := uint32(0); i <= maxRetiredRanges; i++ {
				id := 4*i + 1 // leave a gap between closed peer IDs
				if known {
					m.dispatch(frameData, id, nil)
					s, err := m.Accept()
					require.NoError(t, err)
					_, err = s.Read()
					require.NoError(t, err)
				}
				m.dispatch(frameClose, id, nil)
			}
			select {
			case <-peer.peer:
			case <-time.After(time.Second):
				t.Fatal("fragmented stream history did not close the transport")
			}
			_, err := m.Open()
			require.ErrorIs(t, err, ErrClosed)
			assert.Empty(t, m.retired.ranges, "shutdown must release the history")
			m.dispatch(frameData, 1, []byte("late"))
			assert.Empty(t, m.accept)
		})
	}
}

func TestAcceptAfterMuxClose(t *testing.T) {
	m, _ := newTestMux(t, Options{})
	m.dispatch(frameData, 1, []byte("queued"))
	require.NoError(t, m.Close())
	for range 20 {
		s, err := m.Accept()
		require.ErrorIs(t, err, ErrClosed)
		assert.Nil(t, s)
	}
}

func TestAcceptBacklogRejectsPeerStream(t *testing.T) {
	m, peer := newTestMux(t, Options{})
	for i := uint32(1); i <= 33; i += 2 {
		m.dispatch(frameData, i, nil)
	}
	select {
	case msg := <-peer.in:
		typ, id, _, ok := parse(msg)
		require.True(t, ok)
		assert.Equal(t, frameClose, typ)
		assert.Equal(t, uint32(33), id)
	case <-time.After(time.Second):
		t.Fatal("peer was not notified that the accept backlog rejected its stream")
	}
}

type stalledWriteConn struct {
	*pipeConn
	once    sync.Once
	started chan struct{}
}

func (c *stalledWriteConn) WriteMessage(_ int, _ []byte) error {
	c.once.Do(func() { close(c.started) })
	<-c.done
	return io.ErrClosedPipe
}

func TestStreamOverflowDoesNotBlockDispatch(t *testing.T) {
	local, _ := newPipe()
	conn := &stalledWriteConn{pipeConn: local, started: make(chan struct{})}
	m := newMux(conn, Options{StreamBuffer: 1}, false)
	t.Cleanup(func() { _ = m.Close() })
	m.dispatch(frameData, 1, []byte("fills buffer"))
	returned := make(chan struct{})
	go func() {
		m.dispatch(frameData, 1, []byte("overflow"))
		close(returned)
	}()
	<-conn.started
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("overflow reset blocked the shared read loop")
	}
	m.dispatch(frameData, 3, []byte("other stream"))
	_, err := m.Accept()
	require.NoError(t, err)
	s, err := m.Accept()
	require.NoError(t, err)
	data, err := s.Read()
	require.NoError(t, err)
	assert.Equal(t, []byte("other stream"), data)
}

func TestResetQueueOverflowClosesTunnel(t *testing.T) {
	local, _ := newPipe()
	conn := &stalledWriteConn{pipeConn: local, started: make(chan struct{})}
	m := newMux(conn, Options{}, false)
	t.Cleanup(func() { _ = m.Close() })
	// Fill the accept backlog, then reject enough streams to exhaust the
	// reset queue while the reset writer is blocked on the peer.
	for i := uint32(1); i <= 69; i += 2 {
		m.dispatch(frameData, i, nil)
	}
	select {
	case <-local.done:
	case <-time.After(time.Second):
		t.Fatal("a full reset queue did not close the transport")
	}
}

type failingMessageConn struct{ *pipeConn }

func (c *failingMessageConn) WriteMessage(_ int, _ []byte) error {
	return errors.New("write failed")
}

func TestWriteFailureClosesMux(t *testing.T) {
	local, _ := newPipe()
	m := newMux(&failingMessageConn{local}, Options{}, true)
	t.Cleanup(func() { _ = m.Close() })
	s, err := m.Open()
	require.NoError(t, err)
	require.ErrorIs(t, s.Write([]byte("fails")), ErrClosed)
	select {
	case <-m.done:
	case <-time.After(time.Second):
		t.Fatal("write failure left the mux open")
	}
	_, err = s.Read()
	require.ErrorIs(t, err, io.EOF)
}

func TestReadFailureClosesTransport(t *testing.T) {
	local, peer := newPipe()
	m := newMux(local, Options{}, true)
	t.Cleanup(func() { _ = m.Close() })
	require.NoError(t, peer.CloseNow())
	select {
	case <-local.done:
	case <-time.After(time.Second):
		t.Fatal("read failure leaked the underlying connection")
	}
}

func TestStreamCloseIsIdempotent(t *testing.T) {
	m, peer := newTestMux(t, Options{})
	s, err := m.Open()
	require.NoError(t, err)
	require.NoError(t, s.Close())
	require.NoError(t, s.Close())
	assert.Len(t, peer.in, 1, "a repeated Close must not send another frame")
}

func TestStreamIDsDoNotWrap(t *testing.T) {
	m, _ := newTestMux(t, Options{})
	m.nextID = math.MaxUint32 - 1
	s, err := m.Open()
	require.NoError(t, err)
	assert.Equal(t, uint32(math.MaxUint32-1), s.ID())
	s, err = m.Open()
	require.Error(t, err)
	assert.Nil(t, s)
}

func TestInvalidFramesAreIgnored(t *testing.T) {
	m, _ := newTestMux(t, Options{})
	m.dispatch(frameData, 1, []byte("valid"))
	s, err := m.Accept()
	require.NoError(t, err)
	_, err = s.Read()
	require.NoError(t, err)
	m.dispatch(99, 1, []byte("invalid type"))
	assert.Empty(t, s.recv)
	m.dispatch(frameData, 0, nil)
	assert.Empty(t, m.accept)
}

func TestWebSocketBidirectionalStreams(t *testing.T) {
	ready := make(chan *Mux, 1)
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		ready <- NewServer(conn, Options{})
	}))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial(fmt.Sprintf("ws%s", strings.TrimPrefix(server.URL, "http")), nil)
	require.NoError(t, err)
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	client := NewClient(conn, Options{})
	defer client.Close()
	peer := <-ready
	defer peer.Close()

	cs, err := client.Open()
	require.NoError(t, err)
	ss, err := peer.Open()
	require.NoError(t, err)
	require.NoError(t, cs.Write([]byte("from client")))
	require.NoError(t, ss.Write([]byte("from server")))

	for _, tc := range []struct {
		mux     *Mux
		stream  *Stream
		payload string
	}{{peer, cs, "from client"}, {client, ss, "from server"}} {
		accepted, err := tc.mux.Accept()
		require.NoError(t, err)
		assert.Equal(t, tc.stream.ID(), accepted.ID())
		payload, err := accepted.Read()
		require.NoError(t, err)
		assert.Equal(t, tc.payload, string(payload))
		require.NoError(t, tc.stream.Close())
		_, err = accepted.Read()
		require.ErrorIs(t, err, io.EOF)
	}
}
