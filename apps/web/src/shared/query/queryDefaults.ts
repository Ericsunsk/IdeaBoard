// Shared TanStack Query option presets (frontend).
//
// Converges the ad-hoc `staleTime` / `retry: false` / `refetchInterval` /
// `placeholderData` overrides scattered across account/admin queries. Every
// preset is behavior-identical to the literals it replaces (same ms values,
// same fail-fast semantics); call sites keep ownership of their query keys,
// query functions, and per-surface cadence numbers.
//
// - `noRetry()` — fail-fast reads: surface the error state instead of
//   retrying behind dialogs and list states.
// - `accountScoped()` — account/workspace reads share 15s freshness
//   (wallet, workspace selection); usage overrides to 30s explicitly.
// - `adminPolling(activeCadenceMs, idleCadenceMs, isActive?)` — admin
//   active/idle polling: fail-fast, no background refetch, cadence selected
//   from cached data (defaults to the active cadence). Visibility-aware
//   polling stays on `useQueryPolling`, not here.
// - `keepPrevious()` — keep the previous page mounted while refetching
//   (placeholder identity); the generic instantiates per query, so no
//   type argument is needed at call sites.
//
// Query keys are NOT presets; see `QUERY_KEY_CONVENTIONS` in
// `shared/CONTEXT.md` (generated orval keys when the op exists, hand-rolled
// `*QueryKeys` factories only for composed queries, no inline literals).
export const STALE_TIME_MS = {
  /** Account/workspace reads: wallet, workspace selection. */
  account: 15_000,
  /** Account usage snapshot. */
  usage: 30_000,
  /** Admin statistics. */
  statistics: 60_000,
  /** Static config: brand logo overrides. */
  static: Infinity,
} as const

/** Fail-fast reads: no retry, surface the error state immediately. */
export function noRetry() {
  return { retry: false as const }
}

/** Account/workspace reads: shared freshness; keeps the global retry policy. */
export function accountScoped(staleTime: number = STALE_TIME_MS.account) {
  return { staleTime }
}

type PollStatusQuery = {
  state: { data: unknown }
}

/**
 * Admin active/idle polling: fail-fast, no background refetch, cadence
 * selected from cached data (defaults to the active cadence).
 */
export function adminPolling(
  activeCadenceMs: number | false,
  idleCadenceMs: number | false,
  isActive: (data: unknown) => boolean = () => true,
) {
  return {
    retry: false as const,
    refetchInterval: (query: PollStatusQuery) =>
      isActive(query.state.data) ? activeCadenceMs : idleCadenceMs,
    refetchIntervalInBackground: false as const,
  }
}

/** Keep the previous page mounted while refetching (placeholder identity). */
export function keepPrevious() {
  return {
    placeholderData: <TData>(previous: TData | undefined) => previous,
  }
}
