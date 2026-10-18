import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { setUserLocale } from '../../../../core/i18n'
import { queryClient } from '../../../../core/query/queryClient'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
import { mockAppToast } from '../../../../test/toast'
import { primeAuthenticatedWorkspace, selectWorkspace } from '../../testing'
import { boardQueryKeys } from '../../session/sync/boardQueryKeys'
import { useSharePanel } from '../../session/share/useSharePanel'
import type { ShareTransport } from '../../session/share/commands'
import type {
  WorkspaceBoardAccessFact,
  WorkspaceBoardMeta,
} from '../../session/adapters/boardsResource'
import ShareDialog from './ShareDialog.vue'

vi.mock('../../../../core/composables/useAppToast')
const WORKSPACE = 'ws_share123'
const BOARD = 'board_share1'
const creator = 'usr_creator'
const member = 'usr_member'
const other = 'usr_other'
const panels: Array<ReturnType<typeof useSharePanel>> = []
let pointerStyle: HTMLStyleElement
let toast: ReturnType<typeof mockAppToast>

function setup(
  options: {
    isCreator?: boolean
    access?: Partial<WorkspaceBoardAccessFact>
    isCached?: boolean
  } = {},
) {
  const isCreator = options.isCreator ?? true
  let access: WorkspaceBoardAccessFact = {
    workspaceAccess: 'viewer',
    memberUserIds: [member],
    accessVersion: 2,
    accessState: 'workspace',
    ...options.access,
  }
  const meta: WorkspaceBoardMeta = {
    id: BOARD,
    name: 'Campaign',
    revision: 1,
    schemaVersion: 1,
    createdAt: 1,
    updatedAt: 2,
    contentVersion: '',
    previewScene: null,
    capabilities: { read: true, write: isCreator, manage: isCreator, generate: isCreator },
    accessState: access.accessState,
    workspaceAccess: access.workspaceAccess,
    isCreator,
    hasDirectGrant: !isCreator,
    creatorUserId: creator,
  }
  const transport: ShareTransport = {
    fetchMeta: vi.fn().mockImplementation(async () => meta),
    fetchAccess: vi.fn().mockImplementation(async () => access),
    fetchRoster: vi
      .fn()
      .mockResolvedValue([creator, member, other].map((userId) => ({ userId, role: 'member' }))),
    replaceAccess: vi.fn().mockImplementation(async (_workspace, _board, input, write) => {
      if (write.expectedVersion !== access.accessVersion)
        throw Object.assign(new Error('conflict'), { code: 'board_access_conflict' })
      access = {
        ...input,
        memberUserIds: input.workspaceAccess === 'none' ? [] : [...input.memberUserIds],
        accessVersion: access.accessVersion + 1,
        accessState:
          input.workspaceAccess !== 'none' && input.memberUserIds.length > 0
            ? 'workspace'
            : 'private',
      }
      return access
    }),
  }
  if (options.isCached !== false) queryClient.setQueryData(boardQueryKeys.list(WORKSPACE), [meta])
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const panel = useSharePanel()
  panels.push(panel)
  const view = renderWithUser(ShareDialog, {
    props: { panel, deps: { transport } },
    global: { plugins: [[VueQueryPlugin, { queryClient: client }]] },
  })
  panel.open(BOARD, 'Campaign')
  return { ...view, transport, client, panel, access: () => access, meta }
}

beforeEach(async () => {
  localStorage.clear()
  primeAuthenticatedWorkspace(WORKSPACE)
  await setUserLocale('en')
  toast = mockAppToast()
  queryClient.clear()
  pointerStyle = document.createElement('style')
  pointerStyle.textContent = '.pointer-events-auto{pointer-events:auto}'
  document.head.appendChild(pointerStyle)
})

afterEach(() => {
  panels.splice(0).forEach((panel) => panel.dispose())
  queryClient.clear()
  pointerStyle.remove()
})

describe('selected-member Board sharing', () => {
  test.each([true, false])(
    'pins Creator from Board metadata with catalog cached=%s',
    async (isCached) => {
      const { transport } = setup({ isCached })
      expect(await screen.findByRole('button', { name: /usr_creator/ })).toBeDisabled()
      expect(transport.fetchMeta).toHaveBeenCalledTimes(isCached ? 0 : 1)
    },
  )

  test('pins Creator, displays only selection controls and uses one common role', async () => {
    const { user, transport, access } = setup()
    expect(await screen.findByRole('button', { name: /usr_creator/ })).toBeDisabled()
    const selected = await screen.findByRole('button', { name: /usr_member/ })
    await waitFor(() => expect(selected).toBeEnabled())
    expect(selected).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByRole('button', { name: /usr_other/ })).toHaveAttribute(
      'aria-pressed',
      'false',
    )
    expect(screen.queryByRole('button', { name: 'Make private' })).not.toBeInTheDocument()
    expect(screen.queryByRole('menuitemradio')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Editor' }))
    await waitFor(() => expect(access().workspaceAccess).toBe('editor'))
    expect(transport.replaceAccess).toHaveBeenCalledWith(
      WORKSPACE,
      BOARD,
      { workspaceAccess: 'editor', memberUserIds: [member] },
      expect.anything(),
    )
    expect(screen.getByRole('button', { name: /usr_member/ })).toHaveAccessibleName(
      'usr_member, Editor',
    )
  })

  test('toggling a member replaces the selected set without changing the shared role', async () => {
    const { user, transport } = setup()
    const target = await screen.findByRole('button', { name: /usr_other/ })
    await waitFor(() => expect(target).toBeEnabled())
    await user.click(target)
    await waitFor(() => expect(target).toHaveAttribute('aria-pressed', 'true'))
    expect(transport.replaceAccess).toHaveBeenCalledWith(
      WORKSPACE,
      BOARD,
      { workspaceAccess: 'viewer', memberUserIds: [member, other] },
      expect.anything(),
    )
  })

  test('Off clears selections atomically and re-On starts with nobody selected', async () => {
    const { user, transport } = setup()
    const toggle = await screen.findByRole('button', { name: 'Off' })
    await waitFor(() => expect(toggle).toBeEnabled())
    await user.click(toggle)
    await waitFor(() =>
      expect(screen.getByRole('button', { name: /usr_member/ })).toHaveAttribute(
        'aria-pressed',
        'false',
      ),
    )
    expect(transport.replaceAccess).toHaveBeenNthCalledWith(
      1,
      WORKSPACE,
      BOARD,
      { workspaceAccess: 'none', memberUserIds: [] },
      expect.anything(),
    )
    await waitFor(() => expect(screen.getByRole('button', { name: 'On' })).toBeEnabled())
    await user.click(screen.getByRole('button', { name: 'On' }))
    await waitFor(() => expect(transport.replaceAccess).toHaveBeenCalledTimes(2))
    expect(transport.replaceAccess).toHaveBeenLastCalledWith(
      WORKSPACE,
      BOARD,
      { workspaceAccess: 'viewer', memberUserIds: [] },
      expect.anything(),
    )
    expect(screen.getByRole('button', { name: /usr_member/ })).toHaveAttribute(
      'aria-pressed',
      'false',
    )
  })

  test('selected readers see their permission without a sharing mutation surface', async () => {
    const { transport } = setup({ isCreator: false })
    expect(await screen.findByText('You can view this board.')).toBeVisible()
    expect(screen.queryByRole('button', { name: 'Off' })).not.toBeInTheDocument()
    expect(transport.fetchAccess).not.toHaveBeenCalled()
    expect(transport.fetchRoster).not.toHaveBeenCalled()
  })

  test('a stale conflict refreshes the resource without compensating writes', async () => {
    const { user, transport } = setup()
    vi.mocked(transport.replaceAccess).mockRejectedValueOnce(
      Object.assign(new Error('conflict'), { code: 'board_access_conflict' }),
    )
    const target = await screen.findByRole('button', { name: /usr_other/ })
    await waitFor(() => expect(target).toBeEnabled())
    await user.click(target)
    await waitFor(() => expect(toast.warning).toHaveBeenCalled())
    expect(transport.replaceAccess).toHaveBeenCalledOnce()
    expect(target).toHaveAttribute('aria-pressed', 'false')
  })

  test('unknown outcome retry reuses the same attempt rather than minting a new write', async () => {
    const { user, transport } = setup()
    vi.mocked(transport.replaceAccess).mockRejectedValueOnce(new Error('network'))
    const target = await screen.findByRole('button', { name: /usr_other/ })
    await waitFor(() => expect(target).toBeEnabled())
    await user.click(target)
    await waitFor(() => expect(target).toBeEnabled())
    await user.click(target)
    await waitFor(() => expect(transport.replaceAccess).toHaveBeenCalledTimes(2))
    const calls = vi.mocked(transport.replaceAccess).mock.calls
    expect(calls[0]?.[3]).toEqual(calls[1]?.[3])
  })

  test('a permission downgrade revalidates readable Board metadata and does not close the panel', async () => {
    const { user, transport, meta, panel } = setup()
    const target = await screen.findByRole('button', { name: /usr_other/ })
    await waitFor(() => expect(target).toBeEnabled())
    vi.mocked(transport.fetchMeta).mockResolvedValue({
      ...meta,
      isCreator: false,
      capabilities: { read: true, write: false, manage: false, generate: false },
    })
    vi.mocked(transport.replaceAccess).mockRejectedValueOnce(
      Object.assign(new Error('forbidden'), { code: 'board_forbidden' }),
    )
    await user.click(target)
    expect(await screen.findByText('You can view this board.')).toBeVisible()
    expect(panel.isOpen.value).toBe(true)
  })

  test('A→B→A does not emit success for a write from the old Share residency', async () => {
    const { user, transport } = setup()
    let resolve!: (fact: WorkspaceBoardAccessFact) => void
    vi.mocked(transport.replaceAccess).mockImplementationOnce(
      () =>
        new Promise((done) => {
          resolve = done
        }),
    )
    const target = await screen.findByRole('button', { name: /usr_other/ })
    await waitFor(() => expect(target).toBeEnabled())
    await user.click(target)
    await waitFor(() => expect(transport.replaceAccess).toHaveBeenCalledOnce())
    selectWorkspace('ws_share456')
    selectWorkspace(WORKSPACE)
    resolve({
      workspaceAccess: 'viewer',
      memberUserIds: [member, other],
      accessVersion: 3,
      accessState: 'workspace',
    })
    await waitFor(() => expect(toast.success).not.toHaveBeenCalled())
  })
})
