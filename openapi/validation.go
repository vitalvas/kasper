package openapi

import (
	"fmt"
	"slices"
)

// ValidParameterLocations lists the permitted values for Parameter.In.
//
// See: https://spec.openapis.org/oas/v3.1.0#parameter-object
var ValidParameterLocations = []string{
	ParameterInQuery,
	ParameterInPath,
	ParameterInHeader,
	ParameterInCookie,
}

// ValidSecuritySchemeTypes lists the permitted values for
// SecurityScheme.Type.
//
// See: https://spec.openapis.org/oas/v3.1.0#security-scheme-object
var ValidSecuritySchemeTypes = []string{
	SecurityTypeHTTP,
	SecurityTypeAPIKey,
	SecurityTypeMutualTLS,
	SecurityTypeOAuth2,
	SecurityTypeOpenIDConnect,
}

// ValidSecuritySchemeLocations lists the permitted values for the In
// field of an apiKey SecurityScheme.
//
// See: https://spec.openapis.org/oas/v3.1.0#security-scheme-object
var ValidSecuritySchemeLocations = []string{
	SecurityInHeader,
	SecurityInQuery,
	SecurityInCookie,
}

// Validate reports whether the Parameter conforms to the OpenAPI
// specification. It checks that Name is set and that In is one of the
// values in ValidParameterLocations. Path parameters must additionally
// be Required.
//
// See: https://spec.openapis.org/oas/v3.1.0#parameter-object
func (p *Parameter) Validate() error {
	if p.Name == "" {
		return fmt.Errorf("openapi: parameter name must not be empty")
	}
	if !slices.Contains(ValidParameterLocations, p.In) {
		return fmt.Errorf("openapi: parameter %q has invalid location %q, must be one of %v", p.Name, p.In, ValidParameterLocations)
	}
	if p.In == ParameterInPath && !p.Required {
		return fmt.Errorf("openapi: path parameter %q must be required", p.Name)
	}
	return nil
}

// Validate reports whether the SecurityScheme conforms to the OpenAPI
// specification. It checks that Type is one of the values in
// ValidSecuritySchemeTypes and that the fields required for that type
// are populated.
//
// See: https://spec.openapis.org/oas/v3.1.0#security-scheme-object
func (s *SecurityScheme) Validate() error {
	if !slices.Contains(ValidSecuritySchemeTypes, s.Type) {
		return fmt.Errorf("openapi: invalid security scheme type %q, must be one of %v", s.Type, ValidSecuritySchemeTypes)
	}
	switch s.Type {
	case SecurityTypeAPIKey:
		if s.Name == "" {
			return fmt.Errorf("openapi: apiKey security scheme must set name")
		}
		if !slices.Contains(ValidSecuritySchemeLocations, s.In) {
			return fmt.Errorf("openapi: apiKey security scheme has invalid location %q, must be one of %v", s.In, ValidSecuritySchemeLocations)
		}
	case SecurityTypeHTTP:
		if s.Scheme == "" {
			return fmt.Errorf("openapi: http security scheme must set scheme")
		}
	case SecurityTypeMutualTLS:
	case SecurityTypeOAuth2:
		if s.Flows == nil {
			return fmt.Errorf("openapi: oauth2 security scheme must set flows")
		}
	case SecurityTypeOpenIDConnect:
		if s.OpenIDConnectURL == "" {
			return fmt.Errorf("openapi: openIdConnect security scheme must set openIdConnectUrl")
		}
	}
	return nil
}
