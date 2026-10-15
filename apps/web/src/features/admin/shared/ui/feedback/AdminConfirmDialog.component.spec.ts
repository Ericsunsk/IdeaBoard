import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen } from '../../../../../test/render'
import AdminConfirmDialog from './AdminConfirmDialog.vue'
import { activeAdminConfirm, useAdminConfirm } from '../../session/feedback/useAdminConfirm'

const toast = vi.hoisted(() => ({ error: vi.fn() }))

vi.mock('../../session/feedback/toast', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../session/feedback/toast')>()),
  useAdminToast: () => toast,
}))

describe('AdminConfirmDialog', () => {
  beforeEach(() => {
    activeAdminConfirm.value = null
    vi.clearAllMocks()
  })

  it('shows a safe error and keeps the dialog open when confirmation fails', async () => {
    const error = new Error('operation failed')
    const confirmPromise = useAdminConfirm().confirm({
      title: '确认删除',
      message: '删除后不可恢复。',
      confirmLabel: '删除',
      intent: 'danger',
      onConfirm: async () => {
        throw error
      },
    })
    const rejectedConfirm = confirmPromise.catch((reason: unknown) => reason)
    const { user } = renderWithUser(AdminConfirmDialog)

    expect(await screen.findByRole('alertdialog')).toHaveClass('bg-popover')
    await user.click(await screen.findByRole('button', { name: '删除' }))

    await expect(rejectedConfirm).resolves.toBe(error)
    expect(toast.error).toHaveBeenCalledWith('operation failed')
    expect(activeAdminConfirm.value?.open).toBe(true)
    expect(activeAdminConfirm.value?.loading).toBe(false)
  })
})
