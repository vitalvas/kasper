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
		spec.Handle(r, "/swagger", HandleConfig{})

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
		spec.Handle(r, "/swagger", HandleConfig{})

		w := serveRequest(r, http.MethodGet, "/swagger/schema.yaml")

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/x-yaml", w.Header().Get("Content-Type"))

		var doc map[string]any
		require.NoError(t, yaml.Unmarshal(w.Body.Bytes(), &doc))
		assert.Equal(t, "3.1.0", doc["openapi"])
	})

	t.Run("docs UI at /swagger/", func(t *testing.T) {
		r, spec := setupTestRouter()
		spec.Handle(r, "/swagger", HandleConfig{})

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
		spec.Handle(r, "/swagger", HandleConfig{})

		w := serveRequest(r, http.MethodGet, "/swagger")

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "swagger-ui")
	})

	t.Run("trailing slash in basePath is normalized", func(t *testing.T) {
		r, spec := setupTestRouter()
		spec.Handle(r, "/swagger/", HandleConfig{})

		w := serveRequest(r, http.MethodGet, "/swagger/schema.json")
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	})

	t.Run("custom JSON filename", func(t *testing.T) {
		r, spec := setupTestRouter()
		spec.Handle(r, "/swagger", HandleConfig{JSONFilename: "openapi.json"})

		w := serveRequest(r, http.MethodGet, "/swagger/openapi.json")
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	})

	t.Run("custom YAML filename", func(t *testing.T) {
		r, spec := setupTestRouter()
		spec.Handle(r, "/swagger", HandleConfig{YAMLFilename: "openapi.yaml"})

		w := serveRequest(r, http.MethodGet, "/swagger/openapi.yaml")
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/x-yaml", w.Header().Get("Content-Type"))
	})

	t.Run("disable JSON endpoint", func(t *testing.T) {
		r, spec := setupTestRouter()
		spec.Handle(r, "/swagger", HandleConfig{JSONFilename: "-"})

		w := serveRequest(r, http.MethodGet, "/swagger/schema.json")
		assert.Equal(t, http.StatusNotFound, w.Code)

		w = serveRequest(r, http.MethodGet, "/swagger/schema.yaml")
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("disable YAML endpoint", func(t *testing.T) {
		r, spec := setupTestRouter()
		spec.Handle(r, "/swagger", HandleConfig{YAMLFilename: "-"})

		w := serveRequest(r, http.MethodGet, "/swagger/schema.yaml")
		assert.Equal(t, http.StatusNotFound, w.Code)

		w = serveRequest(r, http.MethodGet, "/swagger/schema.json")
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("disable docs UI", func(t *testing.T) {
		r, spec := setupTestRouter()
		spec.Handle(r, "/swagger", HandleConfig{DisableDocs: true})

		w := serveRequest(r, http.MethodGet, "/swagger/")
		assert.Equal(t, http.StatusNotFound, w.Code)

		w = serveRequest(r, http.MethodGet, "/swagger")
		assert.Equal(t, http.StatusNotFound, w.Code)

		w = serveRequest(r, http.MethodGet, "/swagger/schema.json")
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("docs fallback to YAML when JSON disabled", func(t *testing.T) {
		r, spec := setupTestRouter()
		spec.Handle(r, "/swagger", HandleConfig{JSONFilename: "-"})

		w := serveRequest(r, http.MethodGet, "/swagger/")
		body := w.Body.String()
		assert.Contains(t, body, "/swagger/schema.yaml")
		assert.NotContains(t, body, "schema.json")
	})
}

func TestHandleDocsUI(t *testing.T) {
	t.Run("swagger UI default", func(t *testing.T) {
		r, spec := setupTestRouter()
		spec.Handle(r, "/docs", HandleConfig{})

		w := serveRequest(r, http.MethodGet, "/docs/")
		body := w.Body.String()
		assert.Contains(t, body, "swagger-ui")
		assert.Contains(t, body, "swagger-ui-bundle.js")
	})

	t.Run("rapidoc", func(t *testing.T) {
		r, spec := setupTestRouter()
		spec.Handle(r, "/docs", HandleConfig{UI: DocsRapiDoc})

		w := serveRequest(r, http.MethodGet, "/docs/")
		body := w.Body.String()
		assert.Contains(t, body, "rapi-doc")
		assert.Contains(t, body, "rapidoc")
	})

	t.Run("redoc", func(t *testing.T) {
		r, spec := setupTestRouter()
		spec.Handle(r, "/docs", HandleConfig{UI: DocsRedoc})

		w := serveRequest(r, http.MethodGet, "/docs/")
		body := w.Body.String()
		assert.Contains(t, body, "redoc")
		assert.Contains(t, body, "cdn.redoc.ly")
	})

	t.Run("custom title", func(t *testing.T) {
		r, spec := setupTestRouter()
		spec.Handle(r, "/docs", HandleConfig{Title: "Custom Docs"})

		w := serveRequest(r, http.MethodGet, "/docs/")
		assert.Contains(t, w.Body.String(), "Custom Docs")
	})

	t.Run("spec URL points to schema.json under base path", func(t *testing.T) {
		r, spec := setupTestRouter()
		spec.Handle(r, "/api/v1/docs", HandleConfig{})

		w := serveRequest(r, http.MethodGet, "/api/v1/docs/")
		assert.Contains(t, w.Body.String(), "/api/v1/docs/schema.json")
	})
}

func TestHandleCaching(t *testing.T) {
	t.Run("JSON is cached", func(t *testing.T) {
		r, spec := setupTestRouter()
		spec.Handle(r, "/swagger", HandleConfig{})

		w1 := serveRequest(r, http.MethodGet, "/swagger/schema.json")
		w2 := serveRequest(r, http.MethodGet, "/swagger/schema.json")
		assert.Equal(t, w1.Body.String(), w2.Body.String())
	})

	t.Run("YAML is cached", func(t *testing.T) {
		r, spec := setupTestRouter()
		spec.Handle(r, "/swagger", HandleConfig{})

		w1 := serveRequest(r, http.MethodGet, "/swagger/schema.yaml")
		w2 := serveRequest(r, http.MethodGet, "/swagger/schema.yaml")
		assert.Equal(t, w1.Body.String(), w2.Body.String())
	})

	t.Run("docs page is cached", func(t *testing.T) {
		r, spec := setupTestRouter()
		spec.Handle(r, "/swagger", HandleConfig{})

		w1 := serveRequest(r, http.MethodGet, "/swagger/")
		w2 := serveRequest(r, http.MethodGet, "/swagger/")
		assert.Equal(t, w1.Body.String(), w2.Body.String())
	})
}

func TestHandleHTMLWellFormed(t *testing.T) {
	t.Run("HTML structure", func(t *testing.T) {
		r, spec := setupTestRouter()
		spec.Handle(r, "/swagger", HandleConfig{})

		w := serveRequest(r, http.MethodGet, "/swagger/")
		body := w.Body.String()
		assert.True(t, strings.HasPrefix(body, "<!DOCTYPE html>"))
		assert.Contains(t, body, "</html>")
	})
}

func TestHandleSerializationError(t *testing.T) {
	t.Run("JSON returns 500 on marshal error", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})
		spec.Route(r.HandleFunc("/items", dummyHandler).Methods(http.MethodGet)).
			Response(http.StatusOK, nil)

		// Inject an unserializable value (func) via component example.
		spec.AddComponentExample("bad", &Example{Value: func() {}})

		spec.Handle(r, "/swagger", HandleConfig{})

		w := serveRequest(r, http.MethodGet, "/swagger/schema.json")
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "failed to serialize OpenAPI spec as JSON")
	})

	t.Run("YAML returns 500 on marshal error", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})
		spec.Route(r.HandleFunc("/items", dummyHandler).Methods(http.MethodGet)).
			Response(http.StatusOK, nil)

		// Inject an unserializable value (func) via component example.
		spec.AddComponentExample("bad", &Example{Value: func() {}})

		spec.Handle(r, "/swagger", HandleConfig{})

		w := serveRequest(r, http.MethodGet, "/swagger/schema.yaml")
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "failed to serialize OpenAPI spec as YAML")
	})
}

func TestHandleBothSpecsDisabled(t *testing.T) {
	t.Run("docs UI not registered when both JSON and YAML disabled", func(t *testing.T) {
		r, spec := setupTestRouter()
		spec.Handle(r, "/swagger", HandleConfig{
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
		spec.Handle(r, "/", HandleConfig{})

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
	t.Run("absolute JSON path", func(t *testing.T) {
		r, spec := setupTestRouter()
		spec.Handle(r, "/swagger", HandleConfig{
			JSONFilename: "/api/v1/swagger.json",
			YAMLFilename: "-",
		})

		// JSON spec at absolute path.
		w := serveRequest(r, http.MethodGet, "/api/v1/swagger.json")
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

		var doc Document
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &doc))
		assert.Equal(t, "3.1.0", doc.OpenAPI)

		// Docs UI points to the absolute path.
		w = serveRequest(r, http.MethodGet, "/swagger/")
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "/api/v1/swagger.json")

		// Not served under basePath.
		w = serveRequest(r, http.MethodGet, "/swagger/swagger.json")
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("absolute YAML path", func(t *testing.T) {
		r, spec := setupTestRouter()
		spec.Handle(r, "/swagger", HandleConfig{
			JSONFilename: "-",
			YAMLFilename: "/api/v1/openapi.yaml",
		})

		w := serveRequest(r, http.MethodGet, "/api/v1/openapi.yaml")
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/x-yaml", w.Header().Get("Content-Type"))

		// Docs UI falls back to YAML absolute path.
		w = serveRequest(r, http.MethodGet, "/swagger/")
		assert.Contains(t, w.Body.String(), "/api/v1/openapi.yaml")
	})

	t.Run("relative filename under basePath", func(t *testing.T) {
		r, spec := setupTestRouter()
		spec.Handle(r, "/swagger", HandleConfig{
			JSONFilename: "swagger.json",
		})

		w := serveRequest(r, http.MethodGet, "/swagger/swagger.json")
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("relative nested path under basePath", func(t *testing.T) {
		r, spec := setupTestRouter()
		spec.Handle(r, "/swagger", HandleConfig{
			JSONFilename: "data/openapi.json",
			YAMLFilename: "-",
		})

		w := serveRequest(r, http.MethodGet, "/swagger/data/openapi.json")
		assert.Equal(t, http.StatusOK, w.Code)

		w = serveRequest(r, http.MethodGet, "/swagger/")
		assert.Contains(t, w.Body.String(), "/swagger/data/openapi.json")
	})

	t.Run("mixed absolute JSON and relative YAML", func(t *testing.T) {
		r, spec := setupTestRouter()
		spec.Handle(r, "/swagger", HandleConfig{
			JSONFilename: "/api/v1/swagger.json",
			YAMLFilename: "schema.yaml",
		})

		w := serveRequest(r, http.MethodGet, "/api/v1/swagger.json")
		assert.Equal(t, http.StatusOK, w.Code)

		w = serveRequest(r, http.MethodGet, "/swagger/schema.yaml")
		assert.Equal(t, http.StatusOK, w.Code)

		// Docs UI prefers JSON.
		w = serveRequest(r, http.MethodGet, "/swagger/")
		assert.Contains(t, w.Body.String(), "/api/v1/swagger.json")
	})
}

func TestHandleXSSSafe(t *testing.T) {
	t.Run("title is HTML escaped", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: `<script>alert("xss")</script>`, Version: "1.0.0"})
		spec.Handle(r, "/swagger", HandleConfig{})

		w := serveRequest(r, http.MethodGet, "/swagger/")
		body := w.Body.String()
		assert.NotContains(t, body, `<script>alert("xss")</script>`)
		assert.Contains(t, body, "&lt;script&gt;")
	})
}
