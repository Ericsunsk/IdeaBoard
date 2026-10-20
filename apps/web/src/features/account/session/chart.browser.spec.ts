import { afterEach, describe, expect, it } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import '../../../app/style.css'
import { useTheme } from '../../../theme/composables/useTheme'
import { resolveUsageChartTheme } from './chart'

afterEach(async () => {
  useTheme().setTheme('dark')
  await nextTick()
})

describe('Account chart runtime palette', () => {
  it('reads semantic runtime roles after each theme change', async () => {
    const { setTheme } = useTheme()
    setTheme('dark')
    await nextTick()
    const root = document.createElement('div')
    document.body.append(root)
    const app = createApp({
      setup() {
        return () => {
          const theme = resolveUsageChartTheme()
          return h('span', [theme.axisText, theme.text, theme.accent].join('|'))
        }
      },
    })
    app.mount(root)
    try {
      expect(root.textContent).toBe('#a3a3a3|#f5f5f5|#fcfcfc')
      setTheme('light')
      await nextTick()
      expect(root.textContent).toBe('#737373|#171717|#9278e6')
      setTheme('dark')
      await nextTick()
      expect(root.textContent).toBe('#a3a3a3|#f5f5f5|#fcfcfc')
    } finally {
      app.unmount()
      root.remove()
    }
  })
})
