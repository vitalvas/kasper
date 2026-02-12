package mux

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResponseJSON(t *testing.T) {
	type item struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	tests := []struct {
		name       string
		status     int
		data       any
		wantStatus int
		wantCT     string
		wantNotCT  string
		wantBody   string
		jsonEq     bool
	}{
		{
			name:       "writes JSON with status code",
			status:     http.StatusCreated,
			data:       item{Name: "test", Value: 42},
			wantStatus: http.StatusCreated,
			wantCT:     "application/json",
			wantBody:   `{"name":"test","value":42}`,
			jsonEq:     true,
		},
		{
			name:       "writes JSON array",
			status:     http.StatusOK,
			data:       []item{{Name: "a", Value: 1}, {Name: "b", Value: 2}},
			wantStatus: http.StatusOK,
			wantCT:     "application/json",
			wantBody:   `[{"name":"a","value":1},{"name":"b","value":2}]`,
			jsonEq:     true,
		},
		{
			name:       "writes null for nil",
			status:     http.StatusOK,
			data:       nil,
			wantStatus: http.StatusOK,
			wantCT:     "application/json",
			wantBody:   "null\n",
		},
		{
			name:       "writes 500 on encode error",
			status:     http.StatusOK,
			data:       make(chan int),
			wantStatus: http.StatusInternalServerError,
			wantNotCT:  "application/json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			ResponseJSON(w, tt.status, tt.data)

			assert.Equal(t, tt.wantStatus, w.Code)
			if tt.wantCT != "" {
				assert.Equal(t, tt.wantCT, w.Header().Get("Content-Type"))
			}
			if tt.wantNotCT != "" {
				assert.NotEqual(t, tt.wantNotCT, w.Header().Get("Content-Type"))
			}
			if tt.jsonEq {
				assert.JSONEq(t, tt.wantBody, w.Body.String())
			} else if tt.wantBody != "" {
				assert.Equal(t, tt.wantBody, w.Body.String())
			}
		})
	}
}

func TestResponseXML(t *testing.T) {
	type item struct {
		XMLName xml.Name `xml:"item"`
		Name    string   `xml:"name"`
		Value   int      `xml:"value"`
	}

	type items struct {
		XMLName xml.Name `xml:"items"`
		Items   []item   `xml:"item"`
	}

	tests := []struct {
		name         string
		status       int
		data         any
		wantStatus   int
		wantCT       string
		wantNotCT    string
		wantContains []string
	}{
		{
			name:       "writes XML with status code",
			status:     http.StatusCreated,
			data:       item{Name: "test", Value: 42},
			wantStatus: http.StatusCreated,
			wantCT:     "application/xml",
			wantContains: []string{
				"<item>",
				"<name>test</name>",
				"<value>42</value>",
			},
		},
		{
			name:       "writes XML array",
			status:     http.StatusOK,
			data:       items{Items: []item{{Name: "a", Value: 1}, {Name: "b", Value: 2}}},
			wantStatus: http.StatusOK,
			wantCT:     "application/xml",
			wantContains: []string{
				"<items>",
				"<name>a</name>",
				"<name>b</name>",
			},
		},
		{
			name:       "writes 500 on encode error",
			status:     http.StatusOK,
			data:       make(chan int),
			wantStatus: http.StatusInternalServerError,
			wantNotCT:  "application/xml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			ResponseXML(w, tt.status, tt.data)

			assert.Equal(t, tt.wantStatus, w.Code)
			if tt.wantCT != "" {
				assert.Equal(t, tt.wantCT, w.Header().Get("Content-Type"))
			}
			if tt.wantNotCT != "" {
				assert.NotEqual(t, tt.wantNotCT, w.Header().Get("Content-Type"))
			}
			for _, s := range tt.wantContains {
				assert.Contains(t, w.Body.String(), s)
			}
		})
	}
}
