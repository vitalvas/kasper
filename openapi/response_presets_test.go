package openapi

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/kasper/mux"
)

type errBody struct {
	Error string `json:"error"`
}

func TestOperationApplyJSONResponses(t *testing.T) {
	spec := NewSpec(Info{Title: "T", Version: "1"})
	router := mux.NewRouter()
	r := router.NewRoute().Path("/x").Methods(http.MethodGet).Name("x")
	spec.Group().Route(r).Summary("test").
		ApplyJSONResponses(
			JSONResponse{Status: http.StatusOK, Body: struct{ OK bool }{}},
			JSONResponse{Status: http.StatusBadRequest, Body: errBody{}},
		)

	doc := spec.Build(router)
	op := doc.Paths["/x"].Get
	require.NotNil(t, op)
	assert.Contains(t, op.Responses, "200")
	assert.Contains(t, op.Responses, "400")
}

func TestOperationStandardErrorResponses(t *testing.T) {
	spec := NewSpec(Info{Title: "T", Version: "1"})
	router := mux.NewRouter()
	r := router.NewRoute().Path("/x").Methods(http.MethodGet).Name("x")
	spec.Group().Route(r).Summary("test").
		Response(http.StatusOK, struct{}{}).
		StandardErrorResponses(errBody{})

	doc := spec.Build(router)
	op := doc.Paths["/x"].Get
	require.NotNil(t, op)
	for _, status := range []string{"400", "401", "403"} {
		assert.Contains(t, op.Responses, status, "missing %s", status)
	}
}

func TestRouteGroupApplyJSONResponses(t *testing.T) {
	spec := NewSpec(Info{Title: "T", Version: "1"})
	router := mux.NewRouter()
	r := router.NewRoute().Path("/x").Methods(http.MethodGet).Name("x")
	spec.Group().
		ApplyJSONResponses(
			JSONResponse{Status: http.StatusOK, Body: struct{ OK bool }{}},
			JSONResponse{Status: http.StatusBadRequest, Body: errBody{}},
		).
		Route(r).Summary("test")

	doc := spec.Build(router)
	op := doc.Paths["/x"].Get
	require.NotNil(t, op)
	assert.Contains(t, op.Responses, "200")
	assert.Contains(t, op.Responses, "400")
}

func TestRouteGroupStandardErrorResponses(t *testing.T) {
	spec := NewSpec(Info{Title: "T", Version: "1"})
	router := mux.NewRouter()
	r := router.NewRoute().Path("/x").Methods(http.MethodGet).Name("x")
	spec.Group().
		StandardErrorResponses(errBody{}).
		Route(r).Summary("test").
		Response(http.StatusOK, struct{}{})

	doc := spec.Build(router)
	op := doc.Paths["/x"].Get
	require.NotNil(t, op)
	for _, status := range []string{"400", "401", "403"} {
		assert.Contains(t, op.Responses, status, "missing %s", status)
	}
}
