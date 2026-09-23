package billing

import (
	"testing"

	"ideaboard/api/internal/archtest"
)

// Billing orchestration depends on narrow consumer interfaces, never on the
// concrete taskstore.Repository or the Runtime worker implementation.
func TestTaskbillingDoesNotDependOnConcreteRepositoryOrWorker(t *testing.T) {
	archtest.AssertNoForbiddenImports(t, ".", []string{"ideaboard/api/internal/runtime"})
	archtest.AssertNoForbiddenText(t, ".", []string{"taskstore.Repository"})
}

// Model selection lives in the AI gateway settings namespace; quoting reads
// only SKU catalog facts. This pins the T07 split at compile time: no
// billing file may reach the mapping, so changing models cannot move prices.
func TestTaskbillingNeverReadsModelMappings(t *testing.T) {
	archtest.AssertNoForbiddenImports(t, ".", []string{"ideaboard/api/internal/aigateway"})
	archtest.AssertNoForbiddenText(t, ".", []string{"ai_gateway_model_", "ResolveModel", "ModelStore"})
}
