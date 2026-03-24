package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/vitalvas/kasper/blindrsa"
	"github.com/vitalvas/kasper/mux"
)

func main() {
	priv, pub := loadOrGenerateKey()

	issueHandler, err := blindrsa.IssueHandler(blindrsa.IssuerConfig{
		Key:     priv,
		Variant: blindrsa.VariantSHA384PSSDeterministic,
	})
	if err != nil {
		log.Fatal(err)
	}

	mw, err := blindrsa.VerifyMiddleware(blindrsa.MiddlewareConfig{
		Key:     pub,
		Variant: blindrsa.VariantSHA384PSSDeterministic,
		MessageFunc: func(r *http.Request) ([]byte, error) {
			encoded := r.Header.Get("X-Token-Message")
			if encoded == "" {
				return nil, fmt.Errorf("missing X-Token-Message header")
			}

			return base64.StdEncoding.DecodeString(encoded)
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	// Track redeemed tokens to prevent double-spending.
	redeemed := &tokenStore{}

	router := mux.NewRouter()

	// Public: issuer's public key for clients.
	router.HandleFunc("/api/v1/pubkey", func(w http.ResponseWriter, _ *http.Request) {
		pubDER, err := x509.MarshalPKIXPublicKey(pub)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/x-pem-file")
		pem.Encode(w, &pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	}).Methods(http.MethodGet)

	// Authenticated: client obtains a blind token.
	// In production this would require authentication (e.g., API key, JWT).
	router.Handle("/api/v1/issue", issueHandler).Methods(http.MethodPost)

	// Protected: requires a valid blind signature token.
	// The server can verify the token but cannot link it to who obtained it.
	protected := router.With(mw)

	protected.HandleFunc("/api/v1/download", func(w http.ResponseWriter, r *http.Request) {
		sig := r.Header.Get("Blind-Signature")
		if redeemed.seen(sig) {
			http.Error(w, "token already redeemed", http.StatusForbidden)
			return
		}

		redeemed.add(sig)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "ok",
			"data":   "this is protected content served anonymously",
		})
	}).Methods(http.MethodGet)

	log.Printf("blindrsa server listening on :8081")
	log.Printf("  GET  /api/v1/pubkey   - fetch issuer public key")
	log.Printf("  POST /api/v1/issue    - obtain blind token (authenticated)")
	log.Printf("  GET  /api/v1/download - access resource (anonymous, token required)")

	server := &http.Server{Addr: ":8081", Handler: router}
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

// tokenStore tracks redeemed tokens to prevent double-spending.
type tokenStore struct {
	mu   sync.Mutex
	sigs map[string]struct{}
}

func (s *tokenStore) seen(sig string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, ok := s.sigs[sig]

	return ok
}

func (s *tokenStore) add(sig string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.sigs == nil {
		s.sigs = make(map[string]struct{})
	}

	s.sigs[sig] = struct{}{}
}

func loadOrGenerateKey() (*rsa.PrivateKey, *rsa.PublicKey) {
	const keyFile = "blindrsa_key.pem"

	data, err := os.ReadFile(keyFile)
	if err == nil {
		block, _ := pem.Decode(data)
		if block != nil {
			priv, err := x509.ParsePKCS8PrivateKey(block.Bytes)
			if err == nil {
				rsaPriv := priv.(*rsa.PrivateKey)
				log.Printf("loaded existing key from %s (%d bits)", keyFile, rsaPriv.N.BitLen())

				return rsaPriv, &rsaPriv.PublicKey
			}
		}
	}

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Fatal(err)
	}

	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		log.Fatal(err)
	}

	f, err := os.Create(keyFile)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	pem.Encode(f, &pem.Block{Type: "PRIVATE KEY", Bytes: privDER})
	log.Printf("generated new key and saved to %s (%d bits)", keyFile, priv.N.BitLen())

	return priv, &priv.PublicKey
}
