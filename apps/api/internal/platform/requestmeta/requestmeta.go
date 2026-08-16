package requestmeta

import (
	"context"
	"net"
	"net/http"
	"strings"
)

type contextKey string

const clientIPKey contextKey = "client-ip"

func ClientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); net.ParseIP(forwarded) != nil {
		return forwarded
	}
	if real := strings.TrimSpace(r.Header.Get("X-Real-IP")); net.ParseIP(real) != nil {
		return real
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && net.ParseIP(host) != nil {
		return host
	}
	if net.ParseIP(strings.TrimSpace(r.RemoteAddr)) != nil {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return ""
}

func WithClientIP(ctx context.Context, ip string) context.Context {
	ip = strings.TrimSpace(ip)
	if net.ParseIP(ip) == nil {
		return ctx
	}
	return context.WithValue(ctx, clientIPKey, ip)
}

func FromContext(ctx context.Context) string {
	value, _ := ctx.Value(clientIPKey).(string)
	return value
}

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(WithClientIP(r.Context(), ClientIP(r))))
	})
}
