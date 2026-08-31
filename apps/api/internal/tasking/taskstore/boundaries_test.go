package taskstore

import "testing"

// Capability boundaries are intentionally compile-time contracts. Keeping the
// assertions next to the facade makes accidental widening or return-type drift
// fail during package tests instead of at a distant service wiring site.
func TestRepositoryCapabilityBoundaries(t *testing.T) {
	var repository *Repository
	if repository.Submission() == nil || repository.Runtime() == nil || repository.Queries() == nil || repository.Billing() == nil || repository.Trace() == nil || repository.Statistics() == nil {
		t.Fatal("repository capability boundaries must always be available")
	}
	var _ SubmissionRepository = repository.Submission()
	var _ RuntimeRepository = repository.Runtime()
	var _ QueryRepository = repository.Queries()
	var _ BillingRepository = repository.Billing()
	var _ TraceRepository = repository.Trace()
	var _ StatisticsRepository = repository.Statistics()
}
