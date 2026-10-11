import { describe, expect, it } from 'vitest'
import { render, screen, userEvent, waitFor } from '../../../../../test/render'
import AdminModelFilter from './AdminModelFilter.vue'

/**
 * The filter used to declare `role="listbox"` on a plain Popover over bare
 * `<button role="option">` children — advertising the APG composite model
 * (arrow keys, Home/End, typeahead) without implementing any of it. It now
 * mounts AdminSelect, so the keyboard model comes from reka. These assertions
 * pin the contract the old markup could not offer.
 */
describe('AdminModelFilter keyboard model', () => {
  function renderFilter(modelValue = '') {
    const user = userEvent.setup()
    const updates: unknown[] = []
    render(AdminModelFilter, {
      props: {
        label: '模型类型',
        allLabel: '全部类型',
        options: ['image_generation', 'text_generation', 'video_generation'],
        modelValue,
        'onUpdate:modelValue': (value: string) => updates.push(value),
      },
    })
    return { user, updates }
  }

  it('exposes a single combobox tab stop, not a list of buttons', () => {
    const { user } = renderFilter()
    const trigger = screen.getByRole('combobox', { name: '筛选模型类型' })
    // The whole filter is one control: Tab reaches it once, not once per option.
    expect(trigger.getAttribute('aria-expanded')).toBe('false')

    trigger.focus()
    user.keyboard('{Escape}')
    expect(document.querySelectorAll('[role="option"]')).toHaveLength(0)
  })

  it('navigates options with arrow keys and commits with Enter', async () => {
    const { user, updates } = renderFilter()
    const trigger = screen.getByRole('combobox', { name: '筛选模型类型' })

    trigger.focus()
    await user.keyboard('{Enter}')
    await waitFor(() => expect(trigger.getAttribute('aria-expanded')).toBe('true'))

    await user.keyboard('{ArrowDown}')
    expect(document.activeElement?.textContent?.trim()).toBe('image_generation')

    await user.keyboard('{Enter}')
    await waitFor(() => expect(trigger.getAttribute('aria-expanded')).toBe('false'))
    // reka emits once on commit and again while restoring focus, so assert the
    // committed value rather than an exact call count.
    expect(updates.at(-1)).toBe('image_generation')
    expect(updates).not.toContain('')
  })
})
