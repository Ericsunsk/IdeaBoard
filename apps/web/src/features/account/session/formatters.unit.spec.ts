import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { i18n } from '../../../core/i18n'
import {
  formatAccountExpiryDateTime,
  formatAccountLocalDateTime,
  formatAccountRechargeDateTime,
  formatAccountShanghaiDateTime,
} from './formatters'

const localeTags = {
  en: 'en-US',
  'zh-CN': 'zh-CN',
  es: 'es-ES',
  ko: 'ko-KR',
} as const

const localDateTimeOptions = {
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hourCycle: 'h23',
} as const satisfies Intl.DateTimeFormatOptions

describe('formatters', () => {
  beforeEach(() => {
    i18n.global.locale.value = 'en'
  })

  afterEach(() => {
    i18n.global.locale.value = 'en'
  })

  it.each(['en', 'zh-CN', 'es', 'ko'] as const)(
    'uses %s conventions for device-local account timestamps',
    (locale) => {
      const date = new Date('2026-07-14T09:08:07')
      ;(i18n.global.locale as { value: string }).value = locale

      expect(formatAccountLocalDateTime(date)).toBe(
        new Intl.DateTimeFormat(localeTags[locale], localDateTimeOptions).format(date),
      )
      expect(formatAccountRechargeDateTime(date)).toBe(
        new Intl.DateTimeFormat(localeTags[locale], {
          year: 'numeric',
          month: '2-digit',
          day: '2-digit',
          hour: '2-digit',
          minute: '2-digit',
          hourCycle: 'h23',
        }).format(date),
      )
    },
  )

  it.each(['en', 'zh-CN', 'es', 'ko'] as const)(
    'uses %s conventions while retaining the reward ledger Shanghai timezone',
    (locale) => {
      const date = new Date('2026-07-14T09:08:07Z')
      ;(i18n.global.locale as { value: string }).value = locale

      expect(formatAccountShanghaiDateTime(date)).toBe(
        new Intl.DateTimeFormat(localeTags[locale], {
          ...localDateTimeOptions,
          timeZone: 'Asia/Shanghai',
        }).format(date),
      )
    },
  )

  it('formats pending-order expiry with the shared medium-date short-time shape', () => {
    ;(i18n.global.locale as { value: string }).value = 'en'
    const date = new Date('2026-08-01T01:00:00')
    expect(formatAccountExpiryDateTime(date)).toBe(
      new Intl.DateTimeFormat('en-US', {
        dateStyle: 'medium',
        timeStyle: 'short',
        hour12: false,
      }).format(date),
    )
  })

  it('uses the active locale at render time rather than retaining the initial formatter locale', () => {
    const date = new Date('2026-07-14T09:08:07')

    expect(formatAccountLocalDateTime(date)).toBe(
      new Intl.DateTimeFormat('en-US', localDateTimeOptions).format(date),
    )

    ;(i18n.global.locale as { value: string }).value = 'es'

    expect(formatAccountLocalDateTime(date)).toBe(
      new Intl.DateTimeFormat('es-ES', localDateTimeOptions).format(date),
    )
  })
})
