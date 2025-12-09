.PHONY: air sqlc migrate-create migrate-up migrate-down

BIN_EXT =

ifeq ($(OS),Windows_NT)
    BIN_EXT = .exe
endif

air:
	go tool air --build.cmd "go build -o ./tmp/main$(BIN_EXT) ./cmd/jpcorrect/main.go" --build.entrypoint "./tmp/main$(BIN_EXT)"

sqlc:
	go tool sqlc generate

migrate-create:
	go tool migrate create -ext sql -dir db/migrations -seq $(name)

migrate-up:
	go tool migrate -path db/migrations -database "$(subst postgres://,pgx5://,$(DATABASE_URL))" up

migrate-down:
	go tool migrate -path db/migrations -database "$(subst postgres://,pgx5://,$(DATABASE_URL))" down