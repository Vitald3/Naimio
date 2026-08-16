package auth

import "context"

type actorKey struct{}

type adminSessionKey struct{}

// WithActorID is the trusted boundary used by authentication middleware after
// it has validated a server-side session. Handlers must never derive this value
// from request payloads or untrusted identity headers.
func WithActorID(ctx context.Context, actorID string) context.Context {
	return context.WithValue(ctx, actorKey{}, actorID)
}

func ActorID(ctx context.Context) (string, bool) {
	actorID, ok := ctx.Value(actorKey{}).(string)
	return actorID, ok && actorID != ""
}

func WithAdminSession(ctx context.Context, isAdmin bool) context.Context {
	return context.WithValue(ctx, adminSessionKey{}, isAdmin)
}

func IsAdminSession(ctx context.Context) bool {
	v, ok := ctx.Value(adminSessionKey{}).(bool)
	return ok && v
}
