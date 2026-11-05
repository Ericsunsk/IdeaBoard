import type { User } from '@/api/http/sessionUserType'

export type TestUserOverrides = Partial<User> & {
  ordinal?: number
}

// Converged session-user factory: the ordinal-derived defaults come from
// `auth/session/authSession.contract.spec.ts` (the richest local variant);
// every other local `makeUser(): User` was a fixed subset of these fields.
export function makeSessionUser(overrides: TestUserOverrides = {}): User {
  const ordinal = overrides.ordinal ?? 1
  const { ordinal: _ignored, ...rest } = overrides
  return {
    public_id: `00000000-0000-4000-8000-${String(ordinal).padStart(12, '0')}`,
    display_name: `User ${ordinal}`,
    avatar: '',
    role: 'user',
    ...rest,
  }
}

type MemberQuotaPeriodValue = 'daily' | 'weekly' | 'monthly' | 'fixed' | 'unlimited'

export type MemberQuotaSummaryValue = {
  billingAccountId: string
  userId: string
  period: MemberQuotaPeriodValue
  limitCredits: number | null
  usedCredits: number
  remainingCredits: number | null
  periodStart: string | null
  periodEnd: string | null
}

export type MemberQuotaSummaryOverrides = {
  billingAccountId?: string
  userId?: string
  period?: MemberQuotaPeriodValue
  limitCredits?: number | null
  usedCredits?: number
  remainingCredits?: number | null
  periodStart?: string | null
  periodEnd?: string | null
}

// Converged from `WorkspaceLimitsSection.component.spec.ts`
// (`dailyQuota`/`unlimitedQuota`) and `memberQuotaTableColumns.unit.spec.ts`
// (`buildQuotaSummary`): identical member-quota summary shape; the structural
// value stays assignable to the feature `MemberQuotaSummary` without
// importing the feature module.
export function makeMemberQuotaSummary(
  overrides: MemberQuotaSummaryOverrides = {},
): MemberQuotaSummaryValue {
  return {
    billingAccountId: 'ba_1',
    userId: 'usr_a',
    period: 'daily',
    limitCredits: 100,
    usedCredits: 25,
    remainingCredits: 75,
    periodStart: '2026-10-21T00:00:00Z',
    periodEnd: '2026-10-22T00:00:00Z',
    ...overrides,
  }
}

// Unlimited member quota: no cap, no window; history is unaffected.
export function makeUnlimitedMemberQuota(
  overrides: MemberQuotaSummaryOverrides = {},
): MemberQuotaSummaryValue {
  return makeMemberQuotaSummary({
    period: 'unlimited',
    limitCredits: null,
    remainingCredits: null,
    periodStart: null,
    periodEnd: null,
    ...overrides,
  })
}
