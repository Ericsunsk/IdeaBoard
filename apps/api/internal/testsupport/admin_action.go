package testsupport

import (
	"testing"

	"ideaboard/api/internal/platform/adminaction"
)

func AdminAction(t testing.TB, actorID int64) adminaction.Metadata {
	t.Helper()
	action, err := adminaction.New(actorID)
	if err != nil {
		t.Fatalf("adminaction.New() error = %v", err)
	}
	return action
}
