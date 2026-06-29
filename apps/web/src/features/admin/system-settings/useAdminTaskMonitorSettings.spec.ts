import { describe, expect, it } from 'vitest'
import {
  TASK_MONITOR_MAX_PAGE_SIZE,
  TASK_MONITOR_MIN_REFRESH_INTERVAL_SECONDS,
} from './taskMonitorSettingsResource'
import {
  validateTaskMonitorSetting,
} from './useAdminTaskMonitorSettings'

describe('admin task monitor settings helpers', () => {
  it('accepts valid settings', () => {
    expect(
      validateTaskMonitorSetting({
        list_page_size: 24,
        auto_refresh_enabled: true,
        refresh_interval_seconds: 10,
        result_preview_enabled: true,
      }),
    ).toBe('')
  })

  it('rejects out-of-range settings', () => {
    expect(
      validateTaskMonitorSetting({
        list_page_size: TASK_MONITOR_MAX_PAGE_SIZE + 1,
        auto_refresh_enabled: true,
        refresh_interval_seconds: 4,
        result_preview_enabled: true,
      }),
    ).toBe('任务列表每页数量必须在 5 到 50 之间')
    expect(
      validateTaskMonitorSetting({
        list_page_size: 12,
        auto_refresh_enabled: true,
        refresh_interval_seconds: TASK_MONITOR_MIN_REFRESH_INTERVAL_SECONDS - 1,
        result_preview_enabled: true,
      }),
    ).toBe('刷新间隔必须在 2 到 60 秒之间')
  })
})
