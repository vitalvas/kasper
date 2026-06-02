package mux

import (
	"encoding/xml"
	"html/template"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponseJSON(t *testing.T) {
	type item struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	tests := []struct {
		name        string
		status      int
		data        any
		config      *ResponseConfig
		wantStatus  int
		wantCT      string
		wantNotCT   string
		wantBody    string
		wantHeaders map[string]string
		jsonEq      bool
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
		{
			name:       "overrides content type",
			status:     http.StatusOK,
			data:       item{Name: "test", Value: 42},
			config:     &ResponseConfig{ContentType: ContentTypeApplicationJWT},
			wantStatus: http.StatusOK,
			wantCT:     "application/jwt",
			wantBody:   `{"name":"test","value":42}`,
			jsonEq:     true,
		},
		{
			name:       "empty content type override falls back to JSON",
			status:     http.StatusOK,
			data:       item{Name: "test", Value: 42},
			config:     &ResponseConfig{},
			wantStatus: http.StatusOK,
			wantCT:     "application/json",
			wantBody:   `{"name":"test","value":42}`,
			jsonEq:     true,
		},
		{
			name:       "sets extra headers",
			status:     http.StatusOK,
			data:       item{Name: "test", Value: 42},
			config:     &ResponseConfig{Headers: map[string]string{"X-Custom": "abc", "Cache-Control": "no-store"}},
			wantStatus: http.StatusOK,
			wantCT:     "application/json",
			wantBody:   `{"name":"test","value":42}`,
			jsonEq:     true,
			wantHeaders: map[string]string{
				"X-Custom":      "abc",
				"Cache-Control": "no-store",
			},
		},
		{
			name:       "content type cannot be overridden via headers",
			status:     http.StatusOK,
			data:       item{Name: "test", Value: 42},
			config:     &ResponseConfig{ContentType: ContentTypeApplicationJWT, Headers: map[string]string{"Content-Type": "text/plain"}},
			wantStatus: http.StatusOK,
			wantCT:     "application/jwt",
			wantBody:   `{"name":"test","value":42}`,
			jsonEq:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			if tt.config != nil {
				ResponseJSON(w, tt.status, tt.data, *tt.config)
			} else {
				ResponseJSON(w, tt.status, tt.data)
			}

			assert.Equal(t, tt.wantStatus, w.Code)
			if tt.wantCT != "" {
				assert.Equal(t, tt.wantCT, w.Header().Get("Content-Type"))
			}
			if tt.wantNotCT != "" {
				assert.NotEqual(t, tt.wantNotCT, w.Header().Get("Content-Type"))
			}
			for k, v := range tt.wantHeaders {
				assert.Equal(t, v, w.Header().Get(k))
			}
			if tt.jsonEq {
				assert.JSONEq(t, tt.wantBody, w.Body.String())
			} else if tt.wantBody != "" {
				assert.Equal(t, tt.wantBody, w.Body.String())
			}
		})
	}
}

func TestResponseHTML(t *testing.T) {
	t.Cleanup(func() { SetTemplates(nil) })

	tmpl := template.Must(template.New("hello").Parse(`<p>Hello, {{.Name}}!</p>`))
	template.Must(tmpl.New("error").Parse(`<h1>{{.Title}}</h1><p>{{.Message}}</p>`))
	template.Must(tmpl.New("broken").Parse(`{{.Missing.Field}}`))

	t.Run("renders template with status and content-type", func(t *testing.T) {
		SetTemplates(tmpl)
		w := httptest.NewRecorder()
		ResponseHTML(w, http.StatusOK, "hello", map[string]string{"Name": "World"})
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))
		assert.Equal(t, "<p>Hello, World!</p>", w.Body.String())
	})

	t.Run("respects custom status code", func(t *testing.T) {
		SetTemplates(tmpl)
		w := httptest.NewRecorder()
		ResponseHTML(w, http.StatusForbidden, "error", map[string]string{
			"Title":   "Access denied",
			"Message": "Nope.",
		})
		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Contains(t, w.Body.String(), "Access denied")
		assert.Contains(t, w.Body.String(), "Nope.")
	})

	t.Run("escapes HTML in data", func(t *testing.T) {
		SetTemplates(tmpl)
		w := httptest.NewRecorder()
		ResponseHTML(w, http.StatusOK, "hello", map[string]string{"Name": "<script>x</script>"})
		assert.NotContains(t, w.Body.String(), "<script>")
		assert.Contains(t, w.Body.String(), "&lt;script&gt;")
	})

	t.Run("returns 500 when templates not set", func(t *testing.T) {
		SetTemplates(nil)
		w := httptest.NewRecorder()
		ResponseHTML(w, http.StatusOK, "hello", nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.NotEqual(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))
	})

	t.Run("returns 500 when template not found", func(t *testing.T) {
		SetTemplates(tmpl)
		w := httptest.NewRecorder()
		ResponseHTML(w, http.StatusOK, "missing", nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("returns 500 on execution error", func(t *testing.T) {
		SetTemplates(tmpl)
		w := httptest.NewRecorder()
		ResponseHTML(w, http.StatusOK, "broken", struct{}{})
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.NotContains(t, w.Body.String(), "<")
	})

	t.Run("applies config content-type and headers", func(t *testing.T) {
		SetTemplates(tmpl)
		w := httptest.NewRecorder()
		ResponseHTML(w, http.StatusOK, "hello", map[string]string{"Name": "World"}, ResponseConfig{
			ContentType: ContentTypeTextPlain,
			Headers:     map[string]string{"X-Custom": "abc"},
		})
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "text/plain", w.Header().Get("Content-Type"))
		assert.Equal(t, "abc", w.Header().Get("X-Custom"))
		assert.Equal(t, "<p>Hello, World!</p>", w.Body.String())
	})
}

func TestResponseHTMLTemplate(t *testing.T) {
	t.Run("renders single template", func(t *testing.T) {
		tmpl := template.Must(template.New("page").Parse(`<p>{{.Name}}</p>`))
		w := httptest.NewRecorder()
		ResponseHTMLTemplate(w, http.StatusOK, tmpl, "", map[string]string{"Name": "Alice"})
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))
		assert.Equal(t, "<p>Alice</p>", w.Body.String())
	})

	t.Run("renders named template from set", func(t *testing.T) {
		tmpl := template.Must(template.New("root").Parse(`root`))
		template.Must(tmpl.New("fragment").Parse(`<span>{{.}}</span>`))
		w := httptest.NewRecorder()
		ResponseHTMLTemplate(w, http.StatusOK, tmpl, "fragment", "hi")
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "<span>hi</span>", w.Body.String())
	})

	t.Run("escapes HTML in data", func(t *testing.T) {
		tmpl := template.Must(template.New("p").Parse(`<p>{{.}}</p>`))
		w := httptest.NewRecorder()
		ResponseHTMLTemplate(w, http.StatusOK, tmpl, "", "<script>x</script>")
		assert.NotContains(t, w.Body.String(), "<script>")
		assert.Contains(t, w.Body.String(), "&lt;script&gt;")
	})

	t.Run("returns 500 when tmpl is nil", func(t *testing.T) {
		w := httptest.NewRecorder()
		ResponseHTMLTemplate(w, http.StatusOK, nil, "", nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("returns 500 when named template missing", func(t *testing.T) {
		tmpl := template.Must(template.New("page").Parse(`hi`))
		w := httptest.NewRecorder()
		ResponseHTMLTemplate(w, http.StatusOK, tmpl, "missing", nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("returns 500 on execution error", func(t *testing.T) {
		tmpl := template.Must(template.New("broken").Parse(`{{.Missing.Field}}`))
		w := httptest.NewRecorder()
		ResponseHTMLTemplate(w, http.StatusOK, tmpl, "", struct{}{})
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.NotEqual(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))
	})

	t.Run("applies config content-type and headers", func(t *testing.T) {
		tmpl := template.Must(template.New("page").Parse(`<p>{{.Name}}</p>`))
		w := httptest.NewRecorder()
		ResponseHTMLTemplate(w, http.StatusOK, tmpl, "", map[string]string{"Name": "Alice"}, ResponseConfig{
			ContentType: ContentTypeTextPlain,
			Headers:     map[string]string{"X-Custom": "abc"},
		})
		assert.Equal(t, "text/plain", w.Header().Get("Content-Type"))
		assert.Equal(t, "abc", w.Header().Get("X-Custom"))
		assert.Equal(t, "<p>Alice</p>", w.Body.String())
	})
}

func TestResponseHTMLString(t *testing.T) {
	t.Run("parses and renders inline template", func(t *testing.T) {
		w := httptest.NewRecorder()
		ResponseHTMLString(w, http.StatusOK, `<p>Hello, {{.Name}}!</p>`, map[string]string{"Name": "World"})
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))
		assert.Equal(t, "<p>Hello, World!</p>", w.Body.String())
	})

	t.Run("respects custom status code", func(t *testing.T) {
		w := httptest.NewRecorder()
		ResponseHTMLString(w, http.StatusTeapot, `<h1>{{.}}</h1>`, "I am a teapot")
		assert.Equal(t, http.StatusTeapot, w.Code)
		assert.Equal(t, "<h1>I am a teapot</h1>", w.Body.String())
	})

	t.Run("escapes HTML in data", func(t *testing.T) {
		w := httptest.NewRecorder()
		ResponseHTMLString(w, http.StatusOK, `<p>{{.}}</p>`, "<script>x</script>")
		assert.NotContains(t, w.Body.String(), "<script>")
		assert.Contains(t, w.Body.String(), "&lt;script&gt;")
	})

	t.Run("returns 500 on parse error", func(t *testing.T) {
		w := httptest.NewRecorder()
		ResponseHTMLString(w, http.StatusOK, `{{ unclosed`, nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.NotEqual(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))
	})

	t.Run("returns 500 on execution error", func(t *testing.T) {
		w := httptest.NewRecorder()
		ResponseHTMLString(w, http.StatusOK, `{{.Missing.Field}}`, struct{}{})
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("applies config content-type and headers", func(t *testing.T) {
		w := httptest.NewRecorder()
		ResponseHTMLString(w, http.StatusOK, `<p>{{.Name}}</p>`, map[string]string{"Name": "World"}, ResponseConfig{
			ContentType: ContentTypeTextPlain,
			Headers:     map[string]string{"X-Custom": "abc"},
		})
		assert.Equal(t, "text/plain", w.Header().Get("Content-Type"))
		assert.Equal(t, "abc", w.Header().Get("X-Custom"))
		assert.Equal(t, "<p>World</p>", w.Body.String())
	})
}

func TestSetTemplates(t *testing.T) {
	t.Cleanup(func() { SetTemplates(nil) })

	t.Run("nil clears registry", func(t *testing.T) {
		tmpl := template.Must(template.New("t").Parse("hi"))
		SetTemplates(tmpl)
		SetTemplates(nil)
		w := httptest.NewRecorder()
		ResponseHTML(w, http.StatusOK, "t", nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("replaces previously set templates", func(t *testing.T) {
		first := template.Must(template.New("t").Parse("first"))
		second := template.Must(template.New("t").Parse("second"))
		SetTemplates(first)
		SetTemplates(second)
		w := httptest.NewRecorder()
		ResponseHTML(w, http.StatusOK, "t", nil)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "second", w.Body.String())
	})
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
		config       *ResponseConfig
		wantStatus   int
		wantCT       string
		wantNotCT    string
		wantContains []string
		wantHeaders  map[string]string
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
		{
			name:       "overrides content type",
			status:     http.StatusOK,
			data:       item{Name: "test", Value: 42},
			config:     &ResponseConfig{ContentType: ContentTypeTextXML},
			wantStatus: http.StatusOK,
			wantCT:     "text/xml",
			wantContains: []string{
				"<item>",
				"<name>test</name>",
			},
		},
		{
			name:       "empty content type override falls back to XML",
			status:     http.StatusOK,
			data:       item{Name: "test", Value: 42},
			config:     &ResponseConfig{},
			wantStatus: http.StatusOK,
			wantCT:     "application/xml",
			wantContains: []string{
				"<item>",
			},
		},
		{
			name:       "sets extra headers",
			status:     http.StatusOK,
			data:       item{Name: "test", Value: 42},
			config:     &ResponseConfig{Headers: map[string]string{"X-Custom": "abc"}},
			wantStatus: http.StatusOK,
			wantCT:     "application/xml",
			wantContains: []string{
				"<item>",
			},
			wantHeaders: map[string]string{"X-Custom": "abc"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			if tt.config != nil {
				ResponseXML(w, tt.status, tt.data, *tt.config)
			} else {
				ResponseXML(w, tt.status, tt.data)
			}

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
			for k, v := range tt.wantHeaders {
				assert.Equal(t, v, w.Header().Get(k))
			}
		})
	}
}
