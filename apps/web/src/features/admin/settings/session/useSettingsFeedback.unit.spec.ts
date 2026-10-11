import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAdminMailSettingsFeedback, useAdminSearchProviderFeedback } from './useSettingsFeedback'

const toast = vi.hoisted(() => ({
  success: vi.fn(),
  warning: vi.fn(),
  error: vi.fn(),
  info: vi.fn(),
}))

vi.mock('../../shared/session/feedback/toast', async (importOriginal) => {
  const mod = await importOriginal<typeof import('../../shared/session/feedback/toast')>()
  return { ...mod, useAdminToast: () => toast }
})

beforeEach(() => {
  vi.clearAllMocks()
})

// 对称约定（以图库 image-library 为准）：每个成功 toast 都有失败 toast 对应，
// 失败走 formatAdminDisplayError + 固定 fallback，不透出内部错误与 secret。
describe('Admin mail settings feedback', () => {
  it('covers save/test/clear/load success-failure pairs with safe fallbacks', () => {
    const feedback = useAdminMailSettingsFeedback()

    feedback.saved()
    feedback.testSent()
    feedback.cleared()
    expect(toast.success.mock.calls.map(([message]) => message)).toEqual([
      '邮件配置已保存，请发送 Test Email',
      'Test Email 已发送',
      '邮件配置已删除',
    ])

    // 内部错误（dial tcp）收敛到 fallback，不透出底层细节。
    const internalError = new Error('dial tcp 127.0.0.1:5432: connection refused')
    feedback.loadFailed(internalError)
    feedback.saveFailed(internalError)
    feedback.testFailed(internalError)
    feedback.clearFailed(internalError)
    expect(toast.error.mock.calls.map(([message]) => message)).toEqual([
      '加载邮件配置失败',
      '保存邮件配置失败',
      '发送 Test Email 失败',
      '删除邮件配置失败',
    ])

    // 含 secret 的报错同样收敛到 fallback。
    feedback.saveFailed(new Error('api-key: sk_live_abc123'))
    expect(toast.error).toHaveBeenLastCalledWith('保存邮件配置失败')
  })
})

describe('Admin search provider feedback', () => {
  it('covers key-save/test/toggle/load success-failure pairs with safe fallbacks', () => {
    const feedback = useAdminSearchProviderFeedback()

    feedback.keySaved('Unsplash', false)
    feedback.keySaved('Pexels', true)
    feedback.testPassed('Unsplash')
    feedback.toggled('Unsplash', true)
    feedback.toggled('Unsplash', false)
    expect(toast.success.mock.calls.map(([message]) => message)).toEqual([
      'Unsplash API Key 已保存，请测试连接',
      'Pexels API Key 已保存并已停用，请测试连接后重新启用',
      'Unsplash 连接测试通过',
      'Unsplash 已启用',
      'Unsplash 已停用',
    ])

    const internalError = new Error('dial tcp 127.0.0.1:5432: connection refused')
    feedback.loadFailed(internalError)
    feedback.saveFailed(internalError)
    feedback.testFailed(internalError)
    feedback.toggleFailed(internalError, true)
    feedback.toggleFailed(internalError, false)
    expect(toast.error.mock.calls.map(([message]) => message)).toEqual([
      '加载图片搜索供应商配置失败',
      '保存 API Key 失败',
      '连接测试失败',
      '启用失败',
      '停用失败',
    ])
  })
})
