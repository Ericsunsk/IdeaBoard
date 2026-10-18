package postgres

import (
	"context"
	"ideaboard/api/internal/tenancy"
)

type testMemberGrants struct{}

func (testMemberGrants) RemoveWorkspaceMemberGrants(context.Context, string, int64) ([]tenancy.MemberGrantCleanup, error) {
	return nil, nil
}
