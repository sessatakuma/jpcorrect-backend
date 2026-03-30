# Load environment variables from .env file if it exists
ifneq (,$(wildcard .env))
    include .env
    export
endif

# Determine binary extension based on OS
BIN_EXT =
ifeq ($(OS),Windows_NT)
    BIN_EXT = .exe
endif

.PHONY: air

air:
	go tool air --build.cmd "go build -o ./tmp/main$(BIN_EXT) ./cmd/jpcorrect/main.go" --build.entrypoint "./tmp/main$(BIN_EXT)"
