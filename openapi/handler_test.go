package openapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/kasper/mux"
	"gopkg.in/yaml.v3"
)

func setupTestRouter() (*mux.Router, *Spec) {
	r := mux.NewRouter()
	spec := NewSpec(Info{Title: "Test API", Version: "1.0.0"})

	type Item struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}

	spec.Route(r.HandleFunc("/items", dummyHandler).Methods(http.MethodGet)).
		Summary("List items").
		Tags("items").
		Response(http.StatusOK, []Item{})

	spec.Route(r.HandleFunc("/items/{id:uuid}", dummyHandler).Methods(http.MethodGet)).
		Summary("Get item").
		Tags("items").
		Response(http.StatusOK, Item{})

	return r, spec
}

func serveRequest(r *mux.Router, method, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	return w
}

func TestHandle(t *testing.T) {
	t.Run("JSON spec at /swagger/schema.json", func(t *testing.T) {
		r, spec := setupTestRouter()
		spec.Handle(r, "/swagger", nil)

		w := serveRequest(r, http.MethodGet, "/swagger/schema.json")

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

		var doc Document
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &doc))
		assert.Equal(t, "3.1.0", doc.OpenAPI)
		assert.Equal(t, "Test API", doc.Info.Title)
		assert.Contains(t, doc.Paths, "/items")
		assert.Contains(t, doc.Paths, "/items/{id}")
	})

	t.Run("YAML spec at /swagger/schema.yaml", func(t *testing.T) {
		r, spec := setupTestRouter()
		spec.Handle(r, "/swagger", nil)

		w := serveRequest(r, http.MethodGet, "/swagger/schema.yaml")

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/x-yaml", w.Header().Get("Content-Type"))

		var doc map[string]any
		require.NoError(t, yaml.Unmarshal(w.Body.Bytes(), &doc))
		assert.Equal(t, "3.1.0", doc["openapi"])
	})

	t.Run("docs UI at /swagger/", func(t *testing.T) {
		r, spec := setupTestRouter()
		spec.Handle(r, "/swagger", nil)

		w := serveRequest(r, http.MethodGet, "/swagger/")

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))

		body := w.Body.String()
		assert.Contains(t, body, "swagger-ui")
		assert.Contains(t, body, "Test API")
		assert.Contains(t, body, "/swagger/schema.json")
	})

	t.Run("docs UI at /swagger without trailing slash", func(t *testing.T) {
		r, spec := setupTestRouter()
		spec.Handle(r, "/swagger", nil)

		w := serveRequest(r, http.MethodGet, "/swagger")

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "swagger-ui")
	})

	t.Run("custom filenames", func(t *testing.T) {
		tests := []struct {
			name         string
			basePath     string
			config       *HandleConfig
			path         string
			expectedCode int
			contentType  string
		}{
			{
				name:         "trailing slash in basePath is normalized",
				basePath:     "/swagger/",
				config:       nil,
				path:         "/swagger/schema.json",
				expectedCode: http.StatusOK,
				contentType:  "application/json",
			},
			{
				name:         "custom JSON filename",
				basePath:     "/swagger",
				config:       &HandleConfig{JSONFilename: "openapi.json"},
				path:         "/swagger/openapi.json",
				expectedCode: http.StatusOK,
				contentType:  "application/json",
			},
			{
				name:         "custom YAML filename",
				basePath:     "/swagger",
				config:       &HandleConfig{YAMLFilename: "openapi.yaml"},
				path:         "/swagger/openapi.yaml",
				expectedCode: http.StatusOK,
				contentType:  "application/x-yaml",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				r, spec := setupTestRouter()
				spec.Handle(r, tt.basePath, tt.config)

				w := serveRequest(r, http.MethodGet, tt.path)
				assert.Equal(t, tt.expectedCode, w.Code)
				assert.Equal(t, tt.contentType, w.Header().Get("Content-Type"))
			})
		}
	})

	t.Run("disable JSON endpoint", func(t *testing.T) {
		r, spec := setupTestRouter()
		spec.Handle(r, "/swagger", &HandleConfig{JSONFilename: "-"})

		w := serveRequest(r, http.MethodGet, "/swagger/schema.json")
		assert.Equal(t, http.StatusNotFound, w.Code)

		w = serveRequest(r, http.MethodGet, "/swagger/schema.yaml")
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("disable YAML endpoint", func(t *testing.T) {
		r, spec := setupTestRouter()
		spec.Handle(r, "/swagger", &HandleConfig{YAMLFilename: "-"})

		w := serveRequest(r, http.MethodGet, "/swagger/schema.yaml")
		assert.Equal(t, http.StatusNotFound, w.Code)

		w = serveRequest(r, http.MethodGet, "/swagger/schema.json")
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("disable docs UI", func(t *testing.T) {
		r, spec := setupTestRouter()
		spec.Handle(r, "/swagger", &HandleConfig{DisableDocs: true})

		w := serveRequest(r, http.MethodGet, "/swagger/")
		assert.Equal(t, http.StatusNotFound, w.Code)

		w = serveRequest(r, http.MethodGet, "/swagger")
		assert.Equal(t, http.StatusNotFound, w.Code)

		w = serveRequest(r, http.MethodGet, "/swagger/schema.json")
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("docs fallback to YAML when JSON disabled", func(t *testing.T) {
		r, spec := setupTestRouter()
		spec.Handle(r, "/swagger", &HandleConfig{JSONFilename: "-"})

		w := serveRequest(r, http.MethodGet, "/swagger/")
		body := w.Body.String()
		assert.Contains(t, body, "/swagger/schema.yaml")
		assert.NotContains(t, body, "schema.json")
	})
}

func TestHandleDocsUI(t *testing.T) {
	tests := []struct {
		name         string
		basePath     string
		config       *HandleConfig
		bodyContains []string
	}{
		{
			name:         "swagger UI default",
			basePath:     "/docs",
			config:       nil,
			bodyContains: []string{"swagger-ui", "swagger-ui-bundle.js"},
		},
		{
			name:         "rapidoc",
			basePath:     "/docs",
			config:       &HandleConfig{UI: DocsRapiDoc},
			bodyContains: []string{"rapi-doc", "rapidoc"},
		},
		{
			name:         "redoc",
			basePath:     "/docs",
			config:       &HandleConfig{UI: DocsRedoc},
			bodyContains: []string{"redoc", "cdn.redoc.ly"},
		},
		{
			name:         "custom title",
			basePath:     "/docs",
			config:       &HandleConfig{Title: "Custom Docs"},
			bodyContains: []string{"Custom Docs"},
		},
		{
			name:         "spec URL points to schema.json under base path",
			basePath:     "/api/v1/docs",
			config:       nil,
			bodyContains: []string{"/api/v1/docs/schema.json"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, spec := setupTestRouter()
			spec.Handle(r, tt.basePath, tt.config)

			w := serveRequest(r, http.MethodGet, tt.basePath+"/")
			body := w.Body.String()
			for _, substr := range tt.bodyContains {
				assert.Contains(t, body, substr)
			}
		})
	}
}

func TestHandleCaching(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"JSON is cached", "/swagger/schema.json"},
		{"YAML is cached", "/swagger/schema.yaml"},
		{"docs page is cached", "/swagger/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, spec := setupTestRouter()
			spec.Handle(r, "/swagger", nil)

			w1 := serveRequest(r, http.MethodGet, tt.path)
			w2 := serveRequest(r, http.MethodGet, tt.path)
			assert.Equal(t, w1.Body.String(), w2.Body.String())
		})
	}
}

func TestHandleHTMLWellFormed(t *testing.T) {
	t.Run("HTML structure", func(t *testing.T) {
		r, spec := setupTestRouter()
		spec.Handle(r, "/swagger", nil)

		w := serveRequest(r, http.MethodGet, "/swagger/")
		body := w.Body.String()
		assert.True(t, strings.HasPrefix(body, "<!DOCTYPE html>"))
		assert.Contains(t, body, "</html>")
	})
}

func TestHandleSerializationError(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		expectedMsg string
	}{
		{"JSON returns 500 on marshal error", "/swagger/schema.json", "failed to serialize OpenAPI spec as JSON"},
		{"YAML returns 500 on marshal error", "/swagger/schema.yaml", "failed to serialize OpenAPI spec as YAML"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := mux.NewRouter()
			spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})
			spec.Route(r.HandleFunc("/items", dummyHandler).Methods(http.MethodGet)).
				Response(http.StatusOK, nil)

			spec.AddComponentExample("bad", &Example{Value: func() {}})
			spec.Handle(r, "/swagger", nil)

			w := serveRequest(r, http.MethodGet, tt.path)
			assert.Equal(t, http.StatusInternalServerError, w.Code)
			assert.Contains(t, w.Body.String(), tt.expectedMsg)
		})
	}
}

func TestHandleBothSpecsDisabled(t *testing.T) {
	t.Run("docs UI not registered when both JSON and YAML disabled", func(t *testing.T) {
		r, spec := setupTestRouter()
		spec.Handle(r, "/swagger", &HandleConfig{
			JSONFilename: "-",
			YAMLFilename: "-",
		})

		w := serveRequest(r, http.MethodGet, "/swagger/")
		assert.Equal(t, http.StatusNotFound, w.Code)

		w = serveRequest(r, http.MethodGet, "/swagger")
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestHandleRootBasePath(t *testing.T) {
	t.Run("base path / serves docs and schemas", func(t *testing.T) {
		r, spec := setupTestRouter()
		spec.Handle(r, "/", nil)

		// Docs UI at /.
		w := serveRequest(r, http.MethodGet, "/")
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))
		assert.Contains(t, w.Body.String(), "swagger-ui")
		assert.Contains(t, w.Body.String(), "/schema.json")

		// JSON spec at /schema.json.
		w = serveRequest(r, http.MethodGet, "/schema.json")
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

		// YAML spec at /schema.yaml.
		w = serveRequest(r, http.MethodGet, "/schema.yaml")
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/x-yaml", w.Header().Get("Content-Type"))
	})
}

func TestHandleAbsoluteFilename(t *testing.T) {
	tests := []struct {
		name     string
		config   *HandleConfig
		requests []struct {
			path           string
			expectedCode   int
			contentType    string
			bodyContains   string
			bodyNotContain string
		}
	}{
		{
			name: "absolute JSON path",
			config: &HandleConfig{
				JSONFilename: "/api/v1/swagger.json",
				YAMLFilename: "-",
			},
			requests: []struct {
				path           string
				expectedCode   int
				contentType    string
				bodyContains   string
				bodyNotContain string
			}{
				{"/api/v1/swagger.json", http.StatusOK, "application/json", "", ""},
				{"/swagger/", http.StatusOK, "", "/api/v1/swagger.json", ""},
				{"/swagger/swagger.json", http.StatusNotFound, "", "", ""},
			},
		},
		{
			name: "absolute YAML path",
			config: &HandleConfig{
				JSONFilename: "-",
				YAMLFilename: "/api/v1/openapi.yaml",
			},
			requests: []struct {
				path           string
				expectedCode   int
				contentType    string
				bodyContains   string
				bodyNotContain string
			}{
				{"/api/v1/openapi.yaml", http.StatusOK, "application/x-yaml", "", ""},
				{"/swagger/", http.StatusOK, "", "/api/v1/openapi.yaml", ""},
			},
		},
		{
			name:   "relative filename under basePath",
			config: &HandleConfig{JSONFilename: "swagger.json"},
			requests: []struct {
				path           string
				expectedCode   int
				contentType    string
				bodyContains   string
				bodyNotContain string
			}{
				{"/swagger/swagger.json", http.StatusOK, "", "", ""},
			},
		},
		{
			name: "relative nested path under basePath",
			config: &HandleConfig{
				JSONFilename: "data/openapi.json",
				YAMLFilename: "-",
			},
			requests: []struct {
				path           string
				expectedCode   int
				contentType    string
				bodyContains   string
				bodyNotContain string
			}{
				{"/swagger/data/openapi.json", http.StatusOK, "", "", ""},
				{"/swagger/", http.StatusOK, "", "/swagger/data/openapi.json", ""},
			},
		},
		{
			name: "mixed absolute JSON and relative YAML",
			config: &HandleConfig{
				JSONFilename: "/api/v1/swagger.json",
				YAMLFilename: "schema.yaml",
			},
			requests: []struct {
				path           string
				expectedCode   int
				contentType    string
				bodyContains   string
				bodyNotContain string
			}{
				{"/api/v1/swagger.json", http.StatusOK, "", "", ""},
				{"/swagger/schema.yaml", http.StatusOK, "", "", ""},
				{"/swagger/", http.StatusOK, "", "/api/v1/swagger.json", ""},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, spec := setupTestRouter()
			spec.Handle(r, "/swagger", tt.config)

			for _, req := range tt.requests {
				w := serveRequest(r, http.MethodGet, req.path)
				assert.Equal(t, req.expectedCode, w.Code, "path: %s", req.path)

				if req.contentType != "" {
					assert.Equal(t, req.contentType, w.Header().Get("Content-Type"), "path: %s", req.path)
				}
				if req.bodyContains != "" {
					assert.Contains(t, w.Body.String(), req.bodyContains, "path: %s", req.path)
				}
				if req.bodyNotContain != "" {
					assert.NotContains(t, w.Body.String(), req.bodyNotContain, "path: %s", req.path)
				}
			}
		})
	}
}

func TestHandleSwaggerUIConfig(t *testing.T) {
	t.Run("nil config produces default output", func(t *testing.T) {
		r, spec := setupTestRouter()
		spec.Handle(r, "/swagger", nil)

		w := serveRequest(r, http.MethodGet, "/swagger/")
		body := w.Body.String()
		assert.Contains(t, body, `SwaggerUIBundle({url: "/swagger/schema.json", dom_id: "#swagger-ui"});`)
	})

	t.Run("single option", func(t *testing.T) {
		r, spec := setupTestRouter()
		spec.Handle(r, "/swagger", &HandleConfig{
			SwaggerUIConfig: map[string]any{
				"docExpansion": "none",
			},
		})

		w := serveRequest(r, http.MethodGet, "/swagger/")
		body := w.Body.String()
		assert.Contains(t, body, `docExpansion: "none"`)
	})

	t.Run("multiple options in deterministic order", func(t *testing.T) {
		r, spec := setupTestRouter()
		spec.Handle(r, "/swagger", &HandleConfig{
			SwaggerUIConfig: map[string]any{
				"docExpansion": "none",
				"deepLinking":  true,
			},
		})

		w := serveRequest(r, http.MethodGet, "/swagger/")
		body := w.Body.String()
		// Keys sorted alphabetically: deepLinking before docExpansion.
		deepIdx := strings.Index(body, "deepLinking")
		docIdx := strings.Index(body, "docExpansion")
		require.NotEqual(t, -1, deepIdx)
		require.NotEqual(t, -1, docIdx)
		assert.Less(t, deepIdx, docIdx)
	})

	t.Run("boolean and numeric values", func(t *testing.T) {
		r, spec := setupTestRouter()
		spec.Handle(r, "/swagger", &HandleConfig{
			SwaggerUIConfig: map[string]any{
				"deepLinking":              true,
				"defaultModelsExpandDepth": 0,
			},
		})

		w := serveRequest(r, http.MethodGet, "/swagger/")
		body := w.Body.String()
		assert.Contains(t, body, `deepLinking: true`)
		assert.Contains(t, body, `defaultModelsExpandDepth: 0`)
	})

	t.Run("string values are JSON-encoded", func(t *testing.T) {
		r, spec := setupTestRouter()
		spec.Handle(r, "/swagger", &HandleConfig{
			SwaggerUIConfig: map[string]any{
				"docExpansion": `<script>alert("xss")</script>`,
			},
		})

		w := serveRequest(r, http.MethodGet, "/swagger/")
		body := w.Body.String()
		assert.NotContains(t, body, `<script>alert("xss")</script>`)
		assert.Contains(t, body, `"\u003cscript\u003ealert(\"xss\")\u003c/script\u003e"`)
	})
}

func TestHandleXSSSafe(t *testing.T) {
	t.Run("title is HTML escaped", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: `<script>alert("xss")</script>`, Version: "1.0.0"})
		spec.Handle(r, "/swagger", nil)

		w := serveRequest(r, http.MethodGet, "/swagger/")
		body := w.Body.String()
		assert.NotContains(t, body, `<script>alert("xss")</script>`)
		assert.Contains(t, body, "&lt;script&gt;")
	})
}
