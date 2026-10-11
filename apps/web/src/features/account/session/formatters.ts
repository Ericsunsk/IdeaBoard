import { formatLocaleDate } from '../../../core/i18n/formatters'

const localDateTimeOptions = {
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hourCycle: 'h23',
} as const satisfies Intl.DateTimeFormatOptions

const rechargeDateTimeOptions = {
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
} as const satisfies Intl.DateTimeFormatOptions

const shanghaiDateTimeOptions = {
  ...localDateTimeOptions,
  timeZone: 'Asia/Shanghai',
} as const satisfies Intl.DateTimeFormatOptions

const expiryDateTimeOptions = {
  dateStyle: 'medium',
  timeStyle: 'short',
  hour12: false,
} as const satisfies Intl.DateTimeFormatOptions

export function formatAccountRechargeDateTime(value: Date): string {
  return formatLocaleDate(value, rechargeDateTimeOptions)
}

export function formatAccountLocalDateTime(value: Date): string {
  return formatLocaleDate(value, localDateTimeOptions)
}

export function formatAccountShanghaiDateTime(value: Date): string {
  return formatLocaleDate(value, shanghaiDateTimeOptions)
}

export function formatAccountExpiryDateTime(value: Date): string {
  return formatLocaleDate(value, expiryDateTimeOptions)
}
