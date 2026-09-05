package media

import (
	"ideaboard/api/internal/archtest"
	"testing"
)

// The media seam stays small and depends on narrow interfaces, never on the
// concrete taskstore.Repository or the Runtime worker implementation.
func TestTaskmediaDoesNotDependOnConcreteRepositoryOrWorker(t *testing.T) {
	archtest.AssertNoForbiddenImports(t, ".", []string{"ideaboard/api/internal/runtime"})
	archtest.AssertNoForbiddenText(t, ".", []string{"taskstore.Repository"})
}
