import { defineFeedback } from '../../../shared'

export type BillingPricingFeedback = {
  combinedSaveFailed: (failedTabs: Array<string>, details: Array<string>) => void
}

const COMBINED_SAVE_FAILED_PREFIX = '部分配置保存失败，请重试'

const BILLING_PRICING_COPY = {
  combinedSaveFailed: {
    errorText: (failedTabs: Array<string>, details: Array<string>) =>
      `${COMBINED_SAVE_FAILED_PREFIX}（${failedTabs.join('、')}）${details.length > 0 ? `：${details.join('；')}` : ''}`,
  },
}

export function useBillingPricingFeedback(): BillingPricingFeedback {
  return defineFeedback<BillingPricingFeedback>(BILLING_PRICING_COPY)
}
