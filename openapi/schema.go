package openapi

import (
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Exampler can be implemented by types to provide an example value
// for the generated JSON Schema. The returned value is set as the "example"
// field on the component schema.
//
//	func (u User) OpenAPIExample() any {
//	    return User{ID: "550e8400-e29b-41d4-a716-446655440000", Name: "Alice"}
//	}
//
// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-9.5
type Exampler interface {
	OpenAPIExample() any
}

// SchemaGenerator converts Go types to JSON Schema objects and collects
// named types into a component schemas map for $ref deduplication.
//
// See: https://spec.openapis.org/oas/v3.1.0#schema-object
// See: https://spec.openapis.org/oas/v3.1.0#components-object (schemas)
type SchemaGenerator struct {
	schemas   map[string]*Schema
	visited   map[reflect.Type]bool
	typeNames map[reflect.Type]string // type -> chosen schema name
	nameTypes map[string]reflect.Type // schema name -> type that claimed it
}

// NewSchemaGenerator creates a new schema generator.
//
// See: https://spec.openapis.org/oas/v3.1.0#schema-object
// See: https://spec.openapis.org/oas/v3.1.0#components-object (schemas)
func NewSchemaGenerator() *SchemaGenerator {
	return &SchemaGenerator{
		schemas:   make(map[string]*Schema),
		visited:   make(map[reflect.Type]bool),
		typeNames: make(map[reflect.Type]string),
		nameTypes: make(map[string]reflect.Type),
	}
}

// Schemas returns the collected component schemas.
//
// See: https://spec.openapis.org/oas/v3.1.0#components-object (schemas)
func (g *SchemaGenerator) Schemas() map[string]*Schema {
	return g.schemas
}

// Generate produces a JSON Schema for the given Go value.
// Named struct types are stored in the generator's component schemas
// and referenced via $ref.
//
// See: https://spec.openapis.org/oas/v3.1.0#schema-object
// See: https://json-schema.org/draft/2020-12/json-schema-core#section-8.2.3 ($ref)
func (g *SchemaGenerator) Generate(v any) *Schema {
	if v == nil {
		return nil
	}
	return g.generateType(reflect.TypeOf(v))
}

// generateType produces a Schema for the given Go type, using $ref for named struct
// types and inline schemas for primitives, slices, maps, and anonymous structs.
//
// See: https://spec.openapis.org/oas/v3.1.0#schema-object
// See: https://json-schema.org/draft/2020-12/json-schema-core#section-8.2.3 ($ref)
func (g *SchemaGenerator) generateType(t reflect.Type) *Schema {
	// Unwrap pointer and mark nullable.
	nullable := false
	if t.Kind() == reflect.Pointer {
		nullable = true
		t = t.Elem()
	}

	// Named struct types → $ref (except time.Time which is a special case).
	if t.Kind() == reflect.Struct && t != reflect.TypeOf(time.Time{}) {
		name := g.schemaName(t)
		if name != "" {
			// Generate the schema if not already visited.
			if !g.visited[t] {
				g.visited[t] = true
				schema := g.generateStructSchema(t)

				// Check if the type implements Exampler.
				if ex, ok := reflect.New(t).Interface().(Exampler); ok {
					schema.Example = ex.OpenAPIExample()
				}

				g.schemas[name] = schema
			}

			ref := &Schema{Ref: "#/components/schemas/" + name}
			if nullable {
				return &Schema{
					AnyOf: []*Schema{
						ref,
						{Type: TypeString("null")},
					},
				}
			}
			return ref
		}
	}

	schema := g.generateInlineType(t)
	if nullable && schema != nil {
		applyNullable(schema)
	}
	return schema
}

// generateInlineType maps Go primitive and composite types to JSON Schema types.
//
// See: https://spec.openapis.org/oas/v3.1.0#data-types
// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-6.1.1
func (g *SchemaGenerator) generateInlineType(t reflect.Type) *Schema {
	// Special cases first.
	if t == reflect.TypeOf(time.Time{}) {
		return &Schema{Type: TypeString("string"), Format: "date-time"}
	}

	switch t.Kind() {
	case reflect.Bool:
		return &Schema{Type: TypeString("boolean")}

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &Schema{Type: TypeString("integer")}

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &Schema{Type: TypeString("integer")}

	case reflect.Float32, reflect.Float64:
		return &Schema{Type: TypeString("number")}

	case reflect.String:
		return &Schema{Type: TypeString("string")}

	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			return &Schema{Type: TypeString("string"), Format: "byte"}
		}
		return &Schema{
			Type:  TypeString("array"),
			Items: g.generateType(t.Elem()),
		}

	case reflect.Array:
		return &Schema{
			Type:  TypeString("array"),
			Items: g.generateType(t.Elem()),
		}

	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return &Schema{Type: TypeString("object")}
		}
		return &Schema{
			Type:                 TypeString("object"),
			AdditionalProperties: g.generateType(t.Elem()),
		}

	case reflect.Struct:
		return g.generateStructSchema(t)

	case reflect.Interface:
		return &Schema{}
	}

	return nil
}

// generateStructSchema builds an object schema from struct fields.
//
// See: https://json-schema.org/draft/2020-12/json-schema-core#section-10.3.2 (properties)
// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-6.5.3 (required)
func (g *SchemaGenerator) generateStructSchema(t reflect.Type) *Schema {
	schema := &Schema{
		Type:       TypeString("object"),
		Properties: make(map[string]*Schema),
	}

	g.collectFields(t, schema, false)

	if len(schema.Properties) == 0 {
		schema.Properties = nil
	}

	return schema
}

// collectFields recursively collects struct fields into the schema.
// When allOptional is true, all fields are treated as optional regardless
// of their json tags. This is used for pointer-embedded structs where the
// entire embedded struct can be nil and thus all its fields may be absent.
//
// See: https://json-schema.org/draft/2020-12/json-schema-core#section-10.3.2.1 (properties)
// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-6.5.3 (required)
func (g *SchemaGenerator) collectFields(t reflect.Type, schema *Schema, allOptional bool) {
	for i := range t.NumField() {
		field := t.Field(i)

		// Skip unexported fields.
		if !field.IsExported() {
			continue
		}

		// Handle embedded structs: inline only when the field has no
		// explicit json tag name. encoding/json treats an anonymous field
		// with a tag name as a regular named field, not inlined.
		if field.Anonymous {
			jsonName, _ := parseJSONTag(field.Tag.Get("json"))
			if jsonName == "" {
				ft := field.Type
				isPtr := ft.Kind() == reflect.Pointer
				if isPtr {
					ft = ft.Elem()
				}
				if ft.Kind() == reflect.Struct {
					// Pointer-embedded structs: all inlined fields become
					// optional because the pointer can be nil, omitting
					// all fields from JSON output.
					g.collectFields(ft, schema, allOptional || isPtr)
					continue
				}
			}
		}

		jsonTag := field.Tag.Get("json")
		if jsonTag == "-" {
			continue
		}

		name, opts := parseJSONTag(jsonTag)
		if name == "" {
			name = field.Name
		}

		fieldSchema := g.generateType(field.Type)
		if fieldSchema == nil {
			continue
		}

		applyOpenAPITag(fieldSchema, field.Tag.Get("openapi"))

		// The encoding/json ",string" option encodes numeric and boolean
		// values as JSON strings. Override the schema type accordingly.
		if opts.stringEncode && fieldSchema.Ref == "" && len(fieldSchema.AnyOf) == 0 {
			applyStringEncoding(fieldSchema)
		}

		schema.Properties[name] = fieldSchema

		if !opts.omitempty && !allOptional {
			schema.Required = append(schema.Required, name)
		}
	}
}

type jsonTagOpts struct {
	omitempty    bool
	stringEncode bool // encoding/json ",string" option
}

func parseJSONTag(tag string) (string, jsonTagOpts) {
	if tag == "" {
		return "", jsonTagOpts{}
	}
	name, rest, _ := strings.Cut(tag, ",")
	return name, jsonTagOpts{
		omitempty:    strings.Contains(rest, "omitempty") || strings.Contains(rest, "omitzero"),
		stringEncode: strings.Contains(rest, "string"),
	}
}

// applyOpenAPITag parses the `openapi` struct tag and applies constraints to the schema.
// Tag keys map to JSON Schema and OpenAPI Schema Object keywords.
//
// See: https://spec.openapis.org/oas/v3.1.0#schema-object
// See: https://json-schema.org/draft/2020-12/json-schema-validation
func applyOpenAPITag(schema *Schema, tag string) {
	if tag == "" {
		return
	}

	for part := range strings.SplitSeq(tag, ",") {
		key, value, hasValue := strings.Cut(part, "=")
		key = strings.TrimSpace(key)
		if hasValue {
			value = strings.TrimSpace(value)
		}

		switch key {
		case "description":
			schema.Description = value
		case "example":
			schema.Example = parseExampleValue(schema, value)
		case "format":
			schema.Format = value
		case "minimum":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				schema.Minimum = &v
			}
		case "maximum":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				schema.Maximum = &v
			}
		case "exclusiveMinimum":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				schema.ExclusiveMinimum = &v
			}
		case "exclusiveMaximum":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				schema.ExclusiveMaximum = &v
			}
		case "minLength":
			if v, err := strconv.Atoi(value); err == nil {
				schema.MinLength = &v
			}
		case "maxLength":
			if v, err := strconv.Atoi(value); err == nil {
				schema.MaxLength = &v
			}
		case "pattern":
			schema.Pattern = value
		case "enum":
			values := strings.Split(value, "|")
			schema.Enum = make([]any, len(values))
			for i, v := range values {
				schema.Enum[i] = v
			}
		case "deprecated":
			schema.Deprecated = true
		case "readOnly":
			schema.ReadOnly = true
		case "writeOnly":
			schema.WriteOnly = true
		case "title":
			schema.Title = value
		case "multipleOf":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				schema.MultipleOf = &v
			}
		case "minItems":
			if v, err := strconv.Atoi(value); err == nil {
				schema.MinItems = &v
			}
		case "maxItems":
			if v, err := strconv.Atoi(value); err == nil {
				schema.MaxItems = &v
			}
		case "uniqueItems":
			schema.UniqueItems = true
		case "minProperties":
			if v, err := strconv.Atoi(value); err == nil {
				schema.MinProperties = &v
			}
		case "maxProperties":
			if v, err := strconv.Atoi(value); err == nil {
				schema.MaxProperties = &v
			}
		case "const":
			schema.Const = parseExampleValue(schema, value)
		}
	}
}

// parseExampleValue converts a string tag value to the appropriate Go type
// based on the schema's type field.
//
// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-9.5
func parseExampleValue(schema *Schema, value string) any {
	types := schema.Type.Values()
	if len(types) == 0 {
		return value
	}

	switch types[0] {
	case "integer":
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			return v
		}
	case "number":
		if v, err := strconv.ParseFloat(value, 64); err == nil {
			return v
		}
	case "boolean":
		if v, err := strconv.ParseBool(value); err == nil {
			return v
		}
	}
	return value
}

// schemaName returns a unique schema name for the given type. If two types
// from different packages share the same simple name (e.g., models.User and
// api.User), the second type gets a qualified name using its package's last
// path segment as a prefix (e.g., "ApiUser"). When the prefixed name still
// collides (e.g., three packages with the same suffix, or generic
// instantiations from the same package with different type arguments that
// sanitize to the same name), a numeric suffix is appended (e.g., "ApiUser2").
// Names are used as keys in the Components Object schemas map and in $ref URIs.
//
// See: https://spec.openapis.org/oas/v3.1.0#components-object (schemas)
// See: https://json-schema.org/draft/2020-12/json-schema-core#section-8.2.3 ($ref)
func (g *SchemaGenerator) schemaName(t reflect.Type) string {
	simple := sanitizeSchemaName(t.Name())
	if simple == "" || t.PkgPath() == "" {
		return ""
	}

	if name, ok := g.typeNames[t]; ok {
		return name
	}

	name := simple
	if existing, ok := g.nameTypes[name]; ok && existing != t {
		name = pkgPrefix(t.PkgPath()) + simple
		// If the prefixed name still collides, append a numeric suffix.
		if existing, ok := g.nameTypes[name]; ok && existing != t {
			base := name
			for i := 2; ; i++ {
				candidate := base + strconv.Itoa(i)
				if _, ok := g.nameTypes[candidate]; !ok {
					name = candidate
					break
				}
			}
		}
	}

	g.typeNames[t] = name
	g.nameTypes[name] = t
	return name
}

// pkgPrefix extracts the last segment of a Go package path and capitalizes
// it for use as a schema name prefix (e.g., "net/http" -> "Http").
//
// See: https://spec.openapis.org/oas/v3.1.0#components-object (schemas)
func pkgPrefix(pkgPath string) string {
	if idx := strings.LastIndexByte(pkgPath, '/'); idx >= 0 {
		pkgPath = pkgPath[idx+1:]
	}
	if len(pkgPath) == 0 {
		return ""
	}
	pkgPath = strings.ReplaceAll(pkgPath, "-", "_")
	pkgPath = strings.ReplaceAll(pkgPath, ".", "_")
	return strings.ToUpper(pkgPath[:1]) + pkgPath[1:]
}

// sanitizeSchemaName cleans up Go type names for use as OpenAPI component
// schema keys. Generic type names like "ResponseData[User]" are converted
// to "ResponseDataUser", and "ResponseData[[]User]" becomes
// "ResponseDataUserList". Package paths in type parameters are stripped.
//
// See: https://spec.openapis.org/oas/v3.1.0#components-object (schemas)
func sanitizeSchemaName(name string) string {
	idx := strings.IndexByte(name, '[')
	if idx < 0 {
		return name
	}

	base := name[:idx]
	inner := name[idx+1 : len(name)-1]

	isList := strings.HasPrefix(inner, "[]")
	inner = strings.TrimPrefix(inner, "[]")

	// Strip package path: "github.com/foo/bar.User" → "User".
	if dot := strings.LastIndexByte(inner, '.'); dot >= 0 {
		inner = inner[dot+1:]
	}

	result := base + inner
	if isList {
		result += "List"
	}

	return result
}

// applyNullable modifies a schema to allow null values by converting
// the type to an array (e.g., "string" becomes ["string", "null"]).
// In JSON Schema Draft 2020-12, nullable is expressed via type arrays
// rather than the OpenAPI 3.0 "nullable" keyword.
//
// See: https://json-schema.org/draft/2020-12/json-schema-validation#section-6.1.1
func applyNullable(schema *Schema) {
	if schema.Ref != "" {
		return
	}
	types := schema.Type.Values()
	if len(types) > 0 {
		schema.Type = TypeArray(append(types, "null")...)
	}
}

// applyStringEncoding overrides the schema type to "string" to match the
// encoding/json ",string" tag option, which encodes numeric and boolean
// values as JSON strings. Nullable types preserve the "null" variant.
//
// See: https://spec.openapis.org/oas/v3.1.0#data-types
func applyStringEncoding(schema *Schema) {
	types := schema.Type.Values()
	if len(types) == 0 {
		return
	}
	var hasNull bool
	for _, t := range types {
		if t == "null" {
			hasNull = true
			break
		}
	}
	if hasNull {
		schema.Type = TypeArray("string", "null")
	} else {
		schema.Type = TypeString("string")
	}
}
