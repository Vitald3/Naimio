package ratelimit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"freelance/apps/api/internal/auth"
)

type Class string

const (
	AuthStrict    Class = "AUTH_STRICT"
	PublicRead    Class = "PUBLIC_READ"
	PrivateRead   Class = "PRIVATE_READ"
	WriteStandard Class = "WRITE_STANDARD"
	Upload        Class = "UPLOAD"
	ChatSend      Class = "CHAT_SEND"
	ProposalSend  Class = "PROPOSAL_SEND"
	PublicAI      Class = "PUBLIC_AI"
	Admin         Class = "ADMIN"
)

var Classes = []Class{AuthStrict, PublicRead, PrivateRead, WriteStandard, Upload, ChatSend, ProposalSend, PublicAI, Admin}

type ClassConfig struct {
	Limit  int
	Window time.Duration
}

type Config map[Class]ClassConfig

func DefaultConfig() Config {
	return Config{
		AuthStrict: {Limit: 10, Window: time.Minute}, PublicRead: {Limit: 600, Window: time.Minute},
		PrivateRead: {Limit: 600, Window: time.Minute}, WriteStandard: {Limit: 60, Window: time.Minute}, Upload: {Limit: 20, Window: time.Minute},
		ChatSend: {Limit: 60, Window: time.Minute}, ProposalSend: {Limit: 10, Window: time.Minute},
		PublicAI: {Limit: 10, Window: time.Minute}, Admin: {Limit: 30, Window: time.Minute},
	}
}

func LoadConfig(getenv func(string) string) (Config, error) {
	config := DefaultConfig()
	for _, class := range Classes {
		prefix := "RATE_LIMIT_" + string(class)
		if raw := getenv(prefix + "_LIMIT"); raw != "" {
			limit, err := strconv.Atoi(raw)
			if err != nil || limit <= 0 {
				return nil, fmt.Errorf("%s_LIMIT must be a positive integer", prefix)
			}
			value := config[class]
			value.Limit = limit
			config[class] = value
		}
		if raw := getenv(prefix + "_WINDOW"); raw != "" {
			window, err := time.ParseDuration(raw)
			if err != nil || window <= 0 {
				return nil, fmt.Errorf("%s_WINDOW must be a positive duration", prefix)
			}
			value := config[class]
			value.Window = window
			config[class] = value
		}
	}
	return config, nil
}

type Decision struct {
	Allowed    bool
	RetryAfter time.Duration
}

type Limiter interface {
	Allow(context.Context, Class, string) (Decision, error)
}

type memoryEntry struct {
	count       int
	windowStart time.Time
}

type MemoryLimiter struct {
	mu      sync.Mutex
	config  Config
	now     func() time.Time
	entries map[string]memoryEntry
}

func NewMemory(config Config, now func() time.Time) *MemoryLimiter {
	if now == nil {
		now = time.Now
	}
	return &MemoryLimiter{config: config, now: now, entries: make(map[string]memoryEntry)}
}

func (l *MemoryLimiter) Allow(_ context.Context, class Class, key string) (Decision, error) {
	classConfig, ok := l.config[class]
	if !ok || classConfig.Limit <= 0 || classConfig.Window <= 0 {
		return Decision{}, errors.New("rate limit class is not configured")
	}
	now := l.now()
	entryKey := string(class) + ":" + key
	l.mu.Lock()
	defer l.mu.Unlock()
	entry := l.entries[entryKey]
	if entry.windowStart.IsZero() || now.Sub(entry.windowStart) >= classConfig.Window {
		entry = memoryEntry{windowStart: now}
	}
	entry.count++
	l.entries[entryKey] = entry
	if entry.count > classConfig.Limit {
		return Decision{RetryAfter: classConfig.Window - now.Sub(entry.windowStart)}, nil
	}
	return Decision{Allowed: true}, nil
}

type Middleware struct {
	Limiter  Limiter
	FailOpen bool
}

func (m Middleware) Limit(class Class, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := requestKey(r)
		decision, err := m.Limiter.Allow(r.Context(), class, key)
		if err != nil {
			if m.FailOpen {
				next.ServeHTTP(w, r)
				return
			}
			writeError(w, r, http.StatusServiceUnavailable, "RATE_LIMIT_UNAVAILABLE", "request limiting is temporarily unavailable")
			return
		}
		if !decision.Allowed {
			retryAfter := int(decision.RetryAfter.Round(time.Second).Seconds())
			if retryAfter < 1 {
				retryAfter = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			writeError(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func requestKey(r *http.Request) string {
	if actorID, ok := auth.ActorID(r.Context()); ok {
		return "user:" + actorID
	}
	// API is only reachable through the internal proxy network in Compose. Nginx
	// overwrites X-Forwarded-For with the peer address, so this restores the real
	// client identity rather than putting all public visitors in one proxy bucket.
	if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); net.ParseIP(forwarded) != nil {
		return "ip:" + forwarded
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return "ip:" + host
	}
	return "ip:" + r.RemoteAddr
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	requestID := w.Header().Get("X-Request-ID")
	if requestID == "" {
		requestID = r.Header.Get("X-Request-ID")
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": message, "request_id": requestID}})
}
