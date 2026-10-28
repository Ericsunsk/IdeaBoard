import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useStorageFeedback } from './useStorageFeedback'

const toast = vi.hoisted(() => ({
  success: vi.fn(),
  warning: vi.fn(),
  error: vi.fn(),
  info: vi.fn(),
}))

vi.mock('../../../shared/session/feedback/toast', async (importOriginal) => {
  const mod = await importOriginal<typeof import('../../../shared/session/feedback/toast')>()
  return { ...mod, useAdminToast: () => toast }
})

beforeEach(() => {
  vi.clearAllMocks()
})

// 对称约定：每个成功 toast 都有失败 toast 对应，失败走 formatAdminDisplayError +
// 固定 fallback，不透出内部错误与 secret。
describe('Admin storage feedback', () => {
  it('covers load/refresh failures with safe fallbacks', () => {
    const feedback = useStorageFeedback()

    const internalError = new Error('dial tcp 127.0.0.1:5432: connection refused')
    feedback.loadProfilesFailed(internalError)
    feedback.refreshFailed(internalError)
    expect(toast.error.mock.calls.map(([message]) => message)).toEqual([
      '加载存储配置失败',
      '刷新存储配置失败',
    ])

    feedback.loadProfilesFailed(new Error('api-key: sk_live_abc123'))
    expect(toast.error).toHaveBeenLastCalledWith('加载存储配置失败')
  })

  it('routes static warnings/info without formatting', () => {
    const feedback = useStorageFeedback()

    feedback.staleProfile()
    feedback.noCreatableProviders()
    feedback.bucketDomainsEmpty()
    expect(toast.warning.mock.calls.map(([message]) => message)).toEqual([
      '存储配置已变化，请刷新后重试',
      '当前没有可新增的存储类型',
      '未检测到绑定域名',
    ])

    feedback.unsupportedVendorPreset('S3')
    expect(toast.info).toHaveBeenCalledWith('S3 暂未接入后端，当前先作为预设展示')

    feedback.missingEditableTarget()
    expect(toast.error).toHaveBeenCalledWith('未找到可编辑的存储配置')
  })

  it('prefers server messages for save/apply/test/delete with safe fallbacks', () => {
    const feedback = useStorageFeedback()

    feedback.saveDraftSucceeded('saved')
    feedback.saveDraftSucceeded(undefined)
    feedback.applyCurrentSucceeded('applied')
    feedback.applyCurrentSucceeded(undefined)
    feedback.testConnectionSucceeded('tested')
    feedback.testConnectionSucceeded(undefined)
    feedback.profileDeleted('deleted')
    feedback.profileDeleted(undefined)
    expect(toast.success.mock.calls.map(([message]) => message)).toEqual([
      'saved',
      '保存成功',
      'applied',
      '已设为当前存储',
      'tested',
      '连接成功',
      'deleted',
      '删除成功',
    ])

    const internalError = new Error('dial tcp 127.0.0.1:5432: connection refused')
    feedback.saveDraftFailed(internalError)
    feedback.applyCurrentFailed(internalError)
    feedback.testConnectionFailed(internalError)
    feedback.fetchBucketDomainsFailed(internalError)
    feedback.deleteProfileFailed(internalError)
    expect(toast.error.mock.calls.map(([message]) => message)).toEqual([
      '保存失败',
      '设为当前失败',
      '连接失败',
      '获取失败',
      '删除失败',
    ])
  })

  it('renders bucket domain counts without formatting', () => {
    const feedback = useStorageFeedback()

    feedback.bucketDomainsDetected(3)
    expect(toast.success).toHaveBeenCalledWith('检测到 3 个域名')
  })
})
