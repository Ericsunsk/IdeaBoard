import { beforeEach, describe, expect, test, vi } from 'vitest'
import { defineFeedback } from './defineFeedback'

const toast = vi.hoisted(() => ({
  success: vi.fn(),
  warning: vi.fn(),
  error: vi.fn(),
  info: vi.fn(),
}))

vi.mock('./toast', async (importOriginal) => {
  const mod = await importOriginal<typeof import('./toast')>()
  return { ...mod, useAdminToast: () => toast }
})

beforeEach(() => {
  vi.clearAllMocks()
})

type SampleFeedback = {
  saved: () => void
  greeted: (name: string) => void
  loadFailed: (error: unknown) => void
  toggleFailed: (error: unknown, enabled: boolean) => void
  testPassed: (message: string | null | undefined) => void
  testFailed: (message: string | null | undefined) => void
  skipped: () => void
  warned: (label: string) => void
  blocked: (label: string) => void
}

function useSampleFeedback(): SampleFeedback {
  return defineFeedback<SampleFeedback>({
    saved: { success: '已保存' },
    greeted: { success: (name: string) => `你好，${name}` },
    loadFailed: { error: '加载失败' },
    toggleFailed: { error: (isEnabled: boolean) => (isEnabled ? '启用失败' : '停用失败') },
    testPassed: { successMessage: '连接成功' },
    testFailed: { errorMessage: '连接失败' },
    skipped: { info: '已跳过' },
    warned: { info: (label: string) => `${label}暂未接入` },
    blocked: { errorText: (label: string) => `${label}不可用` },
  })
}

describe('defineFeedback', () => {
  test('routes copy entries to the matching toast tone with safe fallbacks', () => {
    const feedback = useSampleFeedback()

    feedback.saved()
    feedback.greeted('Ada')
    feedback.skipped()
    feedback.loadFailed(new Error('boom'))
    feedback.loadFailed(new Error('dial tcp 127.0.0.1:5432'))
    feedback.toggleFailed(new Error('dial tcp 127.0.0.1:5432'), true)
    feedback.testPassed(null)
    feedback.testPassed('pong')
    feedback.testFailed(undefined)
    feedback.blocked('GPU')
    feedback.warned('S3')

    expect(toast.success.mock.calls.map(([message]) => message)).toEqual([
      '已保存',
      '你好，Ada',
      '连接成功',
      'pong',
    ])
    expect(toast.info.mock.calls.map(([message]) => message)).toEqual(['已跳过', 'S3暂未接入'])
    expect(toast.error.mock.calls.map(([message]) => message)).toEqual([
      'boom',
      '加载失败',
      '启用失败',
      '连接失败',
      'GPU不可用',
    ])
  })

  test('entry/method kind mismatches fail at compile time', () => {
    // 以下每行都必须编译失败（@ts-expect-error）：若某天放行，typecheck 即红，
    // 与运行时单测互为备份。
    // @ts-expect-error - error entry 需要 (error: unknown) 首参
    defineFeedback<{ saved: () => void }>({ saved: { error: '保存失败' } })
    // @ts-expect-error - 静态文案会静默吞掉方法参数
    defineFeedback<{ greeted: (name: string) => void }>({ greeted: { success: '你好' } })
    // @ts-expect-error - successMessage 只属于可选/可空 message 方法
    defineFeedback<{ greeted: (name: string) => void }>({ greeted: { successMessage: '你好' } })
    // error fallback 拼接参数必须与方法 rest 参数一致（@ts-expect-error 必须紧贴报错行）
    defineFeedback<{ toggleFailed: (error: unknown, enabled: boolean) => void }>({
      // @ts-expect-error - error fallback 拼接参数必须与方法 rest 参数一致
      toggleFailed: { error: (enabled: string) => (enabled ? '启用失败' : '停用失败') },
    })
    // @ts-expect-error - 文案表 key 必须与方法名一一对应
    defineFeedback<{ saved: () => void }>({})
  })
})
