package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"jpcorrect-backend/internal/domain"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

var (
	jwksCache keyfunc.Keyfunc
	jwksMutex sync.Mutex
	jwksErr   error
)

// InitializeJWKS initializes the JWKS keyfunc for token validation
func (a *API) InitializeJWKS(ctx context.Context) error {
	jwksMutex.Lock()
	defer jwksMutex.Unlock()

	var err error
	jwksCache, err = keyfunc.NewDefaultCtx(ctx, []string{a.jwksURL})
	if err != nil {
		jwksErr = err
		return fmt.Errorf("failed to initialize JWKS: %w", err)
	}

	jwksErr = nil
	return nil
}

// AuthMiddleware returns a Gin middleware that validates JWT tokens
func (a *API) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		err := a.validateToken(c)
		if err != nil {
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
			c.Abort()
			return
		}

		c.Next()
	}
}

// validateToken validates the JWT token and extracts user information
func (a *API) validateToken(c *gin.Context) error {
	// Get the Authorization header
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		return domain.ErrMissingAuthHeader
	}

	// Extract the token from "Bearer <token>"
	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		return domain.ErrInvalidAuthHeader
	}

	tokenString := parts[1]

	// Check if JWKS is initialized
	jwksMutex.Lock()
	if jwksErr != nil || jwksCache == nil {
		jwksMutex.Unlock()
		return domain.ErrJWKSNotInitialized
	}
	kf := jwksCache
	jwksMutex.Unlock()

	// Parse and validate the token
	token, err := jwt.ParseWithClaims(tokenString, &jwt.RegisteredClaims{}, kf.Keyfunc)
	if err != nil {
		authErr := &domain.AuthError{
			StatusCode: 401,
			Message:    "invalid token",
			Details:    err.Error(),
		}
		return authErr
	}

	if !token.Valid {
		return domain.ErrInvalidToken
	}

	// Extract claims
	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !ok {
		return domain.ErrInvalidTokenClaims
	}

	// Store the user ID (subject) in the context for downstream handlers
	c.Set("userID", claims.Subject)

	return nil
}
