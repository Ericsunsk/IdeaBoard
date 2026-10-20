import { afterEach, describe, expect, it } from 'vitest'
import { createApp, h, nextTick, ref } from 'vue'
import '../app/style.css'
import { useTheme } from '../theme/composables/useTheme'
import { adminChartTheme } from '../features/admin/shared/ui/chart/chart'
import { getDefaultModelColors } from '../features/admin/statistics/core/usageStatisticsChartBase'
import { useMonitorTimeline } from '../features/admin/monitor/session/useMonitorTimeline'

afterEach(async () => {
  useTheme().setTheme('dark')
  await nextTick()
})

describe('Admin chart runtime palette', () => {
  it('renders the current CSS palette after each theme change', async () => {
    const { setTheme } = useTheme()
    setTheme('dark')
    await nextTick()
    const root = document.createElement('div')
    document.body.append(root)
    const app = createApp({
      setup() {
        const monitor = useMonitorTimeline({
          isDemoMode: ref(false),
          demoRows: [],
          chartRows: ref([]),
          filterStatus: ref(null),
        })
        return () =>
          h(
            'span',
            [
              adminChartTheme.axisLine,
              adminChartTheme.brand,
              getDefaultModelColors()[0],
              monitor.timelineChartData.value.datasets[0]?.borderColor,
            ].join('|'),
          )
      },
    })
    app.mount(root)
    try {
      expect(root.textContent).toBe('#3d3d3d|#fcfcfc|#fcfcfc|#60a5fa')
      setTheme('light')
      await nextTick()
      expect(root.textContent).toBe('#e5e7eb|#9278e6|#9278e6|#0284c7')
      setTheme('dark')
      await nextTick()
      expect(root.textContent).toBe('#3d3d3d|#fcfcfc|#fcfcfc|#60a5fa')
    } finally {
      app.unmount()
      root.remove()
    }
  })
})
