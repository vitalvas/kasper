package openapi

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/vitalvas/kasper/mux"
	"gopkg.in/yaml.v3"
)

// DocsUI selects which interactive documentation UI to serve.
// The UI renders the OpenAPI Document as interactive HTML documentation.
//
// See: https://spec.openapis.org/oas/v3.1.0#openapi-document
type DocsUI int

const (
	DocsSwaggerUI DocsUI = iota
	DocsRapiDoc
	DocsRedoc
)

// HandleConfig configures the endpoints registered by Handle.
// JSON and YAML endpoints serve the serialized OpenAPI Document.
//
// See: https://spec.openapis.org/oas/v3.1.0#openapi-document
type HandleConfig struct {
	// UI selects the interactive docs UI (default: DocsSwaggerUI).
	UI DocsUI

	// Title overrides the HTML page title (default: spec info.title).
	Title string

	// JSONFilename is the path for the JSON spec endpoint
	// (default: "schema.json"). Set to "-" to disable.
	//
	// Relative paths are joined with the base path:
	//
	//	"schema.json"       -> <basePath>/schema.json
	//	"data/openapi.json" -> <basePath>/data/openapi.json
	//
	// Absolute paths (starting with "/") are used as-is:
	//
	//	"/api/v1/swagger.json" -> /api/v1/swagger.json
	JSONFilename string

	// YAMLFilename is the path for the YAML spec endpoint
	// (default: "schema.yaml"). Set to "-" to disable.
	// Follows the same absolute/relative rules as JSONFilename.
	YAMLFilename string

	// DisableDocs disables the interactive HTML docs UI endpoint.
	DisableDocs bool

	// SwaggerUIConfig provides additional SwaggerUIBundle configuration options.
	// These are rendered as JavaScript object properties alongside the url and
	// dom_id defaults. For example, {"docExpansion": "none"} produces:
	//
	//	SwaggerUIBundle({url: "...", dom_id: "#swagger-ui", "docExpansion": "none"});
	//
	// Only used when UI is DocsSwaggerUI (the default).
	//
	// See: https://swagger.io/docs/open-source-tools/swagger-ui/usage/configuration/
	SwaggerUIConfig map[string]any
}

// jsonFilename returns the configured JSON spec filename, defaulting to "schema.json".
func (cfg HandleConfig) jsonFilename() string {
	if cfg.JSONFilename == "" {
		return "schema.json"
	}
	return cfg.JSONFilename
}

// yamlFilename returns the configured YAML spec filename, defaulting to "schema.yaml".
func (cfg HandleConfig) yamlFilename() string {
	if cfg.YAMLFilename == "" {
		return "schema.yaml"
	}
	return cfg.YAMLFilename
}

// resolvePath returns the full route path for a filename.
// Absolute filenames (starting with "/") are returned as-is.
// Relative filenames are joined under basePath.
func resolvePath(basePath, filename string) string {
	if strings.HasPrefix(filename, "/") {
		return filename
	}
	if basePath == "" {
		return "/" + filename
	}
	return basePath + "/" + filename
}

// Handle registers OpenAPI endpoints under the given base path on the router.
// The base path is normalized (trailing slash stripped). Depending on config,
// the following routes are registered:
//
//	<basePath>/            - interactive HTML docs (unless DisableDocs)
//	<JSONFilename path>    - OpenAPI spec as JSON  (unless JSONFilename is "-")
//	<YAMLFilename path>    - OpenAPI spec as YAML  (unless YAMLFilename is "-")
//
// The config parameter is optional; pass nil for defaults:
//
//	spec.Handle(r, "/swagger", nil)
//
// Filenames are relative to basePath by default. Use an absolute path
// (starting with "/") to serve the schema at an independent location:
//
//	spec.Handle(r, "/swagger", &HandleConfig{
//	    JSONFilename: "/api/v1/swagger.json",
//	    YAMLFilename: "-",
//	})
//	// /swagger/              -> docs UI pointing to /api/v1/swagger.json
//	// /api/v1/swagger.json   -> JSON spec
//
// Both <basePath> and <basePath>/ serve the docs UI. The spec is built once
// on first request and cached.
//
// See: https://spec.openapis.org/oas/v3.1.0#openapi-document
func (s *Spec) Handle(r *mux.Router, basePath string, cfg *HandleConfig) {
	if cfg == nil {
		cfg = &HandleConfig{}
	}
	basePath = strings.TrimRight(basePath, "/")

	jsonFile := cfg.jsonFilename()
	yamlFile := cfg.yamlFilename()

	var jsonPath, yamlPath string

	if jsonFile != "-" {
		jsonPath = resolvePath(basePath, jsonFile)
		s.registerJSON(r, jsonPath)
	}

	if yamlFile != "-" {
		yamlPath = resolvePath(basePath, yamlFile)
		s.registerYAML(r, yamlPath)
	}

	if !cfg.DisableDocs {
		// The docs UI references the JSON or YAML spec path.
		specURL := jsonPath
		if specURL == "" {
			specURL = yamlPath
		}

		// Skip docs registration when no spec endpoint is available.
		if specURL != "" {
			s.registerDocs(r, basePath, cfg, specURL)
		}
	}
}

// registerJSON registers a handler that serves the OpenAPI Document as JSON.
//
// See: https://spec.openapis.org/oas/v3.1.0#openapi-document
func (s *Spec) registerJSON(r *mux.Router, path string) {
	var (
		once     sync.Once
		data     []byte
		buildErr error
	)
	r.HandleFunc(path, func(w http.ResponseWriter, _ *http.Request) {
		once.Do(func() {
			defer func() {
				if rv := recover(); rv != nil {
					buildErr = fmt.Errorf("%v", rv)
				}
			}()
			doc := s.Build(r)
			data, buildErr = json.MarshalIndent(doc, "", "  ")
		})
		if buildErr != nil {
			http.Error(w, "failed to serialize OpenAPI spec as JSON", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	})
}

// registerYAML registers a handler that serves the OpenAPI Document as YAML.
//
// See: https://spec.openapis.org/oas/v3.1.0#openapi-document
func (s *Spec) registerYAML(r *mux.Router, path string) {
	var (
		once     sync.Once
		data     []byte
		buildErr error
	)
	r.HandleFunc(path, func(w http.ResponseWriter, _ *http.Request) {
		once.Do(func() {
			defer func() {
				if rv := recover(); rv != nil {
					buildErr = fmt.Errorf("%v", rv)
				}
			}()
			doc := s.Build(r)
			data, buildErr = yaml.Marshal(doc)
		})
		if buildErr != nil {
			http.Error(w, "failed to serialize OpenAPI spec as YAML", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/x-yaml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	})
}

// registerDocs registers a handler that serves the interactive HTML documentation UI.
func (s *Spec) registerDocs(r *mux.Router, basePath string, cfg *HandleConfig, specURL string) {
	var (
		once sync.Once
		data []byte
	)
	handler := func(w http.ResponseWriter, _ *http.Request) {
		once.Do(func() {
			title := cfg.Title
			if title == "" {
				title = s.info.Title
			}

			var page string
			switch cfg.UI {
			case DocsRapiDoc:
				page = rapidocTemplate(title, specURL)
			case DocsRedoc:
				page = redocTemplate(title, specURL)
			default:
				page = swaggerUITemplate(title, specURL, cfg.SwaggerUIConfig)
			}
			data = []byte(page)
		})
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}
	if basePath == "" {
		// Root base path: register only "/" to avoid empty path "".
		r.HandleFunc("/", handler)
	} else {
		r.HandleFunc(basePath, handler)
		r.HandleFunc(basePath+"/", handler)
	}
}

func swaggerUITemplate(title, specPath string, config map[string]any) string {
	var extra string
	if len(config) > 0 {
		keys := make([]string, 0, len(config))
		for k := range config {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		var buf strings.Builder
		for _, k := range keys {
			v, err := json.Marshal(config[k])
			if err != nil {
				continue
			}
			fmt.Fprintf(&buf, ", %s: %s", k, v)
		}
		extra = buf.String()
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>%s</title>
<link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist/swagger-ui.css">
</head>
<body>
<div id="swagger-ui"></div>
<script src="https://unpkg.com/swagger-ui-dist/swagger-ui-bundle.js"></script>
<script>
SwaggerUIBundle({url: %q, dom_id: "#swagger-ui"%s});
</script>
</body>
</html>`, html.EscapeString(title), specPath, extra)
}

func rapidocTemplate(title, specPath string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>%s</title>
<script type="module" src="https://unpkg.com/rapidoc/dist/rapidoc-min.js"></script>
</head>
<body>
<rapi-doc spec-url=%q></rapi-doc>
</body>
</html>`, html.EscapeString(title), specPath)
}

func redocTemplate(title, specPath string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>%s</title>
</head>
<body>
<redoc spec-url=%q></redoc>
<script src="https://cdn.redoc.ly/redoc/latest/bundles/redoc.standalone.js"></script>
</body>
</html>`, html.EscapeString(title), specPath)
}
