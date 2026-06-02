package openapi

import "fmt"

// OneOf returns a Schema that validates against exactly one of the given
// subschemas (JSON Schema "oneOf"). Pair it with WithDiscriminator for
// tagged unions.
//
// See: https://json-schema.org/draft/2020-12/json-schema-core#section-10.2.1.3
func OneOf(schemas ...*Schema) *Schema {
	return &Schema{OneOf: schemas}
}

// AnyOf returns a Schema that validates against one or more of the given
// subschemas (JSON Schema "anyOf").
//
// See: https://json-schema.org/draft/2020-12/json-schema-core#section-10.2.1.2
func AnyOf(schemas ...*Schema) *Schema {
	return &Schema{AnyOf: schemas}
}

// AllOf returns a Schema that validates against all of the given
// subschemas (JSON Schema "allOf"), the usual way to express composition
// or inheritance.
//
// See: https://json-schema.org/draft/2020-12/json-schema-core#section-10.2.1.1
func AllOf(schemas ...*Schema) *Schema {
	return &Schema{AllOf: schemas}
}

// Not returns a Schema that validates only when the value does not match
// the given subschema (JSON Schema "not").
//
// See: https://json-schema.org/draft/2020-12/json-schema-core#section-10.2.1.4
func Not(schema *Schema) *Schema {
	return &Schema{Not: schema}
}

// RefSchema returns a Schema that references a named component schema in
// "#/components/schemas/{name}". Use it to point at a type the schema
// generator has already registered.
//
// See: https://spec.openapis.org/oas/v3.1.0#reference-object
func RefSchema(name string) *Schema {
	return &Schema{Ref: fmt.Sprintf("#/components/schemas/%s", name)}
}

// WithDiscriminator sets the OpenAPI discriminator on a composition
// schema (oneOf/anyOf) and returns the schema for chaining. propertyName
// is the payload field that selects the variant; mapping maps each
// discriminator value to a component schema reference (pass nil to rely
// on implicit mapping by schema name).
//
// See: https://spec.openapis.org/oas/v3.1.0#discriminator-object
func (s *Schema) WithDiscriminator(propertyName string, mapping map[string]string) *Schema {
	s.Discriminator = &Discriminator{
		PropertyName: propertyName,
		Mapping:      mapping,
	}
	return s
}
