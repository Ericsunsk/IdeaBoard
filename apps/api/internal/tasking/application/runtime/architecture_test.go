package runtime

import (
	"ideaboard/api/internal/archtest"
	"testing"
)

// The runtime state machine must depend on narrow consumer interfaces, never
// on the concrete taskstore.Repository or the Runtime worker implementation.
func TestTaskruntimeDoesNotDependOnConcreteRepositoryOrWorker(t *testing.T) {
	archtest.AssertNoForbiddenImports(t, ".", []string{"ideaboard/api/internal/runtime/worker"})
	archtest.AssertNoForbiddenText(t, ".", []string{"taskstore.Repository"})
}
