package openapi

// WithMinimum sets the inclusive numeric minimum and returns the schema
// for chaining.
//
// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-6.2.4
func (s *Schema) WithMinimum(value float64) *Schema {
	s.Minimum = &value
	return s
}

// WithMaximum sets the inclusive numeric maximum and returns the schema
// for chaining.
//
// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-6.2.2
func (s *Schema) WithMaximum(value float64) *Schema {
	s.Maximum = &value
	return s
}

// WithExclusiveMinimum sets the exclusive numeric minimum and returns the
// schema for chaining.
//
// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-6.2.5
func (s *Schema) WithExclusiveMinimum(value float64) *Schema {
	s.ExclusiveMinimum = &value
	return s
}

// WithExclusiveMaximum sets the exclusive numeric maximum and returns the
// schema for chaining.
//
// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-6.2.3
func (s *Schema) WithExclusiveMaximum(value float64) *Schema {
	s.ExclusiveMaximum = &value
	return s
}

// WithMultipleOf constrains the value to a multiple of n and returns the
// schema for chaining.
//
// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-6.2.1
func (s *Schema) WithMultipleOf(n float64) *Schema {
	s.MultipleOf = &n
	return s
}

// WithMinLength sets the minimum string length and returns the schema for
// chaining.
//
// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-6.3.2
func (s *Schema) WithMinLength(n int) *Schema {
	s.MinLength = &n
	return s
}

// WithMaxLength sets the maximum string length and returns the schema for
// chaining.
//
// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-6.3.1
func (s *Schema) WithMaxLength(n int) *Schema {
	s.MaxLength = &n
	return s
}

// WithPattern sets the regular-expression pattern a string must match and
// returns the schema for chaining.
//
// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-6.3.3
func (s *Schema) WithPattern(pattern string) *Schema {
	s.Pattern = pattern
	return s
}

// WithMinItems sets the minimum number of array items and returns the
// schema for chaining.
//
// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-6.4.2
func (s *Schema) WithMinItems(n int) *Schema {
	s.MinItems = &n
	return s
}

// WithMaxItems sets the maximum number of array items and returns the
// schema for chaining.
//
// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-6.4.1
func (s *Schema) WithMaxItems(n int) *Schema {
	s.MaxItems = &n
	return s
}

// WithUniqueItems requires array items to be unique and returns the
// schema for chaining.
//
// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-6.4.3
func (s *Schema) WithUniqueItems() *Schema {
	s.UniqueItems = true
	return s
}

// WithEnum sets the allowed values and returns the schema for chaining.
// Unlike EnumSchema, this works on a schema of any type.
//
// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-6.1.2
func (s *Schema) WithEnum(values ...any) *Schema {
	s.Enum = values
	return s
}

// WithItems sets the array element schema and returns the schema for
// chaining.
//
// See: https://json-schema.org/draft/2020-12/json-schema-core#section-10.3.1.2
func (s *Schema) WithItems(items *Schema) *Schema {
	s.Items = items
	return s
}
