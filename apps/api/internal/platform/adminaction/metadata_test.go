package adminaction

import (
	"errors"
	"testing"
)

func TestNew(t *testing.T) {
	t.Parallel()
	for _, actorID := range []int64{-1, 0, 42} {
		metadata, err := New(actorID)
		if actorID <= 0 {
			if !errors.Is(err, ErrInvalidActor) || metadata.IsValid() {
				t.Fatalf("New(%d) = %+v, %v; want invalid actor", actorID, metadata, err)
			}
			continue
		}
		if err != nil || !metadata.IsValid() || metadata.ActorID() != actorID {
			t.Fatalf("New(%d) = %+v, %v", actorID, metadata, err)
		}
	}
}
