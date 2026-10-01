import { describe, expect, it } from 'vitest'
import { ref } from 'vue'
import { render, screen } from '../../../../test/render'
import AdminOptionFilter from './AdminOptionFilter.vue'

/**
 * The dropdown is open-state and keyboard behaviour belongs to reka `Select`,
 * which is exercised in reka's own suite. These tests cover the part this
 * component actually owns: the semantics it advertises, the "all" sentinel
 * mapping, and the active-filter state it paints onto the shared trigger skin.
 */
describe('AdminOptionFilter', () => {
  const options = [
    { label: '全部状态', value: 'all' },
    { label: '已配置', value: 'configured' },
  ]

  it('advertises combobox semantics rather than a hand-rolled listbox', () => {
    // The previous implementation put `aria-haspopup="listbox"` on a plain
    // Popover, advertising a composite widget it did not implement: no arrow
    // keys, no typeahead, no Home/End. It now reports a real combobox.
    render(AdminOptionFilter, {
      props: { modelValue: 'all', options, allValue: 'all', label: '筛选提示词状态' },
    })

    const trigger = screen.getByRole('combobox', { name: '筛选提示词状态' })
    expect(trigger).toHaveAttribute('aria-expanded', 'false')
    expect(trigger).not.toHaveAttribute('aria-haspopup', 'listbox')
  })

  it('shows the label of the current selection on the trigger', () => {
    render(AdminOptionFilter, {
      props: {
        modelValue: 'configured',
        options,
        allValue: 'all',
        label: '筛选提示词状态',
      },
    })
    expect(screen.getByRole('combobox', { name: '筛选提示词状态' })).toHaveTextContent('已配置')
  })

  it('marks the trigger active only when a real filter is applied', () => {
    // The active skin is asserted in adminSelectSkin.spec.ts; here we only pin
    // the decision (default vs filtered), which is this component's own logic.
    const { unmount } = render(AdminOptionFilter, {
      props: { modelValue: 'all', options, allValue: 'all', label: '筛选提示词状态' },
    })
    const idle = screen.getByRole('combobox', { name: '筛选提示词状态' })
    const idleBorder = idle.getAttribute('class')
    unmount()

    render(AdminOptionFilter, {
      props: {
        modelValue: 'configured',
        options,
        allValue: 'all',
        label: '筛选提示词状态',
      },
    })
    const active = screen.getByRole('combobox', { name: '筛选提示词状态' })

    expect(active.getAttribute('class')).not.toBe(idleBorder)
  })

  it('accepts the empty-string "all" value that real filter lists use', () => {
    // AdminBillingPricingPage models "all" as value: ''. reka reserves "" for
    // "show the placeholder" and rejects it as an item value, so it is mapped
    // to a sentinel at this boundary and translated back on the way out.
    render(AdminOptionFilter, {
      props: {
        modelValue: '',
        options: [
          { label: '全部', value: '' },
          { label: '生效中', value: 'active' },
        ],
        label: '筛选状态',
      },
    })
    expect(screen.getByRole('combobox', { name: '筛选状态' })).toHaveTextContent('全部')
  })

  it('keeps re-rendering stable as the value changes', async () => {
    const value = ref('all')
    const { rerender } = render(AdminOptionFilter, {
      props: {
        modelValue: value.value,
        options,
        allValue: 'all',
        label: '筛选提示词状态',
        'onUpdate:modelValue': (next: string) => {
          value.value = next
        },
      },
    })

    expect(screen.getByRole('combobox', { name: '筛选提示词状态' })).toHaveTextContent('全部状态')

    await rerender({ modelValue: 'configured' })
    expect(screen.getByRole('combobox', { name: '筛选提示词状态' })).toHaveTextContent('已配置')
  })
})
