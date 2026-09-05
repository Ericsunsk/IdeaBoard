package submission

import (
	"ideaboard/api/internal/archtest"
	"testing"
)

// Submission owns the atomic task + River enqueue transaction. It depends on
// narrow consumer interfaces, never on the concrete taskstore.Repository or
// the Runtime worker implementation.
func TestTasksubmissionDoesNotDependOnConcreteRepositoryOrWorker(t *testing.T) {
	archtest.AssertNoForbiddenImports(t, ".", []string{"ideaboard/api/internal/runtime"})
	archtest.AssertNoForbiddenText(t, ".", []string{"taskstore.Repository"})
}
