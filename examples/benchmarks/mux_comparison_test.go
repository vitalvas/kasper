package benchmarks

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	gorillamux "github.com/gorilla/mux"
	kaspermux "github.com/vitalvas/kasper/mux"
)

var endpointCounts = []int{5, 10, 50, 100, 500}

var noopHandler = http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {})

func noopMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}

// nopResponseWriter discards all output to avoid measuring response writing overhead.
type nopResponseWriter struct {
	h http.Header
}

func newNopResponseWriter() *nopResponseWriter {
	return &nopResponseWriter{h: make(http.Header)}
}

func (w *nopResponseWriter) Header() http.Header        { return w.h }
func (w *nopResponseWriter) Write(b []byte) (int, error) { return len(b), nil }
func (w *nopResponseWriter) WriteHeader(int)             {}

// --- Request factories ---

func simpleRequest(idx int) *http.Request {
	return httptest.NewRequest(http.MethodGet, fmt.Sprintf("/resource-%d", idx), nil)
}

func mediumRequest(idx int) *http.Request {
	return httptest.NewRequest(http.MethodGet, fmt.Sprintf("/resource-%d/42", idx), nil)
}

func complexRequest(idx int) *http.Request {
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/resource-%d/42?format=json", idx), nil)
	req.Header.Set("Accept", "application/json")
	return req
}

func simpleMissRequest() *http.Request {
	return httptest.NewRequest(http.MethodGet, "/no-such-route", nil)
}

func mediumMissRequest() *http.Request {
	return httptest.NewRequest(http.MethodGet, "/no-such-route/42", nil)
}

func complexMissRequest() *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/no-such-route/42?format=json", nil)
	req.Header.Set("Accept", "application/json")
	return req
}

// --- Kasper route setup ---

func kasperSetupSimple(n int) http.Handler {
	r := kaspermux.NewRouter()
	for i := range n {
		p := fmt.Sprintf("/resource-%d", i)
		r.HandleFunc(p, noopHandler).Methods(http.MethodGet)
		r.HandleFunc(p, noopHandler).Methods(http.MethodPost)
		r.HandleFunc(p+"/list", noopHandler).Methods(http.MethodGet)
		r.HandleFunc(p+"/update", noopHandler).Methods(http.MethodPut)
		r.HandleFunc(p+"/remove", noopHandler).Methods(http.MethodDelete)
	}
	return r
}

func kasperSetupMedium(n int) http.Handler {
	r := kaspermux.NewRouter()
	r.Use(kaspermux.MiddlewareFunc(noopMiddleware))
	for i := range n {
		p := fmt.Sprintf("/resource-%d", i)
		r.HandleFunc(p+"/{id:[0-9]+}", noopHandler).Methods(http.MethodGet)
		r.HandleFunc(p, noopHandler).Methods(http.MethodPost)
		r.HandleFunc(p+"/{id:[0-9]+}/details", noopHandler).Methods(http.MethodGet)
		r.HandleFunc(p+"/{id:[0-9]+}", noopHandler).Methods(http.MethodPut)
		r.HandleFunc(p+"/{id:[0-9]+}", noopHandler).Methods(http.MethodDelete)
	}
	return r
}

func kasperSetupComplex(n int) http.Handler {
	r := kaspermux.NewRouter()
	r.Use(
		kaspermux.MiddlewareFunc(noopMiddleware),
		kaspermux.MiddlewareFunc(noopMiddleware),
		kaspermux.MiddlewareFunc(noopMiddleware),
	)
	for i := range n {
		sub := r.PathPrefix(fmt.Sprintf("/api/v1/resource-%d", i)).Subrouter()
		sub.HandleFunc("/{id:[0-9]+}", noopHandler).
			Methods(http.MethodGet).
			Queries("format", "{format}").
			Headers("Accept", "application/json")
		sub.HandleFunc("", noopHandler).
			Methods(http.MethodPost).
			Headers("Content-Type", "application/json")
		sub.HandleFunc("/{id:[0-9]+}/sub/{subId:[0-9]+}", noopHandler).
			Methods(http.MethodGet)
		sub.HandleFunc("/{id:[0-9]+}", noopHandler).
			Methods(http.MethodPut)
		sub.HandleFunc("/{id:[0-9]+}", noopHandler).
			Methods(http.MethodDelete)
	}
	return r
}

// --- Gorilla route setup ---

func gorillaSetupSimple(n int) http.Handler {
	r := gorillamux.NewRouter()
	for i := range n {
		p := fmt.Sprintf("/resource-%d", i)
		r.HandleFunc(p, noopHandler).Methods(http.MethodGet)
		r.HandleFunc(p, noopHandler).Methods(http.MethodPost)
		r.HandleFunc(p+"/list", noopHandler).Methods(http.MethodGet)
		r.HandleFunc(p+"/update", noopHandler).Methods(http.MethodPut)
		r.HandleFunc(p+"/remove", noopHandler).Methods(http.MethodDelete)
	}
	return r
}

func gorillaSetupMedium(n int) http.Handler {
	r := gorillamux.NewRouter()
	r.Use(gorillamux.MiddlewareFunc(noopMiddleware))
	for i := range n {
		p := fmt.Sprintf("/resource-%d", i)
		r.HandleFunc(p+"/{id:[0-9]+}", noopHandler).Methods(http.MethodGet)
		r.HandleFunc(p, noopHandler).Methods(http.MethodPost)
		r.HandleFunc(p+"/{id:[0-9]+}/details", noopHandler).Methods(http.MethodGet)
		r.HandleFunc(p+"/{id:[0-9]+}", noopHandler).Methods(http.MethodPut)
		r.HandleFunc(p+"/{id:[0-9]+}", noopHandler).Methods(http.MethodDelete)
	}
	return r
}

func gorillaSetupComplex(n int) http.Handler {
	r := gorillamux.NewRouter()
	r.Use(
		gorillamux.MiddlewareFunc(noopMiddleware),
		gorillamux.MiddlewareFunc(noopMiddleware),
		gorillamux.MiddlewareFunc(noopMiddleware),
	)
	for i := range n {
		sub := r.PathPrefix(fmt.Sprintf("/api/v1/resource-%d", i)).Subrouter()
		sub.HandleFunc("/{id:[0-9]+}", noopHandler).
			Methods(http.MethodGet).
			Queries("format", "{format}").
			Headers("Accept", "application/json")
		sub.HandleFunc("", noopHandler).
			Methods(http.MethodPost).
			Headers("Content-Type", "application/json")
		sub.HandleFunc("/{id:[0-9]+}/sub/{subId:[0-9]+}", noopHandler).
			Methods(http.MethodGet)
		sub.HandleFunc("/{id:[0-9]+}", noopHandler).
			Methods(http.MethodPut)
		sub.HandleFunc("/{id:[0-9]+}", noopHandler).
			Methods(http.MethodDelete)
	}
	return r
}

// --- Benchmark harness ---

type benchConfig struct {
	name    string
	setup   func(int) http.Handler
	request func(int) *http.Request
	miss    *http.Request
}

func benchmarkRouter(b *testing.B, configs []benchConfig) {
	b.Helper()
	for _, cfg := range configs {
		for _, n := range endpointCounts {
			b.Run(fmt.Sprintf("Setup/%s/%d", cfg.name, n), func(b *testing.B) {
				for b.Loop() {
					cfg.setup(n)
				}
			})
		}

		for _, n := range endpointCounts {
			router := cfg.setup(n)

			b.Run(fmt.Sprintf("Dispatch_First/%s/%d", cfg.name, n), func(b *testing.B) {
				req := cfg.request(0)
				w := newNopResponseWriter()
				for b.Loop() {
					router.ServeHTTP(w, req)
				}
			})

			b.Run(fmt.Sprintf("Dispatch_Last/%s/%d", cfg.name, n), func(b *testing.B) {
				req := cfg.request(n - 1)
				w := newNopResponseWriter()
				for b.Loop() {
					router.ServeHTTP(w, req)
				}
			})

			b.Run(fmt.Sprintf("Dispatch_Miss/%s/%d", cfg.name, n), func(b *testing.B) {
				w := newNopResponseWriter()
				for b.Loop() {
					router.ServeHTTP(w, cfg.miss)
				}
			})
		}
	}
}

// --- Top-level benchmarks ---

func BenchmarkKasper(b *testing.B) {
	benchmarkRouter(b, []benchConfig{
		{name: "Simple", setup: kasperSetupSimple, request: simpleRequest, miss: simpleMissRequest()},
		{name: "Medium", setup: kasperSetupMedium, request: mediumRequest, miss: mediumMissRequest()},
		{name: "Complex", setup: kasperSetupComplex, request: complexRequest, miss: complexMissRequest()},
	})
}

func BenchmarkGorilla(b *testing.B) {
	benchmarkRouter(b, []benchConfig{
		{name: "Simple", setup: gorillaSetupSimple, request: simpleRequest, miss: simpleMissRequest()},
		{name: "Medium", setup: gorillaSetupMedium, request: mediumRequest, miss: mediumMissRequest()},
		{name: "Complex", setup: gorillaSetupComplex, request: complexRequest, miss: complexMissRequest()},
	})
}
