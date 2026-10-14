package tests

import (
	"context"
	"github.com/gin-gonic/gin"
	"os"
	"testing"
)

type fakeAdminUserImpersonator struct {
	cookieHeader string
	authUserID   string
	setCookies   []string
	err          error
}

func (f *fakeAdminUserImpersonator) ImpersonateUser(_ context.Context, cookieHeader string, authUserID string) ([]string, error) {
	f.cookieHeader = cookieHeader
	f.authUserID = authUserID
	return f.setCookies, f.err
}

type adminImpersonationStatusError int

func (e adminImpersonationStatusError) Error() string {
	return "impersonation failed"
}

func (e adminImpersonationStatusError) StatusCode() int {
	return int(e)
}

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}
