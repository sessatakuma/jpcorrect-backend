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
			a.jwksCancel()
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

	if a.jwksCancel != nil {
		a.jwksCancel()
	}

	a.jwksCache = nil
}

// AuthMiddleware returns a Gin middleware that validates JWT tokens. It only
// proves that the caller holds a valid Supabase token; it does not require a
// local User row, so first-login routes such as /v1/users/init stay reachable.
func (a *API) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		err := a.validateToken(c)
		if err != nil {
			a.respondAuthError(c, err)
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireLocalUser resolves the JWT subject (supabase_user_id) to a local User
// row and exposes it as "userID" (uuid.UUID) and "user". It must run after
// AuthMiddleware. Routes that need a local account mount it; the registration
// routes deliberately do not.
func (a *API) RequireLocalUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		subject, ok := c.Get("supabaseUserID")
		supabaseUserID, _ := subject.(string)
		if !ok || supabaseUserID == "" {
			a.respondAuthError(c, domain.NewAuthError(
				http.StatusUnauthorized,
				"missing authenticated subject",
				"",
			))
			c.Abort()
			return
		}

		user, err := a.userRepo.GetBySupabaseUserID(c.Request.Context(), supabaseUserID)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				a.respondAuthError(c, domain.NewAuthError(
					http.StatusUnauthorized,
					"user not registered",
					"",
				))
			} else {
				a.respondAuthError(c, domain.NewAuthError(
					http.StatusInternalServerError,
					"internal server error",
					"",
				))
			}
			c.Abort()
			return
		}

		c.Set("userID", user.ID)
		c.Set("user", user)
		c.Next()
	}
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
			c.Set("userID", userID)
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

	// Parse and validate the token. Supabase puts the user's email in the
	// token, so a custom claims type carries it for first-login provisioning.
	type supabaseClaims struct {
		jwt.RegisteredClaims
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	}
	token, err := jwt.ParseWithClaims(tokenString, &supabaseClaims{}, kf.Keyfunc)
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

	claims, ok := token.Claims.(*supabaseClaims)
	if !ok {
		return domain.NewAuthError(
			http.StatusUnauthorized,
			"invalid token claims",
			"",
		)
	}

	// The subject is the only identity this service trusts. Reject a missing,
	// blank, or nil-UUID subject here, at the boundary, so no later lookup or
	// write can ever run with it.
	subject := strings.TrimSpace(claims.Subject)
	if subject == "" {
		return domain.NewAuthError(
			http.StatusUnauthorized,
			"missing token subject",
			"",
		)
	}
	if parsed, parseErr := uuid.Parse(subject); parseErr == nil && parsed == uuid.Nil {
		return domain.NewAuthError(
			http.StatusUnauthorized,
			"invalid token subject",
			"",
		)
	}

	c.Set("supabaseUserID", subject)
	if claims.Email != "" {
		c.Set("supabaseEmail", claims.Email)
	}
	c.Set("supabaseEmailVerified", claims.EmailVerified)

	return nil
}
