package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

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
		// Get the Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			c.Abort()
			return
		}

		// Extract the token from "Bearer <token>"
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization header format"})
			c.Abort()
			return
		}

		tokenString := parts[1]

		// Check if JWKS is initialized
		jwksMutex.Lock()
		if jwksErr != nil || jwksCache == nil {
			jwksMutex.Unlock()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "JWKS not initialized"})
			c.Abort()
			return
		}
		kf := jwksCache
		jwksMutex.Unlock()

		// Parse and validate the token
		token, err := jwt.ParseWithClaims(tokenString, &jwt.RegisteredClaims{}, kf.Keyfunc)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token: " + err.Error()})
			c.Abort()
			return
		}

		if !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}

		// Extract claims
		claims, ok := token.Claims.(*jwt.RegisteredClaims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token claims"})
			c.Abort()
			return
		}

		// Store the user ID (subject) in the context for downstream handlers
		c.Set("userID", claims.Subject)

		c.Next()
	}
}
