package execution

import (
	"testing"

	taskqueue "ideaboard/api/internal/tasking/infrastructure/queue"
)

func TestRiverDefaultMaxAttemptsCoversBusinessRetryLimit(t *testing.T) {
	if taskqueue.RiverMaxAttemptsDefault < defaultTaskRetryLimit {
		t.Fatalf(
			"River default max attempts=%d must cover business retry limit=%d",
			taskqueue.RiverMaxAttemptsDefault,
			defaultTaskRetryLimit,
		)
	}
}
