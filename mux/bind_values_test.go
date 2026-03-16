package mux

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBindQuery(t *testing.T) {
	t.Run("decodes basic types", func(t *testing.T) {
		type params struct {
			Name   string  `query:"name"`
			Page   int     `query:"page"`
			Limit  uint    `query:"limit"`
			Score  float64 `query:"score"`
			Active bool    `query:"active"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?name=test&page=2&limit=50&score=9.5&active=true", nil)
		var got params
		require.NoError(t, BindQuery(req, &got))

		assert.Equal(t, "test", got.Name)
		assert.Equal(t, 2, got.Page)
		assert.Equal(t, uint(50), got.Limit)
		assert.Equal(t, 9.5, got.Score)
		assert.True(t, got.Active)
	})

	t.Run("uses field name when no tag", func(t *testing.T) {
		type params struct {
			Name string
		}

		req := httptest.NewRequest(http.MethodGet, "/?Name=hello", nil)
		var got params
		require.NoError(t, BindQuery(req, &got))
		assert.Equal(t, "hello", got.Name)
	})

	t.Run("ignores field with dash tag", func(t *testing.T) {
		type params struct {
			Name   string `query:"name"`
			Secret string `query:"-"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?name=test&Secret=hidden", nil)
		var got params
		require.NoError(t, BindQuery(req, &got))
		assert.Equal(t, "test", got.Name)
		assert.Empty(t, got.Secret)
	})

	t.Run("required field missing returns error", func(t *testing.T) {
		type params struct {
			Name string `query:"name,required"`
		}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		var got params
		err := BindQuery(req, &got)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "required")
	})

	t.Run("required field present succeeds", func(t *testing.T) {
		type params struct {
			Name string `query:"name,required"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?name=ok", nil)
		var got params
		require.NoError(t, BindQuery(req, &got))
		assert.Equal(t, "ok", got.Name)
	})

	t.Run("default value applied when missing", func(t *testing.T) {
		type params struct {
			Page  int    `query:"page,default:1"`
			Limit int    `query:"limit,default:20"`
			Sort  string `query:"sort,default:created_at"`
		}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		var got params
		require.NoError(t, BindQuery(req, &got))

		assert.Equal(t, 1, got.Page)
		assert.Equal(t, 20, got.Limit)
		assert.Equal(t, "created_at", got.Sort)
	})

	t.Run("explicit value overrides default", func(t *testing.T) {
		type params struct {
			Page int `query:"page,default:1"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?page=5", nil)
		var got params
		require.NoError(t, BindQuery(req, &got))
		assert.Equal(t, 5, got.Page)
	})

	t.Run("slice field from multiple values", func(t *testing.T) {
		type params struct {
			IDs []int `query:"id"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?id=1&id=2&id=3", nil)
		var got params
		require.NoError(t, BindQuery(req, &got))
		assert.Equal(t, []int{1, 2, 3}, got.IDs)
	})

	t.Run("string slice", func(t *testing.T) {
		type params struct {
			Tags []string `query:"tag"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?tag=go&tag=http", nil)
		var got params
		require.NoError(t, BindQuery(req, &got))
		assert.Equal(t, []string{"go", "http"}, got.Tags)
	})

	t.Run("pointer field", func(t *testing.T) {
		type params struct {
			Limit *int `query:"limit"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?limit=10", nil)
		var got params
		require.NoError(t, BindQuery(req, &got))
		require.NotNil(t, got.Limit)
		assert.Equal(t, 10, *got.Limit)
	})

	t.Run("pointer field missing stays nil", func(t *testing.T) {
		type params struct {
			Limit *int `query:"limit"`
		}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		var got params
		require.NoError(t, BindQuery(req, &got))
		assert.Nil(t, got.Limit)
	})

	t.Run("invalid int returns error", func(t *testing.T) {
		type params struct {
			Page int `query:"page"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?page=abc", nil)
		var got params
		err := BindQuery(req, &got)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "page")
	})

	t.Run("invalid bool returns error", func(t *testing.T) {
		type params struct {
			Active bool `query:"active"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?active=maybe", nil)
		var got params
		assert.Error(t, BindQuery(req, &got))
	})

	t.Run("non-pointer destination returns error", func(t *testing.T) {
		type params struct {
			Name string `query:"name"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?name=test", nil)
		var got params
		err := BindQuery(req, got)
		assert.ErrorIs(t, err, ErrBindNotPointerToStruct)
	})

	t.Run("nil destination returns error", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		err := BindQuery(req, (*struct{})(nil))
		assert.ErrorIs(t, err, ErrBindNotPointerToStruct)
	})

	t.Run("text unmarshaler", func(t *testing.T) {
		type params struct {
			IP testIP `query:"ip"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?ip=192.168.1.1", nil)
		var got params
		require.NoError(t, BindQuery(req, &got))
		assert.Equal(t, "192.168.1.1", got.IP.val)
	})

	t.Run("skips unexported fields", func(t *testing.T) {
		type params struct {
			Name   string `query:"name"`
			hidden string `query:"hidden"` //nolint:unused
		}

		req := httptest.NewRequest(http.MethodGet, "/?name=ok&hidden=secret", nil)
		var got params
		require.NoError(t, BindQuery(req, &got))
		assert.Equal(t, "ok", got.Name)
	})

	t.Run("int8 int16 int32 int64 types", func(t *testing.T) {
		type params struct {
			A int8  `query:"a"`
			B int16 `query:"b"`
			C int32 `query:"c"`
			D int64 `query:"d"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?a=1&b=2&c=3&d=4", nil)
		var got params
		require.NoError(t, BindQuery(req, &got))
		assert.Equal(t, int8(1), got.A)
		assert.Equal(t, int16(2), got.B)
		assert.Equal(t, int32(3), got.C)
		assert.Equal(t, int64(4), got.D)
	})

	t.Run("float32 type", func(t *testing.T) {
		type params struct {
			Val float32 `query:"val"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?val=3.14", nil)
		var got params
		require.NoError(t, BindQuery(req, &got))
		assert.InDelta(t, float32(3.14), got.Val, 0.001)
	})

	t.Run("nested struct with dot notation", func(t *testing.T) {
		type address struct {
			Street string `query:"street"`
			City   string `query:"city"`
		}
		type params struct {
			Name    string  `query:"name"`
			Address address `query:"address"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?name=alice&address.street=Main+St&address.city=NYC", nil)
		var got params
		require.NoError(t, BindQuery(req, &got))

		assert.Equal(t, "alice", got.Name)
		assert.Equal(t, "Main St", got.Address.Street)
		assert.Equal(t, "NYC", got.Address.City)
	})

	t.Run("deeply nested struct", func(t *testing.T) {
		type geo struct {
			Lat float64 `query:"lat"`
			Lon float64 `query:"lon"`
		}
		type address struct {
			City string `query:"city"`
			Geo  geo    `query:"geo"`
		}
		type params struct {
			Address address `query:"addr"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?addr.city=NYC&addr.geo.lat=40.7&addr.geo.lon=-74.0", nil)
		var got params
		require.NoError(t, BindQuery(req, &got))

		assert.Equal(t, "NYC", got.Address.City)
		assert.InDelta(t, 40.7, got.Address.Geo.Lat, 0.01)
		assert.InDelta(t, -74.0, got.Address.Geo.Lon, 0.01)
	})

	t.Run("embedded struct flattens fields", func(t *testing.T) {
		type Pagination struct {
			Page  int `query:"page"`
			Limit int `query:"limit"`
		}
		type params struct {
			Pagination
			Status string `query:"status"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?page=3&limit=25&status=active", nil)
		var got params
		require.NoError(t, BindQuery(req, &got))

		assert.Equal(t, 3, got.Page)
		assert.Equal(t, 25, got.Limit)
		assert.Equal(t, "active", got.Status)
	})

	t.Run("embedded struct with explicit tag uses dot notation", func(t *testing.T) {
		type Pagination struct {
			Page  int `query:"page"`
			Limit int `query:"limit"`
		}
		type params struct {
			Pagination `query:"paging"`
			Status     string `query:"status"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?paging.page=2&paging.limit=10&status=active", nil)
		var got params
		require.NoError(t, BindQuery(req, &got))

		assert.Equal(t, 2, got.Page)
		assert.Equal(t, 10, got.Limit)
		assert.Equal(t, "active", got.Status)
	})

	t.Run("embedded pointer struct flattens fields", func(t *testing.T) {
		type Pagination struct {
			Page int `query:"page"`
		}
		type params struct {
			*Pagination
			Name string `query:"name"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?page=5&name=test", nil)
		var got params
		require.NoError(t, BindQuery(req, &got))

		require.NotNil(t, got.Pagination)
		assert.Equal(t, 5, got.Page)
		assert.Equal(t, "test", got.Name)
	})

	t.Run("max slice index exceeded returns error", func(t *testing.T) {
		type item struct {
			Name string `query:"name"`
		}
		type params struct {
			Items []item `query:"items"`
		}

		q := fmt.Sprintf("/?items.%d.name=bad", DefaultMaxSliceIndex)
		req := httptest.NewRequest(http.MethodGet, q, nil)
		var got params
		err := BindQuery(req, &got)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds maximum")
	})

	t.Run("slice of structs with indexed dot notation", func(t *testing.T) {
		type item struct {
			Name  string `query:"name"`
			Price int    `query:"price"`
		}
		type params struct {
			Items []item `query:"items"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?items.0.name=A&items.0.price=10&items.1.name=B&items.1.price=20", nil)
		var got params
		require.NoError(t, BindQuery(req, &got))

		require.Len(t, got.Items, 2)
		assert.Equal(t, "A", got.Items[0].Name)
		assert.Equal(t, 10, got.Items[0].Price)
		assert.Equal(t, "B", got.Items[1].Name)
		assert.Equal(t, 20, got.Items[1].Price)
	})

	t.Run("map field with dot notation", func(t *testing.T) {
		type params struct {
			Meta map[string]string `query:"meta"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?meta.env=prod&meta.region=us-east", nil)
		var got params
		require.NoError(t, BindQuery(req, &got))

		assert.Equal(t, "prod", got.Meta["env"])
		assert.Equal(t, "us-east", got.Meta["region"])
	})

	t.Run("map field with int values", func(t *testing.T) {
		type params struct {
			Scores map[string]int `query:"scores"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?scores.math=95&scores.english=88", nil)
		var got params
		require.NoError(t, BindQuery(req, &got))

		assert.Equal(t, 95, got.Scores["math"])
		assert.Equal(t, 88, got.Scores["english"])
	})

	t.Run("map field with multiple values per key", func(t *testing.T) {
		type params struct {
			Tags map[string][]string `query:"tags"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?tags.color=red&tags.color=blue&tags.size=large", nil)
		var got params
		require.NoError(t, BindQuery(req, &got))

		assert.Equal(t, []string{"red", "blue"}, got.Tags["color"])
		assert.Equal(t, []string{"large"}, got.Tags["size"])
	})

	t.Run("empty map stays nil", func(t *testing.T) {
		type params struct {
			Meta map[string]string `query:"meta"`
		}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		var got params
		require.NoError(t, BindQuery(req, &got))
		assert.Nil(t, got.Meta)
	})

	t.Run("pointer to nested struct allocated on demand", func(t *testing.T) {
		type address struct {
			City string `query:"city"`
		}
		type params struct {
			Address *address `query:"address"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?address.city=NYC", nil)
		var got params
		require.NoError(t, BindQuery(req, &got))
		require.NotNil(t, got.Address)
		assert.Equal(t, "NYC", got.Address.City)
	})

	t.Run("pointer to nested struct stays nil when no keys", func(t *testing.T) {
		type address struct {
			City string `query:"city"`
		}
		type params struct {
			Address *address `query:"address"`
		}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		var got params
		require.NoError(t, BindQuery(req, &got))
		assert.Nil(t, got.Address)
	})
}

func TestBindForm(t *testing.T) {
	t.Run("decodes form body", func(t *testing.T) {
		type login struct {
			Username string `form:"username"`
			Password string `form:"password"`
		}

		body := "username=admin&password=secret"
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		var got login
		require.NoError(t, BindForm(req, &got))
		assert.Equal(t, "admin", got.Username)
		assert.Equal(t, "secret", got.Password)
	})

	t.Run("required field missing returns error", func(t *testing.T) {
		type login struct {
			Username string `form:"username,required"`
			Password string `form:"password,required"`
		}

		body := "username=admin"
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		var got login
		err := BindForm(req, &got)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "password")
	})

	t.Run("default value applied", func(t *testing.T) {
		type form struct {
			Role string `form:"role,default:user"`
		}

		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		var got form
		require.NoError(t, BindForm(req, &got))
		assert.Equal(t, "user", got.Role)
	})

	t.Run("multiple values as slice", func(t *testing.T) {
		type form struct {
			Colors []string `form:"color"`
		}

		body := "color=red&color=blue&color=green"
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		var got form
		require.NoError(t, BindForm(req, &got))
		assert.Equal(t, []string{"red", "blue", "green"}, got.Colors)
	})

	t.Run("nested struct", func(t *testing.T) {
		type address struct {
			City string `form:"city"`
		}
		type form struct {
			Address address `form:"address"`
		}

		body := "address.city=NYC"
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		var got form
		require.NoError(t, BindForm(req, &got))
		assert.Equal(t, "NYC", got.Address.City)
	})
}

func TestEncodeQuery(t *testing.T) {
	t.Run("encodes basic types", func(t *testing.T) {
		type params struct {
			Name   string  `query:"name"`
			Page   int     `query:"page"`
			Score  float64 `query:"score"`
			Active bool    `query:"active"`
		}

		vals, err := EncodeQuery(params{
			Name:   "test",
			Page:   2,
			Score:  9.5,
			Active: true,
		})
		require.NoError(t, err)

		assert.Equal(t, "test", vals.Get("name"))
		assert.Equal(t, "2", vals.Get("page"))
		assert.Equal(t, "9.5", vals.Get("score"))
		assert.Equal(t, "true", vals.Get("active"))
	})

	t.Run("skips dash tagged fields", func(t *testing.T) {
		type params struct {
			Name   string `query:"name"`
			Secret string `query:"-"`
		}

		vals, err := EncodeQuery(params{Name: "ok", Secret: "hidden"})
		require.NoError(t, err)
		assert.Equal(t, "ok", vals.Get("name"))
		assert.Empty(t, vals.Get("Secret"))
	})

	t.Run("omitempty skips zero values", func(t *testing.T) {
		type params struct {
			Name string `query:"name"`
			Page int    `query:"page,omitempty"`
		}

		vals, err := EncodeQuery(params{Name: "ok"})
		require.NoError(t, err)
		assert.Equal(t, "ok", vals.Get("name"))
		assert.Empty(t, vals.Get("page"))
	})

	t.Run("encodes slice as multiple values", func(t *testing.T) {
		type params struct {
			IDs []int `query:"id"`
		}

		vals, err := EncodeQuery(params{IDs: []int{1, 2, 3}})
		require.NoError(t, err)
		assert.Equal(t, []string{"1", "2", "3"}, vals["id"])
	})

	t.Run("encodes nested struct with dot notation", func(t *testing.T) {
		type address struct {
			Street string `query:"street"`
			City   string `query:"city"`
		}
		type params struct {
			Name    string  `query:"name"`
			Address address `query:"address"`
		}

		vals, err := EncodeQuery(params{
			Name:    "alice",
			Address: address{Street: "Main St", City: "NYC"},
		})
		require.NoError(t, err)

		assert.Equal(t, "alice", vals.Get("name"))
		assert.Equal(t, "Main St", vals.Get("address.street"))
		assert.Equal(t, "NYC", vals.Get("address.city"))
	})

	t.Run("encodes slice of structs with indexed dot", func(t *testing.T) {
		type item struct {
			Name  string `query:"name"`
			Price int    `query:"price"`
		}
		type params struct {
			Items []item `query:"items"`
		}

		vals, err := EncodeQuery(params{
			Items: []item{
				{Name: "A", Price: 10},
				{Name: "B", Price: 20},
			},
		})
		require.NoError(t, err)

		assert.Equal(t, "A", vals.Get("items.0.name"))
		assert.Equal(t, "10", vals.Get("items.0.price"))
		assert.Equal(t, "B", vals.Get("items.1.name"))
		assert.Equal(t, "20", vals.Get("items.1.price"))
	})

	t.Run("encodes map with dot notation", func(t *testing.T) {
		type params struct {
			Meta map[string]string `query:"meta"`
		}

		vals, err := EncodeQuery(params{
			Meta: map[string]string{"env": "prod", "region": "us-east"},
		})
		require.NoError(t, err)

		assert.Equal(t, "prod", vals.Get("meta.env"))
		assert.Equal(t, "us-east", vals.Get("meta.region"))
	})

	t.Run("encodes map with slice values", func(t *testing.T) {
		type params struct {
			Tags map[string][]string `query:"tags"`
		}

		vals, err := EncodeQuery(params{
			Tags: map[string][]string{
				"color": {"red", "blue"},
				"size":  {"large"},
			},
		})
		require.NoError(t, err)

		assert.Equal(t, []string{"red", "blue"}, vals["tags.color"])
		assert.Equal(t, []string{"large"}, vals["tags.size"])
	})

	t.Run("roundtrip map with multiple values", func(t *testing.T) {
		type params struct {
			Tags map[string][]string `query:"tags"`
		}

		original := params{
			Tags: map[string][]string{
				"a": {"1", "2"},
			},
		}

		vals, err := EncodeQuery(original)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/?"+vals.Encode(), nil)
		var decoded params
		require.NoError(t, BindQuery(req, &decoded))

		assert.Equal(t, original.Tags["a"], decoded.Tags["a"])
	})

	t.Run("encodes embedded struct flattened", func(t *testing.T) {
		type Pagination struct {
			Page  int `query:"page"`
			Limit int `query:"limit"`
		}
		type params struct {
			Pagination
			Status string `query:"status"`
		}

		vals, err := EncodeQuery(params{
			Pagination: Pagination{Page: 2, Limit: 10},
			Status:     "active",
		})
		require.NoError(t, err)

		assert.Equal(t, "2", vals.Get("page"))
		assert.Equal(t, "10", vals.Get("limit"))
		assert.Equal(t, "active", vals.Get("status"))
	})

	t.Run("roundtrip with embedded struct", func(t *testing.T) {
		type Pagination struct {
			Page  int `query:"page"`
			Limit int `query:"limit"`
		}
		type params struct {
			Pagination
			Status string `query:"status"`
		}

		original := params{
			Pagination: Pagination{Page: 3, Limit: 25},
			Status:     "active",
		}

		vals, err := EncodeQuery(original)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/?"+vals.Encode(), nil)
		var decoded params
		require.NoError(t, BindQuery(req, &decoded))

		assert.Equal(t, original, decoded)
	})

	t.Run("skips nil pointer fields", func(t *testing.T) {
		type params struct {
			Name  string `query:"name"`
			Limit *int   `query:"limit"`
		}

		vals, err := EncodeQuery(params{Name: "ok"})
		require.NoError(t, err)
		assert.Equal(t, "ok", vals.Get("name"))
		assert.Empty(t, vals.Get("limit"))
	})

	t.Run("encodes pointer to struct", func(t *testing.T) {
		type params struct {
			Name string `query:"name"`
		}

		vals, err := EncodeQuery(&params{Name: "ok"})
		require.NoError(t, err)
		assert.Equal(t, "ok", vals.Get("name"))
	})

	t.Run("non-struct returns error", func(t *testing.T) {
		_, err := EncodeQuery("not a struct")
		assert.ErrorIs(t, err, ErrEncodeNotStruct)
	})

	t.Run("nil pointer returns error", func(t *testing.T) {
		_, err := EncodeQuery((*struct{})(nil))
		assert.ErrorIs(t, err, ErrEncodeNotStruct)
	})

	t.Run("roundtrip encode then decode", func(t *testing.T) {
		type address struct {
			City string `query:"city"`
			Zip  string `query:"zip"`
		}
		type params struct {
			Name    string  `query:"name"`
			Page    int     `query:"page"`
			Address address `query:"address"`
		}

		original := params{
			Name: "alice",
			Page: 3,
			Address: address{
				City: "NYC",
				Zip:  "10001",
			},
		}

		vals, err := EncodeQuery(original)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/?"+vals.Encode(), nil)
		var decoded params
		require.NoError(t, BindQuery(req, &decoded))

		assert.Equal(t, original, decoded)
	})
}

func TestEncodeForm(t *testing.T) {
	t.Run("encodes struct with form tag", func(t *testing.T) {
		type login struct {
			Username string `form:"username"`
			Password string `form:"password"`
		}

		vals, err := EncodeForm(login{Username: "admin", Password: "secret"})
		require.NoError(t, err)
		assert.Equal(t, "admin", vals.Get("username"))
		assert.Equal(t, "secret", vals.Get("password"))
	})
}

func BenchmarkBindQuery(b *testing.B) {
	type params struct {
		Name   string  `query:"name"`
		Page   int     `query:"page"`
		Limit  uint    `query:"limit"`
		Score  float64 `query:"score"`
		Active bool    `query:"active"`
	}

	req := httptest.NewRequest(http.MethodGet, "/?name=test&page=2&limit=50&score=9.5&active=true", nil)

	b.ResetTimer()
	for b.Loop() {
		var got params
		_ = BindQuery(req, &got)
	}
}

func BenchmarkEncodeQuery(b *testing.B) {
	type params struct {
		Name   string  `query:"name"`
		Page   int     `query:"page"`
		Limit  uint    `query:"limit"`
		Score  float64 `query:"score"`
		Active bool    `query:"active"`
	}

	v := params{Name: "test", Page: 2, Limit: 50, Score: 9.5, Active: true}

	b.ResetTimer()
	for b.Loop() {
		_, _ = EncodeQuery(v)
	}
}

// testIP implements encoding.TextUnmarshaler for testing.
type testIP struct {
	val string
}

func (ip *testIP) UnmarshalText(text []byte) error {
	ip.val = string(text)
	return nil
}
