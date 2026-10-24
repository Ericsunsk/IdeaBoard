package memberquotas

import (
	"context"
	"errors"
	"strings"
	"time"

	"ideaboard/api/internal/consumptionread"
	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/tenancy"

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
// Lock order is load-bearing: settlement checks lock the member quota
// row FOR UPDATE as the per-member serialization point. Task submit and
// Vision reserve run the quota check in the same transaction before the
// Hold, so concurrent submits serialize here and never deadlock each other.
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

// queriesForSnapshot returns tx-bound Queries when ctx carries the ambient
// read snapshot, else the pool-bound Queries. Overview reads share one
// snapshot; independent reads use the pool.
func (s *Store) queriesForSnapshot(ctx context.Context) *dbsqlc.Queries {
	if tx, ok := dbutil.AmbientTxFromContext(ctx); ok {
		return dbsqlc.New(tx)
	}
	return s.q
}

// queryerForSnapshot returns the ambient snapshot transaction when present,
// else the pool, so usage reads observe the same snapshot as the quota rows.
// A nil pool with no carrier fails closed at the call sites below.
func (s *Store) queryerForSnapshot(ctx context.Context) consumptionread.Queryer {
	if tx, ok := dbutil.AmbientTxFromContext(ctx); ok {
		return tx
	}
	return s.db
}

// GetQuotaRow reads one member quota row. ok=false means absent,
// which callers treat as unlimited (the default); it is not an error.
func (s *Store) GetQuotaRow(ctx context.Context, billingAccountID, userID int64) (period string, limitCredits *int64, ok bool, err error) {
	if s == nil || billingAccountID <= 0 || userID <= 0 {
		return "", nil, false, ErrQuotaNotFound
	}
	q := s.queriesForSnapshot(ctx)
	if q == nil {
		return "", nil, false, ErrQuotaNotFound
	}
	row, err := q.GetMemberQuota(ctx, dbsqlc.GetMemberQuotaParams{
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
// members keep their cap and their accrued usage. Callers enforce the
// #953 role matrix before calling (Organization: Owner sets Admin,
// Owner/Admin set Member, Admin never touches Owner/Admin, Owner stays
// unlimited via the check short-circuit; Personal: the account holder
// manages collaborators).
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
// unlimited and intentionally absent. Joins the ambient read snapshot when
// present (billing overview fan-out).
func (s *Store) ListQuotas(ctx context.Context, billingAccountID int64) ([]QuotaRow, error) {
	if s == nil || billingAccountID <= 0 {
		return nil, ErrQuotaNotFound
	}
	q := s.queriesForSnapshot(ctx)
	if q == nil {
		return nil, ErrQuotaNotFound
	}
	rows, err := q.ListMemberQuotasByAccount(ctx, dbsqlc.ListMemberQuotasByAccountParams{
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
// in the same transaction after the Quote and before the Hold, using
// reservedAt for its window:
//
//  1. Organization Owners short-circuit to unlimited: the check locks
//     nothing and allows immediately, so an Owner row (even a capped one
//     left by an earlier write) never gates consumption.
//  2. Lock the member quota row FOR UPDATE (the per-member
//     serialization point; concurrent submits by the same member
//     serialize here and the loser observes the winner's committed
//     Hold, so concurrency cannot bypass).
//  3. Absent/unlimited or non-positive amounts allow immediately.
//  4. Otherwise sum submit-time-attributed usage in the reservation window
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
	if owner, err := isOrganizationOwner(ctx, tx, billingAccountID, userID); err != nil {
		return err
	} else if owner {
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
// allows; over-cap returns ErrQuotaExhausted. Organization Owners
// short-circuit to unlimited, mirroring CheckQuotaInTx.
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
	if s.db != nil {
		if owner, err := isOrganizationOwner(ctx, s.db, billingAccountID, userID); err != nil {
			return err
		} else if owner {
			return nil
		}
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
// and unlimited views). Reads see committed facts only, or the ambient
// read snapshot when present (billing overview fan-out).
func (s *Store) UsageForPeriod(ctx context.Context, billingAccountID, userID int64, periodStart, periodEnd *time.Time) (int64, error) {
	if s == nil || billingAccountID <= 0 || userID <= 0 {
		return 0, ErrQuotaNotFound
	}
	q := s.queryerForSnapshot(ctx)
	if q == nil {
		return 0, ErrQuotaNotFound
	}
	return consumptionread.UsedCreditsForMember(ctx, q, billingAccountID, userID, periodStart, periodEnd)
}

// UsageForPeriodBatch reports many members' consumed credits in one
// statement (GROUP BY user_id), replacing per-member UsageForPeriod round
// trips in quota list normalization and the billing overview fan-out.
// Absent users are omitted from the map (callers treat missing as zero).
// The bounds apply to every member; callers group rows by window or pass
// nil bounds for the fixed/unlimited views.
func (s *Store) UsageForPeriodBatch(ctx context.Context, billingAccountID int64, userIDs []int64, periodStart, periodEnd *time.Time) (map[int64]int64, error) {
	if s == nil || billingAccountID <= 0 {
		return nil, ErrQuotaNotFound
	}
	q := s.queryerForSnapshot(ctx)
	if q == nil {
		return nil, ErrQuotaNotFound
	}
	return consumptionread.UsedCreditsForMembers(ctx, q, billingAccountID, userIDs, periodStart, periodEnd)
}

func int64PtrOrNil(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	out := v.Int64
	return &out
}

// isOrganizationOwner reports whether userID holds the Owner role on the
// Organization backing billingAccountID (#953: Owner is always unlimited).
// Personal accounts (no organization_id) never match; missing accounts or
// memberships read as non-owner (the caller then applies the normal quota
// row). Database failures fail closed.
func isOrganizationOwner(ctx context.Context, q consumptionread.Queryer, billingAccountID, userID int64) (bool, error) {
	row := q.QueryRow(ctx, `SELECT organization_id FROM billing_accounts WHERE internal_id = $1`, billingAccountID)
	var org pgtype.Int8
	if err := row.Scan(&org); err != nil {
		return false, nil
	}
	if !org.Valid {
		return false, nil
	}
	var role string
	if err := q.QueryRow(ctx, `SELECT role FROM organization_memberships WHERE organization_id = $1 AND user_id = $2`, org.Int64, userID).Scan(&role); err != nil {
		return false, nil
	}
	return strings.TrimSpace(role) == tenancy.RoleOwner, nil
}
