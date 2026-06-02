package openapi

// NewExample returns an Example with the given summary and value. Use it
// to build the named entries of an Examples map on a parameter, header,
// or media type.
//
// See: https://spec.openapis.org/oas/v3.1.0#example-object
func NewExample(summary string, value any) *Example {
	return &Example{
		Summary: summary,
		Value:   value,
	}
}

// ExternalExample returns an Example that references an external value by
// URI instead of embedding it. ExternalValue and an inline Value are
// mutually exclusive per the specification.
//
// See: https://spec.openapis.org/oas/v3.1.0#example-object
func ExternalExample(summary, externalValue string) *Example {
	return &Example{
		Summary:       summary,
		ExternalValue: externalValue,
	}
}

// WithDescription sets the example's Description and returns it for
// chaining.
//
// See: https://spec.openapis.org/oas/v3.1.0#example-object
func (e *Example) WithDescription(description string) *Example {
	e.Description = description
	return e
}

// WithExamples sets the named example set on the parameter and returns it
// for chaining. Named examples are an alternative to a single inline
// Example and let one parameter document several representative values.
//
// See: https://spec.openapis.org/oas/v3.1.0#parameter-object (examples)
func (p *Parameter) WithExamples(examples map[string]*Example) *Parameter {
	p.Examples = examples
	return p
}

// WithExamples sets the named example set on the header and returns it
// for chaining.
//
// See: https://spec.openapis.org/oas/v3.1.0#header-object (examples)
func (h *Header) WithExamples(examples map[string]*Example) *Header {
	h.Examples = examples
	return h
}

// WithExamples sets the named example set on the media type and returns
// it for chaining.
//
// See: https://spec.openapis.org/oas/v3.1.0#media-type-object (examples)
func (m *MediaType) WithExamples(examples map[string]*Example) *MediaType {
	m.Examples = examples
	return m
}
