package memberquotas

import (
	"context"
	"errors"
	"testing"

	"ideaboard/api/internal/tenancy"
)

type policyRoleReader struct{ roles map[int64]string }

func (r policyRoleReader) GetOrganizationRole(_ context.Context, _ string, userID int64) (string, bool, error) {
	role, ok := r.roles[userID]
	return role, ok, nil
}

func (r policyRoleReader) GetOrganizationRoles(_ context.Context, _ string, userIDs []int64) (map[int64]string, error) {
	roles := make(map[int64]string, len(userIDs))
	for _, id := range userIDs {
		if role, ok := r.roles[id]; ok {
			roles[id] = role
		}
	}
	return roles, nil
}

func TestPolicyCanManageScopes(t *testing.T) {
	org := Scope{Kind: ScopeOrganization, OrganizationID: "org_1"}
	p := Policy{OrganizationRoles: policyRoleReader{roles: map[int64]string{
		1: tenancy.RoleOwner, 2: tenancy.RoleAdmin, 3: tenancy.RoleMember,
	}}}
	for _, tc := range []struct {
		name    string
		scope   Scope
		actor   int64
		want    bool
		wantErr bool
	}{
		{"organization owner", org, 1, true, false},
		{"organization admin", org, 2, true, false},
		{"organization member", org, 3, false, false},
		{"outsider", org, 9, false, false},
		{"personal scope fails closed", Scope{Kind: ScopePersonal}, 3, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := p.CanManage(context.Background(), tc.scope, tc.actor)
			if tc.wantErr {
				if !errors.Is(err, ErrPersonalQuotaUnsupported) && err == nil {
					t.Fatalf("CanManage() = %v, %v, want organization-only error", got, err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("CanManage() = %v, %v, want %v", got, err, tc.want)
			}
		})
	}
}

func TestPolicyCanSetOrganizationTargets(t *testing.T) {
	p := Policy{OrganizationRoles: policyRoleReader{roles: map[int64]string{
		1: tenancy.RoleOwner, 2: tenancy.RoleAdmin, 3: tenancy.RoleMember,
	}}}
	scope := Scope{Kind: ScopeOrganization, OrganizationID: "org_1"}
	for _, tc := range []struct {
		name          string
		actor, target int64
		period        string
		wantAllowed   bool
	}{
		{"owner sets admin", 1, 2, PeriodDaily, true},
		{"admin sets member", 2, 3, PeriodDaily, true},
		{"admin cannot edit admin", 2, 2, PeriodDaily, false},
		{"admin cannot clear admin", 2, 2, PeriodUnlimited, false},
		{"owner cannot cap owner", 1, 1, PeriodDaily, false},
		{"owner clears owner", 1, 1, PeriodUnlimited, true},
		{"admin cannot cap owner", 2, 1, PeriodDaily, false},
		{"admin cannot clear owner", 2, 1, PeriodUnlimited, false},
		{"member cannot set member", 3, 3, PeriodDaily, false},
		{"member cannot clear owner", 3, 1, PeriodUnlimited, false},
		{"outsider cannot set member", 4, 3, PeriodDaily, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := p.CanSet(context.Background(), scope, tc.actor, tc.target, tc.period)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.wantAllowed {
				t.Fatalf("CanSet() = %v, want %v", got, tc.wantAllowed)
			}
		})
	}
}

func TestPolicyCanSetPersonalFailsClosed(t *testing.T) {
	p := Policy{OrganizationRoles: policyRoleReader{roles: map[int64]string{1: tenancy.RoleOwner}}}
	if _, err := p.CanSet(context.Background(), Scope{Kind: ScopePersonal}, 1, 2, PeriodDaily); !errors.Is(err, ErrPersonalQuotaUnsupported) {
		t.Fatalf("personal CanSet() error = %v, want ErrPersonalQuotaUnsupported", err)
	}
}

func TestPolicyNormalizeRead(t *testing.T) {
	p := Policy{OrganizationRoles: policyRoleReader{roles: map[int64]string{1: tenancy.RoleOwner, 2: tenancy.RoleMember}}}
	org := Scope{Kind: ScopeOrganization, OrganizationID: "org_1"}
	limit := int64(100)
	for _, tc := range []struct {
		name       string
		scope      Scope
		userID     int64
		period     string
		limit      *int64
		wantPeriod string
		wantLimit  *int64
	}{
		{"Organization Owner", org, 1, PeriodDaily, &limit, PeriodUnlimited, nil},
		{"Organization Member", org, 2, PeriodDaily, &limit, PeriodDaily, &limit},
		{"missing member", org, 3, PeriodDaily, &limit, PeriodDaily, &limit},
		{"absent quota", org, 2, "", nil, PeriodUnlimited, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			period, limit, err := p.NormalizeRead(context.Background(), tc.scope, tc.userID, tc.period, tc.limit)
			if err != nil || period != tc.wantPeriod || limit != tc.wantLimit {
				t.Fatalf("NormalizeRead() = %q, %v, %v; want %q, %v", period, limit, err, tc.wantPeriod, tc.wantLimit)
			}
		})
	}
}

type failingPolicyRoleReader struct {
	policyRoleReader
	failedUserID int64
	err          error
}

func (r failingPolicyRoleReader) GetOrganizationRole(ctx context.Context, organizationID string, userID int64) (string, bool, error) {
	if userID == r.failedUserID {
		return "", false, r.err
	}
	return r.policyRoleReader.GetOrganizationRole(ctx, organizationID, userID)
}

func TestPolicyCanSetPropagatesRoleLookupFailures(t *testing.T) {
	lookupErr := errors.New("role lookup failed")
	scope := Scope{Kind: ScopeOrganization, OrganizationID: "org_1"}
	for _, failedUserID := range []int64{1, 2} {
		p := Policy{OrganizationRoles: failingPolicyRoleReader{
			policyRoleReader: policyRoleReader{roles: map[int64]string{1: tenancy.RoleOwner, 2: tenancy.RoleAdmin}},
			failedUserID:     failedUserID, err: lookupErr,
		}}
		decision, err := p.CanSet(context.Background(), scope, 1, 2, PeriodDaily)
		if decision || !errors.Is(err, lookupErr) {
			t.Fatalf("failed lookup for %d: CanSet() = %+v, %v, want denied and lookup error", failedUserID, decision, err)
		}
	}
	decision, err := (Policy{}).CanSet(context.Background(), scope, 1, 2, PeriodDaily)
	if decision || err == nil {
		t.Fatalf("missing reader: CanSet() = %+v, %v, want denied and error", decision, err)
	}
}

func TestPolicyReadAndManagementPropagateRoleLookupFailures(t *testing.T) {
	lookupErr := errors.New("role lookup failed")
	p := Policy{OrganizationRoles: failingPolicyRoleReader{failedUserID: 1, err: lookupErr}}
	scope := Scope{Kind: ScopeOrganization, OrganizationID: "org_1"}
	allowed, err := p.CanManage(context.Background(), scope, 1)
	if allowed || !errors.Is(err, lookupErr) {
		t.Fatalf("CanManage() = %v, %v, want denied and lookup error", allowed, err)
	}
	_, _, err = p.NormalizeRead(context.Background(), scope, 1, PeriodDaily, nil)
	if !errors.Is(err, lookupErr) {
		t.Fatalf("NormalizeRead() error = %v, want %v", err, lookupErr)
	}
}
