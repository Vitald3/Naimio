package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"strings"
	"syscall"

	_ "github.com/jackc/pgx/v5/stdlib"
	"golang.org/x/crypto/argon2"
	"golang.org/x/term"
)

const (
	argonTime    = 3
	argonMemory  = 64 * 1024
	argonThreads = 2
	argonKeyLen  = 32
)

func main() {
	ctx := context.Background()

	dsn := os.Getenv("DATABASE_URL")

	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatal(err)
	}

	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		log.Fatal("database connection failed:", err)
	}

	reader := bufio.NewReader(os.Stdin)

	email := readInput(reader, "Email: ")

	username := readInput(reader, "Username: ")

	displayName := readInput(reader, "Display name: ")

	password := readPassword("Password: ")

	passwordConfirm := readPassword("Confirm password: ")

	if string(password) != string(passwordConfirm) {
		log.Fatal("passwords do not match")
	}

	if len(password) < 12 {
		log.Fatal("password must be at least 12 characters")
	}

	emailNormalized := strings.ToLower(strings.TrimSpace(email))
	usernameNormalized := strings.ToLower(strings.TrimSpace(username))

	exists, err := userExists(ctx, db, emailNormalized)

	if err != nil {
		log.Fatal(err)
	}

	if exists {
		log.Fatal("user with this email already exists")
	}

	passwordHash := hashPassword(password)

	tx, err := db.BeginTx(ctx, nil)

	if err != nil {
		log.Fatal(err)
	}

	defer tx.Rollback()

	var userID string

	err = tx.QueryRowContext(
		ctx,
		"SELECT gen_random_uuid()",
	).Scan(&userID)

	if err != nil {
		log.Fatal(err)
	}

	_, err = tx.ExecContext(
		ctx,
		`
		INSERT INTO users
		(
			id,
			email,
			email_normalized,
			username,
			username_normalized,
			display_name,
			password_hash,
			email_verified_at,
			status
		)
		VALUES
		(
			$1,
			$2,
			$3,
			$4,
			$5,
			$6,
			$7,
			NOW(),
			'ACTIVE'
		)
		`,
		userID,
		email,
		emailNormalized,
		username,
		usernameNormalized,
		displayName,
		passwordHash,
	)

	if err != nil {
		log.Fatal("create user:", err)
	}

	_, err = tx.ExecContext(
		ctx,
		`
		INSERT INTO user_roles
		(
			user_id,
			role,
			granted_by
		)
		VALUES
		($1,'SUPER_ADMIN',$1),
		($1,'ADMIN',$1)
		ON CONFLICT DO NOTHING
		`,
		userID,
	)

	if err != nil {
		log.Fatal("assign roles:", err)
	}

	err = tx.Commit()

	if err != nil {
		log.Fatal(err)
	}

	fmt.Println()
	fmt.Println("=================================")
	fmt.Println("SUPER ADMIN CREATED SUCCESSFULLY")
	fmt.Println("=================================")
	fmt.Println("Email:", email)
	fmt.Println("Username:", username)
	fmt.Println("ID:", userID)
	fmt.Println()
	fmt.Println("You can login now.")
}

func readInput(reader *bufio.Reader, label string) string {

	fmt.Print(label)

	value, err := reader.ReadString('\n')

	if err != nil {
		log.Fatal(err)
	}

	return strings.TrimSpace(value)
}

func readPassword(label string) []byte {

	fmt.Print(label)

	password, err := term.ReadPassword(int(syscall.Stdin))

	fmt.Println()

	if err != nil {
		log.Fatal(err)
	}

	return password
}

func hashPassword(password []byte) string {
	salt := make([]byte, 16)

	_, err := rand.Read(salt)
	if err != nil {
		log.Fatal("failed to generate salt:", err)
	}

	hash := argon2.IDKey(
		password,
		salt,
		argonTime,
		argonMemory,
		argonThreads,
		argonKeyLen,
	)

	return fmt.Sprintf(
		"$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemory,
		argonTime,
		argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)
}

func userExists(
	ctx context.Context,
	db *sql.DB,
	email string,
) (bool, error) {

	var exists bool

	err := db.QueryRowContext(
		ctx,
		`
		SELECT EXISTS(
			SELECT 1
			FROM users
			WHERE email_normalized=$1
			AND deleted_at IS NULL
		)
		`,
		email,
	).Scan(&exists)

	return exists, err
}
