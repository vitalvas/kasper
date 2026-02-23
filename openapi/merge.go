package openapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// MergeDocuments combines multiple OpenAPI documents into a single document.
// The caller-provided Info is set on the result. Paths, webhooks, and all
// component maps are merged. Duplicate paths or webhooks produce a conflict
// error. Component entries with the same name are deduplicated when their
// normalized JSON representations are identical (string-only arrays such as
// "required" are sorted before comparison); differing entries produce a
// conflict error. Tags are deduplicated by name (first non-empty description
// wins) and sorted alphabetically. Security requirements are unioned and
// deduplicated (scope ordering is ignored). Servers from source documents
// are dropped.
//
// All conflicts are collected and returned in a single error (not fail-fast).
//
// See: https://spec.openapis.org/oas/v3.1.0#openapi-object
func MergeDocuments(info Info, docs ...*Document) (*Document, error) {
	result := &Document{
		OpenAPI: "3.1.0",
		Info:    info,
	}

	var conflicts []string

	validateVersions(docs, &conflicts)
	result.JSONSchemaDialect = mergeJSONSchemaDialect(docs, &conflicts)

	result.Paths = mergeNamedMap("paths", extractPaths, docs, &conflicts)
	result.Webhooks = mergeNamedMap("webhooks", extractWebhooks, docs, &conflicts)

	comp := mergeComponents(docs, &conflicts)
	if comp != nil {
		result.Components = comp
	}

	result.Tags = mergeTags(docs, &conflicts)
	result.Security = mergeSecurity(docs)

	if len(conflicts) > 0 {
		sort.Strings(conflicts)
		return nil, errors.New(strings.Join(conflicts, "; "))
	}

	return result, nil
}

// mergeNamedMap merges named maps from multiple documents using a generic
// extractor. Duplicate keys with different JSON representations produce
// conflict entries. Identical duplicates are silently deduplicated.
func mergeNamedMap[T any](kind string, extract func(*Document) map[string]T, docs []*Document, conflicts *[]string) map[string]T {
	merged := make(map[string]T)
	existing := make(map[string][]byte) // name -> JSON bytes for conflict detection

	for _, doc := range docs {
		if doc == nil {
			continue
		}
		m := extract(doc)
		for name, val := range m {
			valBytes, err := json.Marshal(val)
			if err != nil {
				*conflicts = append(*conflicts, fmt.Sprintf("%s: %q: marshal error: %v", kind, name, err))
				continue
			}

			normalizedBytes := normalizeJSONBytes(valBytes)

			if prev, ok := existing[name]; ok {
				if string(prev) != string(normalizedBytes) {
					*conflicts = append(*conflicts, fmt.Sprintf("%s: duplicate %q with different definitions", kind, name))
				}
				continue
			}

			existing[name] = normalizedBytes
			merged[name] = val
		}
	}

	if len(merged) == 0 {
		return nil
	}
	return merged
}

// mergeComponents merges all 10 component map types from the source documents.
func mergeComponents(docs []*Document, conflicts *[]string) *Components {
	schemas := mergeNamedMap("components.schemas", extractSchemas, docs, conflicts)
	responses := mergeNamedMap("components.responses", extractResponses, docs, conflicts)
	parameters := mergeNamedMap("components.parameters", extractParameters, docs, conflicts)
	examples := mergeNamedMap("components.examples", extractExamples, docs, conflicts)
	requestBodies := mergeNamedMap("components.requestBodies", extractRequestBodies, docs, conflicts)
	headers := mergeNamedMap("components.headers", extractHeaders, docs, conflicts)
	securitySchemes := mergeNamedMap("components.securitySchemes", extractSecuritySchemes, docs, conflicts)
	links := mergeNamedMap("components.links", extractLinks, docs, conflicts)
	callbacks := mergeNamedMap("components.callbacks", extractCallbacks, docs, conflicts)
	pathItems := mergeNamedMap("components.pathItems", extractPathItems, docs, conflicts)

	if schemas == nil && responses == nil && parameters == nil &&
		examples == nil && requestBodies == nil && headers == nil &&
		securitySchemes == nil && links == nil && callbacks == nil && pathItems == nil {
		return nil
	}

	return &Components{
		Schemas:         schemas,
		Responses:       responses,
		Parameters:      parameters,
		Examples:        examples,
		RequestBodies:   requestBodies,
		Headers:         headers,
		SecuritySchemes: securitySchemes,
		Links:           links,
		Callbacks:       callbacks,
		PathItems:       pathItems,
	}
}

// mergeTags deduplicates tags by name across documents. When only one
// source provides a description or ExternalDocs, it is used. When both
// provide differing values, a conflict is reported. Results are sorted
// alphabetically by name.
func mergeTags(docs []*Document, conflicts *[]string) []Tag {
	seen := make(map[string]Tag)
	for _, doc := range docs {
		if doc == nil {
			continue
		}
		for _, tag := range doc.Tags {
			existing, ok := seen[tag.Name]
			if !ok {
				seen[tag.Name] = tag
				continue
			}

			if tag.Description != "" {
				if existing.Description == "" {
					existing.Description = tag.Description
				} else if existing.Description != tag.Description {
					*conflicts = append(*conflicts, fmt.Sprintf("tags: %q has conflicting descriptions", tag.Name))
				}
			}

			if tag.ExternalDocs != nil {
				if existing.ExternalDocs == nil {
					existing.ExternalDocs = tag.ExternalDocs
				} else if *existing.ExternalDocs != *tag.ExternalDocs {
					*conflicts = append(*conflicts, fmt.Sprintf("tags: %q has conflicting externalDocs", tag.Name))
				}
			}

			seen[tag.Name] = existing
		}
	}

	if len(seen) == 0 {
		return nil
	}

	tags := make([]Tag, 0, len(seen))
	for _, tag := range seen {
		tags = append(tags, tag)
	}
	sort.Slice(tags, func(i, j int) bool {
		return tags[i].Name < tags[j].Name
	})
	return tags
}

// mergeSecurity unions security requirements across documents,
// deduplicating via normalized JSON serialization. Scope slices
// are sorted before comparison so that order does not affect dedup.
func mergeSecurity(docs []*Document) []SecurityRequirement {
	var result []SecurityRequirement
	seen := make(map[string]bool)

	for _, doc := range docs {
		if doc == nil {
			continue
		}
		for _, sec := range doc.Security {
			key := normalizedSecurityKey(sec)
			if seen[key] {
				continue
			}
			seen[key] = true
			result = append(result, sec)
		}
	}

	return result
}

// normalizedSecurityKey produces a canonical JSON key for a security
// requirement by sorting scope slices before marshaling.
func normalizedSecurityKey(sec SecurityRequirement) string {
	normalized := make(SecurityRequirement, len(sec))
	for k, v := range sec {
		sorted := make([]string, len(v))
		copy(sorted, v)
		slices.Sort(sorted)
		normalized[k] = sorted
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return ""
	}
	return string(data)
}

// validateVersions checks that all source documents declare OpenAPI 3.1.x.
// Documents with an empty version are accepted (they may be internally
// constructed without setting the field). Any other major/minor version
// produces a conflict.
func validateVersions(docs []*Document, conflicts *[]string) {
	for _, doc := range docs {
		if doc == nil {
			continue
		}
		v := doc.OpenAPI
		if v == "" {
			continue
		}
		if !strings.HasPrefix(v, "3.1.") {
			*conflicts = append(*conflicts, fmt.Sprintf("openapi: unsupported version %q (expected 3.1.x)", v))
		}
	}
}

// mergeJSONSchemaDialect reconciles the jsonSchemaDialect field across
// documents. If all non-empty values are identical the value is preserved;
// differing values produce a conflict. When all values are empty the
// default (implied by the OpenAPI 3.1 spec) is used — an empty string.
func mergeJSONSchemaDialect(docs []*Document, conflicts *[]string) string {
	var dialect string
	for _, doc := range docs {
		if doc == nil || doc.JSONSchemaDialect == "" {
			continue
		}
		if dialect == "" {
			dialect = doc.JSONSchemaDialect
			continue
		}
		if dialect != doc.JSONSchemaDialect {
			*conflicts = append(*conflicts, fmt.Sprintf(
				"jsonSchemaDialect: conflicting values %q and %q", dialect, doc.JSONSchemaDialect,
			))
			return ""
		}
	}
	return dialect
}

// Extractor functions for each component map type.

func extractPaths(doc *Document) map[string]*PathItem {
	return doc.Paths
}

func extractWebhooks(doc *Document) map[string]*PathItem {
	return doc.Webhooks
}

func extractSchemas(doc *Document) map[string]*Schema {
	if doc.Components == nil {
		return nil
	}
	return doc.Components.Schemas
}

func extractResponses(doc *Document) map[string]*Response {
	if doc.Components == nil {
		return nil
	}
	return doc.Components.Responses
}

func extractParameters(doc *Document) map[string]*Parameter {
	if doc.Components == nil {
		return nil
	}
	return doc.Components.Parameters
}

func extractExamples(doc *Document) map[string]*Example {
	if doc.Components == nil {
		return nil
	}
	return doc.Components.Examples
}

func extractRequestBodies(doc *Document) map[string]*RequestBody {
	if doc.Components == nil {
		return nil
	}
	return doc.Components.RequestBodies
}

func extractHeaders(doc *Document) map[string]*Header {
	if doc.Components == nil {
		return nil
	}
	return doc.Components.Headers
}

func extractSecuritySchemes(doc *Document) map[string]*SecurityScheme {
	if doc.Components == nil {
		return nil
	}
	return doc.Components.SecuritySchemes
}

func extractLinks(doc *Document) map[string]*Link {
	if doc.Components == nil {
		return nil
	}
	return doc.Components.Links
}

func extractCallbacks(doc *Document) map[string]*Callback {
	if doc.Components == nil {
		return nil
	}
	return doc.Components.Callbacks
}

func extractPathItems(doc *Document) map[string]*PathItem {
	if doc.Components == nil {
		return nil
	}
	return doc.Components.PathItems
}

// normalizeJSONBytes re-encodes JSON bytes with known unordered fields
// sorted, producing a canonical form for comparison. Only fields that
// are semantically unordered sets (like "required" and "enum" in JSON
// Schema) are sorted; other arrays are left in their original order.
// Numbers are decoded with UseNumber to preserve exact precision.
func normalizeJSONBytes(data []byte) []byte {
	var v any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return data
	}
	normalizeUnorderedFields(v)
	normalized, err := json.Marshal(v)
	if err != nil {
		return data
	}
	return normalized
}

// normalizeUnorderedFields recursively traverses a JSON value and sorts
// string arrays under known unordered keys. Only map entries whose key
// is in the unordered set are candidates for sorting; all other arrays
// (e.g., "tags", "allOf", extension fields) keep their original order.
func normalizeUnorderedFields(v any) {
	obj, ok := v.(map[string]any)
	if !ok {
		return
	}
	for key, child := range obj {
		if isUnorderedField(key) {
			if arr, ok := child.([]any); ok && isStringOnlySlice(arr) {
				sort.Slice(arr, func(i, j int) bool {
					return arr[i].(string) < arr[j].(string)
				})
				continue
			}
		}
		switch val := child.(type) {
		case map[string]any:
			normalizeUnorderedFields(val)
		case []any:
			for _, elem := range val {
				normalizeUnorderedFields(elem)
			}
		}
	}
}

// isUnorderedField returns true for JSON Schema / OpenAPI fields whose
// array values are semantically unordered sets.
func isUnorderedField(key string) bool {
	switch key {
	case "required", "enum":
		return true
	}
	return false
}

func isStringOnlySlice(s []any) bool {
	if len(s) == 0 {
		return false
	}
	for _, v := range s {
		if _, ok := v.(string); !ok {
			return false
		}
	}
	return true
}
