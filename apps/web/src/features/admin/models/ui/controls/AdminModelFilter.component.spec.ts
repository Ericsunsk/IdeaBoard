import { describe, expect, it } from 'vitest'
import { render, screen } from '@/test/render'
import AdminModelFilter from './AdminModelFilter.vue'

/**
 * The dropdown's open-state and keyboard behaviour belongs to reka `Select`,
 * which needs a real browser (see `AdminModelFilter.browser.spec.ts`). What is
 * pinned here is the part this adapter owns: the semantics it advertises, the
 * trigger's text, and the empty-string "all" value the models workspace stores.
 *
 * The regression this guards is the ARIA lie. The previous implementation
 * declared `role="listbox"` on a plain `AdminPopoverContent` over bare
 * `<button role="option">` children, advertising a composite widget with no
 * arrow-key navigation, no typeahead and no Home/End.
 */
describe('AdminModelFilter', () => {
  const options = ['image_generation', 'text_generation']

  const baseProps = {
    label: '模型类型',
    allLabel: '全部类型',
    options,
  }

  it('advertises combobox semantics rather than a hand-rolled listbox', () => {
    render(AdminModelFilter, {
      props: { ...baseProps, modelValue: '', 'onUpdate:modelValue': () => {} },
    })

    const trigger = screen.getByRole('combobox', { name: '筛选模型类型' })
    expect(trigger).toHaveAttribute('aria-expanded', 'false')
    expect(trigger).not.toHaveAttribute('aria-haspopup', 'listbox')
  })

  it('reports the applied filter, including the "all" default, on the trigger', () => {
    // Same shape as every other Admin filter after this refactor
    // (`AdminOptionFilter`): the trigger states the current value instead of the
    // field name, so the reset item stays reachable *and* visibly selected.
    render(AdminModelFilter, {
      props: { ...baseProps, modelValue: '', 'onUpdate:modelValue': () => {} },
    })
    expect(screen.getByRole('combobox', { name: '筛选模型类型' })).toHaveTextContent('全部类型')
  })

  it('shows the current selection once a filter is applied', () => {
    render(AdminModelFilter, {
      props: {
        ...baseProps,
        modelValue: 'image_generation',
        'onUpdate:modelValue': () => {},
      },
    })

    expect(screen.getByRole('combobox', { name: '筛选模型类型' })).toHaveTextContent(
      'image_generation',
    )
  })

  it('falls back to the dimension label when the value is not among the options', () => {
    // Options are derived from catalog data, so a filter can briefly outlive the
    // option that produced it. The dimension label is the honest fallback.
    render(AdminModelFilter, {
      props: {
        ...baseProps,
        modelValue: 'embedding',
        'onUpdate:modelValue': () => {},
      },
    })

    expect(screen.getByRole('combobox', { name: '筛选模型类型' })).toHaveTextContent('模型类型')
  })

  it('honours a caller-supplied accessible name', () => {
    // AdminModelsPage does not pass one, but the prop is part of the contract
    // and the trigger needs a programmatic label either way.
    render(AdminModelFilter, {
      props: {
        label: '托管方式',
        allLabel: '全部托管',
        options: ['official'],
        modelValue: '',
        ariaLabel: '按托管方式筛选模型',
        'onUpdate:modelValue': () => {},
      },
    })

    expect(screen.getByRole('combobox', { name: '按托管方式筛选模型' })).toBeInTheDocument()
  })

  it('re-renders the trigger text as the workspace filter changes', async () => {
    const { rerender } = render(AdminModelFilter, {
      props: { ...baseProps, modelValue: '', 'onUpdate:modelValue': () => {} },
    })

    expect(screen.getByRole('combobox', { name: '筛选模型类型' })).toHaveTextContent('全部类型')

    await rerender({ modelValue: 'text_generation' })
    expect(screen.getByRole('combobox', { name: '筛选模型类型' })).toHaveTextContent(
      'text_generation',
    )
  })
})
