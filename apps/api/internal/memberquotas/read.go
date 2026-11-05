package memberquotas

import (
	"context"
	"time"

	"ideaboard/api/internal/tenancy"
)

// ReadStore exposes quota facts and usage without settlement or writes.
type ReadStore interface {
	GetQuotaRow(context.Context, int64, int64) (string, *int64, bool, error)
	ListQuotas(context.Context, int64) ([]QuotaRow, error)
	UsageForPeriod(context.Context, int64, int64, *time.Time, *time.Time) (int64, error)
	UsageForPeriodBatch(context.Context, int64, []int64, *time.Time, *time.Time) (map[int64]int64, error)
}

// Reading is a normalized cap and its live usage in one UTC window.
type Reading struct {
	UserID     int64
	Period     string
	Limit      *int64
	Used       int64
	Start, End *time.Time
}

// Reader keeps normalization, window planning and batch usage together.
// Callers supply their snapshot context; no read opens another transaction.
type Reader struct {
	Store  ReadStore
	Policy Policy
}

func (r Reader) Self(ctx context.Context, scope Scope, accountID, userID int64) (Reading, error) {
	// Org-only (#975): Personal scopes never reach here (callers return
	// the unlimited constant directly without a Reader/Policy/Scope).
	// A Personal scope reaching this point fails closed.
	if err := requireOrganizationScope(scope); err != nil {
		return Reading{}, err
	}
	period, limit, _, err := r.Store.GetQuotaRow(ctx, accountID, userID)
	if err != nil {
		return Reading{}, err
	}
	period, limit, err = r.Policy.NormalizeRead(ctx, scope, userID, period, limit)
	if err != nil {
		return Reading{}, err
	}
	return r.Read(ctx, accountID, userID, period, limit)
}

// Read derives usage for an already normalized (or newly saved) cap.
func (r Reader) Read(ctx context.Context, accountID, userID int64, period string, limit *int64) (Reading, error) {
	reading := newReading(userID, period, limit, time.Now().UTC())
	used, err := r.Store.UsageForPeriod(ctx, accountID, userID, reading.Start, reading.End)
	if err != nil {
		return Reading{}, err
	}
	reading.Used = used
	return reading, nil
}

func (r Reader) Page(ctx context.Context, scope Scope, accountID int64, rows []QuotaRow) ([]Reading, error) {
	// Listing is a management read: Personal scopes fail closed because
	// Personal quotas do not exist (the HTTP layer rejects Personal lists
	// before this point).
	if err := requireOrganizationScope(scope); err != nil {
		return nil, err
	}
	normalized, err := r.Policy.NormalizeReadBatch(ctx, scope, rows)
	if err != nil {
		return nil, err
	}
	type window struct{ start, end time.Time }
	type group struct {
		start, end *time.Time
		userIDs    []int64
	}
	groups := make(map[window]*group)
	readings := make([]Reading, 0, len(normalized))
	now := time.Now().UTC()
	for _, row := range normalized {
		reading := newReading(row.UserID, row.Period, row.LimitCredits, now)
		readings = append(readings, reading)
		key := window{}
		if reading.Start != nil {
			key.start = *reading.Start
		}
		if reading.End != nil {
			key.end = *reading.End
		}
		if groups[key] == nil {
			groups[key] = &group{start: reading.Start, end: reading.End}
		}
		groups[key].userIDs = append(groups[key].userIDs, row.UserID)
	}
	usedByUser := make(map[int64]int64, len(readings))
	for _, g := range groups {
		batch, err := r.Store.UsageForPeriodBatch(ctx, accountID, g.userIDs, g.start, g.end)
		if err != nil {
			return nil, err
		}
		for id, used := range batch {
			usedByUser[id] = used
		}
	}
	for i := range readings {
		readings[i].Used = usedByUser[readings[i].UserID]
	}
	return readings, nil
}

func newReading(userID int64, period string, limit *int64, now time.Time) Reading {
	reading := Reading{UserID: userID, Period: period, Limit: limit}
	if !IsUnlimited(period, limit) {
		reading.Start, reading.End, _ = CurrentBounds(period, now)
	}
	return reading
}

// ScopeForWorkspace follows the immutable Personal/Organization payer scope.
func ScopeForWorkspace(workspace *tenancy.Workspace) Scope {
	if workspace.OrganizationID == nil {
		return Scope{Kind: ScopePersonal}
	}
	return Scope{Kind: ScopeOrganization, OrganizationID: *workspace.OrganizationID}
}
