import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AdminConfirmPort } from './useAdminConfirm'
import {
  ADMIN_PROBE_OPERATOR_REASON,
  ADMIN_REASON_REQUIRED_MESSAGE,
  ADMIN_REVERSIBLE_DEFAULT_REASON,
  confirmSettingsSave,
  readReversibleReason,
  requireDestructiveReason,
} from './confirmSettingsSave'

const toastError = vi.hoisted(() => vi.fn())

vi.mock('./toast', () => ({
  useAdminToast: () => ({
    success: vi.fn(),
    warning: vi.fn(),
    error: toastError,
    info: vi.fn(),
  }),
  formatAdminDisplayError: (error: unknown, fallback: string) =>
    error instanceof Error ? error.message : fallback,
}))

function port(overrides: Partial<AdminConfirmPort> = {}): AdminConfirmPort {
  return {
    confirm: vi.fn().mockResolvedValue(true),
    readReason: () => 'typed reason',
    ...overrides,
  }
}

describe('confirmSettingsSave risk tiers', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('probe skips the dialog and returns the fixed probe note', async () => {
    const confirm = port()

    const reason = await confirmSettingsSave(confirm, {
      title: '测试连接',
      message: 'probe',
      confirmLabel: '测试连接',
      risk: 'probe',
    })

    expect(reason).toBe(ADMIN_PROBE_OPERATOR_REASON)
    expect(confirm.confirm).not.toHaveBeenCalled()
  })

  it('reversible is the default tier and keeps a typed reason', async () => {
    const confirm = port()

    const reason = await confirmSettingsSave(confirm, {
      title: '保存配置',
      message: 'save',
    })

    expect(reason).toBe('typed reason')
    expect(confirm.confirm).toHaveBeenCalledWith(
      expect.objectContaining({ reason: expect.objectContaining({ required: false }) }),
    )
  })

  it('reversible falls back to the default reason when the note is empty', async () => {
    const confirm = port({ readReason: () => '   ' })

    const reason = await confirmSettingsSave(confirm, {
      title: '启用功能',
      message: 'enable',
      confirmLabel: '启用',
    })

    expect(reason).toBe(ADMIN_REVERSIBLE_DEFAULT_REASON)
    expect(toastError).not.toHaveBeenCalled()
  })

  it('reversible returns null without a toast when the operator cancels', async () => {
    const confirm = port({ confirm: vi.fn().mockResolvedValue(false) })

    const reason = await confirmSettingsSave(confirm, {
      title: '保存配置',
      message: 'save',
    })

    expect(reason).toBeNull()
    expect(toastError).not.toHaveBeenCalled()
  })

  it('destructive requires the reason and toasts instead of vanishing on empty', async () => {
    const confirm = port({ readReason: () => '   ' })

    const reason = await confirmSettingsSave(confirm, {
      title: '删除配置',
      message: 'delete',
      confirmLabel: '删除配置',
      risk: 'destructive',
    })

    expect(reason).toBeNull()
    expect(toastError).toHaveBeenCalledWith(ADMIN_REASON_REQUIRED_MESSAGE)
  })

  it('destructive returns the typed reason without a toast', async () => {
    const confirm = port()

    const reason = await confirmSettingsSave(confirm, {
      title: '删除配置',
      message: 'delete',
      confirmLabel: '删除配置',
      risk: 'destructive',
    })

    expect(reason).toBe('typed reason')
    expect(confirm.confirm).toHaveBeenCalledWith(
      expect.objectContaining({
        intent: 'danger',
        reason: expect.objectContaining({ required: true }),
      }),
    )
    expect(toastError).not.toHaveBeenCalled()
  })

  it('requireDestructiveReason toasts and aborts on empty', () => {
    expect(requireDestructiveReason(port({ readReason: () => '' }))).toBeNull()
    expect(toastError).toHaveBeenCalledWith(ADMIN_REASON_REQUIRED_MESSAGE)
    expect(requireDestructiveReason(port())).toBe('typed reason')
  })

  it('readReversibleReason falls back to the default on empty', () => {
    expect(readReversibleReason(port({ readReason: () => '' }))).toBe(
      ADMIN_REVERSIBLE_DEFAULT_REASON,
    )
    expect(readReversibleReason(port())).toBe('typed reason')
  })
})
