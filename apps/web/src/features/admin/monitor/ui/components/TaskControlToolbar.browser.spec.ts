import { describe, expect, it } from 'vitest'
import { ref } from 'vue'
import { render, screen, userEvent, waitFor } from '@/test/render'
import { timeRangePresets } from '../../core/timeRange'
import TaskControlToolbar from './TaskControlToolbar.vue'

/**
 * Browser-level coverage for the keyboard model jsdom cannot verify: reka
 * `Select` renders its list into a portal and only opens on real pointer input,
 * so unit tests can see the trigger but never the options.
 *
 * The regression this guards is specific. The toolbar used to declare
 * `aria-haspopup="listbox"` and hand-roll `role="listbox"` over
 * `<button role="option">` children: the ARIA promised a composite widget, but
 * there was no arrow-key navigation, no typeahead and no Home/End.
 */
describe('TaskControlToolbar browser contract', () => {
  /**
   * Mirrors the loop the page provides: the toolbar emits `selectTimeRange`, the
   * workspace stores the key, and the resolved preset comes back as a prop.
   * Snapshotting the props once would never re-render, which is a test artefact
   * rather than component behaviour.
   */
  const renderBound = (initialKey: (typeof timeRangePresets)[number]['key']) => {
    const key = ref(initialKey)
    const view = render(TaskControlToolbar, {
      props: {
        searchKeyword: '',
        selectedTimeRangeKey: key.value,
        selectedPreset: timeRangePresets.find((preset) => preset.key === key.value)!,
        timeRangePresets,
        onSelectTimeRange: (next: (typeof timeRangePresets)[number]['key']) => {
          key.value = next
          void view.rerender({
            selectedTimeRangeKey: key.value,
            selectedPreset: timeRangePresets.find((preset) => preset.key === key.value)!,
          })
        },
      },
    })
    return { view, key }
  }

  it('opens on click and exposes every preset as a real option role', async () => {
    const user = userEvent.setup()
    const { view } = renderBound('1h')

    const trigger = screen.getByRole('combobox', { name: '选择时间范围' })
    await user.click(trigger)

    await waitFor(() => {
      expect(screen.getByRole('option', { name: '过去 24 小时' })).toBeTruthy()
    })
    for (const preset of timeRangePresets) {
      expect(screen.getByRole('option', { name: preset.label })).toBeTruthy()
    }
    void view
  })

  it('navigates and commits a preset with the keyboard alone', async () => {
    const user = userEvent.setup()
    const { key } = renderBound('1h')

    const trigger = screen.getByRole('combobox', { name: '选择时间范围' })
    trigger.focus()
    await user.keyboard('{Enter}')
    await waitFor(() => expect(trigger.getAttribute('aria-expanded')).toBe('true'))

    // reka highlights the current value on open, so ArrowDown moves to the next
    // preset and End jumps to the last one.
    await user.keyboard('{ArrowDown}')
    await user.keyboard('{End}')
    await user.keyboard('{Enter}')

    await waitFor(() => expect(key.value).toBe('7d'))
    await waitFor(() => expect(trigger.textContent).toContain('过去 7 天'))
  })

  it('keeps the whole picker on one Tab stop', async () => {
    const user = userEvent.setup()
    renderBound('1h')

    const trigger = screen.getByRole('combobox', { name: '选择时间范围' })
    await user.click(trigger)
    await waitFor(() => {
      expect(screen.getByRole('option', { name: '过去 24 小时' })).toBeTruthy()
    })

    // The old hand-rolled listbox put all eight presets in the page's tab order.
    for (const option of screen.getAllByRole('option')) {
      expect(option.getAttribute('tabindex')).not.toBe('0')
    }
  })

  it('closes on Escape and returns focus to the trigger', async () => {
    const user = userEvent.setup()
    renderBound('1h')

    const trigger = screen.getByRole('combobox', { name: '选择时间范围' })
    trigger.focus()
    await user.keyboard('{Enter}')
    await waitFor(() => expect(trigger.getAttribute('aria-expanded')).toBe('true'))

    await user.keyboard('{Escape}')

    await waitFor(() => expect(trigger.getAttribute('aria-expanded')).toBe('false'))
    await waitFor(() => expect(document.activeElement).toBe(trigger))
  })
})
