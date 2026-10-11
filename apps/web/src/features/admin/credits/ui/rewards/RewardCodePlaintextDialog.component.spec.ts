import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen, waitFor } from '../../../../../test/render'
import RewardCodePlaintextDialog from './RewardCodePlaintextDialog.vue'
import { REWARD_CODES_MESSAGES } from './messages'

const toast = vi.hoisted(() => ({ error: vi.fn(), success: vi.fn() }))
const clipboard = vi.hoisted(() => ({ copyText: vi.fn() }))

vi.mock('../../../shared/session/feedback/toast', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../../shared/session/feedback/toast')>()),
  useAdminToast: () => toast,
}))

vi.mock('../../../../../core/composables/useAppClipboard', () => ({
  useAppClipboard: () => clipboard,
}))

describe('RewardCodePlaintextDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    clipboard.copyText.mockResolvedValue(true)
  })

  it('renders the one-time plaintext with the copy action', async () => {
    renderWithUser(RewardCodePlaintextDialog, {
      props: { visible: true, plaintext: 'ABCD12342026' },
    })

    expect(await screen.findByText('ABCD12342026')).toBeVisible()
    expect(screen.getByRole('button', { name: '复制' })).toBeVisible()
  })

  it('keeps the copied label on success without an error toast', async () => {
    const { user } = renderWithUser(RewardCodePlaintextDialog, {
      props: { visible: true, plaintext: 'ABCD12342026' },
    })

    await user.click(await screen.findByRole('button', { name: '复制' }))

    expect(await screen.findByRole('button', { name: '已复制' })).toBeVisible()
    expect(toast.error).not.toHaveBeenCalled()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('reports a copy failure through toast only without an inline alert', async () => {
    clipboard.copyText.mockResolvedValue(false)
    const { user } = renderWithUser(RewardCodePlaintextDialog, {
      props: { visible: true, plaintext: 'ABCD12342026' },
    })

    await user.click(await screen.findByRole('button', { name: '复制' }))

    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(REWARD_CODES_MESSAGES.plaintextCopyFailed),
    )
    expect(screen.getByRole('button', { name: '复制' })).toBeVisible()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(screen.queryByText(REWARD_CODES_MESSAGES.plaintextCopyFailed)).not.toBeInTheDocument()
  })
})
