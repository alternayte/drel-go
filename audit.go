package drel

import "context"

type actorKey struct{}

// WithActor returns a new context carrying the given actor identifier.
func WithActor(ctx context.Context, actor string) context.Context {
	return context.WithValue(ctx, actorKey{}, actor)
}

// ActorFromContext extracts the actor identifier from the context, returning
// an empty string if none is set.
func ActorFromContext(ctx context.Context) string {
	v, _ := ctx.Value(actorKey{}).(string)
	return v
}
