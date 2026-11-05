package adapters

import (
	"context"
	"errors"
	"testing"

	taskdomain "ideaboard/api/internal/tasking/domain"
	"ideaboard/api/internal/tenancy"
)

type readinessWorkspaceStore struct {
	workspace             *tenancy.Workspace
	role                  string
	member                bool
	workspaceErr, roleErr error
}

func (s readinessWorkspaceStore) GetWorkspace(context.Context, string) (*tenancy.Workspace, error) {
	return s.workspace, s.workspaceErr
}
func (s readinessWorkspaceStore) GetWorkspaceRole(context.Context, string, int64) (string, bool, error) {
	return s.role, s.member, s.roleErr
}

func TestReadinessWorkspaceProjection(t *testing.T) {
	orgID := "org_readiness01"
	for _, tc := range []struct {
		name  string
		store readinessWorkspaceStore
		org   bool
		want  error
	}{
		{name: "Personal member", store: readinessWorkspaceStore{workspace: &tenancy.Workspace{}, role: "member", member: true}},
		{name: "Organization member", store: readinessWorkspaceStore{workspace: &tenancy.Workspace{OrganizationID: &orgID}, role: "member", member: true}, org: true},
		{name: "missing", store: readinessWorkspaceStore{}, want: taskdomain.ErrNotFound},
		{name: "workspace error hidden", store: readinessWorkspaceStore{workspaceErr: errors.New("read failed")}, want: taskdomain.ErrNotFound},
		{name: "nonmember", store: readinessWorkspaceStore{workspace: &tenancy.Workspace{}}, want: taskdomain.ErrNotFound},
		{name: "role error hidden", store: readinessWorkspaceStore{workspace: &tenancy.Workspace{}, roleErr: errors.New("role failed")}, want: taskdomain.ErrNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := &ReadinessWorkspaceReader{Store: tc.store}
			facts, err := reader.ResolveReadinessWorkspace(context.Background(), "ws_readiness01", 3)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Resolve=%v want %v", err, tc.want)
			}
			if err == nil && (facts.Organization != tc.org || facts.Role != "member") {
				t.Fatalf("facts=%+v", facts)
			}
		})
	}
}
