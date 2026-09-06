import type { PrimeVueLocaleOptions } from 'primevue/config'
import { userPrimeLocaleZhCN } from '../../../shared/i18n/primeLocale'

/** Preserves the Admin console's established Chinese date and calendar semantics. */
export const ideaBoardAdminPrimeLocale: PrimeVueLocaleOptions = {
  ...userPrimeLocaleZhCN,
  firstDayOfWeek: 1,
  showMonthAfterYear: true,
  dateFormat: 'yy-mm-dd',
}
