package testsupport

import (
	"fmt"
	"testing"

	"ideaboard/api/internal/identity/principal"

	"github.com/gin-gonic/gin"
)

func SetPrincipal(t testing.TB, c *gin.Context, userID int64, role principal.Role) principal.Principal {
	t.Helper()
	identityPrincipal, err := principal.New(
		userID,
		fmt.Sprintf("auth-%d", userID),
		fmt.Sprintf("user-%d", userID),
		role.String(),
	)
	if err != nil {
		t.Fatalf("principal.New() error = %v", err)
	}
	if err := principal.Set(c, identityPrincipal); err != nil {
		t.Fatalf("principal.Set() error = %v", err)
	}
	return identityPrincipal
}
