package securecookie

import "encoding/json"

// Serializer defines how values are converted to and from bytes for storage
// in a cookie.
type Serializer interface {
	Serialize(src any) ([]byte, error)
	Deserialize(src []byte, dst any) error
}

// JSONSerializer uses JSON encoding. This is the default serializer.
type JSONSerializer struct{}

// Serialize marshals src to JSON.
func (JSONSerializer) Serialize(src any) ([]byte, error) {
	return json.Marshal(src)
}

// Deserialize unmarshals JSON bytes into dst.
func (JSONSerializer) Deserialize(src []byte, dst any) error {
	return json.Unmarshal(src, dst)
}
