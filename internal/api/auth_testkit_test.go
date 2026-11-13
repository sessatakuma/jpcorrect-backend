package api

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// testSigner mints RS256 JWTs for the auth tests and publishes the matching
// JWKS document, so the middleware exercises the same path as in a deployment:
// it fetches a JWKS over HTTP and verifies the token signature against it.
type testSigner struct {
	kid string
	key *rsa.PrivateKey
}

// newTestSigner generates a fresh RSA key pair. 2048 bits is what every
// provider in scope publishes, and generating it is still fast enough for
// tests.
func newTestSigner(t *testing.T) *testSigner {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err, "generate RSA key")

	return &testSigner{kid: "test-key", key: key}
}

// jwksDocument renders the public half of the key as a JWKS body.
func (s *testSigner) jwksDocument(t *testing.T) []byte {
	t.Helper()

	document := map[string]any{
		"keys": []any{map[string]any{
			"kty": "RSA",
			"kid": s.kid,
			"use": "sig",
			"alg": "RS256",
			"n":   base64.RawURLEncoding.EncodeToString(s.key.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(s.key.E)).Bytes()),
		}},
	}

	raw, err := json.Marshal(document)
	require.NoError(t, err)
	return raw
}

// token mints a token carrying claims. A zero ExpiresAt defaults to one hour
// from now; set it explicitly to test expiry.
func (s *testSigner) token(t *testing.T, claims supabaseClaims) string {
	t.Helper()

	if claims.ExpiresAt == nil {
		claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(time.Hour))
	}
	claims.IssuedAt = jwt.NewNumericDate(time.Now())

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = s.kid

	signed, err := token.SignedString(s.key)
	require.NoError(t, err, "sign token")
	return signed
}

// rawToken mints a token from arbitrary claims, e.g. to prove that a role claim
// a provider might emit does not influence authorization.
func (s *testSigner) rawToken(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()

	if _, exists := claims["iat"]; !exists {
		claims["iat"] = time.Now().Unix()
	}
	if _, exists := claims["exp"]; !exists {
		claims["exp"] = time.Now().Add(time.Hour).Unix()
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = s.kid

	signed, err := token.SignedString(s.key)
	require.NoError(t, err, "sign token")
	return signed
}

// userToken mints the token a logged-in Supabase user would present. The
// subject goes on the wire as the UUID string Supabase issues; uuid.Nil leaves
// it empty so a caller can test the missing-subject rejection.
func (s *testSigner) userToken(t *testing.T, sub uuid.UUID, email string) string {
	t.Helper()

	subject := ""
	if sub != uuid.Nil {
		subject = sub.String()
	}

	return s.token(t, supabaseClaims{
		Email:            email,
		RegisteredClaims: jwt.RegisteredClaims{Subject: subject},
	})
}

// attachTo points api at a JWKS server for this signer and initializes the
// keyfunc, the way cmd initializes it at startup.
func (s *testSigner) attachTo(t *testing.T, api *API) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(s.jwksDocument(t))
	}))
	t.Cleanup(server.Close)

	api.jwksURL = server.URL

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	require.NoError(t, api.InitializeJWKS(ctx), "initialize JWKS from the test server")
	t.Cleanup(api.ShutdownJWKS)
}
