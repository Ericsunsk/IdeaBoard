import { defineFeedback } from '../../../shared'

export type CreditHealthFeedback = {
  healthLoadFailed: (error: unknown) => void
  queueLoadFailed: (error: unknown) => void
  runHistoryLoadFailed: (error: unknown) => void
  runDetailFailed: (error: unknown) => void
}

const CREDIT_HEALTH_COPY = {
  healthLoadFailed: { error: '数据加载失败，请重试。' },
  queueLoadFailed: { error: '异常队列加载失败。' },
  runHistoryLoadFailed: { error: '检查记录加载失败。' },
  runDetailFailed: { error: 'Run 详情加载失败。' },
}

export function useCreditHealthFeedback(): CreditHealthFeedback {
  return defineFeedback<CreditHealthFeedback>(CREDIT_HEALTH_COPY)
}
