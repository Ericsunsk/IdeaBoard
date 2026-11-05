import { describe, expect, test } from 'vitest'
import { makeMemberQuotaSummary } from '@/test/factories'
import { buildMemberQuotaView } from './memberQuotaTableColumns'
import {
  MEMBER_QUOTA_EDIT_KEYS,
  MEMBER_QUOTA_STATUS_KEYS,
  MEMBER_QUOTA_TABLE_KEYS,
} from './quotaKeys'
import type { MemberQuotaSummary } from '@/features/account/session/quota/resource'

describe('member quota status', () => {
  function statusOf(summary: MemberQuotaSummary): string {
    const view = buildMemberQuotaView(
      [summary],
      () => 'a@example.com',
      () => 'member',
    )
    return view.rows[0]?.status ?? 'missing'
  }

  test('unlimited quotas never exhaust', () => {
    expect(
      statusOf(
        makeMemberQuotaSummary({ period: 'unlimited', limitCredits: null, remainingCredits: null }),
      ),
    ).toBe('unlimited')
  })

  test('usage at the cap reads as exhausted', () => {
    expect(statusOf(makeMemberQuotaSummary({ usedCredits: 100, remainingCredits: 0 }))).toBe('exhausted')
  })

  test('remaining below 20 percent reads as near-exhausted', () => {
    expect(statusOf(makeMemberQuotaSummary({ usedCredits: 85, remainingCredits: 15 }))).toBe(
      'nearExhausted',
    )
  })

  test('remaining at exactly 20 percent stays normal', () => {
    expect(statusOf(makeMemberQuotaSummary({ usedCredits: 80, remainingCredits: 20 }))).toBe('normal')
  })

  test('capped quotas without a remaining figure stay normal', () => {
    expect(statusOf(makeMemberQuotaSummary({ remainingCredits: null }))).toBe('normal')
  })
})

describe('buildMemberQuotaView', () => {
  test('projects labels, roles and statuses in one pass', () => {
    const view = buildMemberQuotaView(
      [
        makeMemberQuotaSummary({ userId: 'usr_a' }),
        makeMemberQuotaSummary({
          userId: 'usr_owner',
          period: 'unlimited',
          limitCredits: null,
          usedCredits: 5,
          remainingCredits: null,
        }),
        makeMemberQuotaSummary({ userId: 'usr_d', usedCredits: 100, remainingCredits: 0 }),
      ],
      (userId) => `${userId}@example.com`,
      (userId) => (userId === 'usr_owner' ? 'owner' : 'member'),
    )

    expect(view.rows).toHaveLength(3)
    expect(view.rows[0]).toMatchObject({
      userId: 'usr_a',
      label: 'usr_a@example.com',
      role: 'member',
      status: 'normal',
    })
    expect(view.rows[1]).toMatchObject({ userId: 'usr_owner', role: 'owner', status: 'unlimited' })
    expect(view.exhaustedCount).toBe(1)
  })

  test('empty quotas summarize to zero', () => {
    expect(
      buildMemberQuotaView(
        [],
        () => '',
        () => null,
      ),
    ).toEqual({ rows: [], exhaustedCount: 0 })
  })
})

describe('member quota table keys', () => {
  test('routes table, badge and dialog copy through the locale bundle', () => {
    expect(MEMBER_QUOTA_TABLE_KEYS.headerMember).toBe('workspace.quota.headerMember')
    expect(MEMBER_QUOTA_TABLE_KEYS.emptyTitle).toBe('workspace.quota.emptyTitle')
    expect(MEMBER_QUOTA_TABLE_KEYS.createFirst).toBe('workspace.quota.createFirst')
    expect(MEMBER_QUOTA_STATUS_KEYS.nearExhausted).toBe('workspace.quota.statusNearExhausted')
    expect(MEMBER_QUOTA_EDIT_KEYS.clearConfirmMessage).toBe('workspace.quota.clearConfirmMessage')
  })
})
