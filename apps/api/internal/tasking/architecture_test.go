package tasking

import (
	"testing"

	"ideaboard/api/internal/archtest"
)

func TestApplicationDoesNotImportConcreteInfrastructure(t *testing.T) {
	archtest.AssertNoForbiddenImports(t, "application", []string{
		"ideaboard/api/internal/tasking/infrastructure",
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
