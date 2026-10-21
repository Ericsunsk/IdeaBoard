// Account default new-member quota focused tests (#947).
//
// The default seeds the per-member row on first join of the account scope
// (same five periods as #946); absent means NOT CONFIGURED (fail closed,
// never implicit unlimited). Changing it never touches existing rows;
// existing rows and history survive removal/rejoin.
//
// PostgreSQL integration tests SKIP without API_TEST_DATABASE_DSN (house
// convention).

package memberquotas

import (
	"context"
	"errors"
	"testing"
)

func TestDefaultPolicySetGetAndValidation(t *testing.T) {
	env := openQuotaEnv(t)
	ctx := context.Background()

	if _, _, ok, err := env.store.GetDefaultPolicy(ctx, env.account); err != nil || ok {
		t.Fatalf("GetDefaultPolicy absent = %v %v, want not-ok", ok, err)
	}

	monthly := int64(1000)
	saved, err := env.store.SetDefaultPolicy(ctx, env.account, PeriodMonthly, &monthly, env.userA)
	if err != nil || saved.Period != PeriodMonthly || saved.LimitCredits == nil || *saved.LimitCredits != 1000 {
		t.Fatalf("SetDefaultPolicy = %+v %v, want monthly 1000", saved, err)
	}
	period, limit, ok, err := env.store.GetDefaultPolicy(ctx, env.account)
	if err != nil || !ok || period != PeriodMonthly || limit == nil || *limit != 1000 {
		t.Fatalf("GetDefaultPolicy = %q %v %v %v, want monthly 1000", period, limit, ok, err)
	}

	for _, tc := range []struct {
		name   string
		period string
		limit  *int64
	}{
		{"monthly without limit", PeriodMonthly, nil},
		{"daily zero", PeriodDaily, ptrQuotaInt64(0)},
		{"unlimited with limit", PeriodUnlimited, ptrQuotaInt64(100)},
		{"unknown period", "yearly", ptrQuotaInt64(100)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := env.store.SetDefaultPolicy(ctx, env.account, tc.period, tc.limit, env.userA); !errors.Is(err, ErrQuotaInvalid) {
				t.Fatalf("SetDefaultPolicy %s = %v, want ErrQuotaInvalid", tc.name, err)
			}
		})
	}
}

func ptrQuotaInt64(v int64) *int64 { return &v }
