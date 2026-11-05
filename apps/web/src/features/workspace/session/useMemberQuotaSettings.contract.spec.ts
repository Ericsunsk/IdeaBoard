import { beforeEach, describe, expect, test, vi } from 'vitest'
import { defineComponent, reactive } from 'vue'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { setUserLocale } from '@/core/i18n'
import { renderWithUser, waitFor } from '@/test/render'
import { mockAppToast, type AppToastMocks } from '@/test/toast'
import { makeMemberQuotaSummary, makeUnlimitedMemberQuota } from '@/test/factories'
import { workspaceQueryKeys } from '../selection/workspaceResource'
import { useMemberQuotaSettings } from './useMemberQuotaSettings'

vi.mock('@/core/composables/useAppToast')

const transport = vi.hoisted(() => ({
  fetchSelf: vi.fn(),
  fetchList: vi.fn(),
  update: vi.fn(),
  fetchOrganizationMembers: vi.fn(),
  fetchRoster: vi.fn(),
}))

vi.mock('@/features/account/session/quota/resource', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/features/account/session/quota/resource')>()
  return {
    ...actual,
    fetchSelfMemberQuota: transport.fetchSelf,
    fetchMemberQuotaList: transport.fetchList,
    updateMemberQuota: transport.update,
  }
})

vi.mock('../selection/workspaceResource', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../selection/workspaceResource')>()
  return { ...actual, fetchOrganizationMembers: transport.fetchOrganizationMembers }
})

vi.mock('@/features/account/session/accountWorkspace', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('@/features/account/session/accountWorkspace')>()
  return {
    ...actual,
    workspaceMembersQueryOptions: () => ({
      queryKey: ['test-quota-roster'],
      queryFn: transport.fetchRoster,
    }),
  }
})

const roster = [
  { userId: 'usr_a', role: 'member', email: 'a@example.com' },
  { userId: 'usr_b', role: 'admin', email: 'b@example.com' },
  { userId: 'usr_owner', role: 'owner', email: 'owner@example.com' },
]

function mountSettings(currentUserId = 'usr_owner') {
  const scope = reactive({
    workspaceId: 'ws_123',
    organizationId: 'org_1',
    currentUserId,
    canManage: true,
    isExpanded: true,
  })
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  let section!: ReturnType<typeof useMemberQuotaSettings>
  const Harness = defineComponent({
    setup() {
      section = useMemberQuotaSettings({
        workspaceId: () => scope.workspaceId,
        organizationId: () => scope.organizationId,
        currentUserId: () => scope.currentUserId,
        canManage: () => scope.canManage,
        isExpanded: () => scope.isExpanded,
      })
      return () => null
    },
  })
  renderWithUser(Harness, { global: { plugins: [[VueQueryPlugin, { queryClient }]] } })
  return {
    scope,
    queryClient,
    get section() {
      return section
    },
  }
}

describe('useMemberQuotaSettings', () => {
  let toast: AppToastMocks

  beforeEach(async () => {
    await setUserLocale('en')
    vi.clearAllMocks()
    toast = mockAppToast()
    transport.fetchSelf.mockResolvedValue(makeUnlimitedMemberQuota())
    transport.fetchList.mockResolvedValue([
      makeMemberQuotaSummary(),
      makeMemberQuotaSummary({ userId: 'usr_listed' }),
    ])
    transport.update.mockResolvedValue(makeMemberQuotaSummary())
    transport.fetchOrganizationMembers.mockResolvedValue(roster)
    transport.fetchRoster.mockResolvedValue(roster)
  })

  test.each(['usr_a', 'usr_org_admin'])(
    'Org Admin %s manages an Org Member despite Workspace ownership or an absent direct grant',
    async (actor) => {
      transport.fetchOrganizationMembers.mockResolvedValue([
        { userId: actor, role: 'admin' },
        { userId: 'usr_owner', role: 'member' },
        { userId: 'usr_b', role: 'owner' },
      ])
      const { section } = mountSettings(actor)
      await waitFor(() => expect(section.canAdjustTarget('usr_owner')).toBe(true))
      expect(section.canManage.value).toBe(true)
      expect(section.canAdjustTarget('usr_b')).toBe(false)
      expect(section.editor.value.members.map((member) => member.value)).toEqual([
        'usr_a',
        'usr_b',
        'usr_listed',
        'usr_owner',
      ])
      expect(section.memberLabel('usr_listed')).toBe('usr_listed')
      expect(section.memberLabel('usr_owner')).toBe('owner@example.com')
      section.openEditDialog(makeMemberQuotaSummary({ userId: 'usr_owner' }))
      section.setLimitDraft('150')
      await section.submitEditDialog()
      expect(transport.update).toHaveBeenCalledWith('ws_123', 'usr_owner', {
        period: 'daily',
        limitCredits: 150,
      })
      expect(section.editor.value.isVisible).toBe(false)
      expect(transport.fetchSelf).not.toHaveBeenCalled()
      expect(transport.fetchOrganizationMembers).toHaveBeenCalledTimes(1)
    },
  )

  test('waits for Organization roles and retries only their failed read', async () => {
    let rejectMembers!: (reason: Error) => void
    transport.fetchOrganizationMembers.mockImplementationOnce(
      () =>
        new Promise((_, reject) => {
          rejectMembers = reject
        }),
    )
    const { section } = mountSettings()
    await waitFor(() => expect(transport.fetchOrganizationMembers).toHaveBeenCalledTimes(1))
    expect(section.isListLoading.value).toBe(true)
    expect(section.canAdjustTarget('usr_a')).toBe(false)
    rejectMembers(new Error('roles unavailable'))
    await waitFor(() => expect(section.hasListError.value).toBe(true))
    expect(section.canAdjustTarget('usr_a')).toBe(false)
    section.retryList()
    await waitFor(() => expect(section.canAdjustTarget('usr_a')).toBe(true))
    expect(section.hasListError.value).toBe(false)
    expect(transport.fetchList).toHaveBeenCalledTimes(1)
  })

  test('drops old roles while another Organization or viewer loads, including late responses', async () => {
    const pending: Array<(members: { userId: string; role: string }[]) => void> = []
    transport.fetchOrganizationMembers.mockImplementation(
      () => new Promise((resolve) => pending.push(resolve)),
    )
    const { section, scope } = mountSettings()
    await waitFor(() => expect(pending).toHaveLength(1))
    pending[0]!([
      { userId: 'usr_owner', role: 'owner' },
      { userId: 'usr_a', role: 'member' },
    ])
    await waitFor(() => expect(section.canAdjustTarget('usr_a')).toBe(true))
    scope.organizationId = 'org_2'
    await waitFor(() => expect(pending).toHaveLength(2))
    expect(section.canAdjustTarget('usr_a')).toBe(false)
    scope.currentUserId = 'usr_b'
    await waitFor(() => expect(pending).toHaveLength(3))
    pending[1]!([
      { userId: 'usr_b', role: 'owner' },
      { userId: 'usr_a', role: 'member' },
    ])
    await waitFor(() => expect(transport.fetchOrganizationMembers).toHaveBeenCalledTimes(3))
    expect(section.canAdjustTarget('usr_a')).toBe(false)
    pending[2]!([
      { userId: 'usr_b', role: 'admin' },
      { userId: 'usr_a', role: 'owner' },
    ])
    await waitFor(() => expect(section.isListLoading.value).toBe(false))
    expect(section.canAdjustTarget('usr_a')).toBe(false)
    section.openEditDialog(makeMemberQuotaSummary())
    expect(section.editor.value.isVisible).toBe(false)
    expect(transport.update).not.toHaveBeenCalled()
  })

  test('stops using cached roles after refresh failure and keeps an armed edit open', async () => {
    const { section, queryClient } = mountSettings()
    await waitFor(() => expect(section.canAdjustTarget('usr_a')).toBe(true))
    section.openEditDialog(makeMemberQuotaSummary())
    transport.fetchOrganizationMembers.mockRejectedValueOnce(new Error('refresh failed'))
    await queryClient.invalidateQueries({
      queryKey: workspaceQueryKeys.organizationMembers('org_1'),
    })
    expect(section.hasListError.value).toBe(true)
    expect(section.canAdjustTarget('usr_a')).toBe(false)
    await section.submitEditDialog()
    expect(transport.update).not.toHaveBeenCalled()
    expect(section.editor.value.isVisible).toBe(true)
    expect(toast.error).toHaveBeenCalledWith('Could not save the quota. Please try again.')
  })

  test.each(['capability', 'viewer', 'role'] as const)(
    'rechecks %s after unlimited confirmation and never sends when authority was revoked',
    async (revocation) => {
      const { section, scope, queryClient } = mountSettings()
      await waitFor(() => expect(section.canAdjustTarget('usr_a')).toBe(true))
      section.openEditDialog(makeMemberQuotaSummary())
      section.pickTargetPeriod('unlimited')
      const submitted = section.submitEditDialog()
      expect(section.confirmation.value.isVisible).toBe(true)
      expect(section.confirmation.value.intent).toBe('warn')
      expect(transport.update).not.toHaveBeenCalled()
      if (revocation === 'capability') scope.canManage = false
      if (revocation === 'viewer') scope.currentUserId = 'usr_a'
      if (revocation === 'role') {
        transport.fetchOrganizationMembers.mockResolvedValue([
          { userId: 'usr_owner', role: 'member' },
          { userId: 'usr_a', role: 'member' },
        ])
        await queryClient.invalidateQueries({
          queryKey: workspaceQueryKeys.organizationMembers('org_1'),
        })
      }
      await section.confirmUnlimited()
      await submitted
      expect(transport.update).not.toHaveBeenCalled()
      expect(section.editor.value.isVisible).toBe(true)
      expect(toast.error).toHaveBeenCalledWith('Could not save the quota. Please try again.')
    },
  )

  test('owns draft reset and confirmation through successful dismissal', async () => {
    const { section } = mountSettings()
    await waitFor(() => expect(section.canAdjustTarget('usr_a')).toBe(true))
    section.openEditDialog(makeMemberQuotaSummary({ period: 'fixed', limitCredits: 200 }))
    expect(section.editor.value).toMatchObject({
      isVisible: true,
      mode: 'edit',
      member: 'usr_a',
      period: 'fixed',
      limit: '200',
    })
    section.closeEditDialog()
    section.openCreateDialog()
    expect(section.editor.value).toMatchObject({
      isVisible: true,
      mode: 'create',
      member: '',
      period: 'daily',
      limit: '',
    })
    section.pickTargetUser('usr_a')
    section.pickTargetPeriod('unlimited')
    const submitted = section.submitEditDialog()
    expect(transport.update).not.toHaveBeenCalled()
    await section.confirmUnlimited()
    await submitted
    expect(transport.update).toHaveBeenCalledWith('ws_123', 'usr_a', { period: 'unlimited' })
    expect(section.editor.value.isVisible).toBe(false)
    expect(toast.success).toHaveBeenCalledWith('Member quota updated.')
  })
})
