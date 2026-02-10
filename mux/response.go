package mux

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"net/http"
)

// ResponseJSON encodes v as JSON and writes it to the response with the given
// status code. The Content-Type header is set to "application/json".
// If encoding fails, an HTTP 500 Internal Server Error is written instead.
func ResponseJSON(w http.ResponseWriter, code int, v any) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
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

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(code)
	w.Write(buf.Bytes())
}
