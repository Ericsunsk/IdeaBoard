package memberquotas

import (
	"context"
	"errors"
	"testing"
	"time"

	"ideaboard/api/internal/tenancy"
)

type quotaReadStore struct {
	ReadStore
	batchCalls  int
	scalarCalls int
	failure     error
}

func (s *quotaReadStore) UsageForPeriod(context.Context, int64, int64, *time.Time, *time.Time) (int64, error) {
	s.scalarCalls++
	return 0, errors.New("batch page unexpectedly used scalar usage")
}
func (s *quotaReadStore) UsageForPeriodBatch(_ context.Context, _ int64, ids []int64, start, end *time.Time) (map[int64]int64, error) {
	s.batchCalls++
	if s.failure != nil {
		return nil, s.failure
	}
	used := map[int64]int64{}
	for _, id := range ids {
		used[id] = id * 10
	}
	return used, nil
}

type quotaReadRoles struct {
	calls   int
	failure error
}

func (r *quotaReadRoles) GetOrganizationRole(context.Context, string, int64) (string, bool, error) {
	return "", false, errors.New("expected batched roles")
}
func (r *quotaReadRoles) GetOrganizationRoles(context.Context, string, []int64) (map[int64]string, error) {
	r.calls++
	return map[int64]string{1: tenancy.RoleOwner}, r.failure
}

func TestReaderPageNormalizesBeforeGroupingUsageWindows(t *testing.T) {
	store, roles := &quotaReadStore{}, &quotaReadRoles{}
	reader := Reader{Store: store, Policy: Policy{OrganizationRoles: roles}}
	limit := int64(100)
	rows := []QuotaRow{
		{UserID: 1, Period: PeriodDaily, LimitCredits: &limit}, // Owner becomes unbounded.
		{UserID: 2, Period: PeriodDaily, LimitCredits: &limit},
		{UserID: 3, Period: PeriodDaily, LimitCredits: &limit},
		{UserID: 4, Period: PeriodFixed, LimitCredits: &limit},
		{UserID: 5, Period: PeriodUnlimited},
	}
	readings, err := reader.Page(t.Context(), Scope{Kind: ScopeOrganization, OrganizationID: "org_test"}, 77, rows)
	if err != nil {
		t.Fatal(err)
	}
	if store.batchCalls != 2 || store.scalarCalls != 0 || roles.calls != 1 {
		t.Fatalf("read fan-out: batch=%d scalar=%d roles=%d", store.batchCalls, store.scalarCalls, roles.calls)
	}
	if len(readings) != 5 {
		t.Fatalf("readings = %+v", readings)
	}
	if rows[0].Period != PeriodDaily || rows[0].LimitCredits != &limit {
		t.Fatalf("normalization mutated persisted rows: %+v", rows)
	}
	if readings[0].Period != PeriodUnlimited || readings[0].Limit != nil || readings[0].Start != nil {
		t.Fatalf("Owner retained a stale cap: %+v", readings[0])
	}
	if readings[1].Start == nil || readings[1].End == nil || !readings[1].Start.Equal(*readings[2].Start) || !readings[1].End.Equal(*readings[2].End) {
		t.Fatalf("daily readings have different windows: %+v", readings)
	}
	for i, reading := range readings {
		if reading.UserID != int64(i+1) || reading.Used != int64(i+1)*10 {
			t.Fatalf("input order/usage attribution = %+v", readings)
		}
		if i == 3 || i == 4 {
			if reading.Start != nil || reading.End != nil {
				t.Fatalf("lifetime usage was bounded: %+v", reading)
			}
		}
	}
}

func TestReaderSelfPersonalFailsClosed(t *testing.T) {
	// Personal scopes never reach Self in production (callers return the
	// unlimited constant directly); a Personal scope here fails closed.
	reader := Reader{Store: &quotaReadStore{}, Policy: Policy{}}
	_, err := reader.Self(t.Context(), Scope{Kind: ScopePersonal}, 77, 11)
	if !errors.Is(err, ErrPersonalQuotaUnsupported) {
		t.Fatalf("personal self error = %v, want ErrPersonalQuotaUnsupported", err)
	}
}

func TestReaderPagePersonalFailsClosed(t *testing.T) {
	reader := Reader{Store: &quotaReadStore{}, Policy: Policy{}}
	_, err := reader.Page(t.Context(), Scope{Kind: ScopePersonal}, 77, []QuotaRow{{UserID: 1, Period: PeriodUnlimited}})
	if !errors.Is(err, ErrPersonalQuotaUnsupported) {
		t.Fatalf("personal page error = %v, want ErrPersonalQuotaUnsupported", err)
	}
}

func TestReaderPageFailsClosedOnRoleAndUsageErrors(t *testing.T) {
	cause := errors.New("read unavailable")
	for _, owner := range []string{"roles", "usage"} {
		t.Run(owner, func(t *testing.T) {
			store, roles := &quotaReadStore{}, &quotaReadRoles{}
			if owner == "roles" {
				roles.failure = cause
			} else {
				store.failure = cause
			}
			_, err := (Reader{Store: store, Policy: Policy{OrganizationRoles: roles}}).Page(t.Context(), Scope{Kind: ScopeOrganization, OrganizationID: "org_test"}, 77, []QuotaRow{{UserID: 1, Period: PeriodUnlimited}})
			if !errors.Is(err, cause) {
				t.Fatalf("read error = %v", err)
			}
			if owner == "roles" && store.batchCalls != 0 {
				t.Fatal("usage started after role lookup failed")
			}
		})
	}
}
