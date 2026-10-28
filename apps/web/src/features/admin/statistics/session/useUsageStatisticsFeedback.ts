import { defineFeedback } from '../../shared'

export type UsageStatisticsFeedback = {
  loadFailed: (error: unknown) => void
}

const USAGE_STATISTICS_COPY = {
  loadFailed: { error: '加载统计数据失败' },
}

export function useUsageStatisticsFeedback(): UsageStatisticsFeedback {
  return defineFeedback<UsageStatisticsFeedback>(USAGE_STATISTICS_COPY)
}
