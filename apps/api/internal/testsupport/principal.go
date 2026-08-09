package testsupport

import (
	"fmt"
	"testing"

	"ideaboard/api/internal/identity/principal"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func SetPrincipal(t testing.TB, c *gin.Context, userID int64, role principal.Role) principal.Principal {
	t.Helper()
	identityPrincipal, err := principal.New(
		userID,
		uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("ideaboard-test-user-%d", userID))),
		fmt.Sprintf("auth-%d", userID),
		fmt.Sprintf("test-user-%d", userID),
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
