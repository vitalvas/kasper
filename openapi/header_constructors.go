package openapi

// StringHeader returns a Header describing a string-valued response
// header, with the given description.
//
// See: https://spec.openapis.org/oas/v3.1.0#header-object
func StringHeader(description string) *Header {
	return &Header{
		Description: description,
		Schema:      StringSchema(),
	}
}

// IntegerHeader returns a Header describing an integer-valued response
// header (e.g., Retry-After in seconds form, Content-Length).
//
// See: https://spec.openapis.org/oas/v3.1.0#header-object
func IntegerHeader(description string) *Header {
	return &Header{
		Description: description,
		Schema:      IntegerSchema(),
	}
}

// HeaderOf returns a Header with the given schema and description.
// Use this for non-primitive header schemas (e.g., enum-constrained
// custom header values).
//
// See: https://spec.openapis.org/oas/v3.1.0#header-object
func HeaderOf(description string, schema *Schema) *Header {
	return &Header{
		Description: description,
		Schema:      schema,
	}
}
