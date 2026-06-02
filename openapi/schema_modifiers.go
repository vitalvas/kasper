package openapi

import "slices"

// WithFormat sets the schema's Format and returns the schema for
// chaining (e.g., StringSchema().WithFormat(FormatUUID)).
//
// See: https://spec.openapis.org/oas/v3.1.0#data-types
func (s *Schema) WithFormat(format string) *Schema {
	s.Format = format
	return s
}

// WithDescription sets the schema's Description and returns the schema
// for chaining.
//
// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-9
func (s *Schema) WithDescription(description string) *Schema {
	s.Description = description
	return s
}

// WithExample sets the schema's Example and returns the schema for
// chaining.
//
// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-9.5
func (s *Schema) WithExample(example any) *Schema {
	s.Example = example
	return s
}

// WithDefault sets the schema's Default and returns the schema for
// chaining.
//
// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-9.2
func (s *Schema) WithDefault(value any) *Schema {
	s.Default = value
	return s
}

// Nullable appends the "null" type to the schema's existing type set,
// making the value nullable per JSON Schema Draft 2020-12, and returns
// the schema for chaining. Calling it on a schema that is already
// nullable, or one with no type set, leaves the type unchanged.
//
// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-6.1.1
func (s *Schema) Nullable() *Schema {
	values := s.Type.Values()
	if len(values) == 0 || slices.Contains(values, "null") {
		return s
	}
	s.Type = TypeArray(append(append([]string(nil), values...), "null")...)
	return s
}
