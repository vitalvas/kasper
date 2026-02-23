package mux

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleJSON(t *testing.T) {
	type reqBody struct {
		Name string `json:"name"`
	}

	type respBody struct {
		Greeting string `json:"greeting"`
	}

	t.Run("successful round-trip", func(t *testing.T) {
		handler := HandleJSON(
			func(_ http.ResponseWriter, _ *http.Request, in reqBody) (respBody, error) {
				return respBody{Greeting: "hello " + in.Name}, nil
			},
			func(w http.ResponseWriter, _ *http.Request, err error) {
				http.Error(w, err.Error(), http.StatusBadRequest)
			},
		)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"world"}`))
		handler.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
		assert.JSONEq(t, `{"greeting":"hello world"}`, w.Body.String())
	})

	t.Run("bind error calls onError", func(t *testing.T) {
		var gotErr error
		handler := HandleJSON(
			func(_ http.ResponseWriter, _ *http.Request, _ reqBody) (respBody, error) {
				t.Fatal("handler must not be called on bind error")
				return respBody{}, nil
			},
			func(w http.ResponseWriter, _ *http.Request, err error) {
				gotErr = err
				http.Error(w, err.Error(), http.StatusBadRequest)
			},
		)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{invalid`))
		handler.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		require.Error(t, gotErr)
	})

	t.Run("handler error calls onError", func(t *testing.T) {
		handlerErr := errors.New("service failure")
		var gotErr error

		handler := HandleJSON(
			func(_ http.ResponseWriter, _ *http.Request, _ reqBody) (respBody, error) {
				return respBody{}, handlerErr
			},
			func(w http.ResponseWriter, _ *http.Request, err error) {
				gotErr = err
				http.Error(w, err.Error(), http.StatusInternalServerError)
			},
		)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"test"}`))
		handler.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.ErrorIs(t, gotErr, handlerErr)
	})

	t.Run("handler accesses route vars", func(t *testing.T) {
		handler := HandleJSON(
			func(_ http.ResponseWriter, r *http.Request, in reqBody) (respBody, error) {
				id, ok := VarGet(r, "id")
				require.True(t, ok)
				return respBody{Greeting: in.Name + ":" + id}, nil
			},
			func(w http.ResponseWriter, _ *http.Request, err error) {
				http.Error(w, err.Error(), http.StatusBadRequest)
			},
		)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/items/42", strings.NewReader(`{"name":"item"}`))
		r = SetURLVars(r, map[string]string{"id": "42"})
		handler.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `{"greeting":"item:42"}`, w.Body.String())
	})

	t.Run("handler sets response headers", func(t *testing.T) {
		handler := HandleJSON(
			func(w http.ResponseWriter, _ *http.Request, in reqBody) (respBody, error) {
				w.Header().Set("X-Custom", "value")
				return respBody{Greeting: in.Name}, nil
			},
			func(w http.ResponseWriter, _ *http.Request, err error) {
				http.Error(w, err.Error(), http.StatusBadRequest)
			},
		)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"test"}`))
		handler.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "value", w.Header().Get("X-Custom"))
	})

	t.Run("nil onError uses default 500", func(t *testing.T) {
		handler := HandleJSON(
			func(_ http.ResponseWriter, _ *http.Request, _ reqBody) (respBody, error) {
				return respBody{}, errors.New("something broke")
			},
			nil,
		)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"test"}`))
		handler.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "something broke")
	})

	t.Run("nil onError on bind error uses default 500", func(t *testing.T) {
		handler := HandleJSON(
			func(_ http.ResponseWriter, _ *http.Request, _ reqBody) (respBody, error) {
				t.Fatal("handler must not be called on bind error")
				return respBody{}, nil
			},
			nil,
		)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{invalid`))
		handler.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("empty struct input", func(t *testing.T) {
		handler := HandleJSON(
			func(_ http.ResponseWriter, _ *http.Request, _ struct{}) (respBody, error) {
				return respBody{Greeting: "ok"}, nil
			},
			func(w http.ResponseWriter, _ *http.Request, err error) {
				http.Error(w, err.Error(), http.StatusBadRequest)
			},
		)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
		handler.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `{"greeting":"ok"}`, w.Body.String())
	})
}

func TestHandleJSONResponse(t *testing.T) {
	type respBody struct {
		Greeting string `json:"greeting"`
	}

	t.Run("successful response", func(t *testing.T) {
		handler := HandleJSONResponse(
			func(_ http.ResponseWriter, _ *http.Request) (respBody, error) {
				return respBody{Greeting: "hello"}, nil
			},
			func(w http.ResponseWriter, _ *http.Request, err error) {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			},
		)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		handler.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
		assert.JSONEq(t, `{"greeting":"hello"}`, w.Body.String())
	})

	t.Run("handler error calls onError", func(t *testing.T) {
		handlerErr := errors.New("not found")
		var gotErr error

		handler := HandleJSONResponse(
			func(_ http.ResponseWriter, _ *http.Request) (respBody, error) {
				return respBody{}, handlerErr
			},
			func(w http.ResponseWriter, _ *http.Request, err error) {
				gotErr = err
				http.Error(w, err.Error(), http.StatusNotFound)
			},
		)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		handler.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.ErrorIs(t, gotErr, handlerErr)
	})

	t.Run("handler accesses route vars", func(t *testing.T) {
		handler := HandleJSONResponse(
			func(_ http.ResponseWriter, r *http.Request) (respBody, error) {
				id, ok := VarGet(r, "id")
				require.True(t, ok)
				return respBody{Greeting: "item:" + id}, nil
			},
			func(w http.ResponseWriter, _ *http.Request, err error) {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			},
		)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/items/42", nil)
		r = SetURLVars(r, map[string]string{"id": "42"})
		handler.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `{"greeting":"item:42"}`, w.Body.String())
	})

	t.Run("nil onError uses default 500", func(t *testing.T) {
		handler := HandleJSONResponse(
			func(_ http.ResponseWriter, _ *http.Request) (respBody, error) {
				return respBody{}, errors.New("something broke")
			},
			nil,
		)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		handler.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "something broke")
	})

	t.Run("handler sets response headers", func(t *testing.T) {
		handler := HandleJSONResponse(
			func(w http.ResponseWriter, _ *http.Request) (respBody, error) {
				w.Header().Set("X-Custom", "value")
				return respBody{Greeting: "ok"}, nil
			},
			func(w http.ResponseWriter, _ *http.Request, err error) {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			},
		)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		handler.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "value", w.Header().Get("X-Custom"))
	})
}
