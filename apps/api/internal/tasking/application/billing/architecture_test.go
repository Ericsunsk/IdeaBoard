package billing

import (
	"testing"

	"ideaboard/api/internal/archtest"
)

// Billing orchestration depends on narrow consumer interfaces, never on the
// concrete taskstore.Repository or the Runtime worker implementation.
func TestTaskbillingDoesNotDependOnConcreteRepositoryOrWorker(t *testing.T) {
	archtest.AssertNoForbiddenImports(t, ".", []string{"ideaboard/api/internal/runtime/worker"})
	archtest.AssertNoForbiddenText(t, ".", []string{"taskstore.Repository"})
}
