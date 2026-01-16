// Compression support for WebSocket permessage-deflate extension (RFC 7692).
// This extension uses the DEFLATE algorithm (RFC 1951) to compress message payloads.
package websocket

import (
	"compress/flate"
	"io"
	"sync"
)

// Compression level constants for DEFLATE (RFC 1951).
const (
	minCompressionLevel     = -2
	maxCompressionLevel     = 9
	defaultCompressionLevel = 1
)

var (
	flateReaderPool sync.Pool
	flateWriterPool sync.Pool
)

type flateReadWrapper struct {
	fr io.ReadCloser
}

func (r *flateReadWrapper) Read(p []byte) (int, error) {
	if r.fr == nil {
		return 0, io.ErrClosedPipe
	}
	n, err := r.fr.Read(p)
	if err == io.EOF {
		r.fr.Close()
		r.fr = nil
	}
	return n, err
}

func (r *flateReadWrapper) Close() error {
	if r.fr != nil {
		err := r.fr.Close()
		r.fr = nil
		return err
	}
	return nil
}

func getFlateReader(r io.Reader) io.ReadCloser {
	fr, ok := flateReaderPool.Get().(io.ReadCloser)
	if ok && fr != nil {
		if resetter, ok := fr.(flate.Resetter); ok {
			_ = resetter.Reset(r, nil)
			return fr
		}
	}
	return flate.NewReader(r)
}

func putFlateReader(fr io.ReadCloser) {
	flateReaderPool.Put(fr)
}

func getFlateWriter(w io.Writer, level int) *flate.Writer {
	fw, ok := flateWriterPool.Get().(*flate.Writer)
	if ok && fw != nil {
		fw.Reset(w)
		return fw
	}
	fw, _ = flate.NewWriter(w, level)
	return fw
}

func putFlateWriter(fw *flate.Writer) {
	flateWriterPool.Put(fw)
}

type compressedReader struct {
	fr io.ReadCloser
	r  io.Reader
}

func newCompressedReader(r io.Reader) *compressedReader {
	return &compressedReader{
		r: io.MultiReader(r, suffixReader{}),
	}
}

func (cr *compressedReader) Read(p []byte) (int, error) {
	if cr.fr == nil {
		cr.fr = getFlateReader(cr.r)
	}
	return cr.fr.Read(p)
}

func (cr *compressedReader) Close() error {
	if cr.fr != nil {
		putFlateReader(cr.fr)
		cr.fr = nil
	}
	return nil
}

// suffixReader appends the DEFLATE empty block suffix (0x00 0x00 0xff 0xff)
// required by RFC 7692, section 7.2.2 for decompression.
type suffixReader struct{}

func (suffixReader) Read(p []byte) (int, error) {
	if len(p) < 4 {
		return 0, io.ErrShortBuffer
	}
	// Append 0x00 0x00 0xff 0xff per RFC 7692, section 7.2.2.
	p[0] = 0x00
	p[1] = 0x00
	p[2] = 0xff
	p[3] = 0xff
	return 4, io.EOF
}

type compressedWriter struct {
	c     *Conn
	fw    *flate.Writer
	buf   []byte
	level int
}

func newCompressedWriter(c *Conn, level int) *compressedWriter {
	return &compressedWriter{
		c:     c,
		level: level,
	}
}

func (cw *compressedWriter) Write(p []byte) (int, error) {
	if cw.fw == nil {
		cw.fw = getFlateWriter(&bufferWriter{cw: cw}, cw.level)
	}
	return cw.fw.Write(p)
}

func (cw *compressedWriter) Close() error {
	if cw.fw != nil {
		if err := cw.fw.Close(); err != nil {
			return err
		}
		putFlateWriter(cw.fw)
		cw.fw = nil
	}

	// Remove trailing 0x00 0x00 0xff 0xff per RFC 7692, section 7.2.1.
	// The sender must remove these bytes; the receiver must append them.
	if len(cw.buf) >= 4 {
		cw.buf = cw.buf[:len(cw.buf)-4]
	}

	return nil
}

func (cw *compressedWriter) Bytes() []byte {
	return cw.buf
}

func (cw *compressedWriter) Reset() {
	cw.buf = cw.buf[:0]
}

type bufferWriter struct {
	cw *compressedWriter
}

func (bw *bufferWriter) Write(p []byte) (int, error) {
	bw.cw.buf = append(bw.cw.buf, p...)
	return len(p), nil
}

func compressData(data []byte, level int) ([]byte, error) {
	cw := newCompressedWriter(nil, level)
	if _, err := cw.Write(data); err != nil {
		return nil, err
	}
	if err := cw.Close(); err != nil {
		return nil, err
	}
	result := make([]byte, len(cw.Bytes()))
	copy(result, cw.Bytes())
	return result, nil
}

func decompressData(data []byte) ([]byte, error) {
	cr := newCompressedReader(&byteReader{data: data})
	defer cr.Close()
	return io.ReadAll(cr)
}

type byteReader struct {
	data []byte
	pos  int
}

func (br *byteReader) Read(p []byte) (int, error) {
	if br.pos >= len(br.data) {
		return 0, io.EOF
	}
	n := copy(p, br.data[br.pos:])
	br.pos += n
	return n, nil
}
