package securecookie

import (
	"bytes"
	"compress/flate"
	"fmt"
	"io"
	"math"
	"sync"
)

// entropyThreshold is the Shannon entropy (bits/byte) above which
// compression is skipped. Data above this threshold is unlikely to
// compress well and attempting deflate wastes CPU.
// 6.5 bits/byte ~ base64-encoded random data.
const entropyThreshold = 6.5

// flateWriterPool reuses flate.Writer instances to avoid allocating the
// ~800 KB internal dictionary and hash tables on every compression call.
var flateWriterPool = sync.Pool{
	New: func() any {
		w, _ := flate.NewWriter(nil, flate.BestCompression)
		return w
	},
}

// newFlateWriter creates or reuses a deflate writer. Tests may override it.
var newFlateWriter = func(w io.Writer) (io.WriteCloser, error) {
	fw := flateWriterPool.Get().(*flate.Writer)
	fw.Reset(w)

	return fw, nil
}

// putFlateWriter returns a flate writer to the pool.
func putFlateWriter(w io.WriteCloser) {
	if fw, ok := w.(*flate.Writer); ok {
		flateWriterPool.Put(fw)
	}
}

func rawPayload(data []byte) []byte {
	raw := make([]byte, 0, 1+len(data))
	raw = append(raw, prefixRaw)
	raw = append(raw, data...)

	return raw
}

// maybeCompress tries deflate compression on data. Returns a prefixed result:
// prefixDeflated + compressed bytes if compression saves space, or
// prefixRaw + original bytes otherwise.
//
// Compression is skipped entirely when the input is shorter than 32 bytes
// or its Shannon entropy exceeds [entropyThreshold], avoiding wasted CPU
// on tiny or high-entropy data.
func maybeCompress(data []byte) []byte {
	if len(data) < 32 || shannonEntropy(data) > entropyThreshold {
		return rawPayload(data)
	}

	var buf bytes.Buffer
	buf.Grow(1 + len(data))
	buf.WriteByte(prefixDeflated)

	w, err := newFlateWriter(&buf)
	if err != nil {
		return rawPayload(data)
	}

	if _, err := w.Write(data); err != nil {
		putFlateWriter(w)
		return rawPayload(data)
	}

	w.Close()
	putFlateWriter(w)

	// Use compressed only if strictly smaller than raw + prefix.
	if buf.Len() < len(data)+1 {
		return buf.Bytes()
	}

	return rawPayload(data)
}

// maybeDecompress reverses maybeCompress based on the prefix byte.
// Decompressed output is limited to [maxDecompressSize] to prevent
// zip-bomb attacks.
func maybeDecompress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	switch data[0] {
	case prefixRaw:
		return data[1:], nil
	case prefixDeflated:
		r := flate.NewReader(bytes.NewReader(data[1:]))
		defer r.Close()

		lr := io.LimitReader(r, maxDecompressSize+1)

		result, err := io.ReadAll(lr)
		if err != nil {
			return nil, err
		}

		if len(result) > maxDecompressSize {
			return nil, fmt.Errorf("decompressed payload exceeds %d bytes", maxDecompressSize)
		}

		return result, nil
	default:
		// No recognized prefix — legacy uncompressed data from before
		// compression was added. Return as-is for backward compatibility.
		// GCM authentication guarantees integrity, so this is safe.
		return data, nil
	}
}

// shannonEntropy computes Shannon entropy in bits per byte.
// Returns 0.0 for empty input, up to 8.0 for uniformly random bytes.
func shannonEntropy(data []byte) float64 {
	if len(data) == 0 {
		return 0
	}

	var freq [256]int
	for _, b := range data {
		freq[b]++
	}

	n := float64(len(data))
	var h float64

	for _, count := range freq {
		if count == 0 {
			continue
		}

		p := float64(count) / n
		h -= p * math.Log2(p)
	}

	return h
}
