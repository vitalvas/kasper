package e2ee

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/kasper/mux"
)

// e2eFixture wires a server (key set + middleware + echo handler) and a
// matching client transport so tests can exercise the full protocol.
type e2eFixture struct {
	server     *httptest.Server
	clientKeys *ClientKeySet
	replay     *MemoryReplayCache
}

func newFixture(t *testing.T, handler http.HandlerFunc) *e2eFixture {
	t.Helper()

	key, err := GenerateServerKey("2026-06", time.Hour, DefaultMaxSkew, AEADAES256GCM, AEADAES128GCM)
	require.NoError(t, err)

	replay := NewMemoryReplayCache()

	router := mux.NewRouter()

	// Build the key set against the eventual server URL; httptest assigns the
	// URL only after Start, so build the set lazily inside Start below.
	srv := httptest.NewUnstartedServer(router)
	srv.Start()
	t.Cleanup(srv.Close)

	set, err := NewServerKeySet(srv.URL, key)
	require.NoError(t, err)

	mw, err := Middleware(MiddlewareConfig{
		Server: ServerConfig{KeySet: set, Replay: replay},
	})
	require.NoError(t, err)

	router.Handle(WellKnownPath, set.Handler())

	api := router.PathPrefix("/api").Subrouter()
	api.Use(mw)
	api.HandleFunc("/echo", handler)

	clientKeys, err := FetchKeySet(srv.Client(), srv.URL)
	require.NoError(t, err)

	return &e2eFixture{server: srv, clientKeys: clientKeys, replay: replay}
}

func echoHandler(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	w.Header().Set("X-Echoed", "yes")
	_, _ = fmt.Fprintf(w, "echo:%s", body)
}

func (f *e2eFixture) client(cfg ClientConfig) *http.Client {
	cfg.KeySet = f.clientKeys

	return &http.Client{
		Transport: NewTransport(f.server.Client().Transport, cfg),
	}
}

// echoURL returns the fixture's /api/echo endpoint URL.
func (f *e2eFixture) echoURL() string {
	return fmt.Sprintf("%s/api/echo", f.server.URL)
}

func TestRoundTripPlaintextRecovered(t *testing.T) {
	f := newFixture(t, echoHandler)

	client := f.client(ClientConfig{ContentType: "application/json"})

	resp, err := client.Post(f.echoURL(), "application/json", strings.NewReader("hello"))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "echo:hello", string(body))

	// Handler-set headers propagate; E2EE-Session is stripped after decrypt.
	assert.Equal(t, "yes", resp.Header.Get("X-Echoed"))
	assert.Empty(t, resp.Header.Get(SessionHeader))
}

func TestRoundTripWithAES128(t *testing.T) {
	f := newFixture(t, echoHandler)

	client := f.client(ClientConfig{AEAD: AEADAES128GCM})

	resp, err := client.Post(f.echoURL(), "application/json", strings.NewReader("data"))
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "echo:data", string(body))
}

func TestServerSeesPlaintext(t *testing.T) {
	var seen string

	f := newFixture(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		seen = string(b)
		// The decrypted inner content type is restored from cty.
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.Write([]byte("ok"))
	})

	client := f.client(ClientConfig{ContentType: "application/json"})

	resp, err := client.Post(f.echoURL(), "application/json", strings.NewReader("secret-payload"))
	require.NoError(t, err)
	resp.Body.Close()

	assert.Equal(t, "secret-payload", seen)
}

func TestIntermediaryCannotReadBody(t *testing.T) {
	// Capture the on-wire request body by inserting a sniffing transport
	// between the client and the real server.
	f := newFixture(t, echoHandler)

	sniff := &sniffingTransport{base: f.server.Client().Transport}

	client := &http.Client{
		Transport: NewTransport(sniff, ClientConfig{KeySet: f.clientKeys}),
	}

	resp, err := client.Post(f.echoURL(), "application/octet-stream", strings.NewReader("topsecret"))
	require.NoError(t, err)
	resp.Body.Close()

	assert.NotContains(t, string(sniff.reqBody), "topsecret", "plaintext must not appear on the wire")
	assert.NotEmpty(t, sniff.reqBody)
}

type sniffingTransport struct {
	base    http.RoundTripper
	reqBody []byte
}

func (s *sniffingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil && req.GetBody != nil {
		b, _ := req.GetBody()
		s.reqBody, _ = io.ReadAll(b)
	}

	return s.base.RoundTrip(req)
}

func TestReplayDetected(t *testing.T) {
	f := newFixture(t, echoHandler)

	// Reuse one ephemeral key (session) and fix the nid so the replay key
	// (kid, epk, nid) collides between the two requests.
	session, err := NewSession()
	require.NoError(t, err)

	cfg := ClientConfig{
		KeySet:  f.clientKeys,
		Session: session,
		newNID:  func() string { return "fixed-nid" },
	}

	client := &http.Client{Transport: NewTransport(f.server.Client().Transport, cfg)}

	resp1, err := client.Post(f.echoURL(), "text/plain", strings.NewReader("a"))
	require.NoError(t, err)
	resp1.Body.Close()
	assert.Equal(t, http.StatusOK, resp1.StatusCode)

	resp2, err := client.Post(f.echoURL(), "text/plain", strings.NewReader("b"))
	require.NoError(t, err)
	defer resp2.Body.Close()

	assert.Equal(t, http.StatusTooEarly, resp2.StatusCode)
	assert.Equal(t, mux.ContentTypeApplicationProblemJSON, resp2.Header.Get("Content-Type"))
}

func TestUnknownKidRejected(t *testing.T) {
	f := newFixture(t, echoHandler)

	// Tamper the resolved key set so the client advertises an unknown kid.
	bad := *f.clientKeys
	bad.keys = []PublicKey{f.clientKeys.keys[0]}
	bad.keys[0].KID = "does-not-exist"
	bad.byKID = map[string]*PublicKey{"does-not-exist": &bad.keys[0]}

	cfg := ClientConfig{KeySet: &bad}
	client := &http.Client{Transport: NewTransport(f.server.Client().Transport, cfg)}

	resp, err := client.Post(f.echoURL(), "text/plain", strings.NewReader("x"))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var prob Problem
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&prob))
	assert.Equal(t, "urn:ietf:params:e2ee:error:key_unknown", prob.Type)
}

func TestTimestampSkewRejected(t *testing.T) {
	f := newFixture(t, echoHandler)

	// Set the client clock far in the past beyond max_skew.
	cfg := ClientConfig{
		KeySet: f.clientKeys,
		now:    func() time.Time { return time.Now().Add(-time.Hour) },
	}
	client := &http.Client{Transport: NewTransport(f.server.Client().Transport, cfg)}

	resp, err := client.Post(f.echoURL(), "text/plain", strings.NewReader("x"))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var prob Problem
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&prob))
	assert.Equal(t, "urn:ietf:params:e2ee:error:timestamp_skew", prob.Type)
}

func TestMissingSessionHeaderRejected(t *testing.T) {
	f := newFixture(t, echoHandler)

	// A plain request without the E2EE-Session header.
	resp, err := f.server.Client().Post(f.echoURL(), "text/plain", strings.NewReader("x"))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestSessionReuseRoundTrips(t *testing.T) {
	f := newFixture(t, echoHandler)

	session, err := NewSession()
	require.NoError(t, err)

	client := f.client(ClientConfig{Session: session})

	// Two requests over the same session, each with a distinct (random) nid,
	// both succeed.
	for _, msg := range []string{"one", "two"} {
		resp, err := client.Post(f.echoURL(), "text/plain", strings.NewReader(msg))
		require.NoError(t, err)

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		assert.Equal(t, fmt.Sprintf("echo:%s", msg), string(body))
	}
}

func TestTransportReturnsProblemUndecrypted(t *testing.T) {
	f := newFixture(t, echoHandler)

	// An unknown kid triggers a Problem Details response, which the transport
	// returns without attempting to decrypt.
	bad := *f.clientKeys
	bad.keys = []PublicKey{f.clientKeys.keys[0]}
	bad.keys[0].KID = "missing"
	bad.byKID = map[string]*PublicKey{"missing": &bad.keys[0]}

	client := &http.Client{Transport: NewTransport(f.server.Client().Transport, ClientConfig{KeySet: &bad})}

	resp, err := client.Post(f.echoURL(), "text/plain", strings.NewReader("x"))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, mux.ContentTypeApplicationProblemJSON, resp.Header.Get("Content-Type"))
}

func TestServerHeaderPropagation(t *testing.T) {
	f := newFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Custom", "value")
		w.Header().Set("Content-Type", "application/json") // must be replaced
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("{}"))
	})

	client := f.client(ClientConfig{})

	resp, err := client.Post(f.echoURL(), "text/plain", strings.NewReader("x"))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.Equal(t, "value", resp.Header.Get("X-Custom"))
}
