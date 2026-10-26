package billing

import (
	"testing"

	"ideaboard/api/internal/archtest"
)

// Durable usage reads never depend on the concrete taskstore.Repository
// or the Runtime worker implementation.
func TestTaskbillingDoesNotDependOnConcreteRepositoryOrWorker(t *testing.T) {
	archtest.AssertNoForbiddenImports(t, ".", []string{"ideaboard/api/internal/runtime"})
	archtest.AssertNoForbiddenText(t, ".", []string{"taskstore.Repository"})
}

// Durable usage aggregation reads persisted billing facts only; live model
// mappings must never affect historical task usage.
func TestTaskbillingNeverReadsModelMappings(t *testing.T) {
	archtest.AssertNoForbiddenImports(t, ".", []string{"ideaboard/api/internal/aigateway"})
	archtest.AssertNoForbiddenText(t, ".", []string{"ai_gateway_model_", "ResolveModel", "ModelStore"})
}
