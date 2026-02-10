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

	t.Run("writes JSON with status code", func(t *testing.T) {
		w := httptest.NewRecorder()
		ResponseJSON(w, http.StatusCreated, item{Name: "test", Value: 42})

		assert.Equal(t, http.StatusCreated, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
		assert.JSONEq(t, `{"name":"test","value":42}`, w.Body.String())
	})

	t.Run("writes JSON array", func(t *testing.T) {
		w := httptest.NewRecorder()
		items := []item{{Name: "a", Value: 1}, {Name: "b", Value: 2}}
		ResponseJSON(w, http.StatusOK, items)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
		assert.JSONEq(t, `[{"name":"a","value":1},{"name":"b","value":2}]`, w.Body.String())
	})

	t.Run("writes null for nil", func(t *testing.T) {
		w := httptest.NewRecorder()
		ResponseJSON(w, http.StatusOK, nil)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
		assert.Equal(t, "null\n", w.Body.String())
	})

	t.Run("writes 500 on encode error", func(t *testing.T) {
		w := httptest.NewRecorder()
		ResponseJSON(w, http.StatusOK, make(chan int))

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.NotEqual(t, "application/json", w.Header().Get("Content-Type"))
	})
}

func TestResponseXML(t *testing.T) {
	type item struct {
		XMLName xml.Name `xml:"item"`
		Name    string   `xml:"name"`
		Value   int      `xml:"value"`
	}

	t.Run("writes XML with status code", func(t *testing.T) {
		w := httptest.NewRecorder()
		ResponseXML(w, http.StatusCreated, item{Name: "test", Value: 42})

		assert.Equal(t, http.StatusCreated, w.Code)
		assert.Equal(t, "application/xml", w.Header().Get("Content-Type"))
		assert.Contains(t, w.Body.String(), "<item>")
		assert.Contains(t, w.Body.String(), "<name>test</name>")
		assert.Contains(t, w.Body.String(), "<value>42</value>")
	})

	t.Run("writes XML array", func(t *testing.T) {
		w := httptest.NewRecorder()
		type items struct {
			XMLName xml.Name `xml:"items"`
			Items   []item   `xml:"item"`
		}
		data := items{Items: []item{{Name: "a", Value: 1}, {Name: "b", Value: 2}}}
		ResponseXML(w, http.StatusOK, data)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/xml", w.Header().Get("Content-Type"))
		assert.Contains(t, w.Body.String(), "<items>")
		assert.Contains(t, w.Body.String(), "<name>a</name>")
		assert.Contains(t, w.Body.String(), "<name>b</name>")
	})

	t.Run("writes 500 on encode error", func(t *testing.T) {
		w := httptest.NewRecorder()
		ResponseXML(w, http.StatusOK, make(chan int))

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.NotEqual(t, "application/xml", w.Header().Get("Content-Type"))
	})
}
