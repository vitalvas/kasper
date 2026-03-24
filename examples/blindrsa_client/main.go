package main

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/vitalvas/kasper/blindrsa"
)

func main() {
	pub := fetchPublicKey("http://localhost:8081")

	client, err := blindrsa.NewClient(blindrsa.ClientConfig{
		Key:           pub,
		Variant:       blindrsa.VariantSHA384PSSDeterministic,
		TokenEndpoint: "http://localhost:8081/api/v1/issue",
	})
	if err != nil {
		log.Fatal(err)
	}

	// Step 1: Obtain a blind token from the issuer.
	// The issuer signs a blinded message without learning its content.
	log.Println("--- Step 1: Obtaining blind token ---")

	msg := []byte("download-token:file-secret-report.pdf")

	sig, state, err := client.ObtainSignature(context.Background(), msg)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("obtained token (%d bytes)", len(sig))

	// Step 2: Use the token to access a protected resource anonymously.
	// The server verifies the token is valid (signed by the issuer) but
	// cannot link it back to the issuance request.
	log.Println("--- Step 2: Accessing protected resource ---")

	body, status := accessResource(
		"http://localhost:8081/api/v1/download",
		sig,
		state.InputMessage(),
	)
	log.Printf("response [%d]: %s", status, body)

	// Step 3: Try to reuse the same token (double-spend).
	// The server should reject it.
	log.Println("--- Step 3: Attempting token reuse ---")

	body, status = accessResource(
		"http://localhost:8081/api/v1/download",
		sig,
		state.InputMessage(),
	)
	log.Printf("response [%d]: %s", status, body)

	// Step 4: Obtain a second token and use it.
	log.Println("--- Step 4: Obtaining and using a second token ---")

	sig2, state2, err := client.ObtainSignature(context.Background(), []byte("download-token:file-quarterly-data.csv"))
	if err != nil {
		log.Fatal(err)
	}

	body, status = accessResource(
		"http://localhost:8081/api/v1/download",
		sig2,
		state2.InputMessage(),
	)
	log.Printf("response [%d]: %s", status, body)
}

func accessResource(url string, sig, inputMsg []byte) (string, int) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		log.Fatal(err)
	}

	req.Header.Set("Blind-Signature", base64.StdEncoding.EncodeToString(sig))
	req.Header.Set("X-Token-Message", base64.StdEncoding.EncodeToString(inputMsg))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}

	data, err := io.ReadAll(resp.Body)
	resp.Body.Close()

	if err != nil {
		log.Fatal(err)
	}

	return string(data), resp.StatusCode
}

func fetchPublicKey(serverURL string) *rsa.PublicKey {
	resp, err := http.Get(fmt.Sprintf("%s/api/v1/pubkey", serverURL))
	if err != nil {
		log.Fatalf("failed to fetch public key: %v", err)
	}

	data, err := io.ReadAll(resp.Body)
	resp.Body.Close()

	if err != nil {
		log.Fatalf("failed to read public key: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Fatalf("server returned status %d", resp.StatusCode)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		log.Fatal("failed to decode PEM block")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		log.Fatalf("failed to parse public key: %v", err)
	}

	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		log.Fatal("public key is not RSA")
	}

	log.Printf("fetched server public key (%d bits)", rsaPub.N.BitLen())

	return rsaPub
}
