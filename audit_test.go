package drel

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithActor_SetsActorInContext(t *testing.T) {
	ctx := WithActor(context.Background(), "user-123")
	actor := ActorFromContext(ctx)
	assert.Equal(t, "user-123", actor)
}

func TestActorFromContext_ReturnsEmptyWhenNotSet(t *testing.T) {
	actor := ActorFromContext(context.Background())
	assert.Equal(t, "", actor)
}
