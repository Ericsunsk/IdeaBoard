package tasking

import (
	"testing"

	"ideaboard/api/internal/archtest"
)

func TestApplicationDoesNotImportConcreteInfrastructure(t *testing.T) {
	archtest.AssertNoForbiddenImports(t, "application", []string{
		"ideaboard/api/internal/tasking/infrastructure",
		// #860: application services stay port-driven. The concrete
		// Billing catalog, Credits service, River client, and Telemetry
		// bridges live in infrastructure/adapters (and the River worker
		// adapter); application code must not import them.
		"ideaboard/api/internal/billing/application",
		"github.com/riverqueue/river",
		"ideaboard/api/internal/platform/telemetry",
	})
}

// TestApplicationDoesNotImportConcreteCreditsService locks the credits
// boundary: application code may use credits domain/ports, but the ledger
// writer (credits.Service, package credits root) is only composed in
// infrastructure/adapters. Prefix matching cannot express "root only", so
// this asserts on the exact quoted import path.
func TestApplicationDoesNotImportConcreteCreditsService(t *testing.T) {
	archtest.AssertNoForbiddenText(t, "application", []string{
		`"ideaboard/api/internal/credits"`,
	})
}

func TestDomainDoesNotImportTaskingAdapters(t *testing.T) {
	archtest.AssertNoForbiddenImports(t, "domain", []string{
		"ideaboard/api/internal/tasking/application",
		"ideaboard/api/internal/tasking/infrastructure",
		"ideaboard/api/internal/tasking/interfaces",
		"ideaboard/api/internal/tasking/ports",
	})
}

// TestTaskingSubpackagesDoNotDependOnConcreteRepositoryOrWorker consolidates
// the four former per-package ratchets (application/task, ports/media,
// application/submission, application/billing): the seams stay small and
// depend on narrow consumer interfaces, never on the concrete
// taskstore.Repository or the Runtime worker implementation.
func TestTaskingSubpackagesDoNotDependOnConcreteRepositoryOrWorker(t *testing.T) {
	for _, dir := range []string{
		"application/task",
		"ports/media",
		"application/submission",
		"application/billing",
	} {
		archtest.AssertNoForbiddenImports(t, dir, []string{"ideaboard/api/internal/runtime"})
		archtest.AssertNoForbiddenText(t, dir, []string{"taskstore.Repository"})
	}
}

// TestTaskBillingNeverReadsModelMappings preserves the durable-usage rule
// from application/billing: historical task usage reads persisted billing
// facts only; live model mappings must never affect it.
func TestTaskBillingNeverReadsModelMappings(t *testing.T) {
	archtest.AssertNoForbiddenImports(t, "application/billing", []string{"ideaboard/api/internal/modelregistry"})
	archtest.AssertNoForbiddenText(t, "application/billing", []string{"ai_gateway_model_", "ResolveModel", "ModelStore"})
}
