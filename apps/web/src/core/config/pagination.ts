// Shared pagination sizes + normalization (frontend).
//
// Cursor lists across workspace/account/admin surfaces converge here so page
// sizes stay behavior-identical in one place: `PAGE_SIZES` owns every
// per-surface default, `normalizePageSize` owns the single clamp (floor to an
// int, floor at 1, cap at the surface max; non-numbers fall back to the
// default). Surface-specific defaults ride the `defaultValue` argument;
// dense gallery lists (boards, library folders, monitor charts) share
// `PAGE_SIZES.dense`.
export const PAGE_SIZES = {
  /** Default cursor page: workspace list, account feeds, run history, orders. */
  standard: 20,
  /** Admin credit ledger pages. */
  adminLedger: 25,
  /** Admin reconciliation finding-queue pages. */
  adminFindingQueue: 25,
  /** Admin users list default. */
  adminUsers: 10,
  /** Admin monitor task list. */
  adminMonitor: 15,
  /** Dense lists: monitor charts, boards, library folders. */
  dense: 100,
  /** Default clamp ceiling for cursor `page_size` params. */
  max: 100,
} as const

export function normalizePageSize(
  value: unknown,
  defaultValue: number = PAGE_SIZES.standard,
  max: number = PAGE_SIZES.max,
): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) return defaultValue
  return Math.min(max, Math.max(1, Math.floor(value)))
}
