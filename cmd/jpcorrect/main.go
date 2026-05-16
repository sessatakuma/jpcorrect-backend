// @title jpcorrect API
// @version 1.0
// @description Japanese language correction platform backend API
// @host localhost:8080
// @BasePath /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description JWT token issued by the configured JWKS provider. Pass as "Bearer <token>".

// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name X-API-Key
// @description Static API key (matches CLIENT_API_KEY) for non-user-specific service access to the /v1 api-tools endpoints. Use this when the caller has no per-user identity; otherwise use BearerAuth.
package main

import (
	"jpcorrect-backend/internal/cmd"

	_ "github.com/joho/godotenv/autoload"
)

func main() {
	// cmd.TestConnection()
	cmd.Execute()
}
