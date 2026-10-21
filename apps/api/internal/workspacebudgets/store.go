package workspacebudgets

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

// Store persists workspace-level consumption budgets and enforces them
// inside caller-owned settlement transactions. Repository methods take
// context.Context first. Reads use the pool; settlement checks join the
// caller's pgx.Tx so Check/Quote/Hold/Task/River stay atomic. The Store
// never writes the Credits Ledger; the ledger Hold that follows a passed
// check is the reservation (no sub-wallet, no separate budget hold row).
type Store struct {
	db *pgxpool.Pool
	q  *dbsqlc.Queries
}

// NewStore builds a budget Store over pool. Pass nil pool in tests that
// stub the checker seam instead.
func NewStore(db *pgxpool.Pool) *Store {
	if db == nil {
		return &Store{}
	}
	return &Store{db: db, q: dbsqlc.New(db)}
}

// GetBudgetRow reads one workspace budget row. ok=false means absent,
// which callers treat as unlimited (the default); it is not an error.
func (s *Store) GetBudgetRow(ctx context.Context, workspaceInternalID int64) (period string, limitCredits *int64, ok bool, err error) {
	if s == nil || s.q == nil || workspaceInternalID <= 0 {
		return "", nil, false, ErrBudgetNotFound
	}
	row, err := s.q.GetWorkspaceBudget(ctx, workspaceInternalID)
	if err != nil {
		if database.IsNotFound(err) {
			return PeriodUnlimited, nil, false, nil
		}
		return "", nil, false, err
	}
	return row.Period, int64PtrOrNil(row.LimitCredits), true, nil
}

// SetResult is the persisted budget cap after a SetBudget call.
type SetResult struct {
	Period       string
	LimitCredits *int64
}

// SetBudget validates and upserts one workspace budget row. period="" is
// normalized to unlimited. Monthly requires 1..MaxLimitCredits; unlimited
// clears the limit. Adjusting never clears usage (usage is derived from
// immutable settlement facts, never stored here) and never tops up the
// wallet. Callers enforce Billing Owner/Admin before calling.
func (s *Store) SetBudget(ctx context.Context, workspaceInternalID int64, period string, limitCredits *int64, updatedByUserID int64) (SetResult, error) {
	period = strings.TrimSpace(period)
	if period == "" {
		period = PeriodUnlimited
	}
	if err := ValidateSet(period, limitCredits); err != nil {
		return SetResult{}, err
	}
	if s == nil || s.q == nil || workspaceInternalID <= 0 {
		return SetResult{}, ErrBudgetNotFound
	}
	var limit pgtype.Int8
	if limitCredits != nil {
		limit = pgtype.Int8{Int64: *limitCredits, Valid: true}
	}
	var updatedBy pgtype.Int8
	if updatedByUserID > 0 {
		updatedBy = pgtype.Int8{Int64: updatedByUserID, Valid: true}
	}
	row, err := s.q.UpsertWorkspaceBudget(ctx, dbsqlc.UpsertWorkspaceBudgetParams{
		WorkspaceID: workspaceInternalID, Period: period,
		LimitCredits: limit, UpdatedByUserID: updatedBy,
	})
	if err != nil {
		return SetResult{}, err
	}
	return SetResult{Period: row.Period, LimitCredits: int64PtrOrNil(row.LimitCredits)}, nil
}

// CheckBudgetInTx enforces the remaining budget inside the caller's
// settlement transaction (Task submit / Vision reserve). It runs between
// Quote and Hold, using reservedAt for the reservation month:
//
//  1. Lock the budget row FOR UPDATE (the per-workspace serialization
//     point; concurrent submits serialize here and the loser observes
//     the winner's committed Hold, so concurrency cannot bypass).
//  2. Absent/unlimited or non-positive amounts allow immediately.
//  3. Otherwise sum submit-time-attributed usage in the reservation UTC
//     month (settled net captures with linked refunds corrected to
//     their original period, plus pending Holds) and fail closed with
//     ErrBudgetExhausted when used+amount exceeds the cap.
//
// The check writes nothing: the ledger Hold that follows is the
// reservation. Terminal Capture/Release needs no re-check; in-flight
// Holds converge on their submit-time period facts.
func (s *Store) CheckBudgetInTx(ctx context.Context, tx pgx.Tx, workspacePublicID string, amount int64, reservedAt time.Time) error {
	if reservedAt.IsZero() {
		return errors.New("workspacebudgets: reservation time is required")
	}
	if tx == nil {
		return errors.New("workspacebudgets: settlement transaction is nil")
	}
	workspacePublicID = strings.TrimSpace(workspacePublicID)
	if workspacePublicID == "" {
		return ErrBudgetNotFound
	}
	if amount <= 0 {
		return nil
	}
	queries := dbsqlc.New(tx)
	var workspaceInternalID int64
	if err := tx.QueryRow(ctx, `SELECT internal_id FROM workspaces WHERE public_id = $1`, workspacePublicID).Scan(&workspaceInternalID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrBudgetNotFound
		}
		return err
	}
	return checkBudgetByInternalIDInTx(ctx, tx, queries, workspaceInternalID, amount, reservedAt)
}

// CheckBudgetByInternalIDInTx is the internal-ID form for callers that
// already resolved the workspace (Vision reserve). Same contract as
// CheckBudgetInTx.
func (s *Store) CheckBudgetByInternalIDInTx(ctx context.Context, tx pgx.Tx, workspaceInternalID int64, amount int64, reservedAt time.Time) error {
	if reservedAt.IsZero() {
		return errors.New("workspacebudgets: reservation time is required")
	}
	if tx == nil {
		return errors.New("workspacebudgets: settlement transaction is nil")
	}
	if workspaceInternalID <= 0 {
		return ErrBudgetNotFound
	}
	if amount <= 0 {
		return nil
	}
	return checkBudgetByInternalIDInTx(ctx, tx, dbsqlc.New(tx), workspaceInternalID, amount, reservedAt)
}

func checkBudgetByInternalIDInTx(ctx context.Context, tx pgx.Tx, queries *dbsqlc.Queries, workspaceInternalID int64, amount int64, reservedAt time.Time) error {
	row, err := queries.GetWorkspaceBudgetForUpdate(ctx, workspaceInternalID)
	if err != nil {
		if database.IsNotFound(err) {
			return nil
		}
		return err
	}
	return checkBudgetUsage(ctx, tx, row, amount, reservedAt)
}

// CheckBudgetForQuote is the preflight (non-transactional) read check for
// quote/readiness: current committed usage + amount vs the cap. It cannot
// fence concurrent submits (only the settlement-transaction check does
// that); it reports not-ready early so callers see the budget reason
// before spending a Hold. A nil pool, unknown workspace, or unlimited
// budget allows; over-cap returns ErrBudgetExhausted.
func (s *Store) CheckBudgetForQuote(ctx context.Context, workspaceInternalID int64, amount int64, reservedAt time.Time) error {
	if reservedAt.IsZero() {
		return errors.New("workspacebudgets: reservation time is required")
	}
	if amount <= 0 {
		return nil
	}
	if s == nil || s.q == nil || workspaceInternalID <= 0 {
		return nil
	}
	row, err := s.q.GetWorkspaceBudget(ctx, workspaceInternalID)
	if err != nil {
		if database.IsNotFound(err) {
			return nil
		}
		return err
	}
	return checkBudgetUsage(ctx, s.db, row, amount, reservedAt)
}

// checkBudgetUsage applies the same cap policy after the caller fetches or locks its row.
func checkBudgetUsage(ctx context.Context, q consumptionread.Queryer, row dbsqlc.WorkspaceBudget, amount int64, reservedAt time.Time) error {
	limit := int64PtrOrNil(row.LimitCredits)
	if IsUnlimited(row.Period, limit) {
		return nil
	}
	periodStart, periodEnd := CurrentMonthBounds(reservedAt)
	used, err := consumptionread.UsedCreditsForWorkspace(ctx, q, row.WorkspaceID, &periodStart, &periodEnd)
	if err != nil {
		return err
	}
	if used+amount > *limit {
		return ErrBudgetExhausted
	}
	return nil
}

// UsageForPeriod reports consumed credits for one workspace in
// [periodStart, periodEnd): settled net captures (linked refunds correct
// their original capture's period) plus pending Holds. A nil periodStart
// sums all time (the unlimited view). Reads see committed facts only.
func (s *Store) UsageForPeriod(ctx context.Context, workspaceInternalID int64, periodStart, periodEnd *time.Time) (int64, error) {
	if s == nil || s.db == nil || workspaceInternalID <= 0 {
		return 0, ErrBudgetNotFound
	}
	return consumptionread.UsedCreditsForWorkspace(ctx, s.db, workspaceInternalID, periodStart, periodEnd)
}

func int64PtrOrNil(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	out := v.Int64
	return &out
}
