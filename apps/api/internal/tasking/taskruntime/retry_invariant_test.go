package taskruntime

import (
	"testing"

	"ideaboard/api/internal/tasking/taskqueue"
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
