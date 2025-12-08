package cmd

import (
	"context"
	"log"
	"os"

	"jpcorrect-backend/internal/api"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func StartAPI() {
	dbpool, err := pgxpool.New(context.Background(), os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
		os.Exit(1)
	}
	defer dbpool.Close()

	a := api.NewAPI(dbpool)

	r := gin.Default()
	api.Register(r, a)
	r.Run() // listen and serve on
}
