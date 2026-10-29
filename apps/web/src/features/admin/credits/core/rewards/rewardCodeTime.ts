import { parseDateGuard } from '@/core/datetime/parse'
import { getShanghaiInputPartsFormat, toShanghaiParts } from '@/core/datetime/shanghai'

const DATE_TIME_INPUT_PATTERN = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})$/

export function shanghaiDateTimeInputToPickerDate(value: string): Date | null {
  const match = DATE_TIME_INPUT_PATTERN.exec(value.trim())
  if (!match) return null
  const [, year, month, day, hour, minute] = match
  const date = new Date(Number(year), Number(month) - 1, Number(day), Number(hour), Number(minute))
  return pickerDateToShanghaiDateTimeInput(date) === value.trim() ? date : null
}

export function pickerDateToShanghaiDateTimeInput(value: Date | null | undefined): string {
  if (!value || Number.isNaN(value.getTime())) return ''
  const pad = (part: number) => String(part).padStart(2, '0')
  return `${value.getFullYear()}-${pad(value.getMonth() + 1)}-${pad(value.getDate())}T${pad(value.getHours())}:${pad(value.getMinutes())}`
}

// 过期 preset 的 wall 日历计算：取此刻的上海 wall 日期加天，落 23:59。
// 不经本地时区中转，否则浏览器非 Asia/Shanghai 且两地日期不一致时会差一天。
export function shanghaiEndOfDayPlusDays(days: number, now: Date = new Date()): string {
  if (!parseDateGuard(now)) return ''
  const wallDate = /^(\d{4})-(\d{2})-(\d{2})T/.exec(isoToShanghaiDateTimeInput(now.toISOString()))
  if (!wallDate) return ''
  const target = new Date(
    Number(wallDate[1]),
    Number(wallDate[2]) - 1,
    Number(wallDate[3]) + days,
    23,
    59,
  )
  return pickerDateToShanghaiDateTimeInput(target)
}

export function isoToShanghaiDateTimeInput(value: string): string {
  const date = parseDateGuard(value)
  if (!date) return ''

  const entries = toShanghaiParts(date, getShanghaiInputPartsFormat())

  if (!entries.year || !entries.month || !entries.day || !entries.hour || !entries.minute) {
    return ''
  }
  return `${entries.year}-${entries.month}-${entries.day}T${entries.hour}:${entries.minute}`
}
