package openapi

import (
	"reflect"
	"strings"
)

// QueryParamsFromStruct reads the `query:"name"` struct tags of v and
// returns one query *Parameter per field. v may be a struct or a pointer to
// a struct; it is inspected by type only and its field values are ignored.
//
// Field handling mirrors mux.BindQuery so the documented parameters match
// what the binder accepts:
//
//   - The parameter name is the first segment of the `query` tag, falling
//     back to the Go field name when the tag is absent or omits a name
//     (e.g. `query:",omitempty"`).
//   - A `,omitempty` option marks the parameter optional; otherwise it is
//     required.
//   - Fields tagged `query:"-"` and unexported fields are skipped.
//
// The field type drives the schema, and an `openapi:"..."` tag on the field
// applies the same constraints as it does for request and response bodies
// (for example `openapi:"maximum=100"`).
//
// See: https://spec.openapis.org/oas/v3.1.0#parameter-object
func QueryParamsFromStruct(v any) []*Parameter {
	if v == nil {
		return nil
	}

	t := reflect.TypeOf(v)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}

	gen := NewSchemaGenerator()
	gen.fieldTag = "query"

	var params []*Parameter
	for i := range t.NumField() {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		name, opts, _ := strings.Cut(field.Tag.Get("query"), ",")
		if name == "-" {
			continue
		}
		if name == "" {
			name = field.Name
		}

		schema := gen.generateType(field.Type)
		applyOpenAPITag(schema, field.Tag.Get("openapi"))

		params = append(params, &Parameter{
			Name:     name,
			In:       ParameterInQuery,
			Required: !strings.Contains(opts, "omitempty"),
			Schema:   schema,
		})
	}

	return params
}

// QueryParam returns a query-string Parameter with the given name,
// schema, and description.
//
// See: https://spec.openapis.org/oas/v3.1.0#parameter-object
func QueryParam(name, description string, schema *Schema) *Parameter {
	return &Parameter{
		Name:        name,
		In:          ParameterInQuery,
		Description: description,
		Schema:      schema,
	}
}

// RequiredQueryParam returns a required query-string Parameter.
func RequiredQueryParam(name, description string, schema *Schema) *Parameter {
	p := QueryParam(name, description, schema)
	p.Required = true
	return p
}

// PathParam returns a path Parameter. Per the OpenAPI specification,
// path parameters are always required, so the Required field is set
// automatically.
//
// See: https://spec.openapis.org/oas/v3.1.0#parameter-object
func PathParam(name, description string, schema *Schema) *Parameter {
	return &Parameter{
		Name:        name,
		In:          ParameterInPath,
		Description: description,
		Required:    true,
		Schema:      schema,
	}
}

// HeaderParam returns a request-header Parameter.
//
// See: https://spec.openapis.org/oas/v3.1.0#parameter-object
func HeaderParam(name, description string, schema *Schema) *Parameter {
	return &Parameter{
		Name:        name,
		In:          ParameterInHeader,
		Description: description,
		Schema:      schema,
	}
}

// RequiredHeaderParam returns a required request-header Parameter.
func RequiredHeaderParam(name, description string, schema *Schema) *Parameter {
	p := HeaderParam(name, description, schema)
	p.Required = true
	return p
}

// CookieParam returns a cookie Parameter.
//
// See: https://spec.openapis.org/oas/v3.1.0#parameter-object
func CookieParam(name, description string, schema *Schema) *Parameter {
	return &Parameter{
		Name:        name,
		In:          ParameterInCookie,
		Description: description,
		Schema:      schema,
	}
}
