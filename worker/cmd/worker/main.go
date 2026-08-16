package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	mail "freelance/worker/internal/email"
	notification "freelance/worker/internal/notification"
	_ "github.com/jackc/pgx/v5/stdlib"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var readiness []func(context.Context) error
	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
		db, err := sql.Open("pgx", databaseURL)
		if err != nil {
			log.Fatal(err)
		}
		defer db.Close()
		configurePool(db)
		readiness = append(readiness, db.PingContext)
		if os.Getenv("APP_ENV") == "production" && (os.Getenv("SMTP_ADDRESS") == "" || os.Getenv("SMTP_FROM") == "" || os.Getenv("PUBLIC_BASE_URL") == "") {
			log.Fatal("SMTP_ADDRESS, SMTP_FROM and PUBLIC_BASE_URL are required in production")
		}
		provider := mail.SMTPProvider{Address: os.Getenv("SMTP_ADDRESS"), Username: os.Getenv("SMTP_USERNAME"), Password: os.Getenv("SMTP_PASSWORD"), From: os.Getenv("SMTP_FROM")}
		publicBaseURL := os.Getenv("PUBLIC_BASE_URL")
		if os.Getenv("APP_ENV") != "production" {
			localIP := os.Getenv("LOCAL_IP_ADDRESS")
			if localIP == "" {
				localIP = os.Getenv("local_ip_address")
			}
			if localIP != "" && localIP != "0.0.0.0" {
				publicBaseURL = "http://" + localIP + ":8088"
			}
		}
		if publicBaseURL == "" {
			publicBaseURL = "http://localhost:8088"
		}
		processor := mail.Processor{Repository: mail.PostgresRepository{DB: db}, Provider: provider, PublicBaseURL: publicBaseURL, MaxAttempts: 5}
		notificationProcessor := notification.Processor{Repository: notification.PostgresRepository{DB: db}}
		digestProcessor := notification.DigestProcessor{Repository: notification.PostgresRepository{DB: db}}
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					runCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
					if err := notificationProcessor.Once(runCtx); err != nil {
						structuredLog("error", "notification_job_failed", err)
					}
					if err := digestProcessor.Once(runCtx); err != nil {
						structuredLog("error", "digest_job_failed", err)
					}
					if err := processor.Once(runCtx); err != nil {
						structuredLog("error", "email_job_failed", err)
					}
					cancel()
				}
			}
		}()
		if target := strings.TrimSpace(os.Getenv("PAYMENT_RECONCILIATION_URL")); target != "" {
			go authenticatedJobLoop(ctx, "payment_reconciliation", target, os.Getenv("PAYMENT_RECONCILIATION_TOKEN"), duration(os.Getenv("PAYMENT_RECONCILIATION_INTERVAL"), time.Minute), positiveInt(os.Getenv("PAYMENT_RECONCILIATION_BATCH"), 100))
		}
		if target := strings.TrimSpace(os.Getenv("PRO_RENEWAL_URL")); target != "" {
			go authenticatedJobLoop(ctx, "pro_renewal", target, os.Getenv("PRO_RENEWAL_TOKEN"), duration(os.Getenv("PRO_RENEWAL_INTERVAL"), 5*time.Minute), positiveInt(os.Getenv("PRO_RENEWAL_BATCH"), 50))
		}
	} else if os.Getenv("APP_ENV") == "production" {
		log.Fatal("DATABASE_URL is required in production")
	}
	server := newServer(":8081", readiness...)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			structuredLog("error", "shutdown_failed", err)
		}
	}()
	structuredLog("info", "listening", nil)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

// authenticatedJobLoop schedules bounded internal API jobs without ever receiving
// provider credentials. The API owns provider/domain state and the worker only
// triggers authenticated, retryable batches.
func authenticatedJobLoop(ctx context.Context, name, target, token string, every time.Duration, batch int) {
	if token == "" {
		structuredLog("error", name+"_disabled_missing_token", nil)
		return
	}
	u, err := url.Parse(target)
	if err != nil || u.Scheme == "" || u.Host == "" {
		structuredLog("error", name+"_disabled_invalid_url", err)
		return
	}
	client := &http.Client{Timeout: 20 * time.Second}
	run := func() {
		runCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(runCtx, http.MethodPost, target+"?limit="+strconv.Itoa(batch), nil)
		if err != nil {
			structuredLog("error", name+"_request", err)
			return
		}
		req.Header.Set("Authorization", "Bearer "+token)
		res, err := client.Do(req)
		if err != nil {
			structuredLog("error", name+"_failed", err)
			return
		}
		defer res.Body.Close()
		if res.StatusCode/100 != 2 {
			structuredLog("error", name+"_rejected", fmt.Errorf("status %d", res.StatusCode))
		}
	}
	run()
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

func configurePool(db *sql.DB) {
	maxOpen := positiveInt(os.Getenv("DB_MAX_OPEN_CONNS"), 10)
	maxIdle := positiveInt(os.Getenv("DB_MAX_IDLE_CONNS"), 3)
	if maxIdle > maxOpen {
		maxIdle = maxOpen
	}
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(duration(os.Getenv("DB_CONN_MAX_LIFETIME"), 30*time.Minute))
	db.SetConnMaxIdleTime(duration(os.Getenv("DB_CONN_MAX_IDLE_TIME"), 5*time.Minute))
}
func positiveInt(raw string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
func duration(raw string, fallback time.Duration) time.Duration {
	value, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
func structuredLog(level, event string, err error) {
	entry := map[string]any{"timestamp": time.Now().UTC().Format(time.RFC3339Nano), "level": level, "service": "worker", "event": event}
	if err != nil {
		entry["error_class"] = fmt.Sprintf("%T", err)
	}
	body, _ := json.Marshal(entry)
	log.Print(string(body))
}

func newServer(address string, checks ...func(context.Context) error) *http.Server {
	mux := http.NewServeMux()
	health := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
	mux.HandleFunc("/health/live", health)
	mux.HandleFunc("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		for _, check := range checks {
			if check(ctx) != nil {
				http.Error(w, "not ready", http.StatusServiceUnavailable)
				return
			}
		}
		w.WriteHeader(http.StatusOK)
	})
	return &http.Server{Addr: address, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
}
