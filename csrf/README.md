# csrf

Cross-Site Request Forgery protection middleware for the kasper/mux router.

The middleware applies defense-in-depth in the order recommended by the OWASP CSRF cheat sheet:

1. Safe methods (GET, HEAD, OPTIONS, TRACE) skip validation entirely (RFC 9110).
2. The `Sec-Fetch-Site` header (Fetch Metadata, W3C) is consulted first when present:
   `same-origin` or `none` accepted; `same-site` or `cross-site` rejected; missing
   or unknown values fall through to step 3.
3. The `Origin` header is checked when present and must match the request host
   or a trusted origin. `Origin: null` is rejected per RFC 6454 §7.3.
4. The `Referer` header is required for HTTPS requests when both `Sec-Fetch-Site`
   and `Origin` are absent (legacy browser fallback) and must satisfy the same check.
5. A signed token cookie is verified against a token submitted via the
   `X-CSRF-Token` header or a form field, using constant-time comparison.

The token cookie is signed with kasper/securecookie (AES-GCM). Submitted tokens are masked with a fresh random byte string per request to defend against BREACH-style compression-oracle attacks.

## Installation

```bash
go get github.com/vitalvas/kasper/csrf
```

## Usage

```go
key, _ := securecookie.GenerateKey(32)

r := mux.NewRouter()
r.Use(csrf.Middleware(csrf.Config{
    Key:            key,
    TrustedOrigins: []string{"https://example.com", "https://*.example.com"},
}))

r.HandleFunc("/login", loginHandler)
```

## Injecting the Token into Pages

The masked token can be carried to the client three ways. All three return values from `csrf.Token(r)` / `csrf.TemplateField(r)`, which produce a freshly masked token on every call (BREACH defense). Do not cache the token across requests -- the cookie carries state, the token in the page is just the unmasked claim for that request.

### 1. Server-rendered form (hidden input)

`csrf.TemplateField` returns a ready-to-embed `<input type="hidden">` element. The form submit carries the token in the `csrf_token` form field, which the middleware extracts automatically:

```go
const formTpl = `
<form method="POST" action="/login">
    {{.CSRFField}}
    <input name="username">
    <input name="password" type="password">
    <button>Submit</button>
</form>
`

func loginPage(w http.ResponseWriter, r *http.Request) {
    mux.ResponseHTMLString(w, http.StatusOK, formTpl, map[string]any{
        "CSRFField": csrf.TemplateField(r),
    })
}
```

Renders to:

```html
<input type="hidden" name="csrf_token" value="<masked-token>">
```

### 2. Server-rendered SPA shell (meta tag)

Embed the raw token in the HTML head; client JS reads it on bootstrap and sends it via `X-CSRF-Token`:

```go
const indexTpl = `
<!DOCTYPE html>
<html>
<head>
    <meta name="csrf-token" content="{{.CSRFToken}}">
</head>
<body>
    <div id="app"></div>
    <script src="/app.js"></script>
</body>
</html>
`

func index(w http.ResponseWriter, r *http.Request) {
    mux.ResponseHTMLString(w, http.StatusOK, indexTpl, map[string]any{
        "CSRFToken": csrf.Token(r),
    })
}
```

```javascript
const csrfToken = document.querySelector('meta[name="csrf-token"]').content

fetch("/api/orders", {
    method: "POST",
    headers: {
        "X-CSRF-Token": csrfToken,
        "Content-Type": "application/json",
    },
    body: JSON.stringify(payload),
})
```

### 3. Pure JSON API (dedicated endpoint)

For a fully decoupled SPA where the backend never serves HTML, expose the token from a JSON endpoint and fetch it on app boot:

```go
r.HandleFunc("/api/csrf", func(w http.ResponseWriter, r *http.Request) {
    mux.ResponseJSON(w, http.StatusOK, map[string]string{
        "csrf_token": csrf.Token(r),
    })
})
```

```javascript
const { csrf_token } = await fetch("/api/csrf").then(r => r.json())
// reuse csrf_token in subsequent X-CSRF-Token headers
```

### Choosing a pattern

| Scenario | Pattern |
|----------|---------|
| Traditional HTML forms | (1) `TemplateField` |
| SSR shell + client-side JS framework | (2) meta tag |
| API-only / fully decoupled SPA | (3) JSON endpoint |

## Token Rotation

Call `csrf.Rotate(w, r)` after privilege transitions such as login or logout:

```go
func handleLogin(w http.ResponseWriter, r *http.Request) {
    if !authenticate(r) {
        http.Error(w, "Unauthorized", http.StatusUnauthorized)
        return
    }
    csrf.Rotate(w, r)
    http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}
```

## Trusted Origins

`Config.TrustedOrigins` accepts full origin strings (`scheme://host`) and supports a single leading `*.` wildcard for subdomain matching:

```go
TrustedOrigins: []string{
    "https://example.com",        // exact match
    "https://*.example.com",      // exactly one subdomain label (api.example.com, www.example.com)
    "https://partner-portal.com", // additional trusted partner
}
```

The `*.` wildcard matches **exactly one** DNS label. `https://*.example.com` matches `api.example.com` and `www.example.com` but not `a.b.example.com` or `example.com` itself. List the apex separately if you need it. Use `TrustedOriginFunc` for arbitrary-depth or pattern-based matching.

For dynamic preview-deployment hostnames (Vercel, Netlify, Cloudflare Pages) where neither a static list nor a single wildcard suffices, use `TrustedOriginFunc`:

```go
TrustedOriginFunc: func(u *url.URL) bool {
    return u.Scheme == "https" && strings.HasSuffix(u.Host, ".preview.vercel.app")
},
```

The function is consulted only after `TrustedOrigins` does not match and after the same-origin check fails. The full parsed `*url.URL` is passed so the predicate can inspect scheme, host, and port.

## Hardened cookie name (`__Host-` prefix)

The `__Host-` cookie prefix (RFC 6265bis §4.1.3) instructs the browser to enforce strict same-site, host-only cookie scoping. For maximum hardening, set the cookie name to `__Host-csrf` (or any name with that prefix):

```go
csrf.Middleware(csrf.Config{
    Key:        key,
    CookieName: "__Host-csrf",
    // CookieSecure defaults to true (required by __Host-).
    // CookieDomain must be empty (default).
    // CookiePath must be "/" (default).
})
```

The middleware validates these constraints at startup and refuses to start with a configuration the browser would silently reject (explicit `CookieSecure: false`, non-empty `CookieDomain`, or `CookiePath != "/"`). The `__Secure-` prefix is also recognized and requires `CookieSecure: true`.

## Reverse Proxies

When the application sits behind a reverse proxy that terminates TLS, install `muxhandlers.ProxyHeadersMiddleware` *before* the CSRF middleware so `r.URL.Scheme` and `r.Host` reflect the client-facing values:

```go
r.Use(muxhandlers.ProxyHeadersMiddleware(muxhandlers.ProxyHeadersConfig{
    TrustedProxies: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")},
}))
r.Use(csrf.Middleware(csrf.Config{Key: key, /* ... */}))
```

Without `ProxyHeadersMiddleware`, the same-origin check sees the internal request URL (often `http://internal-host/...`), which will not match the public `Origin` header.

## Validate (manual / WebSocket flows)

Use `Validate(r)` from non-handler contexts -- WebSocket upgrades, custom protocol bridges, or middleware that needs to drive validation manually -- to run the same checks the middleware performs:

```go
upgrader := websocket.Upgrader{}

r.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
    if err := csrf.Validate(r); err != nil {
        http.Error(w, err.Error(), http.StatusForbidden)
        return
    }
    conn, err := upgrader.Upgrade(w, r, nil)
    // ...
})
```

`Validate` requires that `Middleware` has previously run (so the cookie is present in the request); use it on the same request, never on a freshly constructed one.

## SPA Bootstrap on 403

To let an SPA client recover its CSRF token after a failed validation, return the current token from the error handler:

```go
csrf.Middleware(csrf.Config{
    Key: key,
    ErrorHandler: func(w http.ResponseWriter, r *http.Request, reason error) {
        mux.ResponseJSON(w, http.StatusForbidden, map[string]string{
            "error":      reason.Error(),
            "csrf_token": csrf.Token(r),
        })
    },
})
```

`Token(r)` is available throughout the request lifecycle, including inside the error handler.

## Skipping CSRF on specific routes

Apply the middleware via `mux.With` to scope it to a subset of routes, instead of disabling per-route:

```go
protected := r.With(csrf.Middleware(cfg))
protected.HandleFunc("/users", createUser).Methods(http.MethodPost)

// /webhooks/* skips CSRF entirely (e.g., for incoming server-to-server callbacks)
r.HandleFunc("/webhooks/{provider}", webhookHandler).Methods(http.MethodPost)
```

## Configuration

| Field | Default | Description |
|-------|---------|-------------|
| `Key` | required | Signing key (16/24/32 bytes for AES-128/192/256-GCM) |
| `TrustedOrigins` | nil | Allowlist of cross-origin sources |
| `TrustedOriginFunc` | nil | Predicate consulted after `TrustedOrigins`; useful for dynamic preview hostnames |
| `CookieName` | `csrf_token` | Cookie name |
| `CookiePath` | `/` | Cookie Path attribute |
| `CookieDomain` | empty (host-only) | Cookie Domain attribute |
| `CookieSecure` | `true` | Cookie Secure attribute (`*bool` to allow explicit false) |
| `CookieSameSite` | `http.SameSiteLaxMode` | Cookie SameSite attribute |
| `CookieMaxAge` | 0 (session cookie) | Cookie Max-Age in seconds |
| `HeaderName` | `X-CSRF-Token` | Token submission header |
| `FormFieldName` | `csrf_token` | Token submission form field |
| `SafeMethods` | `GET, HEAD, OPTIONS, TRACE` | Methods that skip validation |
| `Lazy` | `false` | When true, cookie is set only on first `Token`/`TemplateField` call |
| `ErrorHandler` | 403 + plain text reason | Custom handler for validation failures |

## Errors

`ErrorHandler` receives a sentinel error that callers may compare with `errors.Is`:

| Error | When returned |
|-------|---------------|
| `ErrNoCookie` | CSRF cookie missing or unreadable |
| `ErrNoToken` | No token submitted via header or form |
| `ErrTokenMismatch` | Submitted token does not match cookie |
| `ErrOriginRejected` | `Origin` header is not in trusted list |
| `ErrRefererRejected` | `Referer` (HTTPS fallback) is not trusted |
| `ErrRefererMissing` | HTTPS request lacks both `Origin` and `Referer` |

## API Reference

```go
func Middleware(cfg Config) mux.MiddlewareFunc
func Validate(r *http.Request) error
func Token(r *http.Request) string
func TemplateField(r *http.Request) template.HTML
func Rotate(w http.ResponseWriter, r *http.Request)
```
