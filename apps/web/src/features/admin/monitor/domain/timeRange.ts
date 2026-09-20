export type TimeRangePresetKey = '15m' | '1h' | '3h' | '6h' | '12h' | '24h' | '3d' | '7d'

export type TimeRangePreset = {
  key: TimeRangePresetKey
  label: string
  durationMs: number
}

export const timeRangePresets: readonly TimeRangePreset[] = [
  { key: '15m', label: '过去 15 分钟', durationMs: 15 * 60 * 1000 },
  { key: '1h', label: '过去 1 小时', durationMs: 60 * 60 * 1000 },
  { key: '3h', label: '过去 3 小时', durationMs: 3 * 60 * 60 * 1000 },
  { key: '6h', label: '过去 6 小时', durationMs: 6 * 60 * 60 * 1000 },
  { key: '12h', label: '过去 12 小时', durationMs: 12 * 60 * 60 * 1000 },
  { key: '24h', label: '过去 24 小时', durationMs: 24 * 60 * 60 * 1000 },
  { key: '3d', label: '过去 3 天', durationMs: 3 * 24 * 60 * 60 * 1000 },
  { key: '7d', label: '过去 7 天', durationMs: 7 * 24 * 60 * 60 * 1000 },
]
