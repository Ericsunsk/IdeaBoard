import { beforeEach, describe, expect, test, vi } from 'vitest'
import { defineComponent, ref } from 'vue'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { renderWithUser, waitFor } from '@/test/render'
import { mockAppToast, type AppToastMocks } from '@/test/toast'
import { useMemberQuotaSection } from './useMemberQuotaSection'

vi.mock('@/core/composables/useAppToast')

const quotaTransport = vi.hoisted(() => ({
  fetchSelf: vi.fn(),
  fetchList: vi.fn(),
  update: vi.fn(),
}))

vi.mock('./resource', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./resource')>()
  return {
    ...actual,
    fetchSelfMemberQuota: quotaTransport.fetchSelf,
    fetchMemberQuotaList: quotaTransport.fetchList,
    updateMemberQuota: quotaTransport.update,
  }
})

const rosterEntries = vi.hoisted(() => [
  { userId: 'usr_a', role: 'member', email: 'a@example.com' },
  { userId: 'usr_b', role: 'admin', email: 'b@example.com' },
  { userId: 'usr_c', role: 'member', email: 'c@example.com' },
  { userId: 'usr_owner', role: 'owner', email: 'owner@example.com' },
])

vi.mock('../workspace-settings', () => ({
  useWorkspaceSettingsRoster: () => ({
    data: ref(rosterEntries),
    isLoading: ref(false),
    isError: ref(false),
    retry: vi.fn(),
  }),
}))

function selfQuota() {
  return {
    billingAccountId: 'ba_1',
    userId: 'usr_a',
    period: 'unlimited' as const,
    limitCredits: null,
    usedCredits: 40,
    remainingCredits: null,
    periodStart: null,
    periodEnd: null,
  }
}

function managedQuotas() {
  return [
    {
      billingAccountId: 'ba_1',
      userId: 'usr_a',
      period: 'daily' as const,
      limitCredits: 100,
      usedCredits: 25,
      remainingCredits: 75,
      periodStart: '2026-10-21T00:00:00Z',
      periodEnd: '2026-10-22T00:00:00Z',
    },
  ]
}

function mountSection(
  canManage: boolean,
  currentUserId: string | null,
  organizationMembers: { userId: string; role: 'owner' | 'admin' | 'member' }[] = [
    { userId: 'usr_a', role: 'member' },
    { userId: 'usr_b', role: 'admin' },
    { userId: 'usr_c', role: 'member' },
    { userId: 'usr_owner', role: 'owner' },
  ],
) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  let section!: ReturnType<typeof useMemberQuotaSection>
  const Harness = defineComponent({
    setup() {
      section = useMemberQuotaSection({
        workspaceId: 'ws_123',
        canManage,
        isExpanded: true,
        currentUserId,
        organizationMembers,
      })
      return () => null
    },
  })
  renderWithUser(Harness, {
    global: { plugins: [[VueQueryPlugin, { queryClient }]] },
  })
  return {
    get section() {
      return section
    },
  }
}

describe('useMemberQuotaSection Org role guard', () => {
  let toast: AppToastMocks

  beforeEach(async () => {
    vi.clearAllMocks()
    toast = mockAppToast()
    quotaTransport.fetchSelf.mockResolvedValue(selfQuota())
    quotaTransport.fetchList.mockResolvedValue(managedQuotas())
    quotaTransport.update.mockResolvedValue(managedQuotas()[0])
  })

  test('resolves the actor role from Organization membership', async () => {
    const admin = mountSection(true, 'usr_b')
    await waitFor(() => expect(admin.section.memberOptions.value).toContain('usr_owner'))
    expect(admin.section.canAdjustTarget('usr_a')).toBe(true)

    const unknown = mountSection(true, 'usr_ghost')
    await waitFor(() => expect(unknown.section.memberOptions.value).toContain('usr_owner'))
    expect(unknown.section.canAdjustTarget('usr_a')).toBe(false)
  })

  test('encodes the Org adjust matrix in canAdjustTarget', async () => {
    const owner = mountSection(true, 'usr_owner')
    await waitFor(() => expect(owner.section.memberOptions.value).toContain('usr_owner'))
    // Owner sets Admin, Owner/Admin set Member; Owner rows stay unlimited.
    expect(owner.section.canAdjustTarget('usr_a')).toBe(true)
    expect(owner.section.canAdjustTarget('usr_b')).toBe(true)
    expect(owner.section.canAdjustTarget('usr_owner')).toBe(false)

    const admin = mountSection(true, 'usr_b')
    await waitFor(() => expect(admin.section.memberOptions.value).toContain('usr_owner'))
    // Admin never touches Owner/Admin rows.
    expect(admin.section.canAdjustTarget('usr_a')).toBe(true)
    expect(admin.section.canAdjustTarget('usr_b')).toBe(false)
    expect(admin.section.canAdjustTarget('usr_owner')).toBe(false)

    const member = mountSection(true, 'usr_a')
    await waitFor(() => expect(member.section.memberOptions.value).toContain('usr_owner'))
    expect(member.section.canAdjustTarget('usr_a')).toBe(false)

    const denied = mountSection(false, 'usr_owner')
    await waitFor(() => expect(denied.section.memberOptions.value).toContain('usr_owner'))
    expect(denied.section.canAdjustTarget('usr_a')).toBe(false)
  })

  test('rejects unknown users in pickTargetUser without transport', async () => {
    const admin = mountSection(true, 'usr_b')
    await waitFor(() => expect(admin.section.memberOptions.value).toContain('usr_owner'))

    admin.section.pickTargetUser('usr_attacker')
    expect(toast.error).toHaveBeenCalledWith('Could not save the quota. Please try again.')
    expect(admin.section.targetUserId.value).toBe('')
    expect(quotaTransport.update).not.toHaveBeenCalled()
  })

  test('rejects disabled options in pickTargetUser', async () => {
    const admin = mountSection(true, 'usr_b')
    await waitFor(() => expect(admin.section.memberOptions.value).toContain('usr_owner'))

    admin.section.pickTargetUser('usr_owner')
    expect(toast.error).toHaveBeenCalledWith('Could not save the quota. Please try again.')
    expect(admin.section.targetUserId.value).toBe('')
  })

  test('blocks forbidden saves without transport', async () => {
    const admin = mountSection(true, 'usr_b')
    await waitFor(() => expect(admin.section.memberOptions.value).toContain('usr_owner'))

    admin.section.pickTargetUser('usr_a')
    expect(admin.section.targetUserId.value).toBe('usr_a')
    // A forbidden target armed outside the whitelist still never sends.
    admin.section.targetUserId.value = 'usr_owner'
    await expect(admin.section.saveQuota()).resolves.toBe(false)
    expect(quotaTransport.update).not.toHaveBeenCalled()
    expect(toast.error).toHaveBeenCalledWith('Could not save the quota. Please try again.')

    vi.clearAllMocks()
    admin.section.pickTargetPeriod('unlimited')
    await expect(admin.section.saveQuota()).resolves.toBe(false)
    expect(quotaTransport.update).not.toHaveBeenCalled()
  })

  test('allows matrix-permitted saves through the transport', async () => {
    const owner = mountSection(true, 'usr_owner')
    await waitFor(() => expect(owner.section.memberOptions.value).toContain('usr_owner'))

    owner.section.pickTargetUser('usr_b')
    owner.section.limitDraft.value = '150'
    await expect(owner.section.saveQuota()).resolves.toBe(true)
    expect(quotaTransport.update).toHaveBeenCalledWith('ws_123', 'usr_b', {
      period: 'daily',
      limitCredits: 150,
    })
    expect(toast.success).toHaveBeenCalledWith('Member quota updated.')
  })

  test.each(['usr_a', 'usr_ghost'])(
    'allows Org Admin %s despite a missing Workspace manager role',
    async (actor) => {
      const admin = mountSection(true, actor, [
        { userId: actor, role: 'admin' },
        { userId: 'usr_owner', role: 'member' },
        { userId: 'usr_b', role: 'owner' },
      ])
      await waitFor(() => expect(admin.section.memberOptions.value).toContain('usr_owner'))

      expect(admin.section.canManage.value).toBe(true)
      if (actor === 'usr_ghost') expect(admin.section.memberOptions.value).not.toContain(actor)
      expect(admin.section.memberLabel('usr_unknown')).toBe('usr_unknown')
      admin.section.pickTargetUser('usr_owner')
      admin.section.limitDraft.value = '150'
      await expect(admin.section.saveQuota()).resolves.toBe(true)
      expect(quotaTransport.update).toHaveBeenCalledWith('ws_123', 'usr_owner', {
        period: 'daily',
        limitCredits: 150,
      })
      admin.section.pickTargetPeriod('unlimited')
      for (const userId of ['usr_b', 'usr_c']) {
        admin.section.targetUserId.value = userId
        await expect(admin.section.saveQuota()).resolves.toBe(false)
      }
      expect(quotaTransport.update).toHaveBeenCalledTimes(1)
    },
  )
})
