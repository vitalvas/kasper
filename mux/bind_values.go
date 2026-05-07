package mux

import (
	"encoding"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// ErrBindNotPointerToStruct is returned when the destination is not a pointer
// to a struct.
var ErrBindNotPointerToStruct = errors.New("bind: destination must be a pointer to a struct")

// ErrEncodeNotStruct is returned when the source is not a struct or pointer
// to a struct.
var ErrEncodeNotStruct = errors.New("encode: source must be a struct or pointer to a struct")

// DefaultMaxSliceIndex is the maximum slice index allowed during decoding.
// Prevents abuse from keys like "items.999999.name" allocating huge slices.
const DefaultMaxSliceIndex = 1000

// BindQuery decodes URL query parameters into the struct pointed to by v.
// Fields are mapped using the "query" struct tag. Tag options "required" and
// "default:<value>" are supported. Nested structs use dot notation
// (e.g. "address.street"), slices of structs use indexed dot notation
// (e.g. "items.0.name"), and map fields use dot notation for keys
// (e.g. "meta.key1").
func BindQuery(r *http.Request, v any) error {
	return decodeValues(r.URL.Query(), v, "query")
}

// BindForm decodes application/x-www-form-urlencoded request body into the
// struct pointed to by v. Fields are mapped using the "form" struct tag.
// Tag options "required" and "default:<value>" are supported. Nested structs,
// slice-of-structs, and map fields use the same dot notation as BindQuery.
func BindForm(r *http.Request, v any) error {
	if err := r.ParseForm(); err != nil {
		return err
	}
	return decodeValues(r.PostForm, v, "form")
}

// EncodeQuery encodes a struct into url.Values using the "query" struct tag.
// Nested structs use dot notation, slices of structs use indexed dot notation,
// and map fields use dot notation for keys.
func EncodeQuery(v any) (url.Values, error) {
	return encodeValues(v, "query")
}

// EncodeForm encodes a struct into url.Values using the "form" struct tag.
func EncodeForm(v any) (url.Values, error) {
	return encodeValues(v, "form")
}

// fieldMeta holds parsed struct tag metadata for a single field.
type fieldMeta struct {
	index      int
	name       string
	required   bool
	hasDefault bool
	defaultVal string
	omitEmpty  bool
}

// fieldKind classifies how a struct field should be handled during
// decode and encode.
type fieldKind int

const (
	fieldKindBasic         fieldKind = iota // scalar, slice of scalars, pointer
	fieldKindStruct                         // nested struct (not TextUnmarshaler)
	fieldKindSliceOfStruct                  // []SomeStruct
	fieldKindMap                            // map[string]T
	fieldKindEmbedded                       // anonymous struct, flatten to parent
)

// cachedField holds pre-computed metadata for a single struct field.
type cachedField struct {
	meta      fieldMeta
	kind      fieldKind
	index     int
	fieldType reflect.Type // dereferenced type (Ptr unwrapped)
	isPtr     bool         // original field is a pointer
}

// structCacheKey is used to look up cached struct metadata.
type structCacheKey struct {
	typ     reflect.Type
	tagName string
}

// structFieldsCache stores parsed struct metadata keyed by type and tag name.
var structFieldsCache sync.Map

// getStructFields returns cached field metadata for the given struct type
// and tag name. Builds and caches the metadata on first access.
func getStructFields(rt reflect.Type, tagName string) []cachedField {
	key := structCacheKey{
		typ:     rt,
		tagName: tagName,
	}
	if cached, ok := structFieldsCache.Load(key); ok {
		return cached.([]cachedField)
	}

	fields := buildStructFields(rt, tagName)
	structFieldsCache.Store(key, fields)
	return fields
}

// buildStructFields inspects a struct type and pre-computes metadata for
// each exported field.
func buildStructFields(rt reflect.Type, tagName string) []cachedField {
	var fields []cachedField

	for i := range rt.NumField() {
		sf := rt.Field(i)
		if !sf.IsExported() {
			continue
		}

		meta := parseFieldTag(sf, tagName, i)
		if meta.name == "-" {
			continue
		}

		fieldType := sf.Type
		isPtr := fieldType.Kind() == reflect.Pointer
		if isPtr {
			fieldType = fieldType.Elem()
		}

		_, hasExplicitTag := sf.Tag.Lookup(tagName)
		isTextUnmarshaler := implementsTextUnmarshaler(fieldType)

		var kind fieldKind
		switch {
		case sf.Anonymous && fieldType.Kind() == reflect.Struct &&
			!isTextUnmarshaler && !hasExplicitTag:
			kind = fieldKindEmbedded
		case fieldType.Kind() == reflect.Struct && !isTextUnmarshaler:
			kind = fieldKindStruct
		case fieldType.Kind() == reflect.Slice && fieldType.Elem().Kind() == reflect.Struct:
			kind = fieldKindSliceOfStruct
		case fieldType.Kind() == reflect.Map:
			kind = fieldKindMap
		default:
			kind = fieldKindBasic
		}

		fields = append(fields, cachedField{
			meta:      meta,
			kind:      kind,
			index:     i,
			fieldType: fieldType,
			isPtr:     isPtr,
		})
	}

	return fields
}

// decodeValues decodes a map[string][]string into the struct pointed to by v
// using the specified struct tag name.
func decodeValues(src map[string][]string, v any, tagName string) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		return ErrBindNotPointerToStruct
	}

	return decodeStruct(src, rv.Elem(), tagName, "")
}

// decodeStruct decodes values into a struct, with prefix for nested paths.
func decodeStruct(src map[string][]string, rv reflect.Value, tagName, prefix string) error {
	fields := getStructFields(rv.Type(), tagName)

	for _, cf := range fields {
		fv := rv.Field(cf.index)

		if cf.kind == fieldKindEmbedded {
			if cf.isPtr {
				if fv.IsNil() {
					fv.Set(reflect.New(fv.Type().Elem()))
				}
				fv = fv.Elem()
			}
			if err := decodeStruct(src, fv, tagName, prefix); err != nil {
				return err
			}
			continue
		}

		fullName := cf.meta.name
		if prefix != "" {
			fullName = fmt.Sprintf("%s.%s", prefix, cf.meta.name)
		}

		switch cf.kind {
		case fieldKindStruct:
			if cf.isPtr {
				if fv.IsNil() {
					if !hasPrefixedKeys(src, fullName) {
						continue
					}
					fv.Set(reflect.New(fv.Type().Elem()))
				}
				fv = fv.Elem()
			}
			if err := decodeStruct(src, fv, tagName, fullName); err != nil {
				return err
			}

		case fieldKindSliceOfStruct:
			if err := decodeSliceOfStructs(src, fv, tagName, fullName); err != nil {
				return err
			}

		case fieldKindMap:
			if err := decodeMapField(src, fv, fullName); err != nil {
				return err
			}

		default:
			vals, exists := src[fullName]
			if !exists || len(vals) == 0 {
				if cf.meta.required {
					return fmt.Errorf("bind: field %q is required", fullName)
				}
				if cf.meta.hasDefault {
					vals = []string{cf.meta.defaultVal}
				} else {
					continue
				}
			}
			if err := setFieldValue(fv, vals); err != nil {
				return fmt.Errorf("bind: field %q: %w", fullName, err)
			}
		}
	}

	return nil
}

// decodeSliceOfStructs handles indexed dot notation for slices of structs.
// e.g. "items.0.name", "items.1.name"
func decodeSliceOfStructs(src map[string][]string, fv reflect.Value, tagName, prefix string) error {
	indices := collectSliceIndices(src, prefix)

	if len(indices) == 0 {
		return nil
	}

	maxIdx := indices[len(indices)-1]
	if maxIdx >= DefaultMaxSliceIndex {
		return fmt.Errorf("bind: slice index %d exceeds maximum %d for %q", maxIdx, DefaultMaxSliceIndex, prefix)
	}

	slice := reflect.MakeSlice(fv.Type(), maxIdx+1, maxIdx+1)

	for _, idx := range indices {
		elemPrefix := fmt.Sprintf("%s.%d", prefix, idx)
		elem := slice.Index(idx)
		if err := decodeStruct(src, elem, tagName, elemPrefix); err != nil {
			return err
		}
	}

	fv.Set(slice)
	return nil
}

// collectSliceIndices finds all numeric indices used in keys with the given
// prefix. Returns sorted unique indices.
func collectSliceIndices(src map[string][]string, prefix string) []int {
	seen := make(map[int]struct{})
	dotPrefix := fmt.Sprintf("%s.", prefix)

	for key := range src {
		if !strings.HasPrefix(key, dotPrefix) {
			continue
		}

		rest := key[len(dotPrefix):]
		parts := strings.SplitN(rest, ".", 2)
		idx, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		seen[idx] = struct{}{}
	}

	indices := make([]int, 0, len(seen))
	for idx := range seen {
		indices = append(indices, idx)
	}
	sort.Ints(indices)
	return indices
}

// decodeMapField handles dot notation for map fields.
// e.g. "meta.key1" → map["key1"]
// Supports map[string][]string for multiple values per key.
func decodeMapField(src map[string][]string, fv reflect.Value, prefix string) error {
	mapType := fv.Type()
	if mapType.Key().Kind() != reflect.String {
		return fmt.Errorf("bind: map field %q must have string keys", prefix)
	}

	dotPrefix := fmt.Sprintf("%s.", prefix)
	elemType := mapType.Elem()

	for key, vals := range src {
		if !strings.HasPrefix(key, dotPrefix) {
			continue
		}

		mapKey := key[len(dotPrefix):]
		if strings.Contains(mapKey, ".") {
			continue
		}

		if fv.IsNil() {
			fv.Set(reflect.MakeMap(mapType))
		}

		if len(vals) == 0 {
			continue
		}

		if elemType.Kind() == reflect.Slice {
			slice := reflect.MakeSlice(elemType, len(vals), len(vals))
			for i, val := range vals {
				if err := setBasicField(slice.Index(i), val); err != nil {
					return fmt.Errorf("bind: map field %q key %q: %w", prefix, mapKey, err)
				}
			}
			fv.SetMapIndex(reflect.ValueOf(mapKey), slice)
		} else {
			elem := reflect.New(elemType).Elem()
			if err := setBasicField(elem, vals[0]); err != nil {
				return fmt.Errorf("bind: map field %q key %q: %w", prefix, mapKey, err)
			}
			fv.SetMapIndex(reflect.ValueOf(mapKey), elem)
		}
	}

	return nil
}

// hasPrefixedKeys reports whether src contains any key starting with prefix + ".".
func hasPrefixedKeys(src map[string][]string, prefix string) bool {
	dotPrefix := fmt.Sprintf("%s.", prefix)
	for key := range src {
		if strings.HasPrefix(key, dotPrefix) {
			return true
		}
	}
	return false
}

// implementsTextUnmarshaler reports whether the type (or pointer to it)
// implements encoding.TextUnmarshaler.
func implementsTextUnmarshaler(t reflect.Type) bool {
	tu := reflect.TypeFor[encoding.TextUnmarshaler]()
	return t.Implements(tu) || reflect.PointerTo(t).Implements(tu)
}

// parseFieldTag extracts metadata from the struct tag.
func parseFieldTag(field reflect.StructField, tagName string, index int) fieldMeta {
	meta := fieldMeta{
		index: index,
		name:  field.Name,
	}

	tag, ok := field.Tag.Lookup(tagName)
	if !ok {
		return meta
	}

	parts := strings.Split(tag, ",")
	if parts[0] != "" {
		meta.name = parts[0]
	}

	for _, opt := range parts[1:] {
		switch {
		case opt == "required":
			meta.required = true
		case opt == "omitempty":
			meta.omitEmpty = true
		case strings.HasPrefix(opt, "default:"):
			meta.hasDefault = true
			meta.defaultVal = opt[len("default:"):]
		}
	}

	return meta
}

// setFieldValue sets a struct field from a slice of string values.
func setFieldValue(fv reflect.Value, vals []string) error {
	if fv.Kind() == reflect.Slice {
		return setSliceField(fv, vals)
	}

	if fv.Kind() == reflect.Pointer {
		return setPtrField(fv, vals[0])
	}

	if fv.CanAddr() {
		if tu, ok := fv.Addr().Interface().(encoding.TextUnmarshaler); ok {
			return tu.UnmarshalText([]byte(vals[0]))
		}
	}

	return setBasicField(fv, vals[0])
}

// setBasicField converts a string to the target basic type and sets the field.
func setBasicField(fv reflect.Value, val string) error {
	switch fv.Kind() {
	case reflect.String:
		fv.SetString(val)

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(val, 10, fv.Type().Bits())
		if err != nil {
			return err
		}
		fv.SetInt(n)

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(val, 10, fv.Type().Bits())
		if err != nil {
			return err
		}
		fv.SetUint(n)

	case reflect.Float32, reflect.Float64:
		n, err := strconv.ParseFloat(val, fv.Type().Bits())
		if err != nil {
			return err
		}
		fv.SetFloat(n)

	case reflect.Bool:
		b, err := strconv.ParseBool(val)
		if err != nil {
			return err
		}
		fv.SetBool(b)

	default:
		return fmt.Errorf("unsupported type %s", fv.Type())
	}

	return nil
}

// setPtrField allocates a pointer field and sets its value.
func setPtrField(fv reflect.Value, val string) error {
	elem := reflect.New(fv.Type().Elem())

	if tu, ok := elem.Interface().(encoding.TextUnmarshaler); ok {
		if err := tu.UnmarshalText([]byte(val)); err != nil {
			return err
		}
		fv.Set(elem)
		return nil
	}

	if err := setBasicField(elem.Elem(), val); err != nil {
		return err
	}
	fv.Set(elem)
	return nil
}

// setSliceField populates a slice field from multiple string values.
func setSliceField(fv reflect.Value, vals []string) error {
	elemType := fv.Type().Elem()
	slice := reflect.MakeSlice(fv.Type(), len(vals), len(vals))

	for i, val := range vals {
		elem := slice.Index(i)
		if elemType.Kind() == reflect.Pointer {
			p := reflect.New(elemType.Elem())
			if err := setBasicField(p.Elem(), val); err != nil {
				return err
			}
			elem.Set(p)
		} else {
			if err := setBasicField(elem, val); err != nil {
				return err
			}
		}
	}

	fv.Set(slice)
	return nil
}

// encodeValues encodes a struct into url.Values using the specified tag name.
func encodeValues(v any, tagName string) (url.Values, error) {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil, ErrEncodeNotStruct
		}
		rv = rv.Elem()
	}

	if rv.Kind() != reflect.Struct {
		return nil, ErrEncodeNotStruct
	}

	vals := make(url.Values)
	if err := encodeStruct(vals, rv, tagName, ""); err != nil {
		return nil, err
	}
	return vals, nil
}

// encodeStruct recursively encodes struct fields into url.Values.
func encodeStruct(dst url.Values, rv reflect.Value, tagName, prefix string) error {
	fields := getStructFields(rv.Type(), tagName)

	for _, cf := range fields {
		fv := rv.Field(cf.index)

		if cf.kind == fieldKindEmbedded {
			if cf.isPtr {
				if fv.IsNil() {
					continue
				}
				fv = fv.Elem()
			}
			if err := encodeStruct(dst, fv, tagName, prefix); err != nil {
				return err
			}
			continue
		}

		fullName := cf.meta.name
		if prefix != "" {
			fullName = fmt.Sprintf("%s.%s", prefix, cf.meta.name)
		}

		if cf.meta.omitEmpty && fv.IsZero() {
			continue
		}

		if fv.Kind() == reflect.Pointer {
			if fv.IsNil() {
				continue
			}
			fv = fv.Elem()
		}

		switch {
		case fv.Kind() == reflect.Struct && !implementsTextMarshaler(fv.Type()):
			if err := encodeStruct(dst, fv, tagName, fullName); err != nil {
				return err
			}

		case fv.Kind() == reflect.Slice && fv.Type().Elem().Kind() == reflect.Struct:
			for j := range fv.Len() {
				elemPrefix := fmt.Sprintf("%s.%d", fullName, j)
				if err := encodeStruct(dst, fv.Index(j), tagName, elemPrefix); err != nil {
					return err
				}
			}

		case fv.Kind() == reflect.Slice:
			for j := range fv.Len() {
				dst.Add(fullName, formatValue(fv.Index(j)))
			}

		case fv.Kind() == reflect.Map:
			if fv.Type().Key().Kind() != reflect.String {
				continue
			}
			iter := fv.MapRange()
			for iter.Next() {
				key := fmt.Sprintf("%s.%s", fullName, iter.Key().String())
				val := iter.Value()
				if val.Kind() == reflect.Slice {
					for j := range val.Len() {
						dst.Add(key, formatValue(val.Index(j)))
					}
				} else {
					dst.Set(key, formatValue(val))
				}
			}

		default:
			dst.Set(fullName, formatValue(fv))
		}
	}

	return nil
}

// formatValue converts a reflect.Value to its string representation.
func formatValue(v reflect.Value) string {
	if v.CanInterface() {
		if tm, ok := v.Interface().(encoding.TextMarshaler); ok {
			b, err := tm.MarshalText()
			if err == nil {
				return string(b)
			}
		}
	}

	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return ""
		}
		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.String:
		return v.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 10)
	case reflect.Float32:
		return strconv.FormatFloat(v.Float(), 'f', -1, 32)
	case reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'f', -1, 64)
	case reflect.Bool:
		return strconv.FormatBool(v.Bool())
	default:
		return fmt.Sprintf("%v", v.Interface())
	}
}

// implementsTextMarshaler reports whether the type implements
// encoding.TextMarshaler.
func implementsTextMarshaler(t reflect.Type) bool {
	return t.Implements(reflect.TypeFor[encoding.TextMarshaler]())
}
