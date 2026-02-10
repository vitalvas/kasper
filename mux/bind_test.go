package mux

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBindJSON(t *testing.T) {
	type item struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	t.Run("decodes valid JSON", func(t *testing.T) {
		body := `{"name":"test","value":42}`
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

		var got item
		err := BindJSON(r, &got)

		require.NoError(t, err)
		assert.Equal(t, item{Name: "test", Value: 42}, got)
	})

	t.Run("returns error for invalid JSON", func(t *testing.T) {
		body := `{invalid`
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

		var got item
		err := BindJSON(r, &got)

		assert.Error(t, err)
	})

	t.Run("returns error for unknown fields", func(t *testing.T) {
		body := `{"name":"test","value":42,"extra":"field"}`
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

		var got item
		err := BindJSON(r, &got)

		assert.Error(t, err)
	})

	t.Run("allows unknown fields when opted in", func(t *testing.T) {
		body := `{"name":"test","value":42,"extra":"field"}`
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

		var got item
		err := BindJSON(r, &got, true)

		require.NoError(t, err)
		assert.Equal(t, item{Name: "test", Value: 42}, got)
	})

	t.Run("returns error for trailing data", func(t *testing.T) {
		body := `{"name":"a","value":1}{"name":"b","value":2}`
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

		var got item
		err := BindJSON(r, &got)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "trailing data")
	})

	t.Run("returns error for empty body", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))

		var got item
		err := BindJSON(r, &got)

		assert.Error(t, err)
	})
}

func TestBindXML(t *testing.T) {
	type item struct {
		XMLName xml.Name `xml:"item"`
		Name    string   `xml:"name"`
		Value   int      `xml:"value"`
	}

	t.Run("decodes valid XML", func(t *testing.T) {
		body := `<item><name>test</name><value>42</value></item>`
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

		var got item
		err := BindXML(r, &got)

		require.NoError(t, err)
		assert.Equal(t, "test", got.Name)
		assert.Equal(t, 42, got.Value)
	})

	t.Run("returns error for invalid XML", func(t *testing.T) {
		body := `<item><name>test</`
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

		var got item
		err := BindXML(r, &got)

		assert.Error(t, err)
	})

	t.Run("returns error for trailing data", func(t *testing.T) {
		body := `<item><name>a</name><value>1</value></item><item><name>b</name><value>2</value></item>`
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

		var got item
		err := BindXML(r, &got)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "trailing data")
	})

	t.Run("returns error for empty body", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))

		var got item
		err := BindXML(r, &got)

		assert.Error(t, err)
	})
}
