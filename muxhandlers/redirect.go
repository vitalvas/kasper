package muxhandlers

import (
	"bytes"
	"errors"
	"html/template"
	"net/http"
	"strings"

	"github.com/vitalvas/kasper/mux"
)

// ErrRedirectNoRules is returned when RedirectConfig.Rules is empty.
var ErrRedirectNoRules = errors.New("redirect: rules must not be empty")

// ErrRedirectEmptyFrom is returned when a RedirectRule has an empty From field.
var ErrRedirectEmptyFrom = errors.New("redirect: rule From must not be empty")

// ErrRedirectEmptyTo is returned when a RedirectRule has an empty To field.
var ErrRedirectEmptyTo = errors.New("redirect: rule To must not be empty")

// ErrRedirectFromNoSlash is returned when a RedirectRule.From does not start with "/".
var ErrRedirectFromNoSlash = errors.New("redirect: rule From must start with /")

var redirectTemplate = template.Must(template.New("redirect").Parse(
	`<html>
<head><title>{{.StatusText}}</title>
<meta http-equiv="refresh" content="0; url={{.URL}}"></head>
<body>
<center><h1>{{.StatusCode}} {{.StatusText}}</h1></center>
<hr><center><a href="{{.URL}}">The document has moved here.</a></center>
</body>
</html>
`))

type redirectTemplateData struct {
	StatusCode int
	StatusText string
	URL        string
}

// RedirectRule defines a single redirect mapping.
type RedirectRule struct {
	// From is the path to match. Must start with "/".
	// A trailing "*" enables prefix matching: "/old/*" matches any path
	// starting with "/old/" and appends the remainder to To.
	// Without "*", only exact path matches trigger a redirect.
	From string

	// To is the redirect target. For prefix rules, the matched suffix
	// is appended. Can be an absolute URL for external redirects.
	To string

	// StatusCode is the HTTP redirect status code for this rule.
	// Overrides the default from RedirectConfig. If zero, the config
	// default is used.
	StatusCode int
}

// RedirectConfig configures the Redirect middleware.
type RedirectConfig struct {
	// Rules is the list of redirect rules evaluated in order.
	// The first matching rule wins.
	Rules []RedirectRule

	// StatusCode is the default HTTP redirect status code.
	// Defaults to 307 Temporary Redirect.
	StatusCode int
}

type compiledRedirectRule struct {
	prefix     string
	isWildcard bool
	to         string
	statusCode int
}

// RedirectMiddleware returns a middleware that redirects requests based on
// path matching rules. It supports exact path matching and prefix matching
// with a trailing wildcard ("*"). Non-matching requests are passed through
// to the next handler.
//
// The redirect response includes a standard Location header and an HTML body
// with a <meta http-equiv="refresh"> tag for clients that do not follow
// the Location header automatically.
func RedirectMiddleware(cfg RedirectConfig) (mux.MiddlewareFunc, error) {
	if len(cfg.Rules) == 0 {
		return nil, ErrRedirectNoRules
	}

	defaultStatus := cfg.StatusCode
	if defaultStatus == 0 {
		defaultStatus = http.StatusTemporaryRedirect
	}

	compiled := make([]compiledRedirectRule, len(cfg.Rules))
	for i, rule := range cfg.Rules {
		if rule.From == "" {
			return nil, ErrRedirectEmptyFrom
		}

		if rule.To == "" {
			return nil, ErrRedirectEmptyTo
		}

		if !strings.HasPrefix(rule.From, "/") {
			return nil, ErrRedirectFromNoSlash
		}

		status := rule.StatusCode
		if status == 0 {
			status = defaultStatus
		}

		cr := compiledRedirectRule{
			to:         rule.To,
			statusCode: status,
		}

		if prefix, ok := strings.CutSuffix(rule.From, "*"); ok {
			cr.prefix = prefix
			cr.isWildcard = true
		} else {
			cr.prefix = rule.From
		}

		compiled[i] = cr
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path

			for _, rule := range compiled {
				if rule.isWildcard {
					if strings.HasPrefix(path, rule.prefix) {
						suffix := path[len(rule.prefix):]
						writeRedirect(w, r, rule.to+suffix, rule.statusCode)
						return
					}
				} else {
					if path == rule.prefix {
						writeRedirect(w, r, rule.to, rule.statusCode)
						return
					}
				}
			}

			next.ServeHTTP(w, r)
		})
	}, nil
}

func writeRedirect(w http.ResponseWriter, _ *http.Request, url string, statusCode int) {
	var buf bytes.Buffer

	redirectTemplate.Execute(&buf, redirectTemplateData{ //nolint:errcheck
		StatusCode: statusCode,
		StatusText: http.StatusText(statusCode),
		URL:        url,
	})

	w.Header().Set("Location", url)
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(statusCode)
	w.Write(buf.Bytes()) //nolint:errcheck
}
