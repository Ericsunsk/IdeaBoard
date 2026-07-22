import { describe, expect, it } from 'vitest'
import {
  pickerDateToShanghaiDateTimeInput,
  shanghaiDateTimeInputToPickerDate,
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
})
