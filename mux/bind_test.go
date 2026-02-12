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

	tests := []struct {
		name         string
		body         string
		allowUnknown bool
		expectErr    bool
		errContains  string
		expected     *item
	}{
		{
			name:     "decodes valid JSON",
			body:     `{"name":"test","value":42}`,
			expected: &item{Name: "test", Value: 42},
		},
		{
			name:      "returns error for invalid JSON",
			body:      `{invalid`,
			expectErr: true,
		},
		{
			name:      "returns error for unknown fields",
			body:      `{"name":"test","value":42,"extra":"field"}`,
			expectErr: true,
		},
		{
			name:         "allows unknown fields when opted in",
			body:         `{"name":"test","value":42,"extra":"field"}`,
			allowUnknown: true,
			expected:     &item{Name: "test", Value: 42},
		},
		{
			name:        "returns error for trailing data",
			body:        `{"name":"a","value":1}{"name":"b","value":2}`,
			expectErr:   true,
			errContains: "trailing data",
		},
		{
			name:      "returns error for empty body",
			body:      "",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))

			var got item
			var err error
			if tt.allowUnknown {
				err = BindJSON(r, &got, true)
			} else {
				err = BindJSON(r, &got)
			}

			if tt.expectErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, *tt.expected, got)
			}
		})
	}
}

func TestBindXML(t *testing.T) {
	type item struct {
		XMLName xml.Name `xml:"item"`
		Name    string   `xml:"name"`
		Value   int      `xml:"value"`
	}

	tests := []struct {
		name        string
		body        string
		expectErr   bool
		errContains string
		wantName    string
		wantValue   int
	}{
		{
			name:      "decodes valid XML",
			body:      `<item><name>test</name><value>42</value></item>`,
			wantName:  "test",
			wantValue: 42,
		},
		{
			name:      "returns error for invalid XML",
			body:      `<item><name>test</`,
			expectErr: true,
		},
		{
			name:        "returns error for trailing data",
			body:        `<item><name>a</name><value>1</value></item><item><name>b</name><value>2</value></item>`,
			expectErr:   true,
			errContains: "trailing data",
		},
		{
			name:      "returns error for empty body",
			body:      "",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))

			var got item
			err := BindXML(r, &got)

			if tt.expectErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantName, got.Name)
				assert.Equal(t, tt.wantValue, got.Value)
			}
		})
	}
}
