package mux

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// regexpType represents the type of template being compiled.
type regexpType int

const (
	regexpTypePath regexpType = iota
	regexpTypeHost
	regexpTypePrefix
	regexpTypeQuery
)

// routeRegexp stores a compiled regexp and metadata about the template.
type routeRegexp struct {
	// template is the original template string.
	template string
	// matchHost indicates matching against the host.
	matchHost bool
	// matchQuery indicates matching against query strings.
	matchQuery bool
	// strictSlash indicates optional trailing slash matching.
	strictSlash bool
	// useEncodedPath indicates using encoded path for matching.
	useEncodedPath bool
	// regexp is the compiled regular expression.
	regexp *regexp.Regexp
	// reverse is the template with %s placeholders for Sprintf.
	reverse string
	// varsN are the variable names in order.
	varsN []string
	// varsR are the compiled regexps for validating each variable value.
	varsR []*regexp.Regexp
	// wildcard indicates a prefix match (no $ anchor).
	wildcard bool
	// queryKey is the query parameter key (only for query type).
	queryKey string
}

// routeRegexpOptions holds options for regexp compilation.
type routeRegexpOptions struct {
	strictSlash    bool
	useEncodedPath bool
}

// newRouteRegexp parses a route template and returns a compiled routeRegexp.
func newRouteRegexp(tpl string, typ regexpType, options routeRegexpOptions) (*routeRegexp, error) {
	var queryKey string

	// For query templates, extract the key (before =) and use the value as template.
	if typ == regexpTypeQuery {
		parts := strings.SplitN(tpl, "=", 2)
		queryKey = parts[0]
		if len(parts) == 2 {
			tpl = parts[1]
		} else {
			tpl = ""
		}
	}

	idxs, err := braceIndices(tpl)
	if err != nil {
		return nil, err
	}

	defaultPattern := "[^/]+"
	if typ == regexpTypeHost {
		defaultPattern = "[^.]+"
	}
	if typ == regexpTypeQuery {
		defaultPattern = ".*"
	}

	template := tpl
	if typ == regexpTypeHost {
		template = strings.ToLower(tpl)
	}

	var (
		pattern  bytes.Buffer
		reverse  bytes.Buffer
		varsN    []string
		varsR    []*regexp.Regexp
		end      int
		wildcard bool
	)

	pattern.WriteByte('^')

	for i := 0; i < len(idxs); i += 2 {
		// Write the raw text between variables.
		raw := tpl[end:idxs[i]]
		end = idxs[i+1]

		// Extract variable name and optional pattern.
		parts := strings.SplitN(tpl[idxs[i]+1:end-1], ":", 2)
		name := parts[0]
		patt := defaultPattern
		var compiledVarR *regexp.Regexp
		if len(parts) == 2 {
			patt, compiledVarR = expandMacro(parts[1])
		}

		if name == "" {
			return nil, fmt.Errorf("mux: missing name in %q from %q", tpl[idxs[i]:end], tpl)
		}

		// Build pattern and reverse template.
		fmt.Fprintf(&pattern, "%s(%s)", regexp.QuoteMeta(raw), patt)
		reverse.WriteString(strings.ReplaceAll(raw, "%", "%%"))
		reverse.WriteString("%s")

		varsN = append(varsN, name)
		if compiledVarR == nil {
			var err error
			compiledVarR, err = compileRegexp(fmt.Sprintf("^%s$", patt))
			if err != nil {
				return nil, fmt.Errorf("mux: invalid pattern %q in variable %q: %w", patt, name, err)
			}
		}
		varsR = append(varsR, compiledVarR)
	}

	// Write the remaining literal text after the last variable.
	raw := tpl[end:]

	// For strictSlash, strip the trailing slash from the pattern so it can
	// be replaced with an optional [/]? group. The reverse template keeps
	// the original template for URL building.
	rawForPattern := raw
	if options.strictSlash && typ == regexpTypePath && strings.HasSuffix(rawForPattern, "/") {
		rawForPattern = strings.TrimSuffix(rawForPattern, "/")
	}

	pattern.WriteString(regexp.QuoteMeta(rawForPattern))
	reverse.WriteString(strings.ReplaceAll(raw, "%", "%%"))

	if typ == regexpTypePrefix {
		wildcard = true
	} else if options.strictSlash && typ == regexpTypePath {
		pattern.WriteString("[/]?")
	}

	if !wildcard {
		pattern.WriteByte('$')
	}

	reg, err := compileRegexp(pattern.String())
	if err != nil {
		return nil, err
	}

	if err := checkDuplicateVars(varsN); err != nil {
		return nil, err
	}

	return &routeRegexp{
		template:       template,
		matchHost:      typ == regexpTypeHost,
		matchQuery:     typ == regexpTypeQuery,
		strictSlash:    options.strictSlash,
		useEncodedPath: options.useEncodedPath,
		regexp:         reg,
		reverse:        reverse.String(),
		varsN:          varsN,
		varsR:          varsR,
		wildcard:       wildcard,
		queryKey:       queryKey,
	}, nil
}

// Match checks whether the compiled regexp matches the request.
// Matches against the query (RFC 3986 Section 3.4), host
// (RFC 7230 Section 5.4), or path (RFC 3986 Section 3.3) component
// depending on the regexp type.
func (r *routeRegexp) Match(req *http.Request, _ *RouteMatch) bool {
	if r.matchQuery {
		return r.matchQueryString(req)
	}
	if r.matchHost {
		host := getHost(req)
		return r.regexp.MatchString(host)
	}

	p := req.URL.Path
	// Use percent-encoded path per RFC 3986 Section 2.1 when configured.
	if r.useEncodedPath {
		p = requestURIPath(req.URL)
	}
	return r.regexp.MatchString(p)
}

// url builds a URL part from the template and the given variable values.
func (r *routeRegexp) url(values map[string]string) (string, error) {
	urlValues := make([]interface{}, len(r.varsN))
	for i, name := range r.varsN {
		v, ok := values[name]
		if !ok {
			return "", fmt.Errorf("mux: missing route variable %q", name)
		}
		if !r.varsR[i].MatchString(v) {
			return "", fmt.Errorf("mux: variable %q doesn't match, expected %q", name, r.varsR[i].String())
		}
		urlValues[i] = v
	}
	return fmt.Sprintf(r.reverse, urlValues...), nil
}

// getURLVars extracts route variables from the given input string.
func (r *routeRegexp) getURLVars(input string) map[string]string {
	matches := r.regexp.FindStringSubmatch(input)
	if matches == nil {
		return nil
	}
	vars := make(map[string]string, len(r.varsN))
	for i, name := range r.varsN {
		if i+1 < len(matches) {
			vars[name] = matches[i+1]
		}
	}
	return vars
}

// matchQueryString matches the route against query parameter values
// per RFC 3986 Section 3.4.
func (r *routeRegexp) matchQueryString(req *http.Request) bool {
	values := req.URL.Query()
	vals, ok := values[r.queryKey]
	if !ok || len(vals) == 0 {
		// If the template has no variables and no required value, it's a
		// presence check.
		if len(r.varsN) == 0 && r.template == "" {
			return ok
		}
		return false
	}
	for _, v := range vals {
		if r.regexp.MatchString(v) {
			return true
		}
	}
	return false
}

// braceIndices returns the start and end+1 indices of each top-level
// {...} pair in s. Returns an error if braces are unbalanced.
func braceIndices(s string) ([]int, error) {
	var (
		idxs  []int
		level int
	)
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{':
			if level++; level == 1 {
				idxs = append(idxs, i)
			}
		case '}':
			if level--; level == 0 {
				idxs = append(idxs, i+1)
			} else if level < 0 {
				return nil, fmt.Errorf("mux: unbalanced braces in %q", s)
			}
		}
	}
	if level != 0 {
		return nil, fmt.Errorf("mux: unbalanced braces in %q", s)
	}
	return idxs, nil
}

// checkDuplicateVars returns an error if any variable name is repeated.
func checkDuplicateVars(vars []string) error {
	seen := make(map[string]bool, len(vars))
	for _, v := range vars {
		if seen[v] {
			return fmt.Errorf("mux: duplicated route variable %q", v)
		}
		seen[v] = true
	}
	return nil
}

// getHost returns the lowercased hostname without port per
// RFC 7230 Section 5.4 (Host header field, host:port format).
func getHost(r *http.Request) string {
	host := r.Host
	if i := strings.Index(host, ":"); i != -1 {
		host = host[:i]
	}
	return strings.ToLower(host)
}

// routeRegexpGroup groups host, path, and query regexps for a route.
type routeRegexpGroup struct {
	host    *routeRegexp
	path    *routeRegexp
	queries []*routeRegexp
}

// varCount returns the total number of named variables across all regexps.
func (v *routeRegexpGroup) varCount() int {
	n := 0
	if v.host != nil {
		n += len(v.host.varsN)
	}
	if v.path != nil {
		n += len(v.path.varsN)
	}
	for _, q := range v.queries {
		n += len(q.varsN)
	}
	return n
}

// setMatch extracts variables from the request and stores them in the match.
func (v *routeRegexpGroup) setMatch(req *http.Request, m *RouteMatch, _ *Route) {
	if m.Vars == nil {
		m.Vars = make(map[string]string, v.varCount())
	}

	if v.host != nil && len(v.host.varsN) > 0 {
		host := getHost(req)
		v.host.setVars(host, m.Vars)
	}

	if v.path != nil && len(v.path.varsN) > 0 {
		p := req.URL.Path
		if v.path.useEncodedPath {
			p = requestURIPath(req.URL)
		}
		v.path.setVars(p, m.Vars)
		if v.path.useEncodedPath {
			for _, name := range v.path.varsN {
				if val, ok := m.Vars[name]; ok {
					if unescaped, err := url.PathUnescape(val); err == nil {
						m.Vars[name] = unescaped
					}
				}
			}
		}
	}

	if len(v.queries) > 0 {
		values := req.URL.Query()
		for _, q := range v.queries {
			if len(q.varsN) == 0 {
				continue
			}
			if vals, ok := values[q.queryKey]; ok {
				for _, val := range vals {
					if q.setVars(val, m.Vars) {
						break
					}
				}
			}
		}
	}
}

// setVars extracts variables from input and writes them directly into dst.
// Returns true if the input matched the regexp.
func (r *routeRegexp) setVars(input string, dst map[string]string) bool {
	matches := r.regexp.FindStringSubmatch(input)
	if matches == nil {
		return false
	}
	for i, name := range r.varsN {
		if i+1 < len(matches) {
			dst[name] = matches[i+1]
		}
	}
	return true
}
