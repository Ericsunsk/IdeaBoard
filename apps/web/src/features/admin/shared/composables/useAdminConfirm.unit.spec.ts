import { describe, expect, it, vi } from 'vitest'
import { useAdminConfirm } from './useAdminConfirm'

const confirmMock = vi.hoisted(() => ({
  require: vi.fn(),
}))

vi.mock('primevue/useconfirm', () => ({
  useConfirm: () => confirmMock,
}))

describe('useAdminConfirm', () => {
  it('maps admin confirm requests to the PrimeVue confirmation adapter', () => {
    const onConfirm = vi.fn().mockResolvedValue(undefined)
    const adminConfirm = useAdminConfirm()

    adminConfirm.request({
      title: '确认删除',
      message: '确定删除吗？',
      confirmLabel: '确定',
      intent: 'danger',
      onConfirm,
    })

    expect(confirmMock.require).toHaveBeenCalledWith(
      expect.objectContaining({
        header: '确认删除',
        message: '确定删除吗？',
        rejectLabel: '取消',
        acceptLabel: '确定',
        acceptProps: { severity: 'danger' },
        rejectProps: { severity: 'secondary', outlined: true },
      }),
    )

    const accept = confirmMock.require.mock.calls[0]?.[0].accept
    expect(typeof accept).toBe('function')
    accept()

    expect(onConfirm).toHaveBeenCalledTimes(1)
  })
})
