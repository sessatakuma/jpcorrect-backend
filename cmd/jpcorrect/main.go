package main

import (
	"jpcorrect-backend/internal/repository"

	_ "github.com/joho/godotenv/autoload"
)

func main() {
	repository.Test()
}
