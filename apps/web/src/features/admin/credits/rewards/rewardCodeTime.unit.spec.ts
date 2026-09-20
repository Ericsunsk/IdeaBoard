import { describe, expect, it } from 'vitest'
import {
  pickerDateToShanghaiDateTimeInput,
  shanghaiDateTimeInputToPickerDate,
  shanghaiEndOfDayPlusDays,
} from './rewardCodeTime'

describe('RewardCode picker wall time', () => {
  it('round-trips a Shanghai wall time through the DatePicker model', () => {
    const value = '2026-08-18T21:35'
    const date = shanghaiDateTimeInputToPickerDate(value)

    expect(date).not.toBeNull()
    expect(pickerDateToShanghaiDateTimeInput(date)).toBe(value)
  })

  it('rejects invalid wall time and clears absent picker values', () => {
    expect(shanghaiDateTimeInputToPickerDate('2026-02-30T08:00')).toBeNull()
    expect(pickerDateToShanghaiDateTimeInput(null)).toBe('')
  })

  it('computes presets on the Shanghai wall calendar regardless of browser zone', () => {
    // 15:30Z is 23:30 in Shanghai but still the previous wall date in UTC-8.
    const evening = new Date('2026-08-18T15:30:00Z')
    expect(shanghaiEndOfDayPlusDays(0, evening)).toBe('2026-08-18T23:59')
    expect(shanghaiEndOfDayPlusDays(7, evening)).toBe('2026-08-25T23:59')
    // Month rollover stays on the wall calendar: 00:00 Shanghai on Feb 1.
    expect(shanghaiEndOfDayPlusDays(0, new Date('2026-01-31T16:00:00Z'))).toBe('2026-02-01T23:59')
    expect(shanghaiEndOfDayPlusDays(1, new Date('invalid'))).toBe('')
  })
})
