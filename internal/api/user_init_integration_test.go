package api

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jpcorrect-backend/internal/domain"
	"jpcorrect-backend/internal/testutil"
)

// initResult is one concurrent POST /v1/users/init response.
type initResult struct {
	status int
	userID string
	body   string
}

// callUserInitWithToken runs UserInitHandler with the claims AuthMiddleware
// would set from a signed token: subject, email and email_verified.
func callUserInitWithToken(t *testing.T, api *API, subject, email string, emailVerified bool) initResult {
	t.Helper()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/users/init", nil)
	c.Set("supabaseUserID", subject)
	c.Set("supabaseEmail", email)
	c.Set("supabaseEmailVerified", emailVerified)

	api.UserInitHandler(c)

	result := initResult{status: w.Code, body: w.Body.String()}
	if w.Code == http.StatusOK {
		var user domain.User
		if err := json.Unmarshal(w.Body.Bytes(), &user); err != nil {
			// assert, not require: this helper runs in goroutines.
			t.Errorf("decode init response: %v", err)
			return result
		}
		result.userID = user.ID.String()
	}
	return result
}

func TestUserInitConcurrentSameSubjectCreatesOneRow(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := testutil.NewPostgresDB(t)
	api := NewAPI("", "", &http.Transport{}, db, "", nil)

	const callers = 20
	email := "concurrent-" + uuid.NewString() + "@example.com"
	subject := "supabase-" + uuid.NewString()

	results := make([]initResult, callers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range callers {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			results[index] = callUserInitWithToken(t, api, subject, email, true)
		}(i)
	}
	close(start)
	wg.Wait()

	var created []domain.User
	require.NoError(t, db.Where("email = ?", email).Find(&created).Error)
	require.Len(t, created, 1, "concurrent init calls must create exactly one row")

	for _, result := range results {
		assert.Equal(t, http.StatusOK, result.status, "every caller with the same subject must succeed: %s", result.body)
		assert.Equal(t, created[0].ID.String(), result.userID, "every caller must receive the same row")
	}
}

func TestUserInitConcurrentDistinctSubjectsOneEmailCreatesOneRow(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := testutil.NewPostgresDB(t)
	api := NewAPI("", "", &http.Transport{}, db, "", nil)

	const callers = 10
	email := "race-" + uuid.NewString() + "@example.com"

	results := make([]initResult, callers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range callers {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			results[index] = callUserInitWithToken(t, api, "supabase-"+uuid.NewString(), email, true)
		}(i)
	}
	close(start)
	wg.Wait()

	var created []domain.User
	require.NoError(t, db.Where("email = ?", email).Find(&created).Error)
	require.Len(t, created, 1, "only one subject may own an email")

	succeeded := 0
	for _, result := range results {
		switch result.status {
		case http.StatusOK:
			succeeded++
			assert.Equal(t, created[0].ID.String(), result.userID)
		case http.StatusConflict:
			// The loser never adopts the row.
		default:
			t.Errorf("unexpected status %d: %s", result.status, result.body)
		}
	}
	assert.Equal(t, 1, succeeded, "exactly one subject may provision the email")
}

func TestUserInitSignedUnverifiedEmailCannotClaimLegacyRow(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := testutil.NewPostgresDB(t)
	api := NewAPI("", "", &http.Transport{}, db, "", nil)

	email := "legacy-" + uuid.NewString() + "@example.com"
	legacy := &domain.User{Email: email, Name: "legacy owner"}
	require.NoError(t, api.userRepo.Create(t.Context(), legacy))

	// Several signed tokens carry the legacy address, verified or not. None of
	// them may claim the row: linking stays an explicit maintainer action.
	const callers = 10
	results := make([]initResult, callers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range callers {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			results[index] = callUserInitWithToken(t, api, "supabase-"+uuid.NewString(), email, index%2 == 0)
		}(i)
	}
	close(start)
	wg.Wait()

	for _, result := range results {
		assert.Equal(t, http.StatusConflict, result.status)
		assert.Empty(t, result.userID, "a conflict must not return a row")
	}

	stored, err := api.userRepo.GetByID(t.Context(), legacy.ID)
	require.NoError(t, err)
	assert.Empty(t, stored.SupabaseUserID, "the legacy row must stay unlinked")
	assert.Equal(t, "legacy owner", stored.Name, "the legacy row must not be modified")

	var rows []domain.User
	require.NoError(t, db.Where("email = ?", email).Find(&rows).Error)
	assert.Len(t, rows, 1, "no second row may be created for a claimed email")
}

// newJWKSServer serves a JWKS document for pub and returns its URL.
func newJWKSServer(t *testing.T, pub *rsa.PublicKey, kid string) string {
	t.Helper()

	doc := map[string]any{
		"keys": []map[string]string{{
			"kty": "RSA",
			"kid": kid,
			"use": "sig",
			"alg": "RS256",
			"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
		}},
	}
	body, err := json.Marshal(doc)
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	return server.URL
}

// signSupabaseToken signs an RS256 token that carries the Supabase claims the
// middleware reads, so the test exercises the real claim path.
func signSupabaseToken(t *testing.T, key *rsa.PrivateKey, kid, subject, email string, emailVerified bool) string {
	t.Helper()

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub":            subject,
		"email":          email,
		"email_verified": emailVerified,
		"iat":            time.Now().Add(-time.Minute).Unix(),
		"exp":            time.Now().Add(time.Hour).Unix(),
	})
	token.Header["kid"] = kid

	signed, err := token.SignedString(key)
	require.NoError(t, err)
	return signed
}

// TestUserInitRejectsSignedTokenForLegacyRow drives the real AuthMiddleware with
// signed tokens. Neither an unverified nor a verified email claim may claim a
// legacy row, because the policy does not trust the email claim at all.
func TestUserInitRejectsSignedTokenForLegacyRow(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := testutil.NewPostgresDB(t)

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	const kid = "test-key"
	jwksURL := newJWKSServer(t, &key.PublicKey, kid)

	api := NewAPI("", "", &http.Transport{}, db, jwksURL, nil)
	require.NoError(t, api.InitializeJWKS(t.Context()))
	t.Cleanup(api.ShutdownJWKS)

	router := gin.New()
	router.POST("/v1/users/init", api.AuthMiddleware(), api.UserInitHandler)

	email := "signed-legacy-" + uuid.NewString() + "@example.com"
	legacy := &domain.User{Email: email, Name: "signed legacy owner"}
	require.NoError(t, api.userRepo.Create(t.Context(), legacy))

	for _, verified := range []bool{false, true} {
		token := signSupabaseToken(t, key, kid, "supabase-"+uuid.NewString(), email, verified)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/users/init", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusConflict, w.Code, "email_verified=%v: %s", verified, w.Body.String())
	}

	stored, err := api.userRepo.GetByID(t.Context(), legacy.ID)
	require.NoError(t, err)
	assert.Empty(t, stored.SupabaseUserID, "a signed token must not link the legacy row")
	assert.Equal(t, "signed legacy owner", stored.Name)

	var rows []domain.User
	require.NoError(t, db.Where("email = ?", email).Find(&rows).Error)
	assert.Len(t, rows, 1, "no second row may be created for a claimed email")
}
