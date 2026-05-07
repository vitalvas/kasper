// Package csrf implements Cross-Site Request Forgery protection middleware
// for the kasper/mux router.
//
// The middleware applies defense-in-depth in the order recommended by the
// OWASP CSRF cheat sheet:
//
//  1. Safe methods (GET, HEAD, OPTIONS, TRACE per RFC 9110) skip validation.
//  2. The Origin header (RFC 6454) is checked first when present and must
//     match the request host or a trusted origin in Config.TrustedOrigins.
//  3. The Referer header is required for HTTPS requests when Origin is
//     missing (legacy fallback) and must satisfy the same check.
//  4. A signed token cookie is verified against a token submitted via the
//     X-CSRF-Token header or a form field, using constant-time comparison.
//
// Tokens are stored in a single signed cookie produced by kasper/securecookie
// (AES-GCM). The submitted token is masked with a fresh random byte string
// per request to defend against BREACH-style compression-oracle attacks
// (OWASP Token Disclosure via Compression).
//
// Example:
//
//	key, _ := securecookie.GenerateKey(32)
//	r.Use(csrf.Middleware(csrf.Config{
//	    Key:            key,
//	    TrustedOrigins: []string{"https://example.com", "https://*.example.com"},
//	}))
package csrf

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/vitalvas/kasper/mux"
	"github.com/vitalvas/kasper/securecookie"
)

// tokenLength is the size in bytes of the raw CSRF token.
const tokenLength = 32

// randRead is the source of cryptographic randomness. Tests override this
// to exercise error paths.
var randRead = rand.Read

// Sentinel errors for use with ErrorHandler and errors.Is.
var (
	// ErrNoCookie is returned when the CSRF cookie is missing or unreadable.
	ErrNoCookie = errors.New("csrf: cookie not present")
	// ErrNoToken is returned when no token was submitted via the configured
	// header or form field.
	ErrNoToken = errors.New("csrf: token not submitted")
	// ErrTokenMismatch is returned when the submitted token does not match
	// the cookie token after constant-time comparison.
	ErrTokenMismatch = errors.New("csrf: token mismatch")
	// ErrOriginRejected is returned when the Origin header is not in the
	// trusted origin list and does not match the request host.
	ErrOriginRejected = errors.New("csrf: origin not trusted")
	// ErrRefererRejected is returned when the Referer header (used as
	// fallback when Origin is missing) is not trusted.
	ErrRefererRejected = errors.New("csrf: referer not trusted")
	// ErrRefererMissing is returned for HTTPS requests where both Origin
	// and Referer headers are absent.
	ErrRefererMissing = errors.New("csrf: referer missing on https request")
)

// Config configures the CSRF middleware.
type Config struct {
	// Key is the signing key for the CSRF cookie. Must be 16, 24, or 32
	// bytes for AES-128/192/256-GCM. Required.
	Key []byte

	// TrustedOrigins is the allowlist of additional origins permitted in
	// the Origin and Referer headers. Wildcards are supported in the host
	// label: "https://*.example.com" matches any single subdomain. Origins
	// must include the scheme.
	TrustedOrigins []string

	// TrustedOriginFunc is an optional predicate consulted after
	// TrustedOrigins. It receives the parsed Origin (or Referer) URL and
	// returns true to allow the request. Useful for matching dynamic
	// preview-deploy hostnames (Vercel, Netlify, Cloudflare Pages) where
	// a static or wildcard list is impractical.
	TrustedOriginFunc func(*url.URL) bool

	// CookieName is the name of the CSRF cookie. Defaults to "csrf_token".
	CookieName string

	// CookiePath is the cookie Path attribute. Defaults to "/".
	CookiePath string

	// CookieDomain is the cookie Domain attribute. Defaults to empty
	// (host-only cookie).
	CookieDomain string

	// CookieSecure sets the Secure attribute. Defaults to true.
	// Set to false for local HTTP development only.
	CookieSecure *bool

	// CookieSameSite sets the SameSite attribute. Defaults to
	// http.SameSiteLaxMode.
	CookieSameSite http.SameSite

	// CookieMaxAge is the cookie Max-Age in seconds. Zero (default) makes
	// it a session cookie that expires when the browser closes.
	CookieMaxAge int

	// HeaderName is the request header inspected for the submitted token.
	// Defaults to "X-CSRF-Token".
	HeaderName string

	// FormFieldName is the form field inspected for the submitted token
	// when the request body is form-encoded. Defaults to "csrf_token".
	FormFieldName string

	// SafeMethods is the set of HTTP methods that skip CSRF validation
	// entirely. Defaults to GET, HEAD, OPTIONS, TRACE.
	SafeMethods []string

	// Lazy controls when a CSRF cookie is set on the response. When false
	// (default), the middleware issues a cookie on every request that
	// lacks a valid one. When true, the cookie is set lazily on the first
	// call to Token or TemplateField, keeping responses smaller for
	// endpoints that never render forms.
	Lazy bool

	// ErrorHandler is called when validation fails. It receives the
	// failure reason as a sentinel error. If nil, the middleware writes
	// 403 Forbidden with the reason as plain text.
	ErrorHandler func(w http.ResponseWriter, r *http.Request, reason error)
}

// ctxKey is the unexported type for the request context key.
type ctxKey struct{}

// requestState holds per-request CSRF state populated by the middleware.
type requestState struct {
	cfg    *resolvedConfig
	codec  *securecookie.SecureCookie
	cookie string // the unmasked token currently bound to the cookie
	w      http.ResponseWriter
}

// resolvedConfig is Config with defaults applied and TrustedOrigins
// pre-parsed for fast matching.
type resolvedConfig struct {
	cookieName    string
	cookiePath    string
	cookieDomain  string
	cookieSecure  bool
	cookieSameS   http.SameSite
	cookieMaxAge  int
	headerName    string
	formFieldName string
	safeMethods   map[string]struct{}
	trusted       []*originPattern
	trustedFunc   func(*url.URL) bool
	issueAlways   bool
	errorHandler  func(http.ResponseWriter, *http.Request, error)
}

// originPattern represents a parsed entry from Config.TrustedOrigins.
type originPattern struct {
	scheme string
	host   string // exact host or first-label wildcard host (e.g., ".example.com")
	wild   bool   // true when host has a leading "*."
}

// Middleware returns an HTTP middleware that enforces CSRF protection
// using the configuration in cfg.
func Middleware(cfg Config) mux.MiddlewareFunc {
	rc, err := resolveConfig(cfg)
	if err != nil {
		// Configuration errors must not be silently ignored. Returning a
		// middleware that always fails makes the misconfiguration obvious
		// in development and tests.
		return failMiddleware(err)
	}
	codec, err := securecookie.New(cfg.Key)
	if err != nil {
		return failMiddleware(err)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			state := &requestState{cfg: rc, codec: codec, w: w}
			r = r.WithContext(stateContext(r, state))

			cookieToken, hasCookie := readCookie(r, rc, codec)
			if hasCookie {
				state.cookie = cookieToken
			}

			if rc.issueAlways && !hasCookie {
				if err := issueCookie(w, state); err != nil {
					rc.errorHandler(w, r, err)
					return
				}
			}

			if _, safe := rc.safeMethods[r.Method]; safe {
				next.ServeHTTP(w, r)
				return
			}

			if err := validate(r, rc, cookieToken, hasCookie); err != nil {
				rc.errorHandler(w, r, err)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// Validate runs the CSRF validation pipeline (Origin/Referer check, cookie
// presence, submitted token, constant-time compare) for the current request
// without dispatching to a downstream handler. Returns nil on success, or
// one of the sentinel errors on failure.
//
// Use Validate from non-HTTP-handler contexts where the standard middleware
// chain does not apply, such as WebSocket upgrade handlers, gRPC-over-HTTP
// gateways, or custom handler wrappers. The request must have been
// processed by Middleware first so the CSRF cookie is available.
//
// Validate skips the safe-method check; the caller is responsible for
// short-circuiting safe methods if desired.
func Validate(r *http.Request) error {
	state := stateFrom(r)
	if state == nil {
		return ErrNoCookie
	}
	hasCookie := state.cookie != ""
	return validate(r, state.cfg, state.cookie, hasCookie)
}

// validate is the shared validation core used by Middleware and Validate.
func validate(r *http.Request, rc *resolvedConfig, cookieToken string, hasCookie bool) error {
	if reason := verifyFetchMetadataAndOrigin(r, rc); reason != nil {
		return reason
	}
	if !hasCookie {
		return ErrNoCookie
	}
	submitted, err := submittedToken(r, rc)
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare([]byte(submitted), []byte(cookieToken)) != 1 {
		return ErrTokenMismatch
	}
	return nil
}

// verifyFetchMetadataAndOrigin combines the Sec-Fetch-Site fast path with
// the Origin/Referer fallback. Per the OWASP CSRF Prevention Cheat Sheet,
// Sec-Fetch-Site replaces Origin verification when present (modern
// browsers send it on every request); Origin/Referer remain as the
// fallback for clients that do not.
//
// Sec-Fetch-Site values per Fetch Metadata Request Headers (W3C):
//   - "same-origin" or "none": request originates from this site or
//     direct user action; trust the origin layer.
//   - "same-site": registrable-domain match (e.g., a.example.com from
//     b.example.com). OWASP warns against unconditional same-site trust
//     because of subdomain takeover risk; reject.
//   - "cross-site": reject outright.
//   - missing: fall through to Origin/Referer.
func verifyFetchMetadataAndOrigin(r *http.Request, rc *resolvedConfig) error {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin", "none":
		return nil
	case "same-site", "cross-site":
		return ErrOriginRejected
	}
	return verifyOrigin(r, rc)
}

// Token returns the current request's CSRF token, masked for safe
// transport. The returned string changes per call (fresh mask), but each
// value validates against the same cookie. Returns an empty string when
// the middleware has not run for this request.
//
// In Config{IssueOnEveryRequest: false} mode, the first call to Token
// triggers cookie issuance.
func Token(r *http.Request) string {
	state := stateFrom(r)
	if state == nil {
		return ""
	}
	if state.cookie == "" {
		// Lazy issue.
		if err := issueCookie(state.w, state); err != nil {
			return ""
		}
	}
	return mask(state.cookie)
}

// TemplateField returns a hidden HTML input element containing the masked
// CSRF token, suitable for direct embedding in an html/template. Returns
// an empty string when the middleware has not run.
func TemplateField(r *http.Request) template.HTML {
	state := stateFrom(r)
	if state == nil {
		return ""
	}
	tok := Token(r)
	if tok == "" {
		return ""
	}
	field := fmt.Sprintf(
		`<input type="hidden" name="%s" value="%s">`,
		template.HTMLEscapeString(state.cfg.formFieldName),
		template.HTMLEscapeString(tok),
	)
	return template.HTML(field) //nolint:gosec // values are HTML-escaped above.
}

// Rotate forces a new CSRF token for the current request. Useful after
// privilege transitions such as login. Subsequent calls to Token within
// the same request return values derived from the new cookie.
func Rotate(w http.ResponseWriter, r *http.Request) {
	state := stateFrom(r)
	if state == nil {
		return
	}
	raw, err := newRawToken()
	if err != nil {
		return
	}
	state.cookie = raw
	_ = writeCookie(w, state)
}

// --- internal helpers ---

func resolveConfig(cfg Config) (*resolvedConfig, error) {
	rc := &resolvedConfig{
		cookieName:    cfg.CookieName,
		cookiePath:    cfg.CookiePath,
		cookieDomain:  cfg.CookieDomain,
		cookieSameS:   cfg.CookieSameSite,
		cookieMaxAge:  cfg.CookieMaxAge,
		headerName:    cfg.HeaderName,
		formFieldName: cfg.FormFieldName,
		issueAlways:   !cfg.Lazy,
		errorHandler:  cfg.ErrorHandler,
		trustedFunc:   cfg.TrustedOriginFunc,
	}
	if rc.cookieName == "" {
		rc.cookieName = "csrf_token"
	}
	if rc.cookiePath == "" {
		rc.cookiePath = "/"
	}
	if cfg.CookieSecure == nil {
		rc.cookieSecure = true
	} else {
		rc.cookieSecure = *cfg.CookieSecure
	}
	if rc.cookieSameS == 0 {
		rc.cookieSameS = http.SameSiteLaxMode
	}
	if rc.headerName == "" {
		rc.headerName = "X-CSRF-Token"
	}
	if rc.formFieldName == "" {
		rc.formFieldName = "csrf_token"
	}

	// Enforce browser constraints for the __Host- and __Secure- cookie
	// prefixes (RFC 6265bis §4.1.3). Misconfiguration would silently
	// cause browsers to reject the cookie.
	if strings.HasPrefix(rc.cookieName, "__Host-") {
		if !rc.cookieSecure {
			return nil, errors.New(`csrf: cookie name "__Host-" prefix requires CookieSecure: true`)
		}
		if rc.cookieDomain != "" {
			return nil, errors.New(`csrf: cookie name "__Host-" prefix forbids CookieDomain`)
		}
		if rc.cookiePath != "/" {
			return nil, errors.New(`csrf: cookie name "__Host-" prefix requires CookiePath "/"`)
		}
	}
	if strings.HasPrefix(rc.cookieName, "__Secure-") && !rc.cookieSecure {
		return nil, errors.New(`csrf: cookie name "__Secure-" prefix requires CookieSecure: true`)
	}

	methods := cfg.SafeMethods
	if methods == nil {
		methods = []string{http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace}
	}
	rc.safeMethods = make(map[string]struct{}, len(methods))
	for _, m := range methods {
		rc.safeMethods[strings.ToUpper(m)] = struct{}{}
	}

	for _, o := range cfg.TrustedOrigins {
		p, err := parseOriginPattern(o)
		if err != nil {
			return nil, fmt.Errorf("csrf: invalid trusted origin %q: %w", o, err)
		}
		rc.trusted = append(rc.trusted, p)
	}

	if rc.errorHandler == nil {
		rc.errorHandler = defaultErrorHandler
	}

	return rc, nil
}

func failMiddleware(err error) mux.MiddlewareFunc {
	return func(_ http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		})
	}
}

func defaultErrorHandler(w http.ResponseWriter, _ *http.Request, reason error) {
	// Announce that the rejection depends on request-context headers so
	// intermediate caches do not serve a 403 keyed only by URL/method.
	w.Header().Add("Vary", "Sec-Fetch-Site")
	w.Header().Add("Vary", "Origin")
	w.Header().Add("Vary", "Cookie")
	http.Error(w, reason.Error(), http.StatusForbidden)
}

func parseOriginPattern(s string) (*originPattern, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, errors.New("origin must include scheme and host")
	}

	host := strings.ToLower(u.Host)
	wild := false
	if strings.HasPrefix(host, "*.") {
		wild = true
		host = host[1:] // ".example.com"
	}

	return &originPattern{
		scheme: strings.ToLower(u.Scheme),
		host:   host,
		wild:   wild,
	}, nil
}

func (p *originPattern) match(scheme, host string) bool {
	scheme = strings.ToLower(scheme)
	host = strings.ToLower(host)
	if p.scheme != scheme {
		return false
	}
	if p.wild {
		// p.host is stored as ".example.com" (with leading dot).
		// Require exactly one additional DNS label before the suffix:
		// "api.example.com" matches; "a.b.example.com" does not.
		if !strings.HasSuffix(host, p.host) {
			return false
		}
		label := host[:len(host)-len(p.host)]
		if label == "" || strings.Contains(label, ".") {
			return false
		}
		return true
	}
	return p.host == host
}

func verifyOrigin(r *http.Request, rc *resolvedConfig) error {
	origin := r.Header.Get("Origin")
	switch {
	case origin == "null":
		// RFC 6454 §7.3 "null" denotes a privacy-sensitive context with no
		// trustworthy origin (sandboxed iframes, redirected POSTs, etc.).
		// When present, treat it as an explicit rejection regardless of
		// scheme; do not fall through to the Referer fallback or the
		// HTTP "missing-origin" allowance.
		return ErrOriginRejected
	case origin != "":
		if !originAllowed(origin, r, rc) {
			return ErrOriginRejected
		}
		return nil
	}

	// Origin missing: require Referer for HTTPS requests (legacy fallback);
	// HTTP requests with no Origin and no Referer pass at this stage.
	if !isHTTPS(r) {
		return nil
	}

	referer := r.Header.Get("Referer")
	if referer == "" {
		return ErrRefererMissing
	}
	if !originAllowed(referer, r, rc) {
		return ErrRefererRejected
	}
	return nil
}

func originAllowed(raw string, r *http.Request, rc *resolvedConfig) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}

	if u.Scheme == requestScheme(r) && u.Host == r.Host {
		return true
	}

	for _, p := range rc.trusted {
		if p.match(u.Scheme, u.Host) {
			return true
		}
	}

	if rc.trustedFunc != nil && rc.trustedFunc(u) {
		return true
	}

	return false
}

func isHTTPS(r *http.Request) bool {
	return requestScheme(r) == "https"
}

func requestScheme(r *http.Request) string {
	if r.URL != nil && r.URL.Scheme != "" {
		return r.URL.Scheme
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

// --- token cookie handling ---

var maskPool = sync.Pool{
	New: func() any {
		b := make([]byte, tokenLength)
		return &b
	},
}

func newRawToken() (string, error) {
	b := make([]byte, tokenLength)
	if _, err := randRead(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// mask XORs the raw token with a fresh per-call random byte string and
// returns base64(mask || (token XOR mask)). Defends against BREACH.
func mask(rawTok string) string {
	rawBytes, err := base64.RawURLEncoding.DecodeString(rawTok)
	if err != nil {
		return ""
	}

	maskPtr := maskPool.Get().(*[]byte)
	defer maskPool.Put(maskPtr)
	m := *maskPtr
	if _, err := randRead(m); err != nil {
		return ""
	}

	out := make([]byte, len(m)+len(rawBytes))
	copy(out, m)
	for i, b := range rawBytes {
		out[len(m)+i] = b ^ m[i%len(m)]
	}
	return base64.RawURLEncoding.EncodeToString(out)
}

// unmask reverses mask. Returns the original raw-base64 token, or empty
// string on malformed input.
func unmask(s string) string {
	combined, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil || len(combined) < tokenLength*2 {
		return ""
	}
	m := combined[:tokenLength]
	masked := combined[tokenLength:]
	raw := make([]byte, len(masked))
	for i, b := range masked {
		raw[i] = b ^ m[i%len(m)]
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func readCookie(r *http.Request, rc *resolvedConfig, codec *securecookie.SecureCookie) (string, bool) {
	c, err := r.Cookie(rc.cookieName)
	if err != nil || c.Value == "" {
		return "", false
	}
	var raw string
	if err := codec.Decode(c.Value, &raw); err != nil {
		return "", false
	}
	return raw, true
}

func issueCookie(w http.ResponseWriter, state *requestState) error {
	raw, err := newRawToken()
	if err != nil {
		return err
	}
	state.cookie = raw
	return writeCookie(w, state)
}

func writeCookie(w http.ResponseWriter, state *requestState) error {
	encoded, err := state.codec.Encode(state.cookie)
	if err != nil {
		return err
	}
	c := &http.Cookie{
		Name:     state.cfg.cookieName,
		Value:    encoded,
		Path:     state.cfg.cookiePath,
		Domain:   state.cfg.cookieDomain,
		Secure:   state.cfg.cookieSecure,
		HttpOnly: true,
		SameSite: state.cfg.cookieSameS,
		MaxAge:   state.cfg.cookieMaxAge,
	}
	http.SetCookie(w, c)
	return nil
}

func submittedToken(r *http.Request, rc *resolvedConfig) (string, error) {
	if v := r.Header.Get(rc.headerName); v != "" {
		return unmaskOrEmpty(v)
	}

	ctype := r.Header.Get("Content-Type")
	// Only attempt form parsing for form-encoded bodies; do not consume
	// JSON or other bodies, which would interfere with downstream handlers.
	if strings.HasPrefix(ctype, "application/x-www-form-urlencoded") ||
		strings.HasPrefix(ctype, "multipart/form-data") {
		if err := r.ParseForm(); err == nil {
			if v := r.PostForm.Get(rc.formFieldName); v != "" {
				return unmaskOrEmpty(v)
			}
		}
	}

	return "", ErrNoToken
}

func unmaskOrEmpty(submitted string) (string, error) {
	raw := unmask(submitted)
	if raw == "" {
		return "", ErrTokenMismatch
	}
	return raw, nil
}

// --- request context ---

func stateContext(r *http.Request, s *requestState) context.Context {
	return context.WithValue(r.Context(), ctxKey{}, s)
}

func stateFrom(r *http.Request) *requestState {
	if v := r.Context().Value(ctxKey{}); v != nil {
		if s, ok := v.(*requestState); ok {
			return s
		}
	}
	return nil
}
