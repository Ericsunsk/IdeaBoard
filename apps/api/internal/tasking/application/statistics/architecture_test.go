package statistics

import (
	"testing"

	"ideaboard/api/internal/archtest"
)

// The statistics read model must depend on narrow consumer interfaces, never
// on the concrete taskstore.Repository or the Runtime worker implementation.
// It must not write Task lifecycle facts either; lifecycle writes stay behind
// the taskstore seam owned by submission/runtime flows.
var taskLifecycleWrites = []string{
	"ConvergeTask",
	"CreateWithPGXTx",
	"MarkTaskCancelled",
	"FinishTaskAttempt",
	"CreateTaskAttempt",
	"ClaimTaskInputMediaRef",
	"INSERT INTO tasks",
	"UPDATE tasks",
	"DELETE FROM tasks",
}

func TestTaskstatsDoesNotDependOnConcreteRepositoryOrWorker(t *testing.T) {
	archtest.AssertNoForbiddenImports(t, ".", []string{"ideaboard/api/internal/runtime/worker"})
	archtest.AssertNoForbiddenText(t, ".", append([]string{"taskstore.Repository"}, taskLifecycleWrites...))
}
