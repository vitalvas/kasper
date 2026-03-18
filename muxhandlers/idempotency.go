package muxhandlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/vitalvas/kasper/mux"
)

// ErrNoIdempotencyStore is returned when IdempotencyConfig.Store is nil.
var ErrNoIdempotencyStore = errors.New("idempotency: Store must be set")

// IdempotencySkipMetadataKey is the route metadata key used to skip
// idempotency processing for specific routes. Set this key to true
// in route metadata to bypass the middleware.
//
//	r.HandleFunc("/health", handler).Metadata(muxhandlers.IdempotencySkipMetadataKey, true)
const IdempotencySkipMetadataKey = "idempotency:skip"

// IdempotencyLocker is an optional interface for distributed locking of
// in-flight requests. When a lock cannot be acquired (another request
// with the same key is in progress), the middleware returns 409 Conflict.
// Implementations must be safe for concurrent use.
type IdempotencyLocker interface {
	// Lock attempts to acquire a lock for the given key. Returns true if
	// the lock was acquired, false if the key is already locked.
	Lock(ctx context.Context, key string) bool

	// Unlock releases the lock for the given key.
	Unlock(ctx context.Context, key string)
}

// IdempotencyStore is the interface for storing and retrieving cached
// responses keyed by idempotency key. Implementations must be safe for
// concurrent use.
type IdempotencyStore interface {
	// Get retrieves a cached response by key. Returns the serialized
	// response and true if found, or nil and false if not cached.
	Get(ctx context.Context, key string) ([]byte, bool)

	// Set stores a serialized response with the given key and TTL.
	// A zero TTL means the entry does not expire.
	Set(ctx context.Context, key string, value []byte, ttl time.Duration)
}

// IdempotencyConfig configures the Idempotency middleware.
//
// Spec reference: https://datatracker.ietf.org/doc/draft-ietf-httpapi-idempotency-key-header/
type IdempotencyConfig struct {
	// Store is the backing store for cached responses. Required.
	Store IdempotencyStore

	// HeaderName overrides the header used to carry the idempotency key.
	// Defaults to "Idempotency-Key".
	HeaderName string

	// TTL is the time-to-live for cached responses. Defaults to 24 hours.
	// A zero value means entries do not expire.
	TTL time.Duration

	// Methods is the set of HTTP methods that require an idempotency key.
	// When nil, defaults to POST.
	Methods []string

	// EnforceKey, when true, returns 400 Bad Request if the idempotency
	// key header is missing on a matched method. When false (default),
	// requests without the header are passed through without caching.
	EnforceKey bool

	// CacheableStatusCodes is an optional allow list of HTTP status codes
	// that should be cached. When nil, all status codes are cached.
	// When set, only responses with a status code in this list are stored;
	// other responses (e.g. 500) are passed through without caching.
	CacheableStatusCodes []int

	// CacheKeyFunc is an optional function that builds the final cache key
	// from the request and the raw idempotency key header value. Use this
	// to scope cached responses per authenticated user, tenant, or other
	// request-scoped identity. When nil, the default scoping
	// (method + path + header value) is used.
	CacheKeyFunc func(r *http.Request, key string) string

	// ValidateKeyFunc is an optional function to validate the idempotency key
	// format. When set, the middleware calls it before looking up the cache.
	// It receives the request and the raw key value. Return true to accept
	// the key, false to reject it with 400 Bad Request.
	ValidateKeyFunc func(r *http.Request, key string) bool

	// KeyMaxLength is the maximum allowed length of the idempotency key.
	// Keys exceeding this length are rejected with 400 Bad Request.
	// Defaults to 64. Set to -1 for no limit.
	KeyMaxLength int

	// CanCache is an optional pre-check called before any cache lookup or
	// storage. When it returns false, the request is passed through to the
	// handler without idempotency caching. Use this to skip caching based
	// on request properties (e.g. specific paths, headers, or auth state).
	// When nil, all matched requests are eligible for caching.
	CanCache func(r *http.Request) bool

	// OnCacheHit is called when a cached response is found for the
	// idempotency key. Use this for observability (e.g. Prometheus counters).
	// When nil, no callback is invoked.
	OnCacheHit func(r *http.Request, key string)

	// OnCacheMiss is called when no cached response is found and the
	// handler is invoked. Use this for observability (e.g. Prometheus counters).
	// When nil, no callback is invoked.
	OnCacheMiss func(r *http.Request, key string)

	// Locker is an optional distributed lock for in-flight requests.
	// When set, the middleware acquires a lock before invoking the handler
	// and releases it after the response is stored. If the lock cannot be
	// acquired (another request with the same key is in progress), the
	// middleware returns 409 Conflict. When nil, no locking is performed.
	Locker IdempotencyLocker

	// FingerprintFunc is an optional function that computes a fingerprint
	// from the request. The fingerprint is stored alongside the cached
	// response. On cache hit, if the current request's fingerprint does
	// not match the stored one, the middleware returns 422 Unprocessable
	// Entity instead of replaying the cached response. This prevents
	// clients from reusing idempotency keys across different operations.
	// When nil, no fingerprint matching is performed.
	FingerprintFunc func(r *http.Request) string

	// OnConflict is called when a 409 Conflict is returned because the
	// Locker could not acquire a lock (another request with the same
	// key is in-flight). Use this for observability.
	// When nil, no callback is invoked.
	OnConflict func(r *http.Request, key string)

	// OnFingerprintMismatch is called when a 422 Unprocessable Entity is
	// returned because the request fingerprint does not match the cached
	// one. Use this for observability and alerting on key misuse.
	// When nil, no callback is invoked.
	OnFingerprintMismatch func(r *http.Request, key string)

	// RetryAfter is the duration sent in the Retry-After header (as whole
	// seconds) when a 409 Conflict response is returned due to an in-flight
	// lock. When zero, no Retry-After header is sent.
	RetryAfter time.Duration

	// ReplayedHeaderName sets a response header to "true" when a cached
	// response is replayed. Use this to let clients distinguish original
	// responses from replays. When empty, no replay indicator header is
	// set. Example: "X-Idempotency-Replayed".
	ReplayedHeaderName string

	// ErrorHandler is an optional function that writes error responses
	// for all middleware-generated errors (400, 409, 422). When set, it
	// replaces the default http.Error plain-text responses. Use this to
	// return structured JSON errors or RFC 9457 Problem Details.
	// When nil, http.Error is used.
	ErrorHandler func(w http.ResponseWriter, r *http.Request, statusCode int)

	// OnStore is called when a response is successfully stored in the
	// cache. Use this for observability to track cache fill rate. Not
	// called when the response status code is excluded by
	// CacheableStatusCodes. When nil, no callback is invoked.
	OnStore func(r *http.Request, key string, statusCode int)

	// ResponseHeadersFunc is called before writing any response (original,
	// replayed, or error). Use this to inject headers like X-Cache-Age or
	// update the Date header. The replayed parameter is true when the
	// response is a cached replay. When nil, no callback is invoked.
	ResponseHeadersFunc func(w http.Header, r *http.Request, replayed bool)

	// MaxCacheBodySize is the maximum response body size in bytes that
	// will be cached. Responses with bodies exceeding this limit are
	// served to the client but not stored in the cache. When zero, no
	// limit is applied.
	MaxCacheBodySize int64
}

// idempotencyResponse is the serialized format for cached responses.
type idempotencyResponse struct {
	StatusCode  int         `json:"status_code"`
	Header      http.Header `json:"header"`
	Body        []byte      `json:"body"`
	Fingerprint string      `json:"fingerprint,omitempty"`
}

const defaultKeyMaxLength = 64

// idempotencyWriteError writes an error response using the custom ErrorHandler
// if set, otherwise falls back to http.Error.
func idempotencyWriteError(w http.ResponseWriter, r *http.Request, statusCode int, errorHandler func(http.ResponseWriter, *http.Request, int)) {
	if errorHandler != nil {
		errorHandler(w, r, statusCode)
		return
	}
	http.Error(w, http.StatusText(statusCode), statusCode)
}

// IdempotencyMiddleware returns a middleware that caches responses keyed by the
// Idempotency-Key header per draft-ietf-httpapi-idempotency-key-header. When a
// request carries a key that has been seen before, the cached response is
// replayed without invoking the downstream handler. The cached response
// includes the Idempotency-Key header in the replay.
//
// It returns ErrNoIdempotencyStore if Store is nil.
func IdempotencyMiddleware(cfg IdempotencyConfig) (mux.MiddlewareFunc, error) {
	if cfg.Store == nil {
		return nil, ErrNoIdempotencyStore
	}

	headerName := cfg.HeaderName
	if headerName == "" {
		headerName = "Idempotency-Key"
	}

	ttl := cfg.TTL
	if ttl == 0 && cfg.TTL == 0 {
		ttl = 24 * time.Hour
	}

	methods := cfg.Methods
	if methods == nil {
		methods = []string{http.MethodPost}
	}

	methodSet := make(map[string]struct{}, len(methods))
	for _, m := range methods {
		methodSet[m] = struct{}{}
	}

	var cacheableSet map[int]struct{}
	if len(cfg.CacheableStatusCodes) > 0 {
		cacheableSet = make(map[int]struct{}, len(cfg.CacheableStatusCodes))
		for _, code := range cfg.CacheableStatusCodes {
			cacheableSet[code] = struct{}{}
		}
	}

	store := cfg.Store
	enforceKey := cfg.EnforceKey
	cacheKeyFunc := cfg.CacheKeyFunc
	validateKeyFunc := cfg.ValidateKeyFunc
	canCache := cfg.CanCache
	onCacheHit := cfg.OnCacheHit
	onCacheMiss := cfg.OnCacheMiss
	locker := cfg.Locker
	fingerprintFunc := cfg.FingerprintFunc
	onConflict := cfg.OnConflict
	onFingerprintMismatch := cfg.OnFingerprintMismatch
	retryAfter := cfg.RetryAfter
	replayedHeaderName := cfg.ReplayedHeaderName
	errorHandler := cfg.ErrorHandler
	onStore := cfg.OnStore
	responseHeadersFunc := cfg.ResponseHeadersFunc
	maxCacheBodySize := cfg.MaxCacheBodySize

	keyMaxLength := cfg.KeyMaxLength
	if keyMaxLength == 0 {
		keyMaxLength = defaultKeyMaxLength
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := methodSet[r.Method]; !ok {
				next.ServeHTTP(w, r)
				return
			}

			if meta := mux.RequestMetadata(r); meta != nil {
				if skip, ok := meta[IdempotencySkipMetadataKey]; ok {
					if b, isBool := skip.(bool); isBool && b {
						next.ServeHTTP(w, r)
						return
					}
				}
			}

			if canCache != nil && !canCache(r) {
				next.ServeHTTP(w, r)
				return
			}

			key := r.Header.Get(headerName)
			if key == "" {
				if enforceKey {
					idempotencyWriteError(w, r, http.StatusBadRequest, errorHandler)
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			if keyMaxLength > 0 && len(key) > keyMaxLength {
				idempotencyWriteError(w, r, http.StatusBadRequest, errorHandler)
				return
			}

			if validateKeyFunc != nil && !validateKeyFunc(r, key) {
				idempotencyWriteError(w, r, http.StatusBadRequest, errorHandler)
				return
			}

			var cacheKey string
			if cacheKeyFunc != nil {
				cacheKey = cacheKeyFunc(r, key)
			} else {
				cacheKey = fmt.Sprintf("%s:%s:%s", r.Method, r.URL.Path, key)
			}

			var fingerprint string
			if fingerprintFunc != nil {
				fingerprint = fingerprintFunc(r)
			}

			if cached, ok := store.Get(r.Context(), cacheKey); ok {
				if fingerprintFunc != nil {
					var resp idempotencyResponse
					if err := json.Unmarshal(cached, &resp); err == nil && resp.Fingerprint != fingerprint {
						if onFingerprintMismatch != nil {
							onFingerprintMismatch(r, key)
						}
						idempotencyWriteError(w, r, http.StatusUnprocessableEntity, errorHandler)
						return
					}
				}
				if onCacheHit != nil {
					onCacheHit(r, key)
				}
				if replayedHeaderName != "" {
					w.Header().Set(replayedHeaderName, "true")
				}
				if responseHeadersFunc != nil {
					responseHeadersFunc(w.Header(), r, true)
				}
				replayResponse(w, cached, headerName, key)
				return
			}

			if onCacheMiss != nil {
				onCacheMiss(r, key)
			}

			if locker != nil {
				if !locker.Lock(r.Context(), cacheKey) {
					if onConflict != nil {
						onConflict(r, key)
					}
					if retryAfter > 0 {
						w.Header().Set("Retry-After", strconv.FormatInt(int64(retryAfter/time.Second), 10))
					}
					idempotencyWriteError(w, r, http.StatusConflict, errorHandler)
					return
				}
				defer locker.Unlock(r.Context(), cacheKey)

				// Re-check cache after acquiring the lock. A blocking locker
				// implementation may have waited while another request stored
				// the response.
				if cached, ok := store.Get(r.Context(), cacheKey); ok {
					if onCacheHit != nil {
						onCacheHit(r, key)
					}
					if replayedHeaderName != "" {
						w.Header().Set(replayedHeaderName, "true")
					}
					if responseHeadersFunc != nil {
						responseHeadersFunc(w.Header(), r, true)
					}
					replayResponse(w, cached, headerName, key)
					return
				}
			}

			rec := &responseRecorder{
				header: make(http.Header),
				body:   &bytes.Buffer{},
			}

			next.ServeHTTP(rec, r)

			cacheable := cacheableSet == nil
			if !cacheable {
				_, cacheable = cacheableSet[rec.statusCode]
			}

			if cacheable && maxCacheBodySize > 0 && int64(rec.body.Len()) > maxCacheBodySize {
				cacheable = false
			}

			if cacheable {
				resp := idempotencyResponse{
					StatusCode:  rec.statusCode,
					Header:      rec.header,
					Body:        rec.body.Bytes(),
					Fingerprint: fingerprint,
				}

				if data, err := json.Marshal(resp); err == nil {
					store.Set(r.Context(), cacheKey, data, ttl)
					if onStore != nil {
						onStore(r, key, rec.statusCode)
					}
				}
			}

			if responseHeadersFunc != nil {
				responseHeadersFunc(w.Header(), r, false)
			}

			writeRecordedResponse(w, rec, headerName, key)
		})
	}, nil
}

// replayResponse writes a cached response to the client.
func replayResponse(w http.ResponseWriter, data []byte, headerName, key string) {
	var resp idempotencyResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	h := w.Header()
	for k, vals := range resp.Header {
		for _, v := range vals {
			h.Add(k, v)
		}
	}
	h.Set(headerName, key)

	w.WriteHeader(resp.StatusCode)
	w.Write(resp.Body) //nolint:errcheck
}

// writeRecordedResponse writes the recorded response to the actual client.
func writeRecordedResponse(w http.ResponseWriter, rec *responseRecorder, headerName, key string) {
	h := w.Header()
	for k, vals := range rec.header {
		for _, v := range vals {
			h.Add(k, v)
		}
	}
	h.Set(headerName, key)

	if rec.statusCode == 0 {
		rec.statusCode = http.StatusOK
	}

	w.WriteHeader(rec.statusCode)
	w.Write(rec.body.Bytes()) //nolint:errcheck
}

// responseRecorder captures the response from a handler for caching.
type responseRecorder struct {
	header     http.Header
	body       *bytes.Buffer
	statusCode int
}

func (r *responseRecorder) Header() http.Header {
	return r.header
}

func (r *responseRecorder) WriteHeader(code int) {
	if r.statusCode == 0 {
		r.statusCode = code
	}
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if r.statusCode == 0 {
		r.statusCode = http.StatusOK
	}
	return r.body.Write(b)
}
