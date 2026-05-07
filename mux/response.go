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
// header is set to "text/html; charset=utf-8".
//
// The template is rendered into a buffer first; if SetTemplates has not been
// called, the named template is missing, or template execution fails, an
// HTTP 500 Internal Server Error is written instead.
func ResponseHTML(w http.ResponseWriter, code int, name string, data any) {
	templatesMu.RLock()
	tmpl := templates
	templatesMu.RUnlock()

	if tmpl == nil || tmpl.Lookup(name) == nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	executeHTMLTemplate(w, code, tmpl, name, data)
}

// ResponseHTMLTemplate renders an already-parsed template with data and
// writes the result with the given status code. If name is empty, the
// template's own name is used; otherwise the named template is looked up
// in the template set (the result of ParseFiles or ParseFS). The
// Content-Type header is set to "text/html; charset=utf-8".
//
// If tmpl is nil, the named template is not found, or execution fails, an
// HTTP 500 Internal Server Error is written instead.
func ResponseHTMLTemplate(w http.ResponseWriter, code int, tmpl *template.Template, name string, data any) {
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

	executeHTMLTemplate(w, code, tmpl, name, data)
}

// ResponseHTMLString parses tmpl as an html/template and renders it with
// data, writing the result with the given status code. The Content-Type
// header is set to "text/html; charset=utf-8".
//
// The template is parsed on every call, which is slow; prefer SetTemplates
// + ResponseHTML or ResponseHTMLTemplate for templates rendered repeatedly.
// If parsing or execution fails, an HTTP 500 Internal Server Error is
// written instead.
func ResponseHTMLString(w http.ResponseWriter, code int, tmpl string, data any) {
	parsed, err := template.New("inline").Parse(tmpl)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	executeHTMLTemplate(w, code, parsed, parsed.Name(), data)
}

// executeHTMLTemplate renders tmpl[name] into a buffer and writes the result.
// Returns 500 on execution error without partially flushing the response.
func executeHTMLTemplate(w http.ResponseWriter, code int, tmpl *template.Template, name string, data any) {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", ContentTypeTextHTMLUTF8)
	w.WriteHeader(code)
	w.Write(buf.Bytes())
}

// ResponseJSON encodes v as JSON and writes it to the response with the given
// status code. The Content-Type header is set to "application/json".
// If encoding fails, an HTTP 500 Internal Server Error is written instead.
func ResponseJSON(w http.ResponseWriter, code int, v any) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", ContentTypeApplicationJSON)
	w.WriteHeader(code)
	w.Write(buf.Bytes())
}

// ResponseXML encodes v as XML and writes it to the response with the given
// status code. The Content-Type header is set to "application/xml".
// If encoding fails, an HTTP 500 Internal Server Error is written instead.
func ResponseXML(w http.ResponseWriter, code int, v any) {
	var buf bytes.Buffer
	if err := xml.NewEncoder(&buf).Encode(v); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", ContentTypeApplicationXML)
	w.WriteHeader(code)
	w.Write(buf.Bytes())
}
