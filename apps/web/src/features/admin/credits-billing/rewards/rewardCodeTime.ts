const SHANGHAI_OFFSET = '+08:00'
const SHANGHAI_TIME_ZONE = 'Asia/Shanghai'
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

export function shanghaiDateTimeInputToISO(value: string): string | undefined {
  const normalized = value.trim()
  if (!normalized) return undefined
  if (!DATE_TIME_INPUT_PATTERN.test(normalized)) {
    throw new Error('时间格式无效')
  }

  const date = new Date(`${normalized}:00${SHANGHAI_OFFSET}`)
  if (
    Number.isNaN(date.getTime()) ||
    isoToShanghaiDateTimeInput(date.toISOString()) !== normalized
  ) {
    throw new Error('时间格式无效')
  }
  return date.toISOString()
}

export function isoToShanghaiDateTimeInput(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''

  const entries = new Intl.DateTimeFormat('en-CA', {
    timeZone: SHANGHAI_TIME_ZONE,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hourCycle: 'h23',
  })
    .formatToParts(date)
    .reduce<Record<string, string>>((result, part) => {
      if (part.type !== 'literal') result[part.type] = part.value
      return result
    }, {})

  if (!entries.year || !entries.month || !entries.day || !entries.hour || !entries.minute) {
    return ''
  }
  return `${entries.year}-${entries.month}-${entries.day}T${entries.hour}:${entries.minute}`
}
