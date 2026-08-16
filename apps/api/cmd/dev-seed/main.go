package main

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"os"
	"strings"

	"database/sql"
	"encoding/base64"
	_ "github.com/jackc/pgx/v5/stdlib"
	"golang.org/x/crypto/argon2"
)

//go:embed seed.sql
var seedSQL string

const devPassword = "LocalDemo2026!"

func main() {
	if strings.ToLower(os.Getenv("APP_ENV")) != "development" {
		log.Fatal("dev seed is allowed only with APP_ENV=development")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Rollback()
	salt := []byte("freelance-dev-26")
	hash := argon2.IDKey([]byte(devPassword), salt, 3, 64*1024, 2, 32)
	encoded := fmt.Sprintf("$argon2id$v=19$m=65536,t=3,p=2$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash))
	if _, err = tx.ExecContext(ctx, "SELECT set_config('freelance.seed_hash',$1,true)", encoded); err != nil {
		log.Fatal(err)
	}
	if _, err = tx.ExecContext(ctx, seedSQL); err != nil {
		log.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Development demo data is ready. Password for all demo accounts:", devPassword)
}
