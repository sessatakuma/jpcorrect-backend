// @title jpcorrect API
// @version 1.0
// @description Japanese language correction platform backend API
// @BasePath /
// Intentionally no @host: a hardcoded host bakes itself into doc.json and makes
// Swagger UI's "Execute" fire requests at that host regardless of where the UI
// is served. Omitting it lets Swagger UI fall back to the serving origin, so
// "Execute" works for local dev (localhost:8080), dev (api-dev.sessatakuma.dev),
// and any future host without per-env regeneration.
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description JWT issued by the configured JWKS provider. Paste the raw token; the "Bearer " scheme prefix is optional (added automatically if missing).

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
