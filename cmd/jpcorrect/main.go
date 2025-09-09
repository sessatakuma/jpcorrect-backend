package main

import (
	"jpcorrect-backend/internal/cmd"

	_ "github.com/joho/godotenv/autoload"
)

func main() {
	// cmd.TestConnection()
	cmd.StartAPI()
}
