package openapi

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
