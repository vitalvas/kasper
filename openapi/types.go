package openapi

import (
	"encoding/json"
)

// Document represents the root of an OpenAPI v3.1.0 document.
type Document struct {
	OpenAPI           string                `json:"openapi"`
	Info              Info                  `json:"info"`
	JSONSchemaDialect string                `json:"jsonSchemaDialect,omitempty"`
	Servers           []Server              `json:"servers,omitempty"`
	Paths             map[string]*PathItem  `json:"paths,omitempty"`
	Webhooks          map[string]*PathItem  `json:"webhooks,omitempty"`
	Components        *Components           `json:"components,omitempty"`
	Tags              []Tag                 `json:"tags,omitempty"`
	Security          []SecurityRequirement `json:"security,omitempty"`
	ExternalDocs      *ExternalDocs         `json:"externalDocs,omitempty"`
}

// Info provides metadata about the API.
type Info struct {
	Title          string   `json:"title"`
	Summary        string   `json:"summary,omitempty"`
	Description    string   `json:"description,omitempty"`
	TermsOfService string   `json:"termsOfService,omitempty"`
	Contact        *Contact `json:"contact,omitempty"`
	License        *License `json:"license,omitempty"`
	Version        string   `json:"version"`
}

// Contact represents contact information for the API.
type Contact struct {
	Name  string `json:"name,omitempty"`
	URL   string `json:"url,omitempty"`
	Email string `json:"email,omitempty"`
}

// License represents license information for the API.
type License struct {
	Name       string `json:"name"`
	Identifier string `json:"identifier,omitempty"`
	URL        string `json:"url,omitempty"`
}

// Server represents a server.
type Server struct {
	URL         string                     `json:"url"`
	Description string                     `json:"description,omitempty"`
	Variables   map[string]*ServerVariable `json:"variables,omitempty"`
}

// ServerVariable represents a server variable for URL template substitution.
type ServerVariable struct {
	Enum        []string `json:"enum,omitempty"`
	Default     string   `json:"default"`
	Description string   `json:"description,omitempty"`
}

// PathItem describes the operations available on a single path.
type PathItem struct {
	Ref         string       `json:"$ref,omitempty"`
	Summary     string       `json:"summary,omitempty"`
	Description string       `json:"description,omitempty"`
	Get         *Operation   `json:"get,omitempty"`
	Put         *Operation   `json:"put,omitempty"`
	Post        *Operation   `json:"post,omitempty"`
	Delete      *Operation   `json:"delete,omitempty"`
	Options     *Operation   `json:"options,omitempty"`
	Head        *Operation   `json:"head,omitempty"`
	Patch       *Operation   `json:"patch,omitempty"`
	Trace       *Operation   `json:"trace,omitempty"`
	Servers     []Server     `json:"servers,omitempty"`
	Parameters  []*Parameter `json:"parameters,omitempty"`
}

// Operation describes a single API operation on a path.
type Operation struct {
	Tags         []string              `json:"tags,omitempty"`
	Summary      string                `json:"summary,omitempty"`
	Description  string                `json:"description,omitempty"`
	ExternalDocs *ExternalDocs         `json:"externalDocs,omitempty"`
	OperationID  string                `json:"operationId,omitempty"`
	Parameters   []*Parameter          `json:"parameters,omitempty"`
	RequestBody  *RequestBody          `json:"requestBody,omitempty"`
	Responses    map[string]*Response  `json:"responses,omitempty"`
	Callbacks    map[string]*Callback  `json:"callbacks,omitempty"`
	Deprecated   bool                  `json:"deprecated,omitempty"`
	Security     []SecurityRequirement `json:"security,omitempty"`
	Servers      []Server              `json:"servers,omitempty"`
}

// Parameter describes a single operation parameter.
type Parameter struct {
	Name            string                `json:"name"`
	In              string                `json:"in"`
	Description     string                `json:"description,omitempty"`
	Required        bool                  `json:"required,omitempty"`
	Deprecated      bool                  `json:"deprecated,omitempty"`
	AllowEmptyValue bool                  `json:"allowEmptyValue,omitempty"`
	Style           string                `json:"style,omitempty"`
	Explode         *bool                 `json:"explode,omitempty"`
	AllowReserved   bool                  `json:"allowReserved,omitempty"`
	Schema          *Schema               `json:"schema,omitempty"`
	Example         any                   `json:"example,omitempty"`
	Examples        map[string]*Example   `json:"examples,omitempty"`
	Content         map[string]*MediaType `json:"content,omitempty"`
}

// RequestBody describes a single request body.
type RequestBody struct {
	Description string                `json:"description,omitempty"`
	Required    bool                  `json:"required,omitempty"`
	Content     map[string]*MediaType `json:"content,omitempty"`
}

// Response describes a single response from an API operation.
type Response struct {
	Description string                `json:"description"`
	Headers     map[string]*Header    `json:"headers,omitempty"`
	Content     map[string]*MediaType `json:"content,omitempty"`
	Links       map[string]*Link      `json:"links,omitempty"`
}

// MediaType describes a media type with a schema and optional example.
type MediaType struct {
	Schema   *Schema              `json:"schema,omitempty"`
	Example  any                  `json:"example,omitempty"`
	Examples map[string]*Example  `json:"examples,omitempty"`
	Encoding map[string]*Encoding `json:"encoding,omitempty"`
}

// Header describes a single header.
type Header struct {
	Description     string                `json:"description,omitempty"`
	Required        bool                  `json:"required,omitempty"`
	Deprecated      bool                  `json:"deprecated,omitempty"`
	AllowEmptyValue bool                  `json:"allowEmptyValue,omitempty"`
	Style           string                `json:"style,omitempty"`
	Explode         *bool                 `json:"explode,omitempty"`
	AllowReserved   bool                  `json:"allowReserved,omitempty"`
	Schema          *Schema               `json:"schema,omitempty"`
	Example         any                   `json:"example,omitempty"`
	Examples        map[string]*Example   `json:"examples,omitempty"`
	Content         map[string]*MediaType `json:"content,omitempty"`
}

// SchemaType represents a JSON Schema type that can be a single string
// or an array of strings (per JSON Schema Draft 2020-12).
type SchemaType struct {
	value []string
}

// TypeString creates a SchemaType with a single type.
func TypeString(t string) SchemaType {
	return SchemaType{value: []string{t}}
}

// TypeArray creates a SchemaType with multiple types (e.g., ["string", "null"]).
func TypeArray(types ...string) SchemaType {
	return SchemaType{value: types}
}

// Values returns the underlying type values.
func (st SchemaType) Values() []string {
	return st.value
}

// IsEmpty reports whether the schema type is unset.
func (st SchemaType) IsEmpty() bool {
	return len(st.value) == 0
}

// MarshalJSON encodes the schema type as a JSON string (single type)
// or JSON array (multiple types).
func (st SchemaType) MarshalJSON() ([]byte, error) {
	if len(st.value) == 1 {
		return json.Marshal(st.value[0])
	}
	return json.Marshal(st.value)
}

// UnmarshalJSON decodes the schema type from either a JSON string or array.
func (st *SchemaType) UnmarshalJSON(data []byte) error {
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		st.value = []string{single}
		return nil
	}

	var arr []string
	if err := json.Unmarshal(data, &arr); err != nil {
		return err
	}
	st.value = arr
	return nil
}

// Schema represents a JSON Schema object (JSON Schema Draft 2020-12).
type Schema struct {
	// JSON Schema core identifiers.
	ID            string             `json:"$id,omitempty"`
	SchemaURI     string             `json:"$schema,omitempty"`
	Ref           string             `json:"$ref,omitempty"`
	DynamicAnchor string             `json:"$dynamicAnchor,omitempty"`
	Comment       string             `json:"$comment,omitempty"`
	Defs          map[string]*Schema `json:"$defs,omitempty"`

	// Type and format.
	Type   SchemaType `json:"type,omitzero"`
	Format string     `json:"format,omitempty"`

	// Metadata.
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Default     any    `json:"default,omitempty"`
	Example     any    `json:"example,omitempty"`
	Examples    []any  `json:"examples,omitempty"`
	Deprecated  bool   `json:"deprecated,omitempty"`
	ReadOnly    bool   `json:"readOnly,omitempty"`
	WriteOnly   bool   `json:"writeOnly,omitempty"`

	// Numeric constraints.
	MultipleOf       *float64 `json:"multipleOf,omitempty"`
	Minimum          *float64 `json:"minimum,omitempty"`
	Maximum          *float64 `json:"maximum,omitempty"`
	ExclusiveMinimum *float64 `json:"exclusiveMinimum,omitempty"`
	ExclusiveMaximum *float64 `json:"exclusiveMaximum,omitempty"`

	// String constraints.
	MinLength *int   `json:"minLength,omitempty"`
	MaxLength *int   `json:"maxLength,omitempty"`
	Pattern   string `json:"pattern,omitempty"`

	// Array constraints.
	Items            *Schema   `json:"items,omitempty"`
	PrefixItems      []*Schema `json:"prefixItems,omitempty"`
	Contains         *Schema   `json:"contains,omitempty"`
	MinItems         *int      `json:"minItems,omitempty"`
	MaxItems         *int      `json:"maxItems,omitempty"`
	UniqueItems      bool      `json:"uniqueItems,omitempty"`
	UnevaluatedItems *Schema   `json:"unevaluatedItems,omitempty"`

	// Object constraints.
	Properties            map[string]*Schema  `json:"properties,omitempty"`
	PatternProperties     map[string]*Schema  `json:"patternProperties,omitempty"`
	AdditionalProperties  *Schema             `json:"additionalProperties,omitempty"`
	UnevaluatedProperties *Schema             `json:"unevaluatedProperties,omitempty"`
	PropertyNames         *Schema             `json:"propertyNames,omitempty"`
	Required              []string            `json:"required,omitempty"`
	MinProperties         *int                `json:"minProperties,omitempty"`
	MaxProperties         *int                `json:"maxProperties,omitempty"`
	DependentRequired     map[string][]string `json:"dependentRequired,omitempty"`
	DependentSchemas      map[string]*Schema  `json:"dependentSchemas,omitempty"`

	// Enum and const.
	Enum  []any `json:"enum,omitempty"`
	Const any   `json:"const,omitzero"`

	// Composition.
	AllOf []*Schema `json:"allOf,omitempty"`
	OneOf []*Schema `json:"oneOf,omitempty"`
	AnyOf []*Schema `json:"anyOf,omitempty"`
	Not   *Schema   `json:"not,omitempty"`

	// Conditional.
	If   *Schema `json:"if,omitempty"`
	Then *Schema `json:"then,omitempty"`
	Else *Schema `json:"else,omitempty"`

	// Content encoding.
	ContentEncoding  string  `json:"contentEncoding,omitempty"`
	ContentMediaType string  `json:"contentMediaType,omitempty"`
	ContentSchema    *Schema `json:"contentSchema,omitempty"`

	// OpenAPI extensions.
	Discriminator *Discriminator `json:"discriminator,omitempty"`
	ExternalDocs  *ExternalDocs  `json:"externalDocs,omitempty"`
	XML           *XML           `json:"xml,omitempty"`
}

// Components holds reusable OpenAPI objects.
type Components struct {
	Schemas         map[string]*Schema         `json:"schemas,omitempty"`
	Responses       map[string]*Response       `json:"responses,omitempty"`
	Parameters      map[string]*Parameter      `json:"parameters,omitempty"`
	Examples        map[string]*Example        `json:"examples,omitempty"`
	RequestBodies   map[string]*RequestBody    `json:"requestBodies,omitempty"`
	Headers         map[string]*Header         `json:"headers,omitempty"`
	SecuritySchemes map[string]*SecurityScheme `json:"securitySchemes,omitempty"`
	Links           map[string]*Link           `json:"links,omitempty"`
	Callbacks       map[string]*Callback       `json:"callbacks,omitempty"`
	PathItems       map[string]*PathItem       `json:"pathItems,omitempty"`
}

// Tag adds metadata to a single tag.
type Tag struct {
	Name         string        `json:"name"`
	Description  string        `json:"description,omitempty"`
	ExternalDocs *ExternalDocs `json:"externalDocs,omitempty"`
}

// SecurityRequirement lists required security schemes for an operation.
type SecurityRequirement map[string][]string

// ExternalDocs allows referencing external documentation.
type ExternalDocs struct {
	Description string `json:"description,omitempty"`
	URL         string `json:"url"`
}

// Example represents an example value.
type Example struct {
	Summary       string `json:"summary,omitempty"`
	Description   string `json:"description,omitempty"`
	Value         any    `json:"value,omitempty"`
	ExternalValue string `json:"externalValue,omitempty"`
}

// Encoding describes encoding for a single property in a media type.
type Encoding struct {
	ContentType   string             `json:"contentType,omitempty"`
	Headers       map[string]*Header `json:"headers,omitempty"`
	Style         string             `json:"style,omitempty"`
	Explode       *bool              `json:"explode,omitempty"`
	AllowReserved bool               `json:"allowReserved,omitempty"`
}

// Discriminator aids in serialization, deserialization, and validation
// when request bodies or response payloads may be one of several schemas.
type Discriminator struct {
	PropertyName string            `json:"propertyName"`
	Mapping      map[string]string `json:"mapping,omitempty"`
}

// XML describes XML-specific metadata.
type XML struct {
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Prefix    string `json:"prefix,omitempty"`
	Attribute bool   `json:"attribute,omitempty"`
	Wrapped   bool   `json:"wrapped,omitempty"`
}

// SecurityScheme defines a security scheme used by API operations.
type SecurityScheme struct {
	Type             string      `json:"type"`
	Description      string      `json:"description,omitempty"`
	Name             string      `json:"name,omitempty"`
	In               string      `json:"in,omitempty"`
	Scheme           string      `json:"scheme,omitempty"`
	BearerFormat     string      `json:"bearerFormat,omitempty"`
	Flows            *OAuthFlows `json:"flows,omitempty"`
	OpenIDConnectURL string      `json:"openIdConnectUrl,omitempty"`
}

// OAuthFlows describes the available OAuth2 flows.
type OAuthFlows struct {
	Implicit          *OAuthFlow `json:"implicit,omitempty"`
	Password          *OAuthFlow `json:"password,omitempty"`
	ClientCredentials *OAuthFlow `json:"clientCredentials,omitempty"`
	AuthorizationCode *OAuthFlow `json:"authorizationCode,omitempty"`
}

// OAuthFlow describes a single OAuth2 flow.
type OAuthFlow struct {
	AuthorizationURL string            `json:"authorizationUrl,omitempty"`
	TokenURL         string            `json:"tokenUrl,omitempty"`
	RefreshURL       string            `json:"refreshUrl,omitempty"`
	Scopes           map[string]string `json:"scopes"`
}

// Link represents a possible design-time link for a response.
type Link struct {
	OperationRef string         `json:"operationRef,omitempty"`
	OperationID  string         `json:"operationId,omitempty"`
	Parameters   map[string]any `json:"parameters,omitempty"`
	RequestBody  any            `json:"requestBody,omitempty"`
	Description  string         `json:"description,omitempty"`
	Server       *Server        `json:"server,omitempty"`
}

// Callback is a map of runtime expressions to path items,
// describing requests that may be initiated by the API provider.
type Callback map[string]*PathItem
