package task

import (
	"ideaboard/api/internal/archtest"
	"testing"
)

// The application facade must depend on narrow consumer interfaces, never on
// the concrete taskstore.Repository or the Runtime worker implementation.
func TestTaskserviceDoesNotDependOnConcreteRepositoryOrWorker(t *testing.T) {
	archtest.AssertNoForbiddenImports(t, ".", []string{"ideaboard/api/internal/runtime/worker"})
	archtest.AssertNoForbiddenText(t, ".", []string{"taskstore.Repository"})
}
