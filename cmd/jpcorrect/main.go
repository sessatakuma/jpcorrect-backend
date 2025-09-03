package main

import (
	"jpcorrect-backend/internal/db"

	_ "github.com/joho/godotenv/autoload"
)

func main() {
	db.Test()
}
