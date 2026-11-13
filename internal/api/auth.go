package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"jpcorrect-backend/internal/domain"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Context keys for values stored in the gin context by the auth middleware.
const (
	// ContextKeySupabaseID holds the verified JWT subject as a uuid.UUID.
	ContextKeySupabaseID = "supabaseID"
	// ContextKeyEmail holds the email claim of the verified token.
	ContextKeyEmail = "email"
	// ContextKeyUser holds the internal *domain.User resolved from the subject.
	ContextKeyUser = "user"
	// ContextKeyUserID holds the internal user UUID as a string, for handlers
	// that read the caller ID directly.
	ContextKeyUserID = "userID"
)

// usersInitPath is the one authenticated path exempt from the user lookup: it is
// the request that creates the row.
const usersInitPath = "/v1/users/init"

// SupabaseClaims extends standard JWT registered claims with Supabase-specific
// fields (such as the user's email) that are needed by downstream handlers.
type SupabaseClaims struct {
	jwt.RegisteredClaims
	Email string `json:"email,omitempty"`
}

// InitializeJWKS initializes the JWKS keyfunc for token validation
func (a *API) InitializeJWKS(initCtx context.Context) error {
	a.jwksMutex.Lock()
	defer a.jwksMutex.Unlock()

	// Create a long-lived context for the JWKS refresh goroutine
	a.jwksCtx, a.jwksCancel = context.WithCancel(context.Background())

	type initResult struct {
		kf  keyfunc.Keyfunc
		err error
	}
	resultCh := make(chan initResult, 1)

	go func() {
		kf, err := keyfunc.NewDefaultCtx(a.jwksCtx, []string{a.jwksURL})
		resultCh <- initResult{kf: kf, err: err}
	}()

	select {
	case <-initCtx.Done():
		a.jwksCancel()
		return fmt.Errorf("JWKS initialization timeout: %w", initCtx.Err())
	case result := <-resultCh:
		if result.err != nil {
			a.jwksCancel() // Clean up the context if initialization fails
			a.jwksErr = result.err
			return fmt.Errorf("failed to initialize JWKS: %w", result.err)
		}
		a.jwksCache = result.kf
		a.jwksErr = nil
		return nil
	}
}

// ShutdownJWKS gracefully shuts down the JWKS keyfunc to clean up resources
func (a *API) ShutdownJWKS() {
	a.jwksMutex.Lock()
	defer a.jwksMutex.Unlock()

	// Cancel the context to stop the JWKS refresh goroutine
	if a.jwksCancel != nil {
		a.jwksCancel()
	}

	// Set jwksCache to nil to allow garbage collection
	a.jwksCache = nil
}

// AuthMiddleware returns a Gin middleware that validates JWT tokens and
// resolves the internal user the token belongs to.
func (a *API) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		err := a.validateToken(c)
		if err != nil {
			a.respondAuthError(c, err)
			c.Abort()
			return
		}

		// POST /v1/users/init creates the row, so it is the only request that
		// runs without the lookup.
		if !isUsersInitRequest(c) {
			if err := a.resolveUser(c); err != nil {
				a.respondAuthError(c, err)
				c.Abort()
				return
			}
		}

		c.Next()
	}
}

// resolveUser maps the verified Supabase subject to the stored internal user and
// publishes it on the request. Downstream handlers must use the internal user
// ID, never the Supabase subject: guild membership and every other ownership
// check is keyed by the internal ID.
func (a *API) resolveUser(c *gin.Context) error {
	val, exists := c.Get(ContextKeySupabaseID)
	supabaseID, ok := val.(uuid.UUID)
	if !exists || !ok {
		return domain.NewAuthError(
			http.StatusUnauthorized,
			"missing authentication info",
			"",
		)
	}

	user, err := a.userRepo.GetBySupabaseUserID(c.Request.Context(), supabaseID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.NewAuthError(
				http.StatusUnauthorized,
				"user not initialized",
				"call POST /v1/users/init to create the account",
			)
		}
		log.Printf("failed to resolve the user for supabase subject %s: %v", supabaseID, err)
		return domain.NewAuthError(
			http.StatusInternalServerError,
			"failed to resolve user",
			"",
		)
	}

	c.Set(ContextKeyUser, user)
	c.Set(ContextKeyUserID, user.ID.String())

	return nil
}

// currentUser returns the internal user resolved from the request JWT, or nil
// when the request carries no resolved identity.
func currentUser(c *gin.Context) *domain.User {
	val, exists := c.Get(ContextKeyUser)
	if !exists {
		return nil
	}
	user, ok := val.(*domain.User)
	if !ok {
		return nil
	}
	return user
}

// isUsersInitRequest reports whether the request is the one path exempt from the
// user lookup.
func isUsersInitRequest(c *gin.Context) bool {
	return c.Request.Method == http.MethodPost && c.Request.URL.Path == usersInitPath
}

// debugCallerMiddleware supplies an explicitly selected caller without JWT
// validation. Register installs it only in debug mode.
func debugCallerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		caller := c.GetHeader("X-Debug-User-ID")
		if caller != "" {
			userID, err := uuid.Parse(caller)
			if err != nil || userID == uuid.Nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid X-Debug-User-ID header"})
				return
			}
			c.Set("userID", userID.String())
		}
		c.Next()
	}
}

// APIKeyMiddleware requires a static API key in the X-API-Key header that matches
// CLIENT_API_KEY. Intended for non-user-specific service callers; user-scoped
// callers should use the JWT-based AuthMiddleware on regular /v1 routes instead.
func (a *API) APIKeyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if a.clientAPIKey == "" {
			a.respondAuthError(c, domain.NewAuthError(
				http.StatusUnauthorized,
				"API key authentication is not configured",
				"",
			))
			c.Abort()
			return
		}

		providedKey := c.GetHeader("X-API-Key")
		if providedKey == "" {
			a.respondAuthError(c, domain.NewAuthError(
				http.StatusUnauthorized,
				"missing X-API-Key header",
				"",
			))
			c.Abort()
			return
		}

		if subtle.ConstantTimeCompare([]byte(providedKey), []byte(a.clientAPIKey)) != 1 {
			a.respondAuthError(c, domain.NewAuthError(
				http.StatusUnauthorized,
				"invalid API key",
				"",
			))
			c.Abort()
			return
		}

		c.Next()
	}
}

// respondAuthError handles authentication errors and returns appropriate HTTP response
func (a *API) respondAuthError(c *gin.Context, err error) {
	authErr, ok := err.(*domain.AuthError)
	if ok {
		c.JSON(authErr.StatusCode, gin.H{
			"error":   authErr.Message,
			"details": authErr.Details,
		})
	} else {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "internal server error",
		})
	}
}

// validateToken validates the JWT token and extracts user information.
// Accepts "Bearer <token>" (any casing) as well as a bare "<token>" — the
// latter compensates for Swagger UI's apiKey field, which doesn't auto-prefix
// the scheme.
func (a *API) validateToken(c *gin.Context) error {
	authHeader := c.GetHeader("Authorization")
	fields := strings.Fields(authHeader)

	var tokenString string
	switch len(fields) {
	case 0:
		return domain.NewAuthError(
			http.StatusUnauthorized,
			"missing authorization header",
			"",
		)
	case 1:
		tokenString = fields[0]
	case 2:
		if !strings.EqualFold(fields[0], "Bearer") {
			return domain.NewAuthError(
				http.StatusUnauthorized,
				"invalid authorization header format",
				"",
			)
		}
		tokenString = fields[1]
	default:
		return domain.NewAuthError(
			http.StatusUnauthorized,
			"invalid authorization header format",
			"",
		)
	}

	// Check if JWKS is initialized
	a.jwksMutex.Lock()
	if a.jwksErr != nil || a.jwksCache == nil {
		a.jwksMutex.Unlock()
		return domain.NewAuthError(
			http.StatusInternalServerError,
			"JWKS not initialized",
			"",
		)
	}
	kf := a.jwksCache
	a.jwksMutex.Unlock()

	// Parse and validate the token
	token, err := jwt.ParseWithClaims(tokenString, &SupabaseClaims{}, kf.Keyfunc)
	if err != nil {
		log.Printf("invalid token error: %v", err)
		return domain.NewAuthError(
			http.StatusUnauthorized,
			"invalid token",
			"",
		)
	}

	if !token.Valid {
		return domain.NewAuthError(
			http.StatusUnauthorized,
			"invalid token",
			"",
		)
	}

	// Extract claims
	claims, ok := token.Claims.(*SupabaseClaims)
	if !ok {
		return domain.NewAuthError(
			http.StatusUnauthorized,
			"invalid token claims",
			"",
		)
	}

	// The JWT subject is the Supabase user UUID.
	supabaseID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return domain.NewAuthError(
			http.StatusUnauthorized,
			"invalid subject claim",
			"",
		)
	}

	// Expose the Supabase ID and email to downstream handlers. The subject is not
	// the internal user ID: AuthMiddleware resolves that separately.
	c.Set(ContextKeySupabaseID, supabaseID)
	c.Set(ContextKeyEmail, claims.Email)

	return nil
}
