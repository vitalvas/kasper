package openapi

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

// Document represents the root of an OpenAPI v3.1.0 document.
//
// See: https://spec.openapis.org/oas/v3.1.0#openapi-object
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
//
// See: https://spec.openapis.org/oas/v3.1.0#info-object
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
//
// See: https://spec.openapis.org/oas/v3.1.0#contact-object
type Contact struct {
	Name  string `json:"name,omitempty"`
	URL   string `json:"url,omitempty"`
	Email string `json:"email,omitempty"`
}

// License represents license information for the API.
//
// See: https://spec.openapis.org/oas/v3.1.0#license-object
type License struct {
	Name       string `json:"name"`
	Identifier string `json:"identifier,omitempty"`
	URL        string `json:"url,omitempty"`
}

// Server represents a server.
//
// See: https://spec.openapis.org/oas/v3.1.0#server-object
type Server struct {
	URL         string                     `json:"url"`
	Description string                     `json:"description,omitempty"`
	Variables   map[string]*ServerVariable `json:"variables,omitempty"`
}

// ServerVariable represents a server variable for URL template substitution.
//
// See: https://spec.openapis.org/oas/v3.1.0#server-variable-object
type ServerVariable struct {
	Enum        []string `json:"enum,omitempty"`
	Default     string   `json:"default"`
	Description string   `json:"description,omitempty"`
}

// PathItem describes the operations available on a single path.
//
// See: https://spec.openapis.org/oas/v3.1.0#path-item-object
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
//
// See: https://spec.openapis.org/oas/v3.1.0#operation-object
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
// The "in" field determines the parameter location: "query", "header",
// "path", or "cookie". Parameters with the same name and location
// must be unique within an operation.
//
// See: https://spec.openapis.org/oas/v3.1.0#parameter-object
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
//
// See: https://spec.openapis.org/oas/v3.1.0#request-body-object
type RequestBody struct {
	Description string                `json:"description,omitempty"`
	Required    bool                  `json:"required,omitempty"`
	Content     map[string]*MediaType `json:"content,omitempty"`
}

// Response describes a single response from an API operation.
// The description field is REQUIRED per the specification.
//
// See: https://spec.openapis.org/oas/v3.1.0#response-object
type Response struct {
	Description string                `json:"description"`
	Headers     map[string]*Header    `json:"headers,omitempty"`
	Content     map[string]*MediaType `json:"content,omitempty"`
	Links       map[string]*Link      `json:"links,omitempty"`
}

// MediaType describes a media type with a schema and optional example.
// Each Media Type Object is keyed by its MIME type (e.g., "application/json")
// inside a content map.
//
// See: https://spec.openapis.org/oas/v3.1.0#media-type-object
type MediaType struct {
	Schema   *Schema              `json:"schema,omitempty"`
	Example  any                  `json:"example,omitempty"`
	Examples map[string]*Example  `json:"examples,omitempty"`
	Encoding map[string]*Encoding `json:"encoding,omitempty"`
}

// Header describes a single header. Header Object follows the same structure
// as Parameter Object with the following differences: name is specified in
// the key of the containing map and "in" is implicitly "header".
//
// See: https://spec.openapis.org/oas/v3.1.0#header-object
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
// or an array of strings (per JSON Schema Draft 2020-12, section 6.1.1).
//
// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-6.1.1
type SchemaType struct {
	value []string
}

// TypeString creates a SchemaType with a single type.
//
// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-6.1.1
func TypeString(t string) SchemaType {
	return SchemaType{value: []string{t}}
}

// TypeArray creates a SchemaType with multiple types (e.g., ["string", "null"]).
// Used for nullable types per JSON Schema Draft 2020-12.
//
// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-6.1.1
func TypeArray(types ...string) SchemaType {
	return SchemaType{value: types}
}

// Values returns the underlying type values.
//
// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-6.1.1
func (st SchemaType) Values() []string {
	return st.value
}

// IsEmpty reports whether the schema type is unset.
//
// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-6.1.1
func (st SchemaType) IsEmpty() bool {
	return len(st.value) == 0
}

// IsZero implements the yaml.v3 IsZeroer interface so that
// omitempty on YAML struct tags correctly omits an unset type field.
//
// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-6.1.1
func (st SchemaType) IsZero() bool {
	return len(st.value) == 0
}

// MarshalJSON encodes the schema type as a JSON string (single type)
// or JSON array (multiple types).
//
// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-6.1.1
func (st SchemaType) MarshalJSON() ([]byte, error) {
	if len(st.value) == 1 {
		return json.Marshal(st.value[0])
	}
	return json.Marshal(st.value)
}

// UnmarshalJSON decodes the schema type from either a JSON string or array.
//
// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-6.1.1
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

// MarshalYAML encodes the schema type as a YAML scalar (single type)
// or YAML sequence (multiple types).
//
// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-6.1.1
func (st SchemaType) MarshalYAML() (any, error) {
	switch len(st.value) {
	case 0:
		return nil, nil
	case 1:
		return st.value[0], nil
	default:
		return st.value, nil
	}
}

// UnmarshalYAML decodes the schema type from either a YAML scalar or sequence.
//
// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-6.1.1
func (st *SchemaType) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		st.value = []string{node.Value}
		return nil
	case yaml.SequenceNode:
		var arr []string
		if err := node.Decode(&arr); err != nil {
			return err
		}
		st.value = arr
		return nil
	default:
		return fmt.Errorf("unsupported YAML node kind %d for SchemaType", node.Kind)
	}
}

// Schema represents a JSON Schema object used in OpenAPI v3.1.0.
// OpenAPI 3.1.0 aligns fully with JSON Schema Draft 2020-12 and adds
// a few OpenAPI-specific keywords (discriminator, externalDocs, xml).
//
// See: https://spec.openapis.org/oas/v3.1.0#schema-object
// See: https://json-schema.org/draft/2020-12/json-schema-core
// See: https://json-schema.org/draft/2020-12/json-schema-validation
type Schema struct {
	// JSON Schema core identifiers (Draft 2020-12, section 8).
	// See: https://json-schema.org/draft/2020-12/json-schema-core#section-8
	ID            string             `json:"$id,omitempty"`
	SchemaURI     string             `json:"$schema,omitempty"`
	Ref           string             `json:"$ref,omitempty"`
	DynamicAnchor string             `json:"$dynamicAnchor,omitempty"`
	Comment       string             `json:"$comment,omitempty"`
	Defs          map[string]*Schema `json:"$defs,omitempty"`

	// Type and format.
	// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-6.1.1
	// See: https://spec.openapis.org/oas/v3.1.0#data-types
	Type   SchemaType `json:"type,omitzero" yaml:"type,omitempty"`
	Format string     `json:"format,omitempty"`

	// Metadata annotations.
	// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-9
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Default     any    `json:"default,omitempty"`
	Example     any    `json:"example,omitempty"`
	Examples    []any  `json:"examples,omitempty"`
	Deprecated  bool   `json:"deprecated,omitempty"`
	ReadOnly    bool   `json:"readOnly,omitempty"`
	WriteOnly   bool   `json:"writeOnly,omitempty"`

	// Numeric constraints.
	// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-6.2
	MultipleOf       *float64 `json:"multipleOf,omitempty"`
	Minimum          *float64 `json:"minimum,omitempty"`
	Maximum          *float64 `json:"maximum,omitempty"`
	ExclusiveMinimum *float64 `json:"exclusiveMinimum,omitempty"`
	ExclusiveMaximum *float64 `json:"exclusiveMaximum,omitempty"`

	// String constraints.
	// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-6.3
	MinLength *int   `json:"minLength,omitempty"`
	MaxLength *int   `json:"maxLength,omitempty"`
	Pattern   string `json:"pattern,omitempty"`

	// Array constraints.
	// See: https://json-schema.org/draft/2020-12/json-schema-core#section-10.3.1
	// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-6.4
	Items            *Schema   `json:"items,omitempty"`
	PrefixItems      []*Schema `json:"prefixItems,omitempty"`
	Contains         *Schema   `json:"contains,omitempty"`
	MinItems         *int      `json:"minItems,omitempty"`
	MaxItems         *int      `json:"maxItems,omitempty"`
	UniqueItems      bool      `json:"uniqueItems,omitempty"`
	UnevaluatedItems *Schema   `json:"unevaluatedItems,omitempty"`

	// Object constraints.
	// See: https://json-schema.org/draft/2020-12/json-schema-core#section-10.3.2
	// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-6.5
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
	// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-6.1.2
	// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-6.1.3
	Enum  []any `json:"enum,omitempty"`
	Const any   `json:"const,omitzero"`

	// Composition keywords.
	// See: https://json-schema.org/draft/2020-12/json-schema-core#section-10.2.1
	AllOf []*Schema `json:"allOf,omitempty"`
	OneOf []*Schema `json:"oneOf,omitempty"`
	AnyOf []*Schema `json:"anyOf,omitempty"`
	Not   *Schema   `json:"not,omitempty"`

	// Conditional subschemas.
	// See: https://json-schema.org/draft/2020-12/json-schema-core#section-10.2.2
	If   *Schema `json:"if,omitempty"`
	Then *Schema `json:"then,omitempty"`
	Else *Schema `json:"else,omitempty"`

	// Content encoding.
	// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-8
	ContentEncoding  string  `json:"contentEncoding,omitempty"`
	ContentMediaType string  `json:"contentMediaType,omitempty"`
	ContentSchema    *Schema `json:"contentSchema,omitempty"`

	// OpenAPI-specific extensions to JSON Schema.
	// See: https://spec.openapis.org/oas/v3.1.0#fixed-fields-20
	Discriminator *Discriminator `json:"discriminator,omitempty"`
	ExternalDocs  *ExternalDocs  `json:"externalDocs,omitempty"`
	XML           *XML           `json:"xml,omitempty"`
}

// Components holds reusable OpenAPI objects. All objects defined within the
// Components Object have no effect on the API unless explicitly referenced
// from outside the Components Object.
//
// See: https://spec.openapis.org/oas/v3.1.0#components-object
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

// Tag adds metadata to a single tag used by Operation Objects.
//
// See: https://spec.openapis.org/oas/v3.1.0#tag-object
type Tag struct {
	Name         string        `json:"name"`
	Description  string        `json:"description,omitempty"`
	ExternalDocs *ExternalDocs `json:"externalDocs,omitempty"`
}

// SecurityRequirement lists required security schemes for an operation.
// Each key maps to a list of scope names required for execution (can be
// empty for schemes not using scopes, such as HTTP basic auth).
//
// See: https://spec.openapis.org/oas/v3.1.0#security-requirement-object
type SecurityRequirement map[string][]string

// ExternalDocs allows referencing external documentation.
//
// See: https://spec.openapis.org/oas/v3.1.0#external-documentation-object
type ExternalDocs struct {
	Description string `json:"description,omitempty"`
	URL         string `json:"url"`
}

// Example represents an example value. The value field and externalValue
// field are mutually exclusive.
//
// See: https://spec.openapis.org/oas/v3.1.0#example-object
type Example struct {
	Summary       string `json:"summary,omitempty"`
	Description   string `json:"description,omitempty"`
	Value         any    `json:"value,omitempty"`
	ExternalValue string `json:"externalValue,omitempty"`
}

// Encoding describes encoding for a single property in a media type.
// Only applies to Request Body Objects when the media type is
// "multipart" or "application/x-www-form-urlencoded".
//
// See: https://spec.openapis.org/oas/v3.1.0#encoding-object
type Encoding struct {
	ContentType   string             `json:"contentType,omitempty"`
	Headers       map[string]*Header `json:"headers,omitempty"`
	Style         string             `json:"style,omitempty"`
	Explode       *bool              `json:"explode,omitempty"`
	AllowReserved bool               `json:"allowReserved,omitempty"`
}

// Discriminator aids in serialization, deserialization, and validation
// when request bodies or response payloads may be one of several schemas.
// Used with oneOf, anyOf, or allOf composition keywords.
//
// See: https://spec.openapis.org/oas/v3.1.0#discriminator-object
type Discriminator struct {
	PropertyName string            `json:"propertyName"`
	Mapping      map[string]string `json:"mapping,omitempty"`
}

// XML describes XML-specific metadata for properties, used when
// producing XML output.
//
// See: https://spec.openapis.org/oas/v3.1.0#xml-object
type XML struct {
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Prefix    string `json:"prefix,omitempty"`
	Attribute bool   `json:"attribute,omitempty"`
	Wrapped   bool   `json:"wrapped,omitempty"`
}

// SecurityScheme defines a security scheme used by API operations.
// The "type" field determines the scheme: "apiKey", "http",
// "mutualTLS", "oauth2", or "openIdConnect".
//
// See: https://spec.openapis.org/oas/v3.1.0#security-scheme-object
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
//
// See: https://spec.openapis.org/oas/v3.1.0#oauth-flows-object
type OAuthFlows struct {
	Implicit          *OAuthFlow `json:"implicit,omitempty"`
	Password          *OAuthFlow `json:"password,omitempty"`
	ClientCredentials *OAuthFlow `json:"clientCredentials,omitempty"`
	AuthorizationCode *OAuthFlow `json:"authorizationCode,omitempty"`
}

// OAuthFlow describes a single OAuth2 flow configuration.
//
// See: https://spec.openapis.org/oas/v3.1.0#oauth-flow-object
type OAuthFlow struct {
	AuthorizationURL string            `json:"authorizationUrl,omitempty"`
	TokenURL         string            `json:"tokenUrl,omitempty"`
	RefreshURL       string            `json:"refreshUrl,omitempty"`
	Scopes           map[string]string `json:"scopes"`
}

// Link represents a possible design-time link for a response.
// Links provide a known relationship and traversal mechanism between
// responses and other operations.
//
// See: https://spec.openapis.org/oas/v3.1.0#link-object
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
// The key expression is evaluated at runtime to identify the URL for the callback.
//
// See: https://spec.openapis.org/oas/v3.1.0#callback-object
type Callback map[string]*PathItem
