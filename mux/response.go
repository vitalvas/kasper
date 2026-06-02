package mux

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"html/template"
	"net/http"
	"sync"
)

// ErrTemplatesNotSet is returned by ResponseHTML when SetTemplates has not
// been called. Wrapped errors retain it so callers may errors.Is against it.
var ErrTemplatesNotSet = errors.New("mux: templates not set")

// ErrTemplateNotFound is returned by ResponseHTML when the named template
// has not been registered.
var ErrTemplateNotFound = errors.New("mux: template not found")

var (
	templatesMu sync.RWMutex
	templates   *template.Template
)

// SetTemplates registers parsed templates for use by ResponseHTML. Pass the
// result of template.ParseFiles, template.ParseFS, or template.Must on a
// pre-parsed *template.Template. Safe to call concurrently and at any time;
// most applications call it once at startup.
func SetTemplates(tmpl *template.Template) {
	templatesMu.Lock()
	templates = tmpl
	templatesMu.Unlock()
}

// ResponseHTML renders the named template registered via SetTemplates with
// data and writes the result with the given status code. The Content-Type
// header defaults to "text/html; charset=utf-8". Pass an optional
// ResponseConfig to override the Content-Type or set extra response headers.
//
// The template is rendered into a buffer first; if SetTemplates has not been
// called, the named template is missing, or template execution fails, an
// HTTP 500 Internal Server Error is written instead.
func ResponseHTML(w http.ResponseWriter, code int, name string, data any, config ...ResponseConfig) {
	templatesMu.RLock()
	tmpl := templates
	templatesMu.RUnlock()

	if tmpl == nil || tmpl.Lookup(name) == nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	executeHTMLTemplate(w, code, tmpl, name, data, config)
}

// ResponseHTMLTemplate renders an already-parsed template with data and
// writes the result with the given status code. If name is empty, the
// template's own name is used; otherwise the named template is looked up
// in the template set (the result of ParseFiles or ParseFS). The
// Content-Type header defaults to "text/html; charset=utf-8". Pass an
// optional ResponseConfig to override the Content-Type or set extra response
// headers.
//
// If tmpl is nil, the named template is not found, or execution fails, an
// HTTP 500 Internal Server Error is written instead.
func ResponseHTMLTemplate(w http.ResponseWriter, code int, tmpl *template.Template, name string, data any, config ...ResponseConfig) {
	if tmpl == nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	if name == "" {
		name = tmpl.Name()
	}
	if tmpl.Lookup(name) == nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	executeHTMLTemplate(w, code, tmpl, name, data, config)
}

// ResponseHTMLString parses tmpl as an html/template and renders it with
// data, writing the result with the given status code. The Content-Type
// header defaults to "text/html; charset=utf-8". Pass an optional
// ResponseConfig to override the Content-Type or set extra response headers.
//
// The template is parsed on every call, which is slow; prefer SetTemplates
// + ResponseHTML or ResponseHTMLTemplate for templates rendered repeatedly.
// If parsing or execution fails, an HTTP 500 Internal Server Error is
// written instead.
func ResponseHTMLString(w http.ResponseWriter, code int, tmpl string, data any, config ...ResponseConfig) {
	parsed, err := template.New("inline").Parse(tmpl)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	executeHTMLTemplate(w, code, parsed, parsed.Name(), data, config)
}

// executeHTMLTemplate renders tmpl[name] into a buffer and writes the result.
// Returns 500 on execution error without partially flushing the response.
func executeHTMLTemplate(w http.ResponseWriter, code int, tmpl *template.Template, name string, data any, config []ResponseConfig) {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	writeResponse(w, code, buf.Bytes(), ContentTypeTextHTMLUTF8, config)
}

// ResponseConfig customizes a response written by ResponseJSON, ResponseXML,
// ResponseHTML, ResponseHTMLTemplate, or ResponseHTMLString. The zero value
// applies the default Content-Type and no extra headers, so passing no config
// preserves the default behaviour.
type ResponseConfig struct {
	// ContentType overrides the default Content-Type header for the
	// response (for example ContentTypeApplicationJWT). When empty, the
	// helper's default is used: "application/json" for ResponseJSON,
	// "application/xml" for ResponseXML, and "text/html; charset=utf-8"
	// for the HTML helpers.
	ContentType string

	// Headers are additional response headers to set before the body is
	// written. Each key is set via http.Header.Set, replacing any existing
	// value. The Content-Type header is always controlled by ContentType
	// and cannot be overridden here.
	Headers map[string]string

	// XMLProlog is the XML declaration ResponseXML prepends before the
	// encoded body (for example xml.Header). Ignored by the JSON and HTML
	// helpers. Defaults to empty, which preserves the encoder's prolog-less
	// output.
	XMLProlog string

	// Indent sets the per-element indentation used by ResponseJSON and
	// ResponseXML. When non-empty, the body is encoded with an empty prefix
	// and this string (for example "  " for two spaces). Ignored by the HTML
	// helpers. Defaults to empty, which produces compact output.
	Indent string
}

// ResponseJSON encodes v as JSON and writes it to the response with the given
// status code. The Content-Type header defaults to "application/json". Pass an
// optional ResponseConfig to override the Content-Type or set extra response
// headers. If encoding fails, an HTTP 500 Internal Server Error is written
// instead.
func ResponseJSON(w http.ResponseWriter, code int, v any, config ...ResponseConfig) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	if len(config) > 0 && config[0].Indent != "" {
		enc.SetIndent("", config[0].Indent)
	}

	if err := enc.Encode(v); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	writeResponse(w, code, buf.Bytes(), ContentTypeApplicationJSON, config)
}

// ResponseXML encodes v as XML and writes it to the response with the given
// status code. The Content-Type header defaults to "application/xml". Pass an
// optional ResponseConfig to override the Content-Type or set extra response
// headers. If encoding fails, an HTTP 500 Internal Server Error is written
// instead.
func ResponseXML(w http.ResponseWriter, code int, v any, config ...ResponseConfig) {
	var buf bytes.Buffer
	if len(config) > 0 && config[0].XMLProlog != "" {
		buf.WriteString(config[0].XMLProlog)
	}

	enc := xml.NewEncoder(&buf)
	if len(config) > 0 && config[0].Indent != "" {
		enc.Indent("", config[0].Indent)
	}

	if err := enc.Encode(v); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	writeResponse(w, code, buf.Bytes(), ContentTypeApplicationXML, config)
}

// writeResponse applies the optional ResponseConfig and writes body with the
// resolved Content-Type and status code. Extra headers are set before
// Content-Type so the resolved content type always wins.
func writeResponse(w http.ResponseWriter, code int, body []byte, defaultCT string, config []ResponseConfig) {
	ct := defaultCT
	if len(config) > 0 {
		if config[0].ContentType != "" {
			ct = config[0].ContentType
		}
		for k, val := range config[0].Headers {
			w.Header().Set(k, val)
		}
	}

	w.Header().Set("Content-Type", ct)
	w.WriteHeader(code)
	w.Write(body)
}
