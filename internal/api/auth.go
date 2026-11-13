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

// AuthMiddleware returns a Gin middleware that validates JWT tokens.
//
// Every authenticated request runs it in every GIN_MODE (spec §1.2 item 6):
// there is no debug skip and no header-based identity. After the JWKS check the
// middleware resolves the internal User row from the token's `sub` and stores it
// in the context for currentUser. The one exemption is POST /v1/users/init,
// which creates the row and therefore skips only the DB lookup (§1.2 item 4).
//
// A verified `sub` that is not a UUID gets a route-specific answer: 401 here on
// every route that resolves a user, and 400 INVALID_REQUEST from users/init
// itself (spec §2.1). Neither case reaches the repository.
func (a *API) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		auth, err := a.verifyToken(c)
		if err != nil {
			writeError(c, err)
			c.Abort()
			return
		}

		c.Set(supabaseAuthContextKey, auth)

		if isUsersInitRequest(c) {
			c.Next()
			return
		}

		subject, err := parseSubject(auth.Subject)
		if err != nil {
			writeError(c, errorWithMessage(domain.ErrUnauthorized, "invalid token claims"))
			c.Abort()
			return
		}

		user, err := a.userRepo.GetBySupabaseUserID(c.Request.Context(), subject)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				writeError(c, errorWithMessage(domain.ErrUserNotInitialized, "call POST /v1/users/init to create your account"))
				c.Abort()
				return
			}
			writeError(c, err)
			c.Abort()
			return
		}
		c.Set(userContextKey, user)

		c.Next()
	}
}

const (
	// userContextKey holds the *domain.User every authenticated handler reads
	// through currentUser (spec §1.2 item 2). The retired keys were "userID".
	userContextKey = "user"

	// supabaseAuthContextKey holds the verified JWT identity before the DB
	// lookup. Only POST /v1/users/init, which is exempt from that lookup, reads
	// it.
	supabaseAuthContextKey = "supabaseAuth"

	// usersInitPath is the single authenticated path exempted from the user
	// lookup (spec §1.2 item 4).
	usersInitPath = "/v1/users/init"
)

// supabaseAuth is what AuthMiddleware keeps from a verified Supabase JWT. The
// subject stays the raw `sub` claim because the route decides how to reject an
// unusable one: every user-resolving route answers 401, while users/init answers
// 400 INVALID_REQUEST (spec §2.1). AuthMiddleware parses the claim before the
// lookup, so handlers other than init only see validated identities.
type supabaseAuth struct {
	Subject string
	Email   string
}

// parseSubject turns the verified `sub` claim into the UUID the user table
// stores. An empty claim, a claim that is not a UUID, and the nil UUID identify
// no account. Callers choose the status: 401 on authenticated routes and 400
// INVALID_REQUEST on POST /v1/users/init (spec §2.1).
func parseSubject(raw string) (uuid.UUID, error) {
	if raw == "" {
		return uuid.Nil, errors.New("the access token has no subject")
	}

	subject, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, errors.New("the access token subject is not a UUID")
	}
	if subject == uuid.Nil {
		return uuid.Nil, errors.New("the access token subject is the nil UUID")
	}

	return subject, nil
}

// supabaseClaims is the subset of the Supabase JWT the backend reads. Role or
// other authorization claims are deliberately not part of it: authorization
// uses only the stored User row (issue #73 clarification).
type supabaseClaims struct {
	Email string `json:"email"`
	jwt.RegisteredClaims
}

// currentUser returns the internal User resolved from the request's JWT, or nil
// when the request has no resolved user (unauthenticated request, or the
// users/init exemption). Handlers treat nil as "reject with ErrUnauthorized".
func currentUser(c *gin.Context) *domain.User {
	value, exists := c.Get(userContextKey)
	if !exists {
		return nil
	}
	user, ok := value.(*domain.User)
	if !ok {
		return nil
	}
	return user
}

// currentSupabaseAuth returns the verified JWT identity, or nil when the
// request was not authenticated by AuthMiddleware.
func currentSupabaseAuth(c *gin.Context) *supabaseAuth {
	value, exists := c.Get(supabaseAuthContextKey)
	if !exists {
		return nil
	}
	auth, ok := value.(*supabaseAuth)
	if !ok {
		return nil
	}
	return auth
}

// isUsersInitRequest reports whether the request is the one path exempt from the
// user lookup.
func isUsersInitRequest(c *gin.Context) bool {
	return c.Request.Method == http.MethodPost && c.Request.URL.Path == usersInitPath
}

// APIKeyMiddleware requires a static API key in the X-API-Key header that matches
// CLIENT_API_KEY. Intended for non-user-specific service callers; user-scoped
// callers should use the JWT-based AuthMiddleware on regular /v1 routes instead.
func (a *API) APIKeyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if a.clientAPIKey == "" {
			writeError(c, errorWithMessage(domain.ErrUnauthorized, "API key authentication is not configured"))
			c.Abort()
			return
		}

		providedKey := c.GetHeader("X-API-Key")
		if providedKey == "" {
			writeError(c, errorWithMessage(domain.ErrUnauthorized, "missing X-API-Key header"))
			c.Abort()
			return
		}

		if subtle.ConstantTimeCompare([]byte(providedKey), []byte(a.clientAPIKey)) != 1 {
			writeError(c, errorWithMessage(domain.ErrUnauthorized, "invalid API key"))
			c.Abort()
			return
		}

		c.Next()
	}
}

// verifyToken validates the JWT token and extracts the caller's Supabase
// identity.
// Accepts "Bearer <token>" (any casing) as well as a bare "<token>" — the
// latter compensates for Swagger UI's apiKey field, which doesn't auto-prefix
// the scheme.
func (a *API) verifyToken(c *gin.Context) (*supabaseAuth, error) {
	authHeader := c.GetHeader("Authorization")
	fields := strings.Fields(authHeader)

	var tokenString string
	switch len(fields) {
	case 0:
		return nil, errorWithMessage(domain.ErrUnauthorized, "missing authorization header")
	case 1:
		tokenString = fields[0]
	case 2:
		if !strings.EqualFold(fields[0], "Bearer") {
			return nil, errorWithMessage(domain.ErrUnauthorized, "invalid authorization header format")
		}
		tokenString = fields[1]
	default:
		return nil, errorWithMessage(domain.ErrUnauthorized, "invalid authorization header format")
	}

	// Check if JWKS is initialized
	a.jwksMutex.Lock()
	if a.jwksErr != nil || a.jwksCache == nil {
		a.jwksMutex.Unlock()
		// A misconfigured JWKS endpoint is an operator problem: log it and
		// keep the response free of internal detail.
		log.Printf("JWKS is not initialized: %v", a.jwksErr)
		return nil, domain.ErrInternal
	}
	kf := a.jwksCache
	a.jwksMutex.Unlock()

	// Parse and validate the token
	token, err := jwt.ParseWithClaims(tokenString, &supabaseClaims{}, kf.Keyfunc)
	if err != nil {
		log.Printf("invalid token error: %v", err)
		return nil, errorWithMessage(domain.ErrUnauthorized, "invalid token")
	}

	if !token.Valid {
		return nil, errorWithMessage(domain.ErrUnauthorized, "invalid token")
	}

	// Extract claims
	claims, ok := token.Claims.(*supabaseClaims)
	if !ok {
		return nil, errorWithMessage(domain.ErrUnauthorized, "invalid token claims")
	}
	// The subject is deliberately not parsed here. users/init owns the parse so
	// it can answer 400 INVALID_REQUEST for a `sub` that is not a UUID, while
	// every other route answers 401 in AuthMiddleware (spec §2.1).
	return &supabaseAuth{Subject: claims.Subject, Email: claims.Email}, nil
}
