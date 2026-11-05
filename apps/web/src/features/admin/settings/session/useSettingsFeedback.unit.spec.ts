import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  formatAdminBrandingValidation,
  useAdminAccessSettingsFeedback,
  useAdminBrandingSettingsFeedback,
  useAdminMailSettingsFeedback,
  useAdminMediaUploadSettingsFeedback,
  useAdminOutboundPolicyFeedback,
  useAdminSearchProviderFeedback,
  useAdminSupportChannelsFeedback,
} from './useSettingsFeedback'

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
      '邮件设置已保存，请发送 Test Email',
      'Test Email 已发送',
      '邮件设置已删除',
    ])

    // 内部错误（dial tcp）收敛到 fallback，不透出底层细节。
    const internalError = new Error('dial tcp 127.0.0.1:5432: connection refused')
    feedback.loadFailed(internalError)
    feedback.saveFailed(internalError)
    feedback.testFailed(internalError)
    feedback.clearFailed(internalError)
    expect(toast.error.mock.calls.map(([message]) => message)).toEqual([
      '加载邮件设置失败',
      '保存邮件设置失败',
      '发送 Test Email 失败',
      '删除邮件设置失败',
    ])

    // 含 secret 的报错同样收敛到 fallback。
    feedback.saveFailed(new Error('api-key: sk_live_abc123'))
    expect(toast.error).toHaveBeenLastCalledWith('保存邮件设置失败')
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
      '加载图片搜索供应商设置失败',
      '保存 API Key 失败',
      '连接测试失败',
      '启用失败',
      '停用失败',
    ])
  })
})

describe('Admin branding settings feedback', () => {
  it.each([
    ['', ''],
    ['nameRequired', '项目名称不能为空'],
    ['nameTooLong', '项目名称不能超过 40 个字符'],
    ['logoTooLong', 'Logo 地址不能超过 2048 个字符'],
    ['logoInvalid', 'Logo 地址必须为空、站内 Logo 链接或 http(s) URL'],
  ] as const)('presents semantic validation %s with colocated Chinese copy', (error, copy) => {
    expect(formatAdminBrandingValidation(error)).toBe(copy)
  })

  it('covers save/logo pairs with safe fallbacks and validation passthrough', () => {
    const feedback = useAdminBrandingSettingsFeedback()

    feedback.saved()
    feedback.logoUploaded()
    expect(toast.success.mock.calls.map(([message]) => message)).toEqual([
      '项目品牌已保存',
      'Logo 已上传，保存后生效',
    ])

    feedback.validationFailed('品牌名称不能为空')
    expect(toast.error).toHaveBeenCalledWith('品牌名称不能为空')

    const internalError = new Error('dial tcp 127.0.0.1:5432: connection refused')
    feedback.loadFailed(internalError)
    feedback.saveFailed(internalError)
    feedback.logoUploadFailed(internalError)
    expect(toast.error.mock.calls.map(([message]) => message)).toEqual([
      '品牌名称不能为空',
      '加载系统设置失败',
      '保存项目品牌失败',
      '上传 Logo 失败',
    ])
  })
})

describe('Admin access settings feedback', () => {
  it('covers rate-limit/login-lockout/registration pairs with safe fallbacks', () => {
    const feedback = useAdminAccessSettingsFeedback()

    feedback.rateLimitSaved()
    feedback.loginLockoutSaved()
    feedback.registrationUpdated(true)
    feedback.registrationUpdated(false)
    expect(toast.success.mock.calls.map(([message]) => message)).toEqual([
      '访问限流已保存',
      '登录锁定已保存',
      '已开放注册',
      '已关闭注册',
    ])

    feedback.rateLimitValidationFailed('窗口必须大于 0')
    feedback.loginLockoutValidationFailed('阈值必须大于 0')
    expect(toast.error.mock.calls.map(([message]) => message)).toEqual([
      '窗口必须大于 0',
      '阈值必须大于 0',
    ])

    const internalError = new Error('dial tcp 127.0.0.1:5432: connection refused')
    feedback.loadFailed(internalError)
    feedback.rateLimitSaveFailed(internalError)
    feedback.loginLockoutSaveFailed(internalError)
    feedback.registrationUpdateFailed(internalError)
    expect(toast.error.mock.calls.map(([message]) => message)).toEqual([
      '窗口必须大于 0',
      '阈值必须大于 0',
      '加载访问控制设置失败',
      '保存访问限流失败',
      '保存登录锁定失败',
      '更新注册状态失败',
    ])
  })
})

describe('Admin generic setting feedback', () => {
  it.each([
    [
      'media upload',
      useAdminMediaUploadSettingsFeedback,
      '加载上传设置失败',
      '上传设置已保存',
      '保存上传设置失败',
    ],
    [
      'outbound policy',
      useAdminOutboundPolicyFeedback,
      '加载私有地址访问策略失败',
      '私有地址访问策略已保存',
      '保存私有地址访问策略失败',
    ],
    [
      'support channels',
      useAdminSupportChannelsFeedback,
      '读取客户支持失败',
      '客户支持已保存',
      '保存客户支持失败',
    ],
  ] as const)(
    '%s covers load/save pairs with safe fallbacks',
    (_label, useFeedback, loadFallback, savedMessage, saveFallback) => {
      const feedback = useFeedback()

      feedback.saved()
      expect(toast.success).toHaveBeenCalledWith(savedMessage)

      feedback.validationFailed('草稿无效')
      expect(toast.error).toHaveBeenCalledWith('草稿无效')

      const internalError = new Error('dial tcp 127.0.0.1:5432: connection refused')
      feedback.loadFailed(internalError)
      feedback.saveFailed(internalError)
      expect(toast.error.mock.calls.map(([message]) => message)).toEqual([
        '草稿无效',
        loadFallback,
        saveFallback,
      ])
      feedback.saveFailed(new Error('api-key: sk_live_abc123'))
      expect(toast.error).toHaveBeenLastCalledWith(saveFallback)
    },
  )
})
