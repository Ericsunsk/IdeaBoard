import { defineFeedback } from '../../../shared/index'
import { PAYMENTS_MESSAGES } from './messages'

export type StripeSettingsFeedback = {
  saved: () => void
  saveFailed: (error: unknown) => void
  loadFailed: (error: unknown) => void
}

const STRIPE_SETTINGS_COPY = {
  saved: { success: PAYMENTS_MESSAGES.stripeSettingsSaved },
  saveFailed: { error: PAYMENTS_MESSAGES.stripeSaveFailed },
  loadFailed: { error: PAYMENTS_MESSAGES.stripeLoadFailed },
}

export function useStripeSettingsFeedback(): StripeSettingsFeedback {
  return defineFeedback<StripeSettingsFeedback>(STRIPE_SETTINGS_COPY)
}
