import { describe, expect, it } from 'vitest'
import { render, screen } from '@/test/render'
import { timeRangePresets } from '../../core/timeRange'
import TaskControlToolbar from './TaskControlToolbar.vue'

/**
 * Open-state and keyboard behaviour belongs to reka `Select`, which needs a real
 * browser (see `TaskControlToolbar.browser.spec.ts`). What is pinned here is the
 * part this component owns: the semantics the time-range trigger advertises and
 * the preset label it reports.
 *
 * The regression this guards is the ARIA lie. The trigger used to declare
 * `aria-haspopup="listbox"` over a hand-rolled `role="listbox"` of bare
 * `<button role="option">` children — a composite promise with no arrow keys, no
 * typeahead and no Home/End behind it.
 */
describe('TaskControlToolbar time range picker', () => {
  const selectedPreset = timeRangePresets.find((preset) => preset.key === '1h')!

  const baseProps = {
    searchKeyword: '',
    selectedPreset,
    selectedTimeRangeKey: '1h' as const,
    timeRangePresets,
  }

  it('advertises combobox semantics rather than a hand-rolled listbox', () => {
    render(TaskControlToolbar, { props: { ...baseProps } })

    const trigger = screen.getByRole('combobox', { name: '选择时间范围' })
    expect(trigger).toHaveAttribute('aria-expanded', 'false')
    expect(trigger).not.toHaveAttribute('aria-haspopup', 'listbox')
  })

  it('reports the selected preset label on the trigger', () => {
    render(TaskControlToolbar, { props: { ...baseProps } })
    expect(screen.getByRole('combobox', { name: '选择时间范围' })).toHaveTextContent('过去 1 小时')
  })

  it('falls back to the resolved preset when the key is not among the presets', () => {
    // `useMonitorTimeline` always resolves `selectedPreset`, so the toolbar must
    // never render an empty trigger for an unknown key.
    render(TaskControlToolbar, {
      props: {
        ...baseProps,
        timeRangePresets: timeRangePresets.filter((preset) => preset.key !== '1h'),
      },
    })

    expect(screen.getByRole('combobox', { name: '选择时间范围' })).toHaveTextContent('过去 1 小时')
  })
})
