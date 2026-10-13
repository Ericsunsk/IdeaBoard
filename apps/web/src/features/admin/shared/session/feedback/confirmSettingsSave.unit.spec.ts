import { describe, expect, it, vi } from 'vitest'
import { confirmSettingsSave } from './confirmSettingsSave'

describe('confirmSettingsSave', () => {
  it('confirms settings saves with boolean approval', async () => {
    const confirm = { confirm: vi.fn().mockResolvedValue(true) }
    await expect(
      confirmSettingsSave(confirm, { title: '保存配置', message: 'save' }),
    ).resolves.toBe(true)
    expect(confirm.confirm).toHaveBeenCalledWith({
      title: '保存配置',
      message: 'save',
      confirmLabel: '保存',
      intent: 'warn',
    })
  })

  it('retains destructive confirmation and cancellation', async () => {
    const confirm = { confirm: vi.fn().mockResolvedValue(false) }
    await expect(
      confirmSettingsSave(confirm, {
        title: '删除配置',
        message: 'delete',
        confirmLabel: '删除配置',
        risk: 'destructive',
      }),
    ).resolves.toBe(false)
    expect(confirm.confirm).toHaveBeenCalledWith({
      title: '删除配置',
      message: 'delete',
      confirmLabel: '删除配置',
      intent: 'danger',
    })
  })
})
