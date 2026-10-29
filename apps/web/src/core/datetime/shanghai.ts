export const SHANGHAI_TIME_ZONE = 'Asia/Shanghai'

export function toShanghaiParts(
  date: Date,
  formatter: Intl.DateTimeFormat,
): Record<string, string> {
  const result: Record<string, string> = {}
  for (const part of formatter.formatToParts(date)) {
    if (part.type !== 'literal') result[part.type] = part.value
  }
  return result
}

let wallPartsFormat: Intl.DateTimeFormat | null = null
let monthShortFormat: Intl.DateTimeFormat | null = null
let inputPartsFormat: Intl.DateTimeFormat | null = null

/** Monitor wall parts (`en-US`, numeric date + `h23` time). */
export function getShanghaiWallPartsFormat(): Intl.DateTimeFormat {
  if (!wallPartsFormat) {
    wallPartsFormat = new Intl.DateTimeFormat('en-US', {
      timeZone: SHANGHAI_TIME_ZONE,
      year: 'numeric',
      month: 'numeric',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hourCycle: 'h23',
    })
  }
  return wallPartsFormat
}

/** Monitor axis month short name (`en-US`, Shanghai zone). */
export function getShanghaiMonthShortFormat(): Intl.DateTimeFormat {
  if (!monthShortFormat) {
    monthShortFormat = new Intl.DateTimeFormat('en-US', {
      timeZone: SHANGHAI_TIME_ZONE,
      month: 'short',
    })
  }
  return monthShortFormat
}

/** Reward `datetime-local` input parts (`en-CA`, `h23`). */
export function getShanghaiInputPartsFormat(): Intl.DateTimeFormat {
  if (!inputPartsFormat) {
    inputPartsFormat = new Intl.DateTimeFormat('en-CA', {
      timeZone: SHANGHAI_TIME_ZONE,
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      hourCycle: 'h23',
    })
  }
  return inputPartsFormat
}
