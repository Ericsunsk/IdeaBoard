package execution

import (
	"ideaboard/api/internal/archtest"
	"testing"
)

// The execution state machine must depend on narrow consumer interfaces, never
// on the concrete taskstore.Repository or the Runtime worker implementation.
func TestTaskExecutionDoesNotDependOnConcreteRepositoryOrWorker(t *testing.T) {
	archtest.AssertNoForbiddenImports(t, ".", []string{"ideaboard/api/internal/runtime"})
	archtest.AssertNoForbiddenText(t, ".", []string{"taskstore.Repository"})
}
