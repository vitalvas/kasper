package wsmux

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
