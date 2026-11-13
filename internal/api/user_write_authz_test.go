package api

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jpcorrect-backend/internal/domain"
	"jpcorrect-backend/internal/testutil"
)

// TestRequireSystemAdmin pins the gate from issue #78: raw user management is
// reachable only by the trusted, server-resolved User with Role == admin, and
// the gate answers before the handler runs.
func TestRequireSystemAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	admin := &domain.User{ID: uuid.New(), Email: "admin@example.com", Role: domain.UserRoleAdmin, SupabaseUserID: "supabase-admin"}
	member := &domain.User{ID: uuid.New(), Email: "member@example.com", Role: domain.UserRoleUser, SupabaseUserID: "supabase-member"}
	staff := &domain.User{ID: uuid.New(), Email: "staff@example.com", Role: domain.UserRoleStaff, SupabaseUserID: "supabase-staff"}
	api := &API{userRepo: newMockUserRepo(admin, member, staff)}

	// call mounts the gate exactly as the router does, optionally with a caller
	// ID (what RequireLocalUser sets) or a resolved User in the context.
	call := func(callerID *uuid.UUID, resolved *domain.User) (status int, reached bool) {
		r := gin.New()
		r.PUT("/v1/users/:id", func(c *gin.Context) {
			if callerID != nil {
				c.Set("userID", *callerID)
			}
			if resolved != nil {
				c.Set("user", resolved)
			}
		}, api.RequireSystemAdmin(), func(c *gin.Context) {
			reached = true
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/v1/users/"+uuid.NewString(), nil))
		return w.Code, reached
	}

	t.Run("allows an admin", func(t *testing.T) {
		status, reached := call(&admin.ID, nil)
		assert.Equal(t, http.StatusOK, status)
		assert.True(t, reached)
	})

	t.Run("rejects a plain user before the handler", func(t *testing.T) {
		status, reached := call(&member.ID, nil)
		assert.Equal(t, http.StatusForbidden, status)
		assert.False(t, reached)
	})

	t.Run("rejects staff", func(t *testing.T) {
		status, reached := call(&staff.ID, nil)
		assert.Equal(t, http.StatusForbidden, status)
		assert.False(t, reached)
	})

	t.Run("uses the user resolved by RequireLocalUser", func(t *testing.T) {
		status, reached := call(nil, member)
		assert.Equal(t, http.StatusForbidden, status)
		assert.False(t, reached)
	})

	t.Run("rejects an unknown caller", func(t *testing.T) {
		missing := uuid.New()
		status, reached := call(&missing, nil)
		assert.Equal(t, http.StatusUnauthorized, status)
		assert.False(t, reached)
	})

	t.Run("rejects a missing caller", func(t *testing.T) {
		status, reached := call(nil, nil)
		assert.Equal(t, http.StatusUnauthorized, status)
		assert.False(t, reached)
	})
}

// TestUserRawWritesRejectCallerControlledIdentity is the release-mode
// regression for the account-takeover chain:
//
//  1. free the attacker's own supabase_user_id with a raw user write,
//  2. write the attacker's verified subject onto the victim's row,
//  3. let RequireLocalUser resolve the attacker's token to the victim.
//
// It drives the real router in GIN_MODE=release with signed JWTs, so
// AuthMiddleware, RequireLocalUser, the #78 gate and the handlers all run. The
// same test also covers the two flows that must keep working: the owning
// user's allowlisted self-service update and a system admin's raw management.
func TestUserRawWritesRejectCallerControlledIdentity(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	t.Cleanup(func() { gin.SetMode(gin.TestMode) })

	db := testutil.NewPostgresDB(t)
	ctx := t.Context()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	const kid = "authz-key"
	jwksURL := newJWKSServer(t, &key.PublicKey, kid)

	api := NewAPI("", "", &http.Transport{}, db, jwksURL, nil)
	require.NoError(t, api.InitializeJWKS(ctx))
	t.Cleanup(api.ShutdownJWKS)

	router := gin.New()
	Register(router, api)

	attacker := &domain.User{
		Email:          "attacker-" + uuid.NewString() + "@example.com",
		Name:           "attacker",
		SupabaseUserID: "supabase-" + uuid.NewString(),
	}
	victim := &domain.User{
		Email:          "victim-" + uuid.NewString() + "@example.com",
		Name:           "victim",
		SupabaseUserID: "supabase-" + uuid.NewString(),
	}
	admin := &domain.User{
		Email:          "admin-" + uuid.NewString() + "@example.com",
		Name:           "sysadmin",
		SupabaseUserID: "supabase-" + uuid.NewString(),
		Role:           domain.UserRoleAdmin,
	}
	for _, user := range []*domain.User{attacker, victim, admin} {
		require.NoError(t, api.userRepo.Create(ctx, user))
	}

	t.Cleanup(func() {
		api.db.WithContext(ctx).Unscoped().
			Delete(&domain.User{}, "id IN ?", []uuid.UUID{attacker.ID, victim.ID, admin.ID})
	})

	attackerToken := signSupabaseToken(t, key, kid, attacker.SupabaseUserID, attacker.Email, true)
	adminToken := signSupabaseToken(t, key, kid, admin.SupabaseUserID, admin.Email, true)

	do := func(method, path, token string, body any) *httptest.ResponseRecorder {
		var reader io.Reader
		if body != nil {
			payload, err := json.Marshal(body)
			require.NoError(t, err)
			reader = bytes.NewReader(payload)
		}
		req := httptest.NewRequest(method, path, reader)
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	reload := func(id uuid.UUID) *domain.User {
		user, err := api.userRepo.GetByID(ctx, id)
		require.NoError(t, err, "the row must still exist")
		return user
	}

	t.Run("attacker cannot free their own identity mapping", func(t *testing.T) {
		w := do(http.MethodPut, "/v1/users/"+attacker.ID.String(), attackerToken,
			map[string]any{"supabase_user_id": ""})
		require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())

		var body map[string]string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), "the 403 must be one JSON object")
		assert.NotEmpty(t, body["error"])
		assert.Equal(t, attacker.SupabaseUserID, reload(attacker.ID).SupabaseUserID)
	})

	t.Run("attacker cannot assign their subject to the victim", func(t *testing.T) {
		w := do(http.MethodPut, "/v1/users/"+victim.ID.String(), attackerToken,
			map[string]any{"supabase_user_id": attacker.SupabaseUserID})
		require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())

		stored := reload(victim.ID)
		assert.Equal(t, victim.SupabaseUserID, stored.SupabaseUserID, "the victim keeps its subject")
		assert.Equal(t, victim.Email, stored.Email)
		assert.Equal(t, domain.UserRoleUser, stored.Role)
		assert.Equal(t, attacker.SupabaseUserID, reload(attacker.ID).SupabaseUserID)
	})

	t.Run("attacker cannot rewrite the victim through raw profile fields", func(t *testing.T) {
		replacement := "replaced-" + uuid.NewString() + "@example.com"
		w := do(http.MethodPut, "/v1/users/"+victim.ID.String(), attackerToken, map[string]any{
			"name":             "taken over",
			"email":            replacement,
			"role":             string(domain.UserRoleAdmin),
			"status":           string(domain.UserStatusActive),
			"supabase_user_id": attacker.SupabaseUserID,
		})
		require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())

		stored := reload(victim.ID)
		assert.Equal(t, victim.Name, stored.Name)
		assert.Equal(t, victim.Email, stored.Email)
		assert.Equal(t, domain.UserRoleUser, stored.Role)
		assert.Equal(t, victim.SupabaseUserID, stored.SupabaseUserID)
	})

	t.Run("attacker cannot pre-register the victim's subject", func(t *testing.T) {
		preemptEmail := "preempt-" + uuid.NewString() + "@example.com"
		w := do(http.MethodPost, "/v1/users", attackerToken, map[string]any{
			"email":            preemptEmail,
			"name":             "preempt",
			"supabase_user_id": victim.SupabaseUserID,
			"role":             string(domain.UserRoleAdmin),
		})
		require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())

		var rows []domain.User
		require.NoError(t, db.Where("email = ?", preemptEmail).Find(&rows).Error)
		assert.Empty(t, rows, "a rejected raw create must not write a row")
		assert.Equal(t, victim.SupabaseUserID, reload(victim.ID).SupabaseUserID)
	})

	t.Run("attacker cannot delete the victim row", func(t *testing.T) {
		w := do(http.MethodDelete, "/v1/users/"+victim.ID.String(), attackerToken, nil)
		require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
		reload(victim.ID)
	})

	t.Run("attacker still resolves to their own row", func(t *testing.T) {
		w := do(http.MethodGet, "/v1/users/me", attackerToken, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var me domain.User
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &me))
		assert.Equal(t, attacker.ID, me.ID, "identity resolution must stay on the attacker's own row")
		assert.Equal(t, attacker.Email, me.Email)
	})

	t.Run("owner can update the allowlisted profile fields", func(t *testing.T) {
		w := do(http.MethodPut, "/v1/users/me", attackerToken, map[string]any{
			"name":             "renamed",
			"avatar_url":       "https://example.com/avatar.png",
			"email":            "changed-" + uuid.NewString() + "@example.com",
			"role":             string(domain.UserRoleAdmin),
			"supabase_user_id": victim.SupabaseUserID,
		})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		stored := reload(attacker.ID)
		assert.Equal(t, "renamed", stored.Name)
		require.NotNil(t, stored.AvatarURL)
		assert.Equal(t, "https://example.com/avatar.png", *stored.AvatarURL)
		assert.Equal(t, attacker.Email, stored.Email, "the product DTO cannot change the email")
		assert.Equal(t, attacker.SupabaseUserID, stored.SupabaseUserID, "the product DTO cannot change the identity mapping")
		assert.Equal(t, domain.UserRoleUser, stored.Role)
	})

	t.Run("system admin keeps raw management", func(t *testing.T) {
		// Round-trip the row exactly as the admin read it, then change one field.
		w := do(http.MethodGet, "/v1/users/"+victim.ID.String(), adminToken, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var current domain.User
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &current))
		current.Name = "victim renamed by admin"

		w = do(http.MethodPut, "/v1/users/"+victim.ID.String(), adminToken, current)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		stored := reload(victim.ID)
		assert.Equal(t, "victim renamed by admin", stored.Name)
		assert.Equal(t, victim.SupabaseUserID, stored.SupabaseUserID, "raw admin writes keep their existing fields")
		assert.Equal(t, victim.Email, stored.Email)
		assert.Equal(t, domain.UserRoleUser, stored.Role)
	})
}
