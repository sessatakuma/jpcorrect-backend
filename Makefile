.PHONY: air sqlc createdb updb downdb

air:
	go tool air --build.cmd "go build -o ./tmp/main.exe ./cmd/jpcorrect/main.go" --build.entrypoint "./tmp/main.exe"

sqlc:
	go tool sqlc generate

createdb:
	go tool migrate create -ext sql -dir db/migrations -seq $(name)

updb:
	go tool migrate -path db/migrations -database "$(subst postgres://,pgx5://,$(DATABASE_URL))" up

downdb:
	go tool migrate -path db/migrations -database "$(subst postgres://,pgx5://,$(DATABASE_URL))" down