package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"

	"jpcorrect-backend/internal/domain"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// InitializeJWKS initializes the JWKS keyfunc for token validation
func (a *API) InitializeJWKS(ctx context.Context) error {
	a.jwksMutex.Lock()
	defer a.jwksMutex.Unlock()

	// Create a long-lived context for the JWKS refresh goroutine
	a.jwksCtx, a.jwksCancel = context.WithCancel(context.Background())

	var err error
	a.jwksCache, err = keyfunc.NewDefaultCtx(a.jwksCtx, []string{a.jwksURL})
	if err != nil {
		a.jwksCancel() // Clean up the context if initialization fails
		a.jwksErr = err
		return fmt.Errorf("failed to initialize JWKS: %w", err)
	}

	a.jwksErr = nil
	return nil
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

// AuthMiddleware returns a Gin middleware that validates JWT tokens
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

// validateToken validates the JWT token and extracts user information
func (a *API) validateToken(c *gin.Context) error {
	// Get the Authorization header
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		return domain.NewAuthError(
			http.StatusUnauthorized,
			"missing authorization header",
			"",
		)
	}

	// Extract the token from "Bearer <token>"
	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		return domain.NewAuthError(
			http.StatusUnauthorized,
			"invalid authorization header format",
			"",
		)
	}

	tokenString := strings.TrimSpace(parts[1])
	if tokenString == "" {
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
	token, err := jwt.ParseWithClaims(tokenString, &jwt.RegisteredClaims{}, kf.Keyfunc)
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
	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !ok {
		return domain.NewAuthError(
			http.StatusUnauthorized,
			"invalid token claims",
			"",
		)
	}

	// Store the user ID (subject) in the context for downstream handlers
	c.Set("userID", claims.Subject)

	return nil
}
