package openapi

// StringSchema returns a Schema describing a JSON string value.
//
// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-6.1.1
func StringSchema() *Schema { return &Schema{Type: SchemaTypeString} }

// IntegerSchema returns a Schema describing a JSON integer value.
func IntegerSchema() *Schema { return &Schema{Type: SchemaTypeInteger} }

// NumberSchema returns a Schema describing a JSON number (integer or
// floating point) value.
func NumberSchema() *Schema { return &Schema{Type: SchemaTypeNumber} }

// BooleanSchema returns a Schema describing a JSON boolean value.
func BooleanSchema() *Schema { return &Schema{Type: SchemaTypeBoolean} }

// EnumSchema returns a Schema describing a JSON string value
// constrained to a fixed set of literal values. Use for headers and
// query parameters whose value is drawn from a closed set (e.g.,
// Sec-Fetch-Dest tokens, an OAuth response_type).
//
// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-6.1.2
func EnumSchema(values ...any) *Schema {
	return &Schema{Type: SchemaTypeString, Enum: values}
}

// ArraySchema returns a Schema describing a JSON array whose elements
// are described by items.
//
// See: https://json-schema.org/draft/2020-12/json-schema-core#section-10.3.1
func ArraySchema(items *Schema) *Schema {
	return &Schema{Type: SchemaTypeArray, Items: items}
}

// ObjectSchema returns a Schema describing a JSON object. Populate the
// returned Schema's Properties to describe its fields.
//
// See: https://json-schema.org/draft/2020-12/json-schema-core#section-10.3.2
func ObjectSchema() *Schema {
	return &Schema{Type: SchemaTypeObject}
}
