package retention

// PurgedTaskStats reports one retention pass. Statistics for every purged
// date are frozen into task_daily_stats before the raw rows disappear;
// financial facts (billing snapshots/settlements/claims, credit ledger)
// are never touched by retention.
type PurgedTaskStats struct {
	BackfilledDates      int
	TasksDeleted         int64
	CancellationsDeleted int64
}
