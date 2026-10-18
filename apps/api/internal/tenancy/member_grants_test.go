package tenancy

import (
	"context"
)

type testMemberGrants struct{}

func (testMemberGrants) RemoveWorkspaceMemberGrants(context.Context, string, int64) ([]MemberGrantCleanup, error) {
	return nil, nil
}
