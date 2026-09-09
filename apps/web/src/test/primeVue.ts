import type { PrimeVueConfiguration } from 'primevue/config'
import { ideaBoardPrimePt } from '../app/primePassThrough'
import { userPrimeLocaleEn } from '../app/primeLocale'
import { ideaBoardAdminPrimeLocale } from '../features/admin/app/primeLocale'
import { ideaBoardPrimePreset } from '../app/primeTheme'

const sharedPrimeVueTestOptions = {
  pt: ideaBoardPrimePt,
  ptOptions: {
    mergeSections: true,
    mergeProps: true,
  },
  theme: {
    preset: ideaBoardPrimePreset,
    options: {
      prefix: 'p',
      darkModeSelector: '.dark',
      cssLayer: false,
    },
  },
} satisfies Omit<PrimeVueConfiguration, 'locale'>

export const userPrimeVueTestOptions = {
  ...sharedPrimeVueTestOptions,
  locale: userPrimeLocaleEn,
} satisfies PrimeVueConfiguration

export const adminPrimeVueTestOptions = {
  ...sharedPrimeVueTestOptions,
  locale: ideaBoardAdminPrimeLocale,
} satisfies PrimeVueConfiguration
