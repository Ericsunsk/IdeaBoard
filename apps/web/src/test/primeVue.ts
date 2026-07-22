import type { PrimeVueConfiguration } from 'primevue/config'
import { ideaBoardPrimePt } from '../app/primePassThrough'
import { ideaBoardPrimeLocale } from '../app/primeLocale'
import { ideaBoardPrimePreset } from '../app/primeTheme'

export const primeVueTestOptions = {
  locale: ideaBoardPrimeLocale,
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
} satisfies PrimeVueConfiguration
