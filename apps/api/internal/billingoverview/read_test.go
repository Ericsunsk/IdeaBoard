package billingoverview

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	creditsdomain "ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/memberquotas"
	"ideaboard/api/internal/tenancy"
	"ideaboard/api/internal/tenancy/funds"
	usageapplication "ideaboard/api/internal/usage/application"
	usagedomain "ideaboard/api/internal/usage/domain"
)

type snapshotMarker struct{}

type accountFixture struct {
	check     func(context.Context, bool)
	role      string
	roleCalls int
}

func (a *accountFixture) ResolveFunds(ctx context.Context, workspacePublicID string) (funds.Identity, error) {
	a.check(ctx, false)
	return funds.Identity{
		WorkspaceInternalID:    1,
		WorkspacePublicID:      workspacePublicID,
		BillingAccountID:       77,
		BillingAccountPublicID: "ba_test",
		OwnerType:              funds.OwnerTypeWorkspace,
	}, nil
}
func (a *accountFixture) GetBillingAccountRole(ctx context.Context, _, _ int64) (string, bool, error) {
	a.check(ctx, false)
	a.roleCalls++
	return a.role, a.role != "", nil
}
func (a *accountFixture) ListBillingAccountMembers(ctx context.Context, _, _ int64) ([]tenancy.BillingAccountMember, error) {
	a.check(ctx, true)
	return []tenancy.BillingAccountMember{{UserID: 11, Role: tenancy.BillingRoleOwner}}, nil
}
func (a *accountFixture) ListBillingTransferInvitesForAccount(ctx context.Context, _, _, _ int64, _ int) ([]*tenancy.BillingTransferInvite, error) {
	a.check(ctx, true)
	return []*tenancy.BillingTransferInvite{nil, {FromUserID: 11, ToUserID: 12, Status: tenancy.BillingTransferPending}, {FromUserID: 11, ToUserID: 13, Status: tenancy.BillingTransferAccepted}}, nil
}

type tenancyFixture struct{ check func(context.Context, bool) }

func (s tenancyFixture) ListWorkspaceMembers(ctx context.Context, _ string) ([]*tenancy.WorkspaceMember, error) {
	s.check(ctx, true)
	return []*tenancy.WorkspaceMember{{UserID: 11}, {UserID: 12}}, nil
}
func (s tenancyFixture) GetOrganizationRole(context.Context, string, int64) (string, bool, error) {
	return "", false, nil
}

func (s tenancyFixture) GetOrganizationRoles(context.Context, string, []int64) (map[int64]string, error) {
	return nil, nil
}

type quotaFixture struct {
	check                func(context.Context, bool)
	listCalls, selfCalls int
}

func (q *quotaFixture) GetQuotaRow(ctx context.Context, _, _ int64) (string, *int64, bool, error) {
	q.check(ctx, true)
	q.selfCalls++
	return "", nil, false, nil
}
func (q *quotaFixture) ListQuotas(ctx context.Context, _ int64) ([]memberquotas.QuotaRow, error) {
	q.check(ctx, true)
	q.listCalls++
	return []memberquotas.QuotaRow{{UserID: 12, Period: memberquotas.PeriodUnlimited}}, nil
}
func (q *quotaFixture) UsageForPeriod(ctx context.Context, _, userID int64, _, _ *time.Time) (int64, error) {
	q.check(ctx, true)
	return userID, nil
}

func (q *quotaFixture) UsageForPeriodBatch(ctx context.Context, _ int64, userIDs []int64, _, _ *time.Time) (map[int64]int64, error) {
	q.check(ctx, true)
	used := make(map[int64]int64, len(userIDs))
	for _, id := range userIDs {
		used[id] = id
	}
	return used, nil
}

type walletFunc func(context.Context, int64) (creditsdomain.WalletDetails, error)

func (f walletFunc) GetWallet(ctx context.Context, id int64) (creditsdomain.WalletDetails, error) {
	return f(ctx, id)
}

type usageFunc func(context.Context, int64, creditsdomain.UsageFilter, usagedomain.Range) (usageapplication.Usage, error)

func (f usageFunc) GetCurrent(ctx context.Context, id int64, filter creditsdomain.UsageFilter, r usagedomain.Range) (usageapplication.Usage, error) {
	return f(ctx, id, filter, r)
}

func overviewFixture(t *testing.T) (Reader, Input, *accountFixture, *quotaFixture) {
	t.Helper()
	check := func(ctx context.Context, snapshot bool) {
		t.Helper()
		if got := ctx.Value(snapshotMarker{}) == true; got != snapshot {
			t.Errorf("snapshot carrier = %v, want %v", got, snapshot)
		}
	}
	accounts := &accountFixture{check: check, role: tenancy.BillingRoleOwner}
	quotas := &quotaFixture{check: check}
	reader := Reader{
		Accounts: accounts, Tenancy: tenancyFixture{check: check}, Quotas: quotas,
		Snapshot: func(ctx context.Context, read func(context.Context) error) error {
			return read(context.WithValue(ctx, snapshotMarker{}, true))
		},
		People: func(ctx context.Context, ids []int64) (map[int64]Person, error) {
			check(ctx, true)
			people := map[int64]Person{}
			for _, id := range ids {
				people[id] = Person{PublicID: fmt.Sprintf("usr_%d", id)}
			}
			return people, nil
		},
		Wallets: walletFunc(func(ctx context.Context, id int64) (creditsdomain.WalletDetails, error) {
			check(ctx, false)
			return creditsdomain.WalletDetails{}, nil
		}),
		Usage: usageFunc(func(ctx context.Context, id int64, filter creditsdomain.UsageFilter, r usagedomain.Range) (usageapplication.Usage, error) {
			check(ctx, false)
			return usageapplication.Usage{}, nil
		}),
	}
	input := Input{Workspace: &tenancy.Workspace{InternalID: 1, PublicID: "ws_test"}, WorkspaceRole: tenancy.RoleOwner, ActorUserID: 11}
	return reader, input, accounts, quotas
}

func TestReaderOwnsSnapshotAndVisibility(t *testing.T) {
	for _, manager := range []bool{true, false} {
		t.Run(fmt.Sprint(manager), func(t *testing.T) {
			reader, input, accounts, quotas := overviewFixture(t)
			if !manager {
				accounts.role = ""
				input.WorkspaceRole = tenancy.RoleAdmin
			}
			result, err := reader.Read(t.Context(), input)
			if err != nil {
				t.Fatal(err)
			}
			if accounts.roleCalls != 1 {
				t.Fatalf("billing role reads = %d, want 1", accounts.roleCalls)
			}
			if result.MembersCount != 2 || len(result.LiveInvites) != 1 {
				t.Fatalf("roster/invites = %+v", result)
			}
			if result.CanManageQuotas != manager {
				t.Fatalf("quota capability = %v, want %v", result.CanManageQuotas, manager)
			}
			if result.Quota.UserID != 11 || result.Quota.Period != memberquotas.PeriodUnlimited || result.Quota.Used != 11 {
				t.Fatalf("self quota = %+v", result.Quota)
			}
			if manager {
				if result.Scope != "account" || len(result.BillingMembers) != 1 || len(result.Quotas) != 1 || quotas.selfCalls != 0 {
					t.Fatalf("manager projection = %+v", result)
				}
				if result.People[12].PublicID != "usr_12" {
					t.Fatalf("people = %+v", result.People)
				}
			} else if result.Scope != "workspace" || len(result.BillingMembers) != 0 || len(result.Quotas) != 0 || quotas.listCalls != 0 || quotas.selfCalls != 1 {
				t.Fatalf("member projection leaked manager facts: %+v", result)
			}
		})
	}
}

// The lifetime guarantee belongs to Reader.Read, so this test deliberately
// exercises no Gin handler. Every early snapshot exit must cancel and join.
func TestReaderSnapshotFailuresCancelAndJoinOwnerReads(t *testing.T) {
	for _, failure := range []string{"open", "projection", "finalize"} {
		t.Run(failure, func(t *testing.T) {
			reader, input, _, _ := overviewFixture(t)
			finished := make(chan string, 2)
			reader.Wallets = walletFunc(func(ctx context.Context, _ int64) (creditsdomain.WalletDetails, error) {
				<-ctx.Done()
				finished <- "wallet"
				return creditsdomain.WalletDetails{}, ctx.Err()
			})
			reader.Usage = usageFunc(func(ctx context.Context, _ int64, _ creditsdomain.UsageFilter, _ usagedomain.Range) (usageapplication.Usage, error) {
				<-ctx.Done()
				finished <- "usage"
				return usageapplication.Usage{}, ctx.Err()
			})
			cause := errors.New("snapshot failure")
			snapshot := reader.Snapshot
			if failure == "projection" {
				reader.People = func(context.Context, []int64) (map[int64]Person, error) { return nil, cause }
			}
			reader.Snapshot = func(ctx context.Context, read func(context.Context) error) error {
				if failure == "open" {
					return cause
				}
				if err := snapshot(ctx, read); err != nil {
					return err
				}
				return cause
			}
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			_, err := reader.Read(ctx, input)
			if !errors.Is(err, cause) {
				t.Fatalf("read error = %v", err)
			}
			if ctx.Err() != nil {
				t.Fatal("cleanup waited for parent deadline")
			}
			if len(finished) != 2 {
				t.Fatalf("returned with only %d/2 reads joined", len(finished))
			}
		})
	}
}

func TestReaderErrorPrecedence(t *testing.T) {
	reader, input, _, _ := overviewFixture(t)
	walletErr, usageErr := errors.New("wallet"), errors.New("usage")
	reader.Wallets = walletFunc(func(context.Context, int64) (creditsdomain.WalletDetails, error) {
		return creditsdomain.WalletDetails{}, walletErr
	})
	reader.Usage = usageFunc(func(context.Context, int64, creditsdomain.UsageFilter, usagedomain.Range) (usageapplication.Usage, error) {
		return usageapplication.Usage{}, usageErr
	})
	_, err := reader.Read(t.Context(), input)
	if !errors.Is(err, walletErr) {
		t.Fatalf("wallet must win over usage: %v", err)
	}
	snapshotErr := errors.New("snapshot")
	reader.Snapshot = func(context.Context, func(context.Context) error) error { return snapshotErr }
	_, err = reader.Read(t.Context(), input)
	if !errors.Is(err, snapshotErr) {
		t.Fatalf("snapshot must win over wallet: %v", err)
	}
}

func TestReaderRequiresExplicitSnapshot(t *testing.T) {
	reader, input, accounts, _ := overviewFixture(t)
	reader.Snapshot = nil
	_, err := reader.Read(t.Context(), input)
	var readErr *Error
	if !errors.As(err, &readErr) || readErr.Stage != SnapshotRead {
		t.Fatalf("missing snapshot must fail closed: %v", err)
	}
	if accounts.roleCalls != 0 {
		t.Fatal("account reads started without a snapshot adapter")
	}
}
