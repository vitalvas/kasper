package websocket

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompressDecompress(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{
			name:  "Simple text",
			input: []byte("Hello, WebSocket!"),
		},
		{
			name:  "Repeated text",
			input: bytes.Repeat([]byte("hello"), 100),
		},
		{
			name:  "Binary data",
			input: []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05},
		},
		{
			name:  "Empty",
			input: []byte{},
		},
		{
			name:  "Large text",
			input: bytes.Repeat([]byte("The quick brown fox jumps over the lazy dog. "), 1000),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compressed, err := compressData(tt.input, defaultCompressionLevel)
			require.NoError(t, err)

			decompressed, err := decompressData(compressed)
			require.NoError(t, err)

			assert.Equal(t, tt.input, decompressed)
		})
	}
}

func TestCompressDataReducesSize(t *testing.T) {
	input := bytes.Repeat([]byte("compressible data "), 100)

	compressed, err := compressData(input, defaultCompressionLevel)
	require.NoError(t, err)

	assert.Less(t, len(compressed), len(input))
}

func TestCompressionLevels(t *testing.T) {
	input := bytes.Repeat([]byte("test data for compression "), 50)

	for level := minCompressionLevel; level <= maxCompressionLevel; level++ {
		t.Run("level_"+string(rune('0'+level)), func(t *testing.T) {
			compressed, err := compressData(input, level)
			require.NoError(t, err)

			decompressed, err := decompressData(compressed)
			require.NoError(t, err)

			assert.Equal(t, input, decompressed)
		})
	}
}

func TestCompressedReader(t *testing.T) {
	t.Run("Read compressed data", func(t *testing.T) {
		input := []byte("Hello, compressed world!")
		compressed, err := compressData(input, defaultCompressionLevel)
		require.NoError(t, err)

		cr := newCompressedReader(&byteReader{data: compressed})
		defer cr.Close()

		result, err := io.ReadAll(cr)
		require.NoError(t, err)
		assert.Equal(t, input, result)
	})

	t.Run("Close without read", func(t *testing.T) {
		cr := newCompressedReader(&byteReader{data: []byte{}})
		err := cr.Close()
		require.NoError(t, err)
	})

	t.Run("Double close", func(t *testing.T) {
		input := []byte("test")
		compressed, err := compressData(input, defaultCompressionLevel)
		require.NoError(t, err)

		cr := newCompressedReader(&byteReader{data: compressed})
		_, _ = io.ReadAll(cr)
		err = cr.Close()
		require.NoError(t, err)
		err = cr.Close()
		require.NoError(t, err)
	})
}

func TestCompressedWriter(t *testing.T) {
	t.Run("Write and get bytes", func(t *testing.T) {
		cw := newCompressedWriter(nil, defaultCompressionLevel)

		input := []byte("Hello, compressed world!")
		_, err := cw.Write(input)
		require.NoError(t, err)

		err = cw.Close()
		require.NoError(t, err)

		result := cw.Bytes()
		assert.NotEmpty(t, result)

		decompressed, err := decompressData(result)
		require.NoError(t, err)
		assert.Equal(t, input, decompressed)
	})

	t.Run("Reset clears buffer", func(t *testing.T) {
		cw := newCompressedWriter(nil, defaultCompressionLevel)

		_, _ = cw.Write([]byte("data"))
		_ = cw.Close()

		cw.Reset()
		assert.Empty(t, cw.Bytes())
	})

	t.Run("Multiple writes", func(t *testing.T) {
		cw := newCompressedWriter(nil, defaultCompressionLevel)

		_, err := cw.Write([]byte("Hello, "))
		require.NoError(t, err)
		_, err = cw.Write([]byte("World!"))
		require.NoError(t, err)

		err = cw.Close()
		require.NoError(t, err)

		decompressed, err := decompressData(cw.Bytes())
		require.NoError(t, err)
		assert.Equal(t, []byte("Hello, World!"), decompressed)
	})
}

func TestFlateReaderPool(t *testing.T) {
	t.Run("Reuse reader from pool", func(t *testing.T) {
		input := []byte("test data")
		compressed, err := compressData(input, defaultCompressionLevel)
		require.NoError(t, err)

		for i := 0; i < 3; i++ {
			result, err := decompressData(compressed)
			require.NoError(t, err)
			assert.Equal(t, input, result)
		}
	})
}

func TestFlateWriterPool(t *testing.T) {
	t.Run("Reuse writer from pool", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			buf := new(bytes.Buffer)
			fw := getFlateWriter(buf, defaultCompressionLevel)
			require.NotNil(t, fw)

			_, err := fw.Write([]byte("test"))
			require.NoError(t, err)
			err = fw.Close()
			require.NoError(t, err)

			putFlateWriter(fw)
		}
	})
}

func TestSuffixReader(t *testing.T) {
	sr := suffixReader{}

	t.Run("Read suffix bytes", func(t *testing.T) {
		buf := make([]byte, 10)
		n, err := sr.Read(buf)
		assert.Equal(t, 4, n)
		assert.Equal(t, io.EOF, err)
		assert.Equal(t, []byte{0x00, 0x00, 0xff, 0xff}, buf[:4])
	})

	t.Run("Buffer too small", func(t *testing.T) {
		buf := make([]byte, 2)
		_, err := sr.Read(buf)
		assert.Equal(t, io.ErrShortBuffer, err)
	})
}

func TestFlateReadWrapper(t *testing.T) {
	t.Run("Read after close returns error", func(t *testing.T) {
		w := &flateReadWrapper{fr: nil}
		_, err := w.Read(make([]byte, 10))
		assert.Equal(t, io.ErrClosedPipe, err)
	})

	t.Run("Close nil reader", func(t *testing.T) {
		w := &flateReadWrapper{fr: nil}
		err := w.Close()
		assert.NoError(t, err)
	})

	t.Run("Read and auto close on EOF", func(t *testing.T) {
		input := []byte("test data for flate wrapper")
		compressed, err := compressData(input, defaultCompressionLevel)
		require.NoError(t, err)

		br := &byteReader{data: compressed}
		suffixed := io.MultiReader(br, suffixReader{})
		fr := getFlateReader(suffixed)
		w := &flateReadWrapper{fr: fr}

		result, err := io.ReadAll(w)
		require.NoError(t, err)
		assert.Equal(t, input, result)
		assert.Nil(t, w.fr)
	})

	t.Run("Close with active reader", func(t *testing.T) {
		input := []byte("test")
		compressed, err := compressData(input, defaultCompressionLevel)
		require.NoError(t, err)

		br := &byteReader{data: compressed}
		suffixed := io.MultiReader(br, suffixReader{})
		fr := getFlateReader(suffixed)
		w := &flateReadWrapper{fr: fr}

		buf := make([]byte, 2)
		_, _ = w.Read(buf)

		err = w.Close()
		require.NoError(t, err)
		assert.Nil(t, w.fr)
	})
}

func TestByteReader(t *testing.T) {
	t.Run("Read all data", func(t *testing.T) {
		br := &byteReader{data: []byte("hello")}

		buf := make([]byte, 10)
		n, err := br.Read(buf)
		assert.NoError(t, err)
		assert.Equal(t, 5, n)
		assert.Equal(t, []byte("hello"), buf[:n])

		n, err = br.Read(buf)
		assert.Equal(t, io.EOF, err)
		assert.Equal(t, 0, n)
	})

	t.Run("Partial reads", func(t *testing.T) {
		br := &byteReader{data: []byte("hello")}

		buf := make([]byte, 2)
		n, err := br.Read(buf)
		assert.NoError(t, err)
		assert.Equal(t, 2, n)
		assert.Equal(t, []byte("he"), buf)

		n, err = br.Read(buf)
		assert.NoError(t, err)
		assert.Equal(t, 2, n)
		assert.Equal(t, []byte("ll"), buf)

		n, err = br.Read(buf)
		assert.NoError(t, err)
		assert.Equal(t, 1, n)
		assert.Equal(t, byte('o'), buf[0])
	})
}

func TestCompressedWriterClose(t *testing.T) {
	t.Run("Close without write", func(t *testing.T) {
		cw := newCompressedWriter(nil, defaultCompressionLevel)
		err := cw.Close()
		require.NoError(t, err)
	})

	t.Run("Close twice", func(t *testing.T) {
		cw := newCompressedWriter(nil, defaultCompressionLevel)
		_, _ = cw.Write([]byte("test"))
		err := cw.Close()
		require.NoError(t, err)

		err = cw.Close()
		require.NoError(t, err)
	})
}

func BenchmarkCompression(b *testing.B) {
	sizes := []struct {
		name string
		data []byte
	}{
		{"Compressible", bytes.Repeat([]byte("compressible data pattern "), 100)},
		{"Random", func() []byte {
			d := make([]byte, 2500)
			for i := range d {
				d[i] = byte((i * 17) % 256)
			}
			return d
		}()},
	}

	for _, size := range sizes {
		b.Run("Compress_"+size.name, func(b *testing.B) {
			b.SetBytes(int64(len(size.data)))

			for b.Loop() {
				_, _ = compressData(size.data, defaultCompressionLevel)
			}
		})

		compressed, _ := compressData(size.data, defaultCompressionLevel)

		b.Run("Decompress_"+size.name, func(b *testing.B) {
			b.SetBytes(int64(len(compressed)))

			for b.Loop() {
				_, _ = decompressData(compressed)
			}
		})
	}
}

func FuzzCompressDecompress(f *testing.F) {
	f.Add([]byte("hello world"))
	f.Add([]byte(""))
	f.Add(bytes.Repeat([]byte("a"), 1000))
	f.Add([]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05})

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 100000 {
			data = data[:100000]
		}

		compressed, err := compressData(data, defaultCompressionLevel)
		if err != nil {
			return
		}

		decompressed, err := decompressData(compressed)
		if err != nil {
			t.Errorf("decompression failed: %v", err)
			return
		}

		if !bytes.Equal(data, decompressed) {
			t.Errorf("data mismatch after compress/decompress cycle")
		}
	})
}
