// Command apigateway demonstrates aggregating OpenAPI schemas from several
// child services into a single specification.
//
// Two child services (users and posts) each run their own HTTP server and
// expose their own OpenAPI document. The aggregator starts the children,
// pulls each child's schema over HTTP, merges them with
// openapi.MergeDocuments, and serves the combined specification. Incoming API
// requests are reverse-proxied to the owning child service.
//
//	aggregator  :8080  -> serves merged spec, proxies /api/v1/*
//	users       :8081  -> owns /api/v1/users
//	posts       :8082  -> owns /api/v1/posts
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/vitalvas/kasper/mux"
	"github.com/vitalvas/kasper/openapi"
)

const (
	rootAddr  = "127.0.0.1:8080"
	usersAddr = "127.0.0.1:8081"
	postsAddr = "127.0.0.1:8082"

	usersBaseURL = "http://127.0.0.1:8081"
	postsBaseURL = "http://127.0.0.1:8082"

	schemaPath = "/swagger/schema.json"
)

// --- Shared domain types ---

// ErrorResponse is the standard error envelope. Both child services declare
// an identical schema, so the merge deduplicates it instead of conflicting.
type ErrorResponse struct {
	Code    string `json:"code" openapi:"description=Machine-readable error code"`
	Message string `json:"message" openapi:"description=Human-readable description"`
}

// User is owned by the users service.
type User struct {
	ID    string `json:"id" openapi:"description=User identifier,format=uuid,readOnly"`
	Name  string `json:"name" openapi:"description=Full name,minLength=1,maxLength=200"`
	Email string `json:"email" openapi:"description=Email address,format=email"`
}

// Post is owned by the posts service.
type Post struct {
	ID       string `json:"id" openapi:"description=Post identifier,format=uuid,readOnly"`
	AuthorID string `json:"author_id" openapi:"description=Author user identifier,format=uuid"`
	Title    string `json:"title" openapi:"description=Post title,minLength=1,maxLength=300"`
	Body     string `json:"body" openapi:"description=Post body"`
}

func main() {
	// Start the child services in the background.
	go func() { log.Fatal(http.ListenAndServe(usersAddr, usersService())) }()
	go func() { log.Fatal(http.ListenAndServe(postsAddr, postsService())) }()

	// Wait for both children to serve their schema endpoints.
	children := map[string]string{
		"users": usersBaseURL,
		"posts": postsBaseURL,
	}
	for name, base := range children {
		if err := waitForSchema(schemaURL(base), 5*time.Second); err != nil {
			log.Fatalf("child %q schema never became available: %v", name, err)
		}
	}

	// Pull each child schema and combine them into one spec.
	spec, err := buildAggregatedSpec(children)
	if err != nil {
		log.Fatalf("aggregate child schemas: %v", err)
	}

	r := mux.NewRouter()

	// Serve the aggregated specification and Swagger UI.
	spec.Handle(r, "/swagger", nil)

	// Reverse-proxy API traffic to the owning child service.
	if err := proxyTo(r, "/api/v1/users", usersBaseURL); err != nil {
		log.Fatalf("configure users proxy: %v", err)
	}
	if err := proxyTo(r, "/api/v1/posts", postsBaseURL); err != nil {
		log.Fatalf("configure posts proxy: %v", err)
	}

	fmt.Printf("Listening on http://%s\n", rootAddr)
	fmt.Printf("  OpenAPI JSON: http://%s%s\n", rootAddr, schemaPath)
	fmt.Printf("  Swagger UI:   http://%s/swagger/\n", rootAddr)
	fmt.Printf("  Users API:    http://%s/api/v1/users\n", rootAddr)
	fmt.Printf("  Posts API:    http://%s/api/v1/posts\n", rootAddr)

	log.Fatal(http.ListenAndServe(rootAddr, r))
}

// buildAggregatedSpec fetches every child schema and combines them into a
// single spec that serves the whole API surface and its Swagger UI.
func buildAggregatedSpec(children map[string]string) (*openapi.Spec, error) {
	docs := make([]*openapi.Document, 0, len(children))
	for name, base := range children {
		doc, err := fetchSchema(schemaURL(base))
		if err != nil {
			return nil, fmt.Errorf("fetch %q schema: %w", name, err)
		}
		docs = append(docs, doc)
	}

	return openapi.SpecFromDocuments(docs...)
}

// fetchSchema retrieves and parses an OpenAPI document from a URL.
func fetchSchema(specURL string) (*openapi.Document, error) {
	resp, err := http.Get(specURL) //nolint:gosec // fixed local child addresses
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return openapi.DocumentFromJSON(data)
}

// waitForSchema polls a schema URL until it responds 200 or the timeout
// elapses.
func waitForSchema(specURL string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, specURL, nil)
		if err != nil {
			return err
		}
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// proxyTo reverse-proxies every request under prefix to the given target.
func proxyTo(r *mux.Router, prefix, target string) error {
	u, err := url.Parse(target)
	if err != nil {
		return err
	}
	r.PathPrefix(prefix).Handler(httputil.NewSingleHostReverseProxy(u))
	return nil
}

// schemaURL returns the schema endpoint URL for a child base URL.
func schemaURL(base string) string {
	return fmt.Sprintf("%s%s", base, schemaPath)
}

// --- Users child service ---

func usersService() http.Handler {
	r := mux.NewRouter()

	spec := openapi.NewSpec(openapi.Info{Title: "Users Service", Version: "1.0.0"})

	users := spec.Group().Tags("users").Response(http.StatusNotFound, ErrorResponse{})

	users.Route(r.HandleFunc("/api/v1/users", listUsers).Methods(http.MethodGet)).
		OperationID("listUsers").
		Summary("List users").
		Response(http.StatusOK, []User{})

	users.Route(r.HandleFunc("/api/v1/users", createUser).Methods(http.MethodPost)).
		OperationID("createUser").
		Summary("Create user").
		Request(User{}).
		Response(http.StatusCreated, User{}).
		Response(http.StatusBadRequest, ErrorResponse{})

	spec.Handle(r, "/swagger", nil)
	return r
}

func listUsers(w http.ResponseWriter, _ *http.Request) {
	mux.ResponseJSON(w, http.StatusOK, []User{
		{ID: "11111111-1111-1111-1111-111111111111", Name: "Alice", Email: "alice@example.com"},
	})
}

func createUser(w http.ResponseWriter, r *http.Request) {
	var u User
	if err := mux.BindJSON(r, &u); err != nil {
		mux.ResponseJSON(w, http.StatusBadRequest, ErrorResponse{Code: "INVALID_JSON", Message: err.Error()})
		return
	}
	u.ID = "22222222-2222-2222-2222-222222222222"
	mux.ResponseJSON(w, http.StatusCreated, u)
}

// --- Posts child service ---

func postsService() http.Handler {
	r := mux.NewRouter()

	spec := openapi.NewSpec(openapi.Info{Title: "Posts Service", Version: "1.0.0"})

	posts := spec.Group().Tags("posts").Response(http.StatusNotFound, ErrorResponse{})

	posts.Route(r.HandleFunc("/api/v1/posts", listPosts).Methods(http.MethodGet)).
		OperationID("listPosts").
		Summary("List posts").
		Response(http.StatusOK, []Post{})

	posts.Route(r.HandleFunc("/api/v1/posts", createPost).Methods(http.MethodPost)).
		OperationID("createPost").
		Summary("Create post").
		Request(Post{}).
		Response(http.StatusCreated, Post{}).
		Response(http.StatusBadRequest, ErrorResponse{})

	spec.Handle(r, "/swagger", nil)
	return r
}

func listPosts(w http.ResponseWriter, _ *http.Request) {
	mux.ResponseJSON(w, http.StatusOK, []Post{
		{
			ID:       "33333333-3333-3333-3333-333333333333",
			AuthorID: "11111111-1111-1111-1111-111111111111",
			Title:    "Hello",
			Body:     "First post",
		},
	})
}

func createPost(w http.ResponseWriter, r *http.Request) {
	var p Post
	if err := mux.BindJSON(r, &p); err != nil {
		mux.ResponseJSON(w, http.StatusBadRequest, ErrorResponse{Code: "INVALID_JSON", Message: err.Error()})
		return
	}
	p.ID = "44444444-4444-4444-4444-444444444444"
	mux.ResponseJSON(w, http.StatusCreated, p)
}
