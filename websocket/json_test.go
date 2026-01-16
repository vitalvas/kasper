package websocket

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testMessage struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

func TestJSONReadWrite(t *testing.T) {
	upgrader := &Upgrader{
		CheckOrigin: func(_ *http.Request) bool { return true },
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		var msg testMessage
		if err := conn.ReadJSON(&msg); err != nil {
			return
		}

		msg.Value *= 2
		_ = conn.WriteJSON(msg)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	t.Run("Write and read JSON", func(t *testing.T) {
		d := &Dialer{}
		conn, _, err := d.Dial(wsURL, nil)
		require.NoError(t, err)
		defer conn.Close()

		sent := testMessage{Name: "test", Value: 21}
		err = conn.WriteJSON(sent)
		require.NoError(t, err)

		var received testMessage
		err = conn.ReadJSON(&received)
		require.NoError(t, err)

		assert.Equal(t, "test", received.Name)
		assert.Equal(t, 42, received.Value)
	})

	t.Run("Write complex object", func(t *testing.T) {
		d := &Dialer{}
		conn, _, err := d.Dial(wsURL, nil)
		require.NoError(t, err)
		defer conn.Close()

		type complexMessage struct {
			ID      int      `json:"id"`
			Tags    []string `json:"tags"`
			Nested  testMessage
			Enabled bool `json:"enabled"`
		}

		err = conn.WriteJSON(complexMessage{
			ID:      1,
			Tags:    []string{"a", "b", "c"},
			Nested:  testMessage{Name: "nested", Value: 10},
			Enabled: true,
		})
		require.NoError(t, err)
	})
}

func TestReadJSONErrors(t *testing.T) {
	tests := []struct {
		name        string
		serverData  string
		description string
	}{
		{"Invalid JSON", "not valid json", "server sends invalid JSON"},
		{"Empty message", "", "server sends empty message"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverData := tt.serverData
			upgrader := &Upgrader{
				CheckOrigin: func(_ *http.Request) bool { return true },
			}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer conn.Close()
				_ = conn.WriteMessage(TextMessage, []byte(serverData))
			}))
			defer server.Close()

			wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

			d := &Dialer{}
			conn, _, err := d.Dial(wsURL, nil)
			require.NoError(t, err)
			defer conn.Close()

			var msg testMessage
			err = conn.ReadJSON(&msg)
			require.Error(t, err)
		})
	}
}

func TestWriteJSONError(t *testing.T) {
	upgrader := &Upgrader{
		CheckOrigin: func(_ *http.Request) bool { return true },
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn.Close()
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	t.Run("Write to closed connection", func(t *testing.T) {
		d := &Dialer{}
		conn, _, err := d.Dial(wsURL, nil)
		require.NoError(t, err)

		conn.Close()

		err = conn.WriteJSON(testMessage{Name: "test", Value: 1})
		require.Error(t, err)
	})
}

func TestJSONWithArrays(t *testing.T) {
	upgrader := &Upgrader{
		CheckOrigin: func(_ *http.Request) bool { return true },
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		var msgs []testMessage
		if err := conn.ReadJSON(&msgs); err != nil {
			return
		}

		_ = conn.WriteJSON(msgs)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	t.Run("Array of objects", func(t *testing.T) {
		d := &Dialer{}
		conn, _, err := d.Dial(wsURL, nil)
		require.NoError(t, err)
		defer conn.Close()

		sent := []testMessage{
			{Name: "first", Value: 1},
			{Name: "second", Value: 2},
		}
		err = conn.WriteJSON(sent)
		require.NoError(t, err)

		var received []testMessage
		err = conn.ReadJSON(&received)
		require.NoError(t, err)

		assert.Len(t, received, 2)
		assert.Equal(t, sent, received)
	})
}

func TestJSONWithMap(t *testing.T) {
	upgrader := &Upgrader{
		CheckOrigin: func(_ *http.Request) bool { return true },
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		var msg map[string]any
		if err := conn.ReadJSON(&msg); err != nil {
			return
		}

		_ = conn.WriteJSON(msg)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	t.Run("Map object", func(t *testing.T) {
		d := &Dialer{}
		conn, _, err := d.Dial(wsURL, nil)
		require.NoError(t, err)
		defer conn.Close()

		sent := map[string]any{
			"key1": "value1",
			"key2": float64(123),
		}
		err = conn.WriteJSON(sent)
		require.NoError(t, err)

		var received map[string]any
		err = conn.ReadJSON(&received)
		require.NoError(t, err)

		assert.Equal(t, sent, received)
	})
}

func TestWriteJSONEncodingError(t *testing.T) {
	upgrader := &Upgrader{
		CheckOrigin: func(_ *http.Request) bool { return true },
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		_, _, _ = conn.ReadMessage()
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	t.Run("Unencodable value", func(t *testing.T) {
		d := &Dialer{}
		conn, _, err := d.Dial(wsURL, nil)
		require.NoError(t, err)
		defer conn.Close()

		err = conn.WriteJSON(make(chan int))
		require.Error(t, err)
	})
}

func BenchmarkJSON(b *testing.B) {
	type benchStruct struct {
		ID      int      `json:"id"`
		Name    string   `json:"name"`
		Tags    []string `json:"tags"`
		Enabled bool     `json:"enabled"`
	}

	upgrader := &Upgrader{
		CheckOrigin: func(_ *http.Request) bool { return true },
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		for {
			msgType, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if err := conn.WriteMessage(msgType, msg); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	b.Run("WriteJSON", func(b *testing.B) {
		d := &Dialer{}
		conn, _, err := d.Dial(wsURL, nil)
		if err != nil {
			b.Fatal(err)
		}
		defer conn.Close()

		msg := benchStruct{
			ID:      123,
			Name:    "benchmark",
			Tags:    []string{"test", "bench", "json"},
			Enabled: true,
		}

		b.ResetTimer()

		for b.Loop() {
			_ = conn.WriteJSON(msg)
			_, _, _ = conn.ReadMessage()
		}
	})

	b.Run("ReadJSON", func(b *testing.B) {
		d := &Dialer{}
		conn, _, err := d.Dial(wsURL, nil)
		if err != nil {
			b.Fatal(err)
		}
		defer conn.Close()

		msg := benchStruct{
			ID:      123,
			Name:    "benchmark",
			Tags:    []string{"test", "bench", "json"},
			Enabled: true,
		}

		b.ResetTimer()

		for b.Loop() {
			_ = conn.WriteJSON(msg)
			var received benchStruct
			_ = conn.ReadJSON(&received)
		}
	})
}
