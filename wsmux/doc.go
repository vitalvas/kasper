// Package wsmux multiplexes many logical streams over a single WebSocket
// connection from the websocket package.
//
// Each message on the wire is a binary frame with a 5-byte header
// ([1-byte type][4-byte big-endian stream id]) followed by the payload. No
// length field is carried: the underlying connection frames every message and
// reports its length, so the payload is everything after the header. Stream
// ids are allocated with fixed parity per side: the client (Open-initiating)
// side uses odd ids, the server side even, so the two ends never collide.
//
// Concurrency:
//
// A single internal goroutine owns all reads and is the sole sender to, and
// closer of, every stream's receive channel. Stream teardown therefore never
// races a delivery. Delivery is non-blocking: a stream whose buffer is full is
// reset rather than allowed to stall the shared read loop, so one slow
// consumer cannot head-of-line block the other streams.
//
// Server example:
//
//	conn, err := upgrader.Upgrade(w, r, nil)
//	if err != nil {
//	    return
//	}
//	mux := wsmux.NewServer(conn, wsmux.Options{})
//	defer mux.Close()
//
//	for {
//	    stream, err := mux.Accept()
//	    if err != nil {
//	        return
//	    }
//	    go handle(stream)
//	}
//
// Client example:
//
//	mux := wsmux.NewClient(conn, wsmux.Options{})
//	defer mux.Close()
//
//	stream, _ := mux.Open()
//	_ = stream.Write([]byte("ping"))
//	resp, _ := stream.Read()
//
// This is single-connection multiplexing: all streams share one TCP pipe and
// its ordering. It suits logically independent request/response streams, not
// cases that need the isolation of separate connections.
package wsmux
