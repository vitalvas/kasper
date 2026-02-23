package openapi

import (
	"encoding/json"

	"gopkg.in/yaml.v3"
)

// JSON serializes the document as indented JSON bytes.
//
// See: https://spec.openapis.org/oas/v3.1.0#openapi-object
func (d *Document) JSON() ([]byte, error) {
	return json.MarshalIndent(d, "", "  ")
}

// YAML serializes the document as YAML bytes.
//
// See: https://spec.openapis.org/oas/v3.1.0#openapi-object
func (d *Document) YAML() ([]byte, error) {
	return yaml.Marshal(d)
}

// DocumentFromJSON parses a JSON-encoded OpenAPI document.
//
// See: https://spec.openapis.org/oas/v3.1.0#openapi-object
func DocumentFromJSON(data []byte) (*Document, error) {
	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

// DocumentFromYAML parses a YAML-encoded OpenAPI document.
//
// See: https://spec.openapis.org/oas/v3.1.0#openapi-object
func DocumentFromYAML(data []byte) (*Document, error) {
	var doc Document
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}
