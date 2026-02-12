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
type Exampler interface {
	OpenAPIExample() any
}

// SchemaGenerator converts Go types to JSON Schema objects and collects
// named types into a component schemas map for $ref deduplication.
type SchemaGenerator struct {
	schemas map[string]*Schema
	visited map[reflect.Type]bool
}

// NewSchemaGenerator creates a new schema generator.
func NewSchemaGenerator() *SchemaGenerator {
	return &SchemaGenerator{
		schemas: make(map[string]*Schema),
		visited: make(map[reflect.Type]bool),
	}
}

// Schemas returns the collected component schemas.
func (g *SchemaGenerator) Schemas() map[string]*Schema {
	return g.schemas
}

// Generate produces a JSON Schema for the given Go value.
// Named struct types are stored in the generator's component schemas
// and referenced via $ref.
func (g *SchemaGenerator) Generate(v any) *Schema {
	if v == nil {
		return nil
	}
	return g.generateType(reflect.TypeOf(v))
}

func (g *SchemaGenerator) generateType(t reflect.Type) *Schema {
	// Unwrap pointer and mark nullable.
	nullable := false
	if t.Kind() == reflect.Pointer {
		nullable = true
		t = t.Elem()
	}

	// Named struct types → $ref (except time.Time which is a special case).
	if t.Kind() == reflect.Struct && t != reflect.TypeOf(time.Time{}) {
		name := sanitizeSchemaName(t.Name())
		if name != "" && t.PkgPath() != "" {
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

		schema.Properties[name] = fieldSchema

		if !opts.omitempty && !allOptional {
			schema.Required = append(schema.Required, name)
		}
	}
}

type jsonTagOpts struct {
	omitempty bool
}

func parseJSONTag(tag string) (string, jsonTagOpts) {
	if tag == "" {
		return "", jsonTagOpts{}
	}
	name, rest, _ := strings.Cut(tag, ",")
	return name, jsonTagOpts{
		omitempty: strings.Contains(rest, "omitempty") || strings.Contains(rest, "omitzero"),
	}
}

// applyOpenAPITag parses the `openapi` struct tag and applies constraints to the schema.
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

// sanitizeSchemaName cleans up Go type names for use as OpenAPI component
// schema keys. Generic type names like "ResponseData[User]" are converted
// to "ResponseDataUser", and "ResponseData[[]User]" becomes
// "ResponseDataUserList". Package paths in type parameters are stripped.
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
func applyNullable(schema *Schema) {
	if schema.Ref != "" {
		return
	}
	types := schema.Type.Values()
	if len(types) > 0 {
		schema.Type = TypeArray(append(types, "null")...)
	}
}
