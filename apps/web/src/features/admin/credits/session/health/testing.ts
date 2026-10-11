import type { HttpHandler } from 'msw'
import { http, HttpResponse } from '../../../../../test/msw'

// Shared fixtures + MSW builders for the credit-health surface
// (`/api/control/v1/credits/health`, `.../reconciliation/runs`). State stays
// caller-owned (board `testing/factories.ts` pattern): specs drive fixtures
// through the state object and failure injection via `healthError`, while
// bespoke orchestration (finding pagination gates) stays in the spec.
export function makeCreditHealth(overrides: Record<string, unknown> = {}) {
  return {
    status: 'healthy',
    open_findings: { total: 0, warning: 0, error: 0, critical: 0, high_risk: 0 },
    restricted_wallet_count: 0,
    ...overrides,
  }
}

export function makeReconciliationRun(overrides: Record<string, unknown> = {}) {
  return {
    id: 11,
    status: 'completed',
    trigger: 'scheduled',
    finding_count: 0,
    critical_count: 0,
    isolated_wallet_count: 0,
    attempt_count: 1,
    requested_at: '2026-07-17T02:00:00Z',
    started_at: '2026-07-17T02:00:01Z',
    completed_at: '2026-07-17T02:00:04Z',
    ...overrides,
  }
}

export function makeCreditFinding(overrides: Record<string, unknown> = {}) {
  return {
    id: 41,
    fingerprint: 'a'.repeat(64),
    finding_type: 'wallet_projection_mismatch',
    severity: 'critical',
    risk: 'high',
    wallet_public_id: '00000000-0000-0000-0000-000000000012',
    wallet_display_name: 'User #12',
    subject_type: 'credit_account',
    subject_id: '17',
    details: { account_id: 17, ledger_balance: 10, projected_balance: 17 },
    status: 'open',
    isolated: true,
    occurrence_count: 3,
    first_seen_at: '2026-07-17T01:00:00Z',
    last_seen_at: '2026-07-17T02:00:00Z',
    ...overrides,
  }
}

export type CreditHealthMswState = {
  health: Record<string, unknown>
  getHealth: () => Record<string, unknown>
  healthError: { status: number; body: Record<string, unknown> } | null
  healthRequests: number
  startRequests: unknown[]
  nextRunId: number
  onRunStarted: ((run: Record<string, unknown>) => void) | null
  runRequests: Array<string | null>
  getRunsPage: (beforeId: string | null) => { runs: unknown[]; next_before_id: number | undefined }
}

export function createCreditHealthMswState(
  overrides: Partial<CreditHealthMswState> & { health?: Record<string, unknown> } = {},
): CreditHealthMswState {
  const { health, ...rest } = overrides
  const state: CreditHealthMswState = {
    health: health ?? makeCreditHealth(),
    getHealth: () => state.health,
    healthError: null,
    healthRequests: 0,
    startRequests: [],
    nextRunId: 14,
    onRunStarted: null,
    runRequests: [],
    getRunsPage: () => ({ runs: [], next_before_id: undefined }),
    ...rest,
  }
  return state
}

export function createCreditHealthHandlers(state: CreditHealthMswState): HttpHandler[] {
  return [
    http.get('/api/control/v1/credits/health', () => {
      state.healthRequests += 1
      if (state.healthError) {
        return HttpResponse.json(state.healthError.body, { status: state.healthError.status })
      }
      return HttpResponse.json({ data: state.getHealth() })
    }),
    http.post('/api/control/v1/credits/reconciliation/runs', async ({ request }) => {
      state.startRequests.push(await request.json())
      const activeRun = makeReconciliationRun({
        id: state.nextRunId,
        status: 'queued',
        completed_at: undefined,
      })
      state.onRunStarted?.(activeRun)
      return HttpResponse.json({ data: activeRun })
    }),
    http.get('/api/control/v1/credits/reconciliation/runs', ({ request }) => {
      const beforeId = new URL(request.url).searchParams.get('before_id')
      state.runRequests.push(beforeId)
      return HttpResponse.json({ data: state.getRunsPage(beforeId) })
    }),
  ]
}
