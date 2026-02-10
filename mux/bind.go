package mux

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
)

// BindJSON decodes the request body as JSON into v.
// By default the decoder rejects unknown fields that do not map to exported
// struct fields. Pass false to allow unknown fields.
// Exactly one JSON value must be present in the body; trailing data is an error.
func BindJSON(r *http.Request, v any, allowUnknownFields ...bool) error {
	dec := json.NewDecoder(r.Body)

	if len(allowUnknownFields) == 0 || !allowUnknownFields[0] {
		dec.DisallowUnknownFields()
	}

	if err := dec.Decode(v); err != nil {
		return err
	}

	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("unexpected trailing data after JSON value")
	}

	return nil
}

// BindXML decodes the request body as XML into v.
// Exactly one XML element must be present in the body; trailing data is an error.
func BindXML(r *http.Request, v any) error {
	dec := xml.NewDecoder(r.Body)

	if err := dec.Decode(v); err != nil {
		return err
	}

	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("unexpected trailing data after XML value")
	}

	return nil
}
