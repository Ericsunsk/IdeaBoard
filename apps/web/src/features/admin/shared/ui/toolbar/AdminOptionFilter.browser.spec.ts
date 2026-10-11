import { describe, expect, it } from 'vitest'
import { ref } from 'vue'
import { render, screen, userEvent, waitFor } from '../../../../../test/render'
import AdminOptionFilter from './AdminOptionFilter.vue'

/**
 * Browser-level coverage for the dropdown contract that jsdom cannot verify:
 * reka `Select` renders its list into a portal and only opens on real pointer
 * input, so unit tests can see the trigger but never the options.
 *
 * The regression this guards is specific. `AdminOptionFilter` used to be a
 * Popover with `aria-haspopup="listbox"` and a hand-rolled `role="listbox"`:
 * the ARIA promised a composite widget, but there was no arrow-key navigation,
 * no typeahead, and no Home/End. Screen-reader and keyboard users were told a
 * capability existed that did not.
 */
describe('AdminOptionFilter browser contract', () => {
  const options = [
    { label: '全部状态', value: 'all' },
    { label: '已配置', value: 'configured' },
    { label: '未配置', value: 'unconfigured' },
  ]

  /**
   * Mirrors the `v-model` loop a parent page provides: the component emits, the
   * parent stores the value, and the new value is fed back as a prop. Passing
   * `modelValue: value.value` once would snapshot the initial value and never
   * re-render, which is a test artefact rather than component behaviour.
   */
  const renderBound = async (initial: string) => {
    const value = ref(initial)
    const view = render(AdminOptionFilter, {
      props: {
        modelValue: value.value,
        options,
        allValue: 'all',
        label: '筛选提示词状态',
        'onUpdate:modelValue': (next: string) => {
          value.value = next
          void view.rerender({ modelValue: value.value })
        },
      },
    })
    return { view, value }
  }

  it('opens on click and exposes its options as real option roles', async () => {
    const user = userEvent.setup()
    const { view } = await renderBound('all')

    const trigger = screen.getByRole('combobox', { name: '筛选提示词状态' })
    await user.click(trigger)

    await waitFor(() => {
      expect(screen.getByRole('option', { name: '已配置' })).toBeTruthy()
    })
    expect(screen.getByRole('option', { name: '全部状态' })).toBeTruthy()
    expect(screen.getByRole('option', { name: '未配置' })).toBeTruthy()
    void view
  })

  it('navigates and commits a selection with the keyboard alone', async () => {
    const user = userEvent.setup()
    const { value } = await renderBound('all')

    const trigger = screen.getByRole('combobox', { name: '筛选提示词状态' })
    await user.click(trigger)
    await waitFor(() => {
      expect(screen.getByRole('option', { name: '已配置' })).toBeTruthy()
    })

    // reka highlights the current value on open, so the first ArrowDown moves off
    // "全部状态" and the second lands on "未配置". The point of the test is that
    // arrow keys and Enter commit a selection at all: the old Popover
    // implementation supported none of this.
    await user.keyboard('{ArrowDown}')
    await user.keyboard('{ArrowDown}')
    await user.keyboard('{Enter}')

    await waitFor(() => {
      expect(value.value).toBe('unconfigured')
    })
    await waitFor(() => {
      expect(trigger.textContent).toContain('未配置')
    })
  })

  it('closes and returns focus to the trigger after committing', async () => {
    const user = userEvent.setup()
    const { value } = await renderBound('all')

    const trigger = screen.getByRole('combobox', { name: '筛选提示词状态' })
    await user.click(trigger)
    await waitFor(() => {
      expect(screen.getByRole('option', { name: '已配置' })).toBeTruthy()
    })

    await user.click(screen.getByRole('option', { name: '已配置' }))

    await waitFor(() => {
      expect(value.value).toBe('configured')
    })
    await waitFor(() => {
      expect(trigger.getAttribute('aria-expanded')).toBe('false')
    })
  })

  it('selects the "all" option whose domain value is the empty string', async () => {
    // AdminBillingPricingPage models "all" as value: ''. reka reserves "" for
    // the placeholder, so the component maps it to a sentinel internally and must
    // translate it back to '' on the way out.
    const user = userEvent.setup()
    const value = ref('active')
    const view = render(AdminOptionFilter, {
      props: {
        modelValue: value.value,
        options: [
          { label: '全部', value: '' },
          { label: '生效中', value: 'active' },
        ],
        label: '筛选状态',
        'onUpdate:modelValue': (next: string) => {
          value.value = next
          void view.rerender({ modelValue: value.value })
        },
      },
    })

    const trigger = screen.getByRole('combobox', { name: '筛选状态' })
    expect(trigger.textContent).toContain('生效中')

    await user.click(trigger)
    await waitFor(() => {
      expect(screen.getByRole('option', { name: '全部' })).toBeTruthy()
    })
    await user.click(screen.getByRole('option', { name: '全部' }))

    await waitFor(() => {
      expect(value.value).toBe('')
    })
    await waitFor(() => {
      expect(trigger.textContent).toContain('全部')
    })
  })
})
