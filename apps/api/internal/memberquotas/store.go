package memberquotas

import (
	"context"
	"errors"
	"strings"
	"time"

	"ideaboard/api/internal/consumptionread"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/database"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store persists member-level consumption quotas and enforces them
// inside caller-owned settlement transactions. Repository methods take
// context.Context first. Reads use the pool; settlement checks join the
// caller's pgx.Tx so Check/Quote/Hold/Task/River stay atomic. The Store
// never writes the Credits Ledger; the ledger Hold that follows a passed
// check is the reservation (no sub-wallet, no separate quota hold row).
//
// Lock order is load-bearing: settlement checks lock the workspace budget
// row first and the member quota row second. Task submit and Vision
// reserve both run the budget check before the quota check in the same
// transaction, so concurrent submits serialize in one order and never
// deadlock each other.
type Store struct {
	db *pgxpool.Pool
	q  *dbsqlc.Queries
}

// NewStore builds a quota Store over pool. Pass nil pool in tests that
// stub the checker seam instead.
func NewStore(db *pgxpool.Pool) *Store {
	if db == nil {
		return &Store{}
	}
	return &Store{db: db, q: dbsqlc.New(db)}
}

// QuotaRow is one persisted (Billing Account, User) quota cap.
type QuotaRow struct {
	BillingAccountID int64
	UserID           int64
	Period           string
	LimitCredits     *int64
}

// GetQuotaRow reads one member quota row. ok=false means absent,
// which callers treat as unlimited (the default); it is not an error.
func (s *Store) GetQuotaRow(ctx context.Context, billingAccountID, userID int64) (period string, limitCredits *int64, ok bool, err error) {
	if s == nil || s.q == nil || billingAccountID <= 0 || userID <= 0 {
		return "", nil, false, ErrQuotaNotFound
	}
	row, err := s.q.GetMemberQuota(ctx, dbsqlc.GetMemberQuotaParams{
		BillingAccountID: billingAccountID, UserID: userID,
	})
	if err != nil {
		if database.IsNotFound(err) {
			return PeriodUnlimited, nil, false, nil
		}
		return "", nil, false, err
	}
	return row.Period, int64PtrOrNil(row.LimitCredits), true, nil
}

// SetResult is the persisted quota cap after a SetQuota call.
type SetResult struct {
	Period       string
	LimitCredits *int64
}

// SetQuota validates and upserts one member quota row. period="" is
// normalized to unlimited. Every capped period requires
// 1..MaxLimitCredits; unlimited clears the limit. Adjusting never clears
// usage (usage is derived from immutable settlement facts, never stored
// here) and never tops up the wallet. Existing rows survive: rejoining
// members keep their cap and their accrued usage. Callers enforce
// Billing Owner/Admin before calling.
func (s *Store) SetQuota(ctx context.Context, billingAccountID, userID int64, period string, limitCredits *int64, updatedByUserID int64) (SetResult, error) {
	period = strings.TrimSpace(period)
	if period == "" {
		period = PeriodUnlimited
	}
	if err := ValidateSet(period, limitCredits); err != nil {
		return SetResult{}, err
	}
	if s == nil || s.q == nil || billingAccountID <= 0 || userID <= 0 {
		return SetResult{}, ErrQuotaNotFound
	}
	var limit pgtype.Int8
	if limitCredits != nil {
		limit = pgtype.Int8{Int64: *limitCredits, Valid: true}
	}
	var updatedBy pgtype.Int8
	if updatedByUserID > 0 {
		updatedBy = pgtype.Int8{Int64: updatedByUserID, Valid: true}
	}
	row, err := s.q.UpsertMemberQuota(ctx, dbsqlc.UpsertMemberQuotaParams{
		BillingAccountID: billingAccountID, UserID: userID, Period: period,
		LimitCredits: limit, UpdatedByUserID: updatedBy,
	})
	if err != nil {
		return SetResult{}, err
	}
	return SetResult{Period: row.Period, LimitCredits: int64PtrOrNil(row.LimitCredits)}, nil
}

// ListQuotas lists the explicitly set quota rows for one account, oldest
// member first, bounded by MaxListQuotas. Members without a row are
// unlimited and intentionally absent.
func (s *Store) ListQuotas(ctx context.Context, billingAccountID int64) ([]QuotaRow, error) {
	if s == nil || s.q == nil || billingAccountID <= 0 {
		return nil, ErrQuotaNotFound
	}
	rows, err := s.q.ListMemberQuotasByAccount(ctx, dbsqlc.ListMemberQuotasByAccountParams{
		BillingAccountID: billingAccountID, RowLimit: MaxListQuotas,
	})
	if err != nil {
		return nil, err
	}
	out := make([]QuotaRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, QuotaRow{
			BillingAccountID: row.BillingAccountID, UserID: row.UserID,
			Period: row.Period, LimitCredits: int64PtrOrNil(row.LimitCredits),
		})
	}
	return out, nil
}

// CheckQuotaInTx enforces the member's remaining quota inside the
// caller's settlement transaction (Task submit / Vision reserve). It runs
// after the workspace-budget check in the same transaction (fixed lock
// order: budget row first, quota row second), using reservedAt for its window:
//
//  1. Lock the member quota row FOR UPDATE (the per-member
//     serialization point; concurrent submits by the same member
//     serialize here and the loser observes the winner's committed
//     Hold, so concurrency cannot bypass).
//  2. Absent/unlimited or non-positive amounts allow immediately.
//  3. Otherwise sum submit-time-attributed usage in the reservation window
//     (settled net captures with linked refunds corrected to their
//     original period, plus pending Holds, across every Workspace
//     sharing the account) and fail closed with ErrQuotaExhausted when
//     used+amount exceeds the cap.
//
// The check writes nothing: the ledger Hold that follows is the
// reservation. Terminal Capture/Release needs no re-check; in-flight
// Holds converge on their submit-time period facts.
func (s *Store) CheckQuotaInTx(ctx context.Context, tx pgx.Tx, billingAccountID, userID int64, amount int64, reservedAt time.Time) error {
	if reservedAt.IsZero() {
		return errors.New("memberquotas: reservation time is required")
	}
	if tx == nil {
		return errors.New("memberquotas: settlement transaction is nil")
	}
	if billingAccountID <= 0 || userID <= 0 {
		return ErrQuotaNotFound
	}
	if amount <= 0 {
		return nil
	}
	row, err := dbsqlc.New(tx).GetMemberQuotaForUpdate(ctx, dbsqlc.GetMemberQuotaForUpdateParams{
		BillingAccountID: billingAccountID, UserID: userID,
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil
		}
		return err
	}
	return checkQuotaUsage(ctx, tx, row, amount, reservedAt)
}

// CheckQuotaForQuote is the preflight (non-transactional) read check for
// quote/readiness: current committed usage + amount vs the cap. It cannot
// fence concurrent submits (only the settlement-transaction check does
// that); it reports not-ready early so callers see the quota reason
// before spending a Hold. A nil pool, unknown member, or unlimited quota
// allows; over-cap returns ErrQuotaExhausted.
func (s *Store) CheckQuotaForQuote(ctx context.Context, billingAccountID, userID int64, amount int64, reservedAt time.Time) error {
	if reservedAt.IsZero() {
		return errors.New("memberquotas: reservation time is required")
	}
	if amount <= 0 {
		return nil
	}
	if s == nil || s.q == nil || billingAccountID <= 0 || userID <= 0 {
		return nil
	}
	row, err := s.q.GetMemberQuota(ctx, dbsqlc.GetMemberQuotaParams{
		BillingAccountID: billingAccountID, UserID: userID,
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil
		}
		return err
	}
	return checkQuotaUsage(ctx, s.db, row, amount, reservedAt)
}

// checkQuotaUsage applies the same cap policy after the caller fetches or locks its row.
func checkQuotaUsage(ctx context.Context, q consumptionread.Queryer, row dbsqlc.BillingAccountMemberQuota, amount int64, reservedAt time.Time) error {
	limit := int64PtrOrNil(row.LimitCredits)
	if IsUnlimited(row.Period, limit) {
		return nil
	}
	periodStart, periodEnd, ok := CurrentBounds(row.Period, reservedAt)
	if !ok {
		return ErrQuotaInvalid
	}
	used, err := consumptionread.UsedCreditsForMember(ctx, q, row.BillingAccountID, row.UserID, periodStart, periodEnd)
	if err != nil {
		return err
	}
	if used+amount > *limit {
		return ErrQuotaExhausted
	}
	return nil
}

// UsageForPeriod reports one member's consumed credits in
// [periodStart, periodEnd) across every Workspace sharing the account:
// settled net captures (linked refunds correct their original capture's
// period) plus pending Holds. A nil periodStart sums all time (the fixed
// and unlimited views). Reads see committed facts only.
func (s *Store) UsageForPeriod(ctx context.Context, billingAccountID, userID int64, periodStart, periodEnd *time.Time) (int64, error) {
	if s == nil || s.db == nil || billingAccountID <= 0 || userID <= 0 {
		return 0, ErrQuotaNotFound
	}
	return consumptionread.UsedCreditsForMember(ctx, s.db, billingAccountID, userID, periodStart, periodEnd)
}

func int64PtrOrNil(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	out := v.Int64
	return &out
}

// GetDefaultPolicy reads one account's default new-member quota policy.
// ok=false means absent (NOT CONFIGURED); callers fail closed instead
// of treating it as unlimited.
func (s *Store) GetDefaultPolicy(ctx context.Context, billingAccountID int64) (period string, limitCredits *int64, ok bool, err error) {
	if s == nil || s.q == nil || billingAccountID <= 0 {
		return "", nil, false, ErrQuotaNotFound
	}
	row, err := s.q.GetDefaultMemberQuota(ctx, billingAccountID)
	if err != nil {
		if database.IsNotFound(err) {
			return "", nil, false, nil
		}
		return "", nil, false, err
	}
	return row.Period, int64PtrOrNil(row.LimitCredits), true, nil
}

// SetDefaultPolicy validates and upserts one account's default new-member
// quota policy. period="" normalizes to unlimited. Every capped period
// requires 1..MaxLimitCredits; unlimited clears the limit. Changing the
// default only affects later joins; existing per-member rows keep their
// cap and accrued usage. Callers enforce Billing Owner/Admin before
// calling.
func (s *Store) SetDefaultPolicy(ctx context.Context, billingAccountID int64, period string, limitCredits *int64, updatedByUserID int64) (SetResult, error) {
	period = strings.TrimSpace(period)
	if period == "" {
		period = PeriodUnlimited
	}
	if err := ValidateSet(period, limitCredits); err != nil {
		return SetResult{}, err
	}
	if s == nil || s.q == nil || billingAccountID <= 0 {
		return SetResult{}, ErrQuotaNotFound
	}
	var limit pgtype.Int8
	if limitCredits != nil {
		limit = pgtype.Int8{Int64: *limitCredits, Valid: true}
	}
	var updatedBy pgtype.Int8
	if updatedByUserID > 0 {
		updatedBy = pgtype.Int8{Int64: updatedByUserID, Valid: true}
	}
	row, err := s.q.UpsertDefaultMemberQuota(ctx, dbsqlc.UpsertDefaultMemberQuotaParams{
		BillingAccountID: billingAccountID, Period: period,
		LimitCredits: limit, UpdatedByUserID: updatedBy,
	})
	if err != nil {
		return SetResult{}, err
	}
	return SetResult{Period: row.Period, LimitCredits: int64PtrOrNil(row.LimitCredits)}, nil
}
