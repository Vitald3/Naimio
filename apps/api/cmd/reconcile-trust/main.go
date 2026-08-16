package main

import (
	"context"
	"database/sql"
	"freelance/apps/api/internal/reviews"
	_ "github.com/jackc/pgx/v5/stdlib"
	"log"
	"os"
)

func main() {
	databaseURL, userID := os.Getenv("DATABASE_URL"), os.Getenv("USER_ID")
	if databaseURL == "" || userID == "" {
		log.Fatal("DATABASE_URL and USER_ID are required")
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	stats, err := (reviews.Service{Repository: reviews.PostgresRepository{DB: db}}).Recalculate(context.Background(), userID)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("trust stats reconciled reviews=%d completed_projects=%d", stats.ReviewsCount, stats.CompletedProjectsCount)
}
