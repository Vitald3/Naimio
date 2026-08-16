//go:build integration

package profiles

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPostgresProfileRepository(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("DATABASE_URL is required for integration tests")
	}
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	ctx := context.Background()
	userID := "44444444-4444-4444-8444-444444444444"
	categoryID := "55555555-5555-4555-8555-555555555555"
	skillID := "66666666-6666-4666-8666-666666666666"
	if _, err = database.ExecContext(ctx, `INSERT INTO users (id, email, email_normalized, password_hash, username, username_normalized, display_name) VALUES ($1, 'profile-test@example.invalid', 'profile-test@example.invalid', 'test-only', 'ProfileTest', 'profiletest', 'Profile Test')`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err = database.ExecContext(ctx, `INSERT INTO professional_profiles (user_id, availability, profile_visibility) VALUES ($1, 'UNAVAILABLE', 'PRIVATE')`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err = database.ExecContext(ctx, `INSERT INTO categories (id, slug, name) VALUES ($1, 'profile-test-category', 'Profile Test Category')`, categoryID); err != nil {
		t.Fatal(err)
	}
	if _, err = database.ExecContext(ctx, `INSERT INTO skills (id, slug, name) VALUES ($1, 'profile-test-skill', 'Profile Test Skill')`, skillID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, q := range []struct {
			query string
			args  []any
		}{
			{query: "DELETE FROM profile_languages WHERE user_id = $1", args: []any{userID}},
			{query: "DELETE FROM profile_skills WHERE user_id = $1", args: []any{userID}},
			{query: "DELETE FROM profile_categories WHERE user_id = $1", args: []any{userID}},
			{query: "DELETE FROM professional_profiles WHERE user_id = $1", args: []any{userID}},
			{query: "DELETE FROM users WHERE id = $1", args: []any{userID}},
			{query: "DELETE FROM skills WHERE id = $1", args: []any{skillID}},
			{query: "DELETE FROM categories WHERE id = $1", args: []any{categoryID}},
		} {
			_, _ = database.ExecContext(context.Background(), q.query, q.args...)
		}
	})

	repository := PostgresRepository{DB: database}
	experienceYears := 7
	hourlyRate := int64(250000)
	minimumOrder := int64(500000)
	update := UpdateRequest{
		ProfessionalTitle: "Go developer", Bio: "Plain text", LocationText: "Москва",
		CountryCode: "RU", ExperienceYears: &experienceYears, HourlyRateKopecks: &hourlyRate, MinimumOrderKopecks: &minimumOrder,
		Availability: "AVAILABLE", ProfileVisibility: "PUBLIC",
		Categories: []CategorySelection{{ID: categoryID, IsPrimary: true}},
		Skills:     []SkillSelection{{ID: skillID, Level: "EXPERT", IsFeatured: true}},
		Languages:  []Language{{Code: "RU", Level: "NATIVE"}},
	}
	for attempt := 0; attempt < 2; attempt++ {
		profile, updateErr := repository.Update(ctx, userID, update)
		if updateErr != nil || len(profile.Categories) != 1 || len(profile.Skills) != 1 || len(profile.Languages) != 1 {
			t.Fatalf("attempt %d profile = %#v, error = %v", attempt, profile, updateErr)
		}
	}

	publicProfile, err := repository.Public(ctx, "PROFILETEST")
	if err != nil || publicProfile.Username != "ProfileTest" || publicProfile.Skills[0].Name != "Profile Test Skill" ||
		publicProfile.CountryCode != "RU" || publicProfile.HourlyRateKopecks == nil || *publicProfile.HourlyRateKopecks != hourlyRate {
		t.Fatalf("public profile = %#v, error = %v", publicProfile, err)
	}
	page, err := repository.PublicList(ctx, "Go developer", nil, 20)
	if err != nil || len(page.Items) != 1 || page.Items[0].Bio != "" || page.Items[0].LocationText != "" || len(page.Items[0].Skills) != 1 {
		t.Fatalf("public list = %#v, error = %v", page.Items, err)
	}

	if _, err := repository.ReplaceCategories(ctx, userID, []CategorySelection{{ID: "77777777-7777-4777-8777-777777777777"}}); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("unknown category error = %v", err)
	}
	var categoryCount int
	if err := database.QueryRowContext(ctx, "SELECT count(*) FROM profile_categories WHERE user_id = $1", userID).Scan(&categoryCount); err != nil || categoryCount != 1 {
		t.Fatalf("category count = %d, error = %v", categoryCount, err)
	}

	if _, err := repository.Update(ctx, "88888888-8888-4888-8888-888888888888", update); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user update error = %v", err)
	}
	update.Categories, update.Skills, update.Languages = nil, nil, nil
	update.ProfileVisibility = "PRIVATE"
	if _, err := repository.Update(ctx, userID, update); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Public(ctx, "profiletest"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("private profile error = %v", err)
	}
}
