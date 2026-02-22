package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/vitalvas/kasper/mux"
	"github.com/vitalvas/kasper/openapi"
)

// --- Domain types ---

// ResponseData is a Cloudflare-style API response wrapper.
// Errors and Messages are []any to allow strings, maps, or structured objects.
// Each generic instantiation (e.g., ResponseData[User]) produces a distinct
// OpenAPI component schema (e.g., ResponseDataUser).
type ResponseData[T any] struct {
	Success  bool  `json:"success" openapi:"description=Whether the request succeeded"`
	Errors   []any `json:"errors,omitempty" openapi:"description=Error details if success is false"`
	Messages []any `json:"messages,omitempty" openapi:"description=Informational messages"`
	Result   T     `json:"result" openapi:"description=Response payload"`
}

// OpenAPIExample returns a representative Cloudflare-style response for the OpenAPI spec.
func (ResponseData[T]) OpenAPIExample() any {
	return map[string]any{
		"success": true,
		"errors": []map[string]any{
			{
				"code":              1000,
				"message":           "message",
				"documentation_url": "documentation_url",
				"source": map[string]any{
					"pointer": "pointer",
				},
			},
		},
		"messages": []map[string]any{
			{
				"code":              1000,
				"message":           "message",
				"documentation_url": "documentation_url",
				"source": map[string]any{
					"pointer": "pointer",
				},
			},
		},
		"result": map[string]any{
			"hostnames": []string{"api.example.com"},
		},
	}
}

// User represents a user account.
type User struct {
	ID        string    `json:"id" openapi:"description=Unique user identifier,format=uuid,readOnly"`
	Name      string    `json:"name" openapi:"description=Full name,example=John Doe,minLength=1,maxLength=200"`
	Email     string    `json:"email" openapi:"description=Email address,format=email"`
	Role      string    `json:"role" openapi:"description=User role,enum=admin|editor|viewer"`
	CreatedAt time.Time `json:"created_at" openapi:"description=Account creation timestamp,readOnly"`
}

// OpenAPIExample returns a representative User for the OpenAPI spec.
func (User) OpenAPIExample() any {
	return User{
		ID:        "550e8400-e29b-41d4-a716-446655440000",
		Name:      "Alice Smith",
		Email:     "alice@example.com",
		Role:      "editor",
		CreatedAt: time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
	}
}

// CreateUserRequest is the request body for creating a user.
type CreateUserRequest struct {
	Name  string `json:"name" openapi:"description=Full name,minLength=1,maxLength=200"`
	Email string `json:"email" openapi:"description=Email address,format=email"`
	Role  string `json:"role" openapi:"description=User role,enum=admin|editor|viewer"`
}

// OpenAPIExample returns a representative CreateUserRequest for the OpenAPI spec.
func (CreateUserRequest) OpenAPIExample() any {
	return CreateUserRequest{
		Name:  "Alice Smith",
		Email: "alice@example.com",
		Role:  "editor",
	}
}

// UpdateUserRequest is the request body for partially updating a user.
type UpdateUserRequest struct {
	Name  *string `json:"name,omitempty" openapi:"description=Full name,minLength=1,maxLength=200"`
	Email *string `json:"email,omitempty" openapi:"description=Email address,format=email"`
}

// ErrorResponse is a standard error response.
type ErrorResponse struct {
	Code    string `json:"code" openapi:"description=Machine-readable error code"`
	Message string `json:"message" openapi:"description=Human-readable error description"`
}

// OpenAPIExample returns a representative ErrorResponse for the OpenAPI spec.
func (ErrorResponse) OpenAPIExample() any {
	return ErrorResponse{
		Code:    "NOT_FOUND",
		Message: "The requested resource was not found",
	}
}

// FileMetadata describes an uploaded file.
type FileMetadata struct {
	ID          string    `json:"id" openapi:"description=File identifier,format=uuid"`
	Filename    string    `json:"filename" openapi:"description=Original filename"`
	ContentType string    `json:"content_type" openapi:"description=MIME content type"`
	Size        int64     `json:"size" openapi:"description=File size in bytes,minimum=0"`
	UploadedAt  time.Time `json:"uploaded_at" openapi:"description=Upload timestamp"`
}

// FileUploadForm describes a multipart file upload.
type FileUploadForm struct {
	File        string `json:"file" openapi:"description=The file to upload"`
	Description string `json:"description,omitempty" openapi:"description=Optional file description,maxLength=500"`
}

// UserEvent is a webhook event payload.
type UserEvent struct {
	Event     string `json:"event" openapi:"description=Event type,enum=user.created|user.updated|user.deleted"`
	UserID    string `json:"user_id" openapi:"description=Affected user ID,format=uuid"`
	Timestamp string `json:"timestamp" openapi:"description=ISO 8601 timestamp,format=date-time"`
}

// SubscriptionRequest is the request body for subscribing to events.
type SubscriptionRequest struct {
	CallbackURL string   `json:"callback_url" openapi:"description=URL to receive webhook events,format=uri"`
	Events      []string `json:"events" openapi:"description=Event types to subscribe to,minItems=1"`
}

// SubscriptionResponse confirms a webhook subscription.
type SubscriptionResponse struct {
	ID     string `json:"id" openapi:"description=Subscription identifier,format=uuid"`
	Status string `json:"status" openapi:"description=Subscription status,enum=active|paused"`
}

// HealthResponse is the health check response.
type HealthResponse struct {
	Status  string `json:"status" openapi:"description=Service health status,enum=ok|degraded"`
	Version string `json:"version" openapi:"description=API version"`
}

// AuditEntry represents an audit log entry.
type AuditEntry struct {
	ID        string    `json:"id" openapi:"format=uuid"`
	Action    string    `json:"action" openapi:"description=Action performed"`
	UserID    string    `json:"user_id" openapi:"format=uuid"`
	Timestamp time.Time `json:"timestamp"`
}

// --- Handlers ---

func listUsers(w http.ResponseWriter, _ *http.Request)    { mux.ResponseJSON(w, http.StatusOK, nil) }
func createUser(w http.ResponseWriter, _ *http.Request)   { mux.ResponseJSON(w, http.StatusCreated, nil) }
func getUser(w http.ResponseWriter, _ *http.Request)      { mux.ResponseJSON(w, http.StatusOK, nil) }
func updateUser(w http.ResponseWriter, _ *http.Request)   { mux.ResponseJSON(w, http.StatusOK, nil) }
func deleteUser(w http.ResponseWriter, _ *http.Request)   { w.WriteHeader(http.StatusNoContent) }
func exportUsers(w http.ResponseWriter, _ *http.Request)  { mux.ResponseJSON(w, http.StatusOK, nil) }
func uploadFile(w http.ResponseWriter, _ *http.Request)   { mux.ResponseJSON(w, http.StatusCreated, nil) }
func downloadFile(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
func subscribeEvt(w http.ResponseWriter, _ *http.Request) {
	mux.ResponseJSON(w, http.StatusCreated, nil)
}
func healthCheck(w http.ResponseWriter, _ *http.Request) { mux.ResponseJSON(w, http.StatusOK, nil) }
func listAudit(w http.ResponseWriter, _ *http.Request)   { mux.ResponseJSON(w, http.StatusOK, nil) }

func main() {
	r := mux.NewRouter()

	// Spec with full Info metadata.
	spec := openapi.NewSpec(openapi.Info{
		Title:          "Example API",
		Summary:        "Demonstrates kasper/openapi features",
		Description:    "Each feature is attached directly to a route.",
		TermsOfService: "https://example.com/terms",
		Contact: &openapi.Contact{
			Name:  "API Support",
			URL:   "https://example.com/support",
			Email: "support@example.com",
		},
		License: &openapi.License{
			Name:       "MIT",
			Identifier: "MIT",
		},
		Version: "2.0.0",
	})

	// Servers: one with URL template variables, one static.
	spec.AddServer(openapi.Server{
		URL:         "https://{environment}.example.com/api/v1",
		Description: "Configurable environment",
		Variables: map[string]*openapi.ServerVariable{
			"environment": {
				Default:     "api",
				Enum:        []string{"api", "api.staging", "api.dev"},
				Description: "Server environment",
			},
		},
	})
	spec.AddServer(openapi.Server{
		URL:         "http://localhost:8080",
		Description: "Local development",
	})

	// Document-level external docs.
	spec.SetExternalDocs("https://example.com/docs", "Full API documentation")

	// Security schemes and document-level security.
	spec.AddSecurityScheme("bearerAuth", &openapi.SecurityScheme{
		Type:         "http",
		Scheme:       "bearer",
		BearerFormat: "JWT",
	})
	spec.AddSecurityScheme("oauth2", &openapi.SecurityScheme{
		Type:        "oauth2",
		Description: "OAuth 2.0 authorization code flow",
		Flows: &openapi.OAuthFlows{
			AuthorizationCode: &openapi.OAuthFlow{
				AuthorizationURL: "https://auth.example.com/authorize",
				TokenURL:         "https://auth.example.com/token",
				RefreshURL:       "https://auth.example.com/refresh",
				Scopes: map[string]string{
					"read:users":  "Read user data",
					"write:users": "Create and update users",
					"read:files":  "Download files",
					"write:files": "Upload files",
					"admin":       "Full administrative access",
				},
			},
		},
	})
	spec.AddSecurityScheme("apiKey", &openapi.SecurityScheme{
		Type: "apiKey",
		Name: "X-API-Key",
		In:   "header",
	})
	spec.SetSecurity(openapi.SecurityRequirement{"bearerAuth": {}})

	// Tag descriptions (referenced by routes below).
	spec.AddTag(openapi.Tag{
		Name:         "users",
		Description:  "User management operations",
		ExternalDocs: &openapi.ExternalDocs{URL: "https://example.com/docs/users"},
	})
	spec.AddTag(openapi.Tag{Name: "files", Description: "File upload and download"})
	spec.AddTag(openapi.Tag{Name: "events", Description: "Event subscription and webhooks"})
	spec.AddTag(openapi.Tag{Name: "health", Description: "Health check endpoints"})
	spec.AddTag(openapi.Tag{Name: "admin", Description: "Administrative operations (deprecated)"})

	// Path-level metadata: applies to all operations under /api/v1/users/{id}.
	spec.SetPathSummary("/api/v1/users/{id}", "Single user resource")
	spec.SetPathDescription("/api/v1/users/{id}", "Operations on an individual user identified by UUID.")
	spec.AddPathParameter("/api/v1/users/{id}", &openapi.Parameter{
		Name: "X-Tenant-ID", In: "header",
		Description: "Tenant identifier for multi-tenancy",
		Schema:      &openapi.Schema{Type: openapi.TypeString("string")},
	})

	// Subrouter: all routes share the /api/v1 prefix.
	api := r.PathPrefix("/api/v1").Subrouter()

	// ---------------------------------------------------------------
	// Health: public endpoint (Security() override with no arguments).
	// ---------------------------------------------------------------
	spec.Route(api.HandleFunc("/health", healthCheck).Methods(http.MethodGet)).
		OperationID("healthCheck").
		Summary("Health check").
		Description("Returns service health. No authentication required.").
		Tags("health").
		Security().
		Response(http.StatusOK, HealthResponse{})

	// ---------------------------------------------------------------
	// User routes via Group: shared tags, shared error responses (JSON + XML),
	// shared default error, and shared response header.
	// Every route in this group inherits these automatically.
	// ---------------------------------------------------------------
	users := spec.Group().
		Tags("users").
		Security(openapi.SecurityRequirement{"oauth2": {"read:users", "write:users"}}).
		Response(http.StatusForbidden, ErrorResponse{}).
		ResponseContent(http.StatusForbidden, "application/xml", ErrorResponse{}).
		ResponseDescription(http.StatusForbidden, "Insufficient permissions").
		Response(http.StatusNotFound, ErrorResponse{}).
		ResponseContent(http.StatusNotFound, "application/xml", ErrorResponse{}).
		ResponseDescription(http.StatusNotFound, "User not found").
		DefaultResponse(ErrorResponse{}).
		DefaultResponseDescription("Unexpected error").
		DefaultResponseHeader("X-Request-ID", &openapi.Header{
			Description: "Request trace identifier",
			Schema:      &openapi.Schema{Type: openapi.TypeString("string"), Format: "uuid"},
		})

	// List users: wrapped in ResponseData, query parameters, response headers, links.
	// 403, 404, default error, and X-Request-ID header inherited from group.
	users.Route(api.HandleFunc("/users", listUsers).Methods(http.MethodGet)).
		OperationID("listUsers").
		Summary("List users").
		Description("Returns a paginated list of users.").
		ExternalDocs("https://example.com/docs/users#list", "Pagination guide").
		Parameter(&openapi.Parameter{
			Name: "page", In: "query",
			Description: "Page number",
			Schema:      &openapi.Schema{Type: openapi.TypeString("integer")},
		}).
		Parameter(&openapi.Parameter{
			Name: "per_page", In: "query",
			Description: "Items per page",
			Schema:      &openapi.Schema{Type: openapi.TypeString("integer")},
		}).
		Response(http.StatusOK, ResponseData[[]User]{}).
		ResponseHeader(http.StatusOK, "X-Total-Count", &openapi.Header{
			Description: "Total number of users",
			Schema:      &openapi.Schema{Type: openapi.TypeString("integer")},
		}).
		ResponseLink(http.StatusOK, "GetNextPage", &openapi.Link{
			OperationID: "listUsers",
			Parameters:  map[string]any{"page": "$response.body#/next_page"},
		})

	// Create user: accepts JSON and XML, response wrapped in ResponseData.
	users.Route(api.HandleFunc("/users", createUser).Methods(http.MethodPost)).
		OperationID("createUser").
		Summary("Create user").
		Request(CreateUserRequest{}).
		RequestContent("application/xml", CreateUserRequest{}).
		RequestDescription("User account to create").
		Response(http.StatusCreated, ResponseData[User]{}).
		ResponseDescription(http.StatusCreated, "The newly created user").
		ResponseLink(http.StatusCreated, "GetCreatedUser", &openapi.Link{
			OperationID: "getUser",
			Parameters:  map[string]any{"id": "$response.body#/result/id"},
		}).
		Response(http.StatusBadRequest, ErrorResponse{}).
		ResponseDescription(http.StatusBadRequest, "Validation error")

	// Get user: path parameter auto-typed from {id:uuid} mux macro,
	// response wrapped in ResponseData, JSON and XML.
	// 403 and 404 inherited from group.
	users.Route(api.HandleFunc("/users/{id:uuid}", getUser).Methods(http.MethodGet)).
		OperationID("getUser").
		Summary("Get user by ID").
		Response(http.StatusOK, ResponseData[User]{}).
		ResponseContent(http.StatusOK, "application/xml", ResponseData[User]{})

	// Update user: optional request body (RequestRequired=false),
	// response wrapped in ResponseData. 403 and 404 inherited from group.
	users.Route(api.HandleFunc("/users/{id:uuid}", updateUser).Methods(http.MethodPut)).
		OperationID("updateUser").
		Summary("Update user").
		Request(UpdateUserRequest{}).
		RequestRequired(false).
		Response(http.StatusOK, ResponseData[User]{})

	// Delete user: 204 No Content (nil body). 403 and 404 inherited from group.
	users.Route(api.HandleFunc("/users/{id:uuid}", deleteUser).Methods(http.MethodDelete)).
		OperationID("deleteUser").
		Summary("Delete user").
		Response(http.StatusNoContent, nil)

	// Export users: multiple content types on the same response (JSON, XML, CSV).
	users.Route(api.HandleFunc("/users/export", exportUsers).Methods(http.MethodGet)).
		OperationID("exportUsers").
		Summary("Export users").
		Description("Returns users in JSON, XML, or CSV based on Accept header.").
		Response(http.StatusOK, []User{}).
		ResponseContent(http.StatusOK, "application/xml", []User{}).
		ResponseContent(http.StatusOK, "text/csv", &openapi.Schema{
			Type: openapi.TypeString("string"),
		})

	// ---------------------------------------------------------------
	// File routes via Group: shared tags, server override, shared errors,
	// and shared response link on 404.
	// ---------------------------------------------------------------
	files := spec.Group().
		Tags("files").
		Security(openapi.SecurityRequirement{"oauth2": {"read:files", "write:files"}}).
		Server(openapi.Server{URL: "https://files.example.com", Description: "File storage"}).
		Response(http.StatusForbidden, ErrorResponse{}).
		Response(http.StatusNotFound, ErrorResponse{}).
		ResponseLink(http.StatusNotFound, "ListFiles", &openapi.Link{
			OperationID: "uploadFile",
			Description: "Upload a new file",
		})

	// Upload file: multipart/form-data request content type.
	files.Route(api.HandleFunc("/files", uploadFile).Methods(http.MethodPost)).
		OperationID("uploadFile").
		Summary("Upload file").
		RequestContent("multipart/form-data", FileUploadForm{}).
		RequestDescription("File with optional metadata").
		Response(http.StatusCreated, FileMetadata{})

	// Download file: binary response content type, path-level server override,
	// response header (Content-Disposition). 403 and 404 inherited from group.
	spec.AddPathServer("/api/v1/files/{id}", openapi.Server{
		URL:         "https://cdn.example.com",
		Description: "CDN for downloads",
	})

	files.Route(api.HandleFunc("/files/{id:uuid}", downloadFile).Methods(http.MethodGet)).
		OperationID("downloadFile").
		Summary("Download file").
		ResponseContent(http.StatusOK, "application/octet-stream", &openapi.Schema{
			Type: openapi.TypeString("string"), Format: "binary",
		}).
		ResponseHeader(http.StatusOK, "Content-Disposition", &openapi.Header{
			Description: "Suggested filename",
			Schema:      &openapi.Schema{Type: openapi.TypeString("string")},
		})

	// ---------------------------------------------------------------
	// Event subscription: callback attached to the route.
	// ---------------------------------------------------------------
	webhookCallback := openapi.Callback{
		"{$request.body#/callback_url}": &openapi.PathItem{
			Post: &openapi.Operation{
				Summary: "Receive user event",
				RequestBody: &openapi.RequestBody{
					Required: true,
					Content: map[string]*openapi.MediaType{
						"application/json": {
							Schema: &openapi.Schema{Ref: "#/components/schemas/UserEvent"},
						},
					},
				},
				Responses: map[string]*openapi.Response{
					"200": {Description: "Event acknowledged"},
				},
			},
		},
	}

	spec.Route(api.HandleFunc("/events/subscribe", subscribeEvt).Methods(http.MethodPost)).
		OperationID("subscribeToEvents").
		Summary("Subscribe to events").
		Tags("events").
		Request(SubscriptionRequest{}).
		Response(http.StatusCreated, SubscriptionResponse{}).
		Response(http.StatusBadRequest, ErrorResponse{}).
		Callback("userEvent", &webhookCallback)

	// ---------------------------------------------------------------
	// Webhooks: API-initiated callbacks not tied to mux routes.
	// ---------------------------------------------------------------
	spec.Webhook("userCreated", http.MethodPost).
		Summary("User created event").
		Tags("events").
		OperationID("onUserCreated").
		Request(UserEvent{}).
		Response(http.StatusOK, nil).
		ResponseDescription(http.StatusOK, "Event acknowledged")

	spec.Webhook("userDeleted", http.MethodPost).
		Summary("User deleted event").
		Tags("events").
		OperationID("onUserDeleted").
		Request(UserEvent{}).
		Response(http.StatusOK, nil)

	// Webhook via Group: inherits group tags and security.
	adminEvents := spec.Group().
		Tags("admin", "events").
		Security(openapi.SecurityRequirement{"apiKey": {}})

	adminEvents.Webhook("auditEvent", http.MethodPost).
		Summary("Audit event").
		OperationID("onAuditEvent").
		Request(AuditEntry{}).
		Response(http.StatusOK, nil)

	// ---------------------------------------------------------------
	// Deprecated group: shared deprecated flag, security override,
	// group-level parameter and external docs.
	// ---------------------------------------------------------------
	admin := spec.Group().
		Tags("admin").
		Deprecated().
		Security(openapi.SecurityRequirement{"apiKey": {}}).
		Parameter(&openapi.Parameter{
			Name: "X-Admin-Token", In: "header",
			Description: "Admin authorization token",
			Schema:      &openapi.Schema{Type: openapi.TypeString("string")},
		}).
		ExternalDocs("https://example.com/docs/admin", "Admin API guide")

	// Audit log: deprecated route with default response link.
	admin.Route(api.HandleFunc("/admin/audit", listAudit).Methods(http.MethodGet)).
		OperationID("listAuditLog").
		Summary("List audit log").
		Description("Deprecated: use the events API instead.").
		Parameter(&openapi.Parameter{
			Name: "since", In: "query",
			Description: "Filter entries after this date",
			Schema:      &openapi.Schema{Type: openapi.TypeString("string"), Format: "date"},
		}).
		Response(http.StatusOK, []AuditEntry{}).
		DefaultResponse(ErrorResponse{}).
		DefaultResponseLink("SubscribeEvents", &openapi.Link{
			OperationID: "subscribeToEvents",
			Description: "Use the events API instead",
		})

	// Serve OpenAPI docs.
	spec.Handle(r, "/swagger", nil)

	fmt.Println("Server listening on http://localhost:8080")
	fmt.Println("  Swagger UI:    http://localhost:8080/swagger/")
	fmt.Println("  OpenAPI JSON:  http://localhost:8080/swagger/schema.json")
	fmt.Println("  OpenAPI YAML:  http://localhost:8080/swagger/schema.yaml")

	log.Fatal(http.ListenAndServe(":8080", r))
}
