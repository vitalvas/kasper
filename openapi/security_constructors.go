package openapi

// BearerAuth returns an HTTP bearer-token SecurityScheme. bearerFormat
// is an optional hint about the token format (e.g., "JWT"); pass an
// empty string to omit it.
//
// See: https://spec.openapis.org/oas/v3.1.0#security-scheme-object
func BearerAuth(bearerFormat string) *SecurityScheme {
	return &SecurityScheme{
		Type:         SecurityTypeHTTP,
		Scheme:       SchemeBearer,
		BearerFormat: bearerFormat,
	}
}

// BasicAuth returns an HTTP basic-authentication SecurityScheme.
//
// See: https://spec.openapis.org/oas/v3.1.0#security-scheme-object
func BasicAuth() *SecurityScheme {
	return &SecurityScheme{
		Type:   SecurityTypeHTTP,
		Scheme: SchemeBasic,
	}
}

// APIKeyAuth returns an apiKey SecurityScheme. in must be one of
// SecurityInHeader, SecurityInQuery, or SecurityInCookie; name is the
// header, query-parameter, or cookie name carrying the key.
//
// See: https://spec.openapis.org/oas/v3.1.0#security-scheme-object
func APIKeyAuth(in, name string) *SecurityScheme {
	return &SecurityScheme{
		Type: SecurityTypeAPIKey,
		In:   in,
		Name: name,
	}
}

// OpenIDConnectAuth returns an openIdConnect SecurityScheme pointing at
// the given OpenID Connect discovery URL.
//
// See: https://spec.openapis.org/oas/v3.1.0#security-scheme-object
func OpenIDConnectAuth(url string) *SecurityScheme {
	return &SecurityScheme{
		Type:             SecurityTypeOpenIDConnect,
		OpenIDConnectURL: url,
	}
}

// RequireScheme returns a SecurityRequirement naming a single security
// scheme and its required scopes. Pass no scopes for schemes that do
// not use them (e.g., HTTP basic or bearer auth).
//
// See: https://spec.openapis.org/oas/v3.1.0#security-requirement-object
func RequireScheme(name string, scopes ...string) SecurityRequirement {
	if scopes == nil {
		scopes = []string{}
	}
	return SecurityRequirement{name: scopes}
}
