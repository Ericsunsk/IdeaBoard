import { describe, expect, it } from 'vitest'
import {
  DEFAULT_ADMIN_TASK_MONITOR_SETTING,
  normalizeTaskMonitorSetting,
} from './taskMonitorSettingsResource'

describe('task monitor settings resource helpers', () => {
  it('normalizes missing and invalid values to defaults', () => {
    expect(normalizeTaskMonitorSetting(null)).toEqual(DEFAULT_ADMIN_TASK_MONITOR_SETTING)
    expect(
      normalizeTaskMonitorSetting({
        list_page_size: 1000,
        auto_refresh_enabled: false,
        refresh_interval_seconds: 1,
        result_preview_enabled: false,
      }),
    ).toEqual({
      ...DEFAULT_ADMIN_TASK_MONITOR_SETTING,
      auto_refresh_enabled: false,
      result_preview_enabled: false,
    })
  })
})
