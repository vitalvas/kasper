package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vitalvas/kasper/mux"
	"github.com/vitalvas/kasper/openapi"
)

// Item represents a simple resource.
type Item struct {
	ID        string    `json:"id" openapi:"description=Item identifier,format=uuid,readOnly"`
	Title     string    `json:"title" openapi:"description=Item title,minLength=1,maxLength=200"`
	CreatedAt time.Time `json:"created_at" openapi:"description=Creation timestamp,readOnly"`
}

// CreateItemRequest is the request body for creating or updating an item.
type CreateItemRequest struct {
	Title string `json:"title" openapi:"description=Item title,minLength=1,maxLength=200"`
}

// ListItemsQuery declares the query parameters for listing items. The same
// struct drives both mux.BindQuery (decoding) and the OpenAPI parameters
// (documentation), so they cannot drift apart.
type ListItemsQuery struct {
	Limit  int    `query:"limit,omitempty" openapi:"description=Maximum items to return,minimum=1,maximum=100"`
	Offset int    `query:"offset,omitempty" openapi:"description=Number of items to skip,minimum=0"`
	Title  string `query:"title,omitempty" openapi:"description=Filter by title substring"`
}

// ErrorResponse is a standard error response.
type ErrorResponse struct {
	Code    string `json:"code" openapi:"description=Machine-readable error code"`
	Message string `json:"message" openapi:"description=Human-readable description"`
}

func (ErrorResponse) OpenAPIName() string {
	return "GenericErrorResponse"
}

// store is a simple in-memory item store.
type store struct {
	items map[string]Item
}

func main() {
	db := &store{items: make(map[string]Item)}

	r := mux.NewRouter()

	spec := openapi.NewSpec(openapi.Info{
		Title:   "Simple Items API",
		Version: "1.0.0",
	})

	api := r.PathPrefix("/api/v1").Subrouter()

	items := spec.Group().
		Tags("items").
		Response(http.StatusNotFound, ErrorResponse{})

	items.Route(api.HandleFunc("/items", db.listItems).Methods(http.MethodGet)).
		Query(ListItemsQuery{}).
		Response(http.StatusOK, []Item{})

	items.Route(api.HandleFunc("/items", db.createItem).Methods(http.MethodPost)).
		Request(CreateItemRequest{}).
		Response(http.StatusCreated, Item{}).
		Response(http.StatusBadRequest, ErrorResponse{})

	items.Route(api.HandleFunc("/items/{id:uuid}", db.getItem).Methods(http.MethodGet)).
		Response(http.StatusOK, Item{})

	items.Route(api.HandleFunc("/items/{id:uuid}", db.updateItem).Methods(http.MethodPut)).
		Request(CreateItemRequest{}).
		Response(http.StatusOK, Item{}).
		Response(http.StatusBadRequest, ErrorResponse{})

	items.Route(api.HandleFunc("/items/{id:uuid}", db.deleteItem).Methods(http.MethodDelete)).
		Response(http.StatusNoContent, nil)

	spec.Handle(r, "/swagger", nil)

	fmt.Println("Server listening on http://localhost:8080")
	fmt.Println("  Swagger UI: http://localhost:8080/swagger/")

	log.Fatal(http.ListenAndServe(":8080", r))
}

func (s *store) listItems(w http.ResponseWriter, r *http.Request) {
	var query ListItemsQuery
	if err := mux.BindQuery(r, &query); err != nil {
		mux.ResponseJSON(w, http.StatusBadRequest, ErrorResponse{
			Code:    "INVALID_QUERY",
			Message: err.Error(),
		})
		return
	}

	result := make([]Item, 0, len(s.items))
	for _, item := range s.items {
		if query.Title != "" && !strings.Contains(item.Title, query.Title) {
			continue
		}
		result = append(result, item)
	}

	if query.Offset > 0 {
		if query.Offset >= len(result) {
			result = result[:0]
		} else {
			result = result[query.Offset:]
		}
	}

	if query.Limit > 0 && query.Limit < len(result) {
		result = result[:query.Limit]
	}

	mux.ResponseJSON(w, http.StatusOK, result)
}

func (s *store) createItem(w http.ResponseWriter, r *http.Request) {
	var req CreateItemRequest
	if err := mux.BindJSON(r, &req); err != nil {
		mux.ResponseJSON(w, http.StatusBadRequest, ErrorResponse{
			Code:    "INVALID_JSON",
			Message: err.Error(),
		})
		return
	}

	if req.Title == "" {
		mux.ResponseJSON(w, http.StatusBadRequest, ErrorResponse{
			Code:    "VALIDATION_ERROR",
			Message: "Title is required",
		})
		return
	}

	item := Item{
		ID:        uuid.New().String(),
		Title:     req.Title,
		CreatedAt: time.Now().UTC(),
	}
	s.items[item.ID] = item

	mux.ResponseJSON(w, http.StatusCreated, item)
}

func (s *store) getItem(w http.ResponseWriter, r *http.Request) {
	id, _ := mux.VarGet(r, "id")

	item, ok := s.items[id]
	if !ok {
		mux.ResponseJSON(w, http.StatusNotFound, ErrorResponse{
			Code:    "NOT_FOUND",
			Message: "Item not found",
		})
		return
	}

	mux.ResponseJSON(w, http.StatusOK, item)
}

func (s *store) updateItem(w http.ResponseWriter, r *http.Request) {
	id, _ := mux.VarGet(r, "id")

	var req CreateItemRequest
	if err := mux.BindJSON(r, &req); err != nil {
		mux.ResponseJSON(w, http.StatusBadRequest, ErrorResponse{
			Code:    "INVALID_JSON",
			Message: err.Error(),
		})
		return
	}

	if req.Title == "" {
		mux.ResponseJSON(w, http.StatusBadRequest, ErrorResponse{
			Code:    "VALIDATION_ERROR",
			Message: "Title is required",
		})
		return
	}

	item, ok := s.items[id]
	if !ok {
		mux.ResponseJSON(w, http.StatusNotFound, ErrorResponse{
			Code:    "NOT_FOUND",
			Message: "Item not found",
		})
		return
	}

	item.Title = req.Title
	s.items[id] = item

	mux.ResponseJSON(w, http.StatusOK, item)
}

func (s *store) deleteItem(w http.ResponseWriter, r *http.Request) {
	id, _ := mux.VarGet(r, "id")

	if _, ok := s.items[id]; !ok {
		mux.ResponseJSON(w, http.StatusNotFound, ErrorResponse{
			Code:    "NOT_FOUND",
			Message: "Item not found",
		})
		return
	}

	delete(s.items, id)

	w.WriteHeader(http.StatusNoContent)
}
