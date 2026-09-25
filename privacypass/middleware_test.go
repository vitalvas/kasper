package privacypass

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMiddlewareConfigErrors(t *testing.T) {
	t.Run("missing public key", func(t *testing.T) {
		_, err := Middleware(Config{Challenge: testChallenge()})
		require.ErrorIs(t, err, ErrNoPublicKey)
	})
	t.Run("missing challenge", func(t *testing.T) {
		priv := testRSAKey(t)
		_, err := Middleware(Config{PublicKey: &priv.PublicKey})
		require.ErrorIs(t, err, ErrMalformed)
	})
	t.Run("invalid challenge", func(t *testing.T) {
		priv := testRSAKey(t)
		bad := &TokenChallenge{TokenType: TokenType, RedemptionContext: []byte{1, 2, 3}}
		_, err := Middleware(Config{PublicKey: &priv.PublicKey, Challenge: bad})
		require.ErrorIs(t, err, ErrMalformed)
	})
}

func TestMiddlewareChallengesWithoutToken(t *testing.T) {
	priv := testRSAKey(t)
	mw, err := Middleware(Config{PublicKey: &priv.PublicKey, Challenge: testChallenge()})
	require.NoError(t, err)

	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.True(t, len(rec.Header().Get("WWW-Authenticate")) > 0)
	assert.Contains(t, rec.Header().Get("WWW-Authenticate"), "PrivateToken")
}

func TestMiddlewareAdmitsValidToken(t *testing.T) {
	priv := testRSAKey(t)
	challenge := testChallenge()
	mw, err := Middleware(Config{PublicKey: &priv.PublicKey, Challenge: challenge})
	require.NoError(t, err)

	tok := issueToken(t, priv, challenge)
	authHeader, err := BuildAuthorizationHeader(tok)
	require.NoError(t, err)

	admitted := false
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		admitted = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.True(t, admitted)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestMiddlewareRejectsReplay(t *testing.T) {
	priv := testRSAKey(t)
	challenge := testChallenge()
	mw, err := Middleware(Config{PublicKey: &priv.PublicKey, Challenge: challenge})
	require.NoError(t, err)

	tok := issueToken(t, priv, challenge)
	authHeader, err := BuildAuthorizationHeader(tok)
	require.NoError(t, err)

	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	do := func() int {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", authHeader)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	assert.Equal(t, http.StatusOK, do())
	// Same token replayed is rejected.
	assert.Equal(t, http.StatusUnauthorized, do())
}

func TestMiddlewareCustomOnError(t *testing.T) {
	priv := testRSAKey(t)
	called := false
	mw, err := Middleware(Config{
		PublicKey: &priv.PublicKey,
		Challenge: testChallenge(),
		OnError: func(w http.ResponseWriter, _ *http.Request, _ error) {
			called = true
			w.WriteHeader(http.StatusForbidden)
		},
	})
	require.NoError(t, err)

	h := mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.True(t, called)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}
