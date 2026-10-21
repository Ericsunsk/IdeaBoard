import {
  getWorkspaceBudget,
  setWorkspaceBudget,
} from '../../../../api/generated-query/public/workspace-billing/workspace-billing'
import type { WorkspaceBudget } from '../../../../api/generated-query/public/model'
import { i18n } from '../../../../core/i18n'
import { isOpaqueId, opaqueIdPrefix } from '../../../../api/contractKernel'
import { requireAccountWorkspaceId, resolveWorkspaceBillingData } from '../workspaceBilling'

// Workspace shared consumption budget boundary (issue #945).
//
// A Billing Account (ba_) holds the unified balance; a Workspace records
// the consumption location. The budget caps joint member consumption
// (paid Task generations + Vision Search) per period: monthly (UTC
// calendar month) or unlimited (no cap, the default). There are no
// sub-wallets and no reservations: adjusting the limit never clears
// usage, and a monthly reset starts a new UTC month without deleting
// history. Only Billing Owner/Admin writes (workspace admins cannot
// raise a pool-granted budget); any member reads.
type WorkspaceBudgetPeriod = 'monthly' | 'unlimited'

export type WorkspaceBudgetSummary = {
  workspaceId: string
  billingAccountId: string
  period: WorkspaceBudgetPeriod
  limitCredits: number | null
  usedCredits: number
  remainingCredits: number | null
  periodStart: string | null
  periodEnd: string | null
}

/** Credits per-command ceiling mirrored from the backend budget bound. */
export const MAX_WORKSPACE_BUDGET_LIMIT = 9000000000000000

export const workspaceBudgetQueryKeys = {
  all: ['workspace', 'budget'] as const,
  detail: (workspaceId: string) => ['workspace', 'budget', 'detail', workspaceId] as const,
}

function requireBudgetPeriod(value: unknown): WorkspaceBudgetPeriod {
  if (value === 'monthly' || value === 'unlimited') return value
  throw new Error('Invalid workspace budget period')
}

function optionalPositiveInt(value: unknown): number | null {
  if (value == null) return null
  if (typeof value !== 'number' || !Number.isInteger(value) || value < 0) {
    throw new Error('Invalid workspace budget credits')
  }
  return value
}

function optionalDateTime(value: unknown): string | null {
  if (value == null) return null
  if (typeof value !== 'string' || !value) throw new Error('Invalid workspace budget period bound')
  return value
}

export function parseWorkspaceBudgetData(input: unknown): WorkspaceBudgetSummary {
  const data = input as WorkspaceBudget
  if (typeof data.workspace_id !== 'string' || !isOpaqueId(data.workspace_id)) {
    throw new Error('Invalid workspace budget workspace')
  }
  if (
    typeof data.billing_account_id !== 'string' ||
    !isOpaqueId(data.billing_account_id) ||
    opaqueIdPrefix(data.billing_account_id) !== 'ba'
  ) {
    throw new Error('Invalid workspace budget account')
  }
  const period = requireBudgetPeriod(data.period)
  const limitCredits = optionalPositiveInt(data.limit_credits ?? null)
  if (period === 'monthly' && limitCredits == null) {
    throw new Error('Monthly workspace budget requires a limit')
  }
  if (period === 'unlimited' && limitCredits != null) {
    throw new Error('Unlimited workspace budget carries no limit')
  }
  const usedCredits = optionalPositiveInt(data.used_credits)
  if (usedCredits == null) throw new Error('Invalid workspace budget usage')
  const remainingCredits = optionalPositiveInt(data.remaining_credits ?? null)
  return {
    workspaceId: data.workspace_id,
    billingAccountId: data.billing_account_id,
    period,
    limitCredits,
    usedCredits,
    remainingCredits,
    periodStart: optionalDateTime(data.period_start ?? null),
    periodEnd: optionalDateTime(data.period_end ?? null),
  }
}

/** True when a monthly cap is set and usage reached it. */
export function isBudgetExhausted(budget: WorkspaceBudgetSummary): boolean {
  return (
    budget.period === 'monthly' &&
    budget.limitCredits != null &&
    budget.usedCredits >= budget.limitCredits
  )
}

export async function fetchWorkspaceBudget(
  workspaceId: string,
  signal?: AbortSignal,
): Promise<WorkspaceBudgetSummary> {
  const id = requireAccountWorkspaceId(workspaceId)
  const fallback = i18n.global.t('workspace.budget.loadFailed')
  const raw = await resolveWorkspaceBillingData<unknown>(
    getWorkspaceBudget(id, { signal }),
    fallback,
  )
  return parseWorkspaceBudgetData(raw)
}

export type UpdateWorkspaceBudgetInput =
  { period: 'monthly'; limitCredits: number } | { period: 'unlimited' }

export function normalizeBudgetInput(input: UpdateWorkspaceBudgetInput): {
  period: WorkspaceBudgetPeriod
  limit_credits?: number
} {
  if (input.period === 'monthly') {
    if (
      !Number.isInteger(input.limitCredits) ||
      input.limitCredits <= 0 ||
      input.limitCredits > MAX_WORKSPACE_BUDGET_LIMIT
    ) {
      throw new Error(
        i18n.global.t('workspace.budget.invalidLimit', { max: MAX_WORKSPACE_BUDGET_LIMIT }),
      )
    }
    return { period: 'monthly', limit_credits: input.limitCredits }
  }
  return { period: 'unlimited' }
}

export async function updateWorkspaceBudget(
  workspaceId: string,
  input: UpdateWorkspaceBudgetInput,
  signal?: AbortSignal,
): Promise<WorkspaceBudgetSummary> {
  const id = requireAccountWorkspaceId(workspaceId)
  const fallback = i18n.global.t('workspace.budget.saveFailed')
  const body = normalizeBudgetInput(input)
  const raw = await resolveWorkspaceBillingData<unknown>(
    setWorkspaceBudget(id, { data: body }, { signal }),
    fallback,
  )
  return parseWorkspaceBudgetData(raw)
}
