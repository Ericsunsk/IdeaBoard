import { describe, expect, it } from 'vitest'
import { ref } from 'vue'
import {
  calculateTimelineYScale,
  formatXAxisTime,
  toGmt8Date,
  useMonitorTimeline,
} from './useMonitorTimeline'
import type { AdminTaskRow } from './useAdminTaskList'

describe('useMonitorTimeline', () => {
  describe('calculateTimelineYScale', () => {
    it('maintains the baseline 0-10 scale with 5 intervals when max <= 10', () => {
      expect(calculateTimelineYScale(0)).toEqual({ max: 10, stepSize: 2 })
      // min:10 先兜底：小计数保持 10，只有跨过 10/headroom 才上探。
      expect(calculateTimelineYScale(1)).toEqual({ max: 10, stepSize: 2 })
      expect(calculateTimelineYScale(5)).toEqual({ max: 10, stepSize: 2 })
      expect(calculateTimelineYScale(10)).toEqual({ max: 15, stepSize: 3 })
    })

    it('dynamically calculates clean headroom and intervals when max > 10', () => {
      // 15 * 1.1 = 16.5 -> nice 20, step 4 (5 intervals)
      const scale15 = calculateTimelineYScale(15)
      expect(scale15.stepSize).toBe(4)
      expect(scale15.max).toBeGreaterThanOrEqual(15)
      expect(scale15.max % scale15.stepSize).toBe(0)

      // 35 * 1.1 = 38.5 -> nice 50, step 10 (5 intervals)
      const scale35 = calculateTimelineYScale(35)
      expect(scale35.stepSize).toBe(10)
      expect(scale35.max).toBeGreaterThanOrEqual(35)
      expect(scale35.max % scale35.stepSize).toBe(0)

      // 100 * 1.1 = 110 -> nice 150, step 30 (5 intervals)
      const scale100 = calculateTimelineYScale(100)
      expect(scale100.stepSize).toBe(30)
      expect(scale100.max).toBeGreaterThanOrEqual(100)
      expect(scale100.max % scale100.stepSize).toBe(0)

      // 1,200 * 1.1 = 1,320 -> nice 1,500, step 300 (5 intervals)
      const scale1200 = calculateTimelineYScale(1200)
      expect(scale1200.stepSize).toBe(300)
      expect(scale1200.max).toBeGreaterThanOrEqual(1200)
      expect(scale1200.max % scale1200.stepSize).toBe(0)
    })
  })

  describe('toGmt8Date', () => {
    it('shifts date correctly to GMT+8', () => {
      const utcDate = new Date('2026-09-19T00:00:00Z')
      const gmt8 = toGmt8Date(utcDate)
      expect(gmt8.getHours()).toBe(8)
    })

    it('preserves the Shanghai wall-clock date parts', () => {
      const gmt8 = toGmt8Date(new Date('2026-09-19T00:00:00Z'))
      expect(gmt8.getFullYear()).toBe(2026)
      expect(gmt8.getMonth()).toBe(8)
      expect(gmt8.getDate()).toBe(19)
      expect(gmt8.getMinutes()).toBe(0)
    })
  })

  describe('formatXAxisTime', () => {
    // 2026-09-19T00:00:00Z is 08:00 on Sep 19 in Asia/Shanghai.
    const instant = new Date('2026-09-19T00:00:00Z')

    it('pins HH:mm for short ranges (<=12h)', () => {
      expect(formatXAxisTime(instant, 60 * 60 * 1000)).toBe('08:00')
      expect(formatXAxisTime(instant, 12 * 3600 * 1000)).toBe('08:00')
    })

    it('pins Mon D, HH:mm for day ranges (<=24h)', () => {
      expect(formatXAxisTime(instant, 20 * 3600 * 1000)).toBe('Sep 19, 08:00')
    })

    it('pins Mon D HH:00 for long ranges (>24h)', () => {
      expect(formatXAxisTime(instant, 48 * 3600 * 1000)).toBe('Sep 19 08:00')
    })

    it('pins midnight correctly (hourCycle h23, never 24:xx)', () => {
      // 2026-09-18T16:00:00Z is exactly 00:00 Shanghai wall time.
      expect(formatXAxisTime(new Date('2026-09-18T16:00:00Z'), 60 * 60 * 1000)).toBe('00:00')
    })
  })

  describe('useMonitorTimeline composable', () => {
    it('constructs dynamic y scale based on data rows', () => {
      const isDemoMode = ref(false)
      const demoRows = ref<AdminTaskRow[]>([])
      const chartRows = ref<AdminTaskRow[]>([
        {
          task_id: '1',
          status: 'completed',
          created_at: new Date().toISOString(),
        } as AdminTaskRow,
      ])
      const filterStatus = ref(null)

      const timeline = useMonitorTimeline({
        isDemoMode,
        demoRows,
        chartRows,
        filterStatus,
      })

      expect(timeline.selectedPreset.value.key).toBe('1h')
      expect(timeline.timelineChartData.value.datasets.length).toBe(5)
      expect(timeline.timelineChartOptions.value.scales?.y).toMatchObject({
        min: 0,
        max: 10,
      })
    })
  })
})
