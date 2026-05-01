// @title jpcorrect API
// @version 1.0
// @description Japanese language correction platform backend API
// @host localhost:8080
// @BasePath /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
package main

import (
	"jpcorrect-backend/internal/cmd"

	_ "github.com/joho/godotenv/autoload"
)

func main() {
	// cmd.TestConnection()
	cmd.Execute()
}
