import { describe, expect, it, vi } from 'vitest'
import { defineComponent, h, readonly, ref, type Ref } from 'vue'
import { render, screen, userEvent, waitFor } from '../../../test/render'
import { Select, SelectContent, SelectItem, SelectItemText, SelectTrigger, SelectValue, SelectViewport } from './index'

describe('select primitive', () => {
  it('exposes a combobox trigger', () => {
    render(Select, {
      props: { modelValue: '' },
      slots: {
        default: () => [
          h(SelectTrigger, { 'aria-label': '测试选择' }, () => h(SelectValue, {}, () => '')),
          h(SelectContent, {}, () => h(SelectViewport, {}, () => [
            h(SelectItem, { value: 'a' }, () => h(SelectItemText, {}, () => '甲')),
          ])),
        ],
      },
    })
    expect(screen.getByRole('combobox', { name: '测试选择' })).toBeTruthy()
  })

  it('opens its option list on click in a real browser', async () => {
    const user = userEvent.setup()
    render(Select, {
      props: { modelValue: '' },
      slots: {
        default: () => [
          h(SelectTrigger, { 'aria-label': '测试选择' }, () => h(SelectValue, {}, () => '')),
          h(SelectContent, {}, () => h(SelectViewport, {}, () => [
            h(SelectItem, { value: 'a' }, () => h(SelectItemText, {}, () => '甲')),
            h(SelectItem, { value: 'b' }, () => h(SelectItemText, {}, () => '乙')),
          ])),
        ],
      },
    })

    await user.click(screen.getByRole('combobox', { name: '测试选择' }))
    await waitFor(() => {
      expect(screen.getByRole('option', { name: '甲' })).toBeTruthy()
    })
  })

  it('navigates and commits with the keyboard alone', async () => {
    const user = userEvent.setup()
    const commits: unknown[] = []
    render(Select, {
      props: {
        modelValue: 'a',
        'onUpdate:modelValue': (value: unknown) => commits.push(value),
      },
      slots: {
        default: () => [
          h(SelectTrigger, { 'aria-label': '测试选择' }, () => h(SelectValue)),
          h(SelectContent, {}, () => h(SelectViewport, {}, () => [
            h(SelectItem, { value: 'a' }, () => h(SelectItemText, {}, () => '甲')),
            h(SelectItem, { value: 'b' }, () => h(SelectItemText, {}, () => '乙')),
          ])),
        ],
      },
    })

    const trigger = screen.getByRole('combobox', { name: '测试选择' })
    trigger.focus()
    await user.keyboard('{Enter}')
    await waitFor(() => expect(trigger.getAttribute('aria-expanded')).toBe('true'))

    await user.keyboard('{ArrowDown}')
    expect(document.activeElement?.textContent?.trim()).toBe('乙')

    await user.keyboard('{Enter}')
    // reka emits once on commit and again while restoring focus; the subject
    // here is that the chosen value reaches the parent at all.
    expect(commits).toContain('b')
    await waitFor(() => expect(trigger.getAttribute('aria-expanded')).toBe('false'))
  })

  it('routes selection through a command when the model is readonly', async () => {
    // Controllers hand pages a readonly viewModel, so a page cannot bind
    // `v-model`; it must forward the new value to a command. This mirrors how
    // BoardsPage drives its sort Select.
    const user = userEvent.setup()
    const model = ref<'recent' | 'name'>('recent')
    const setSortMode = vi.fn((next: 'recent' | 'name') => {
      model.value = next
    })
    const readonlyModel = readonly(model) as Ref<'recent' | 'name'>

    const Harness = defineComponent({
      setup() {
        return () =>
          h(
            Select,
            {
              modelValue: readonlyModel.value,
              'onUpdate:modelValue': (value: unknown) => {
                if (value === 'recent' || value === 'name') setSortMode(value)
              },
            },
            () => [
              h(SelectTrigger, { 'aria-label': '排序' }, () => h(SelectValue)),
              h(SelectContent, {}, () =>
                h(SelectViewport, {}, () => [
                  h(SelectItem, { value: 'recent' }, () => h(SelectItemText, {}, () => '最近')),
                  h(SelectItem, { value: 'name' }, () => h(SelectItemText, {}, () => '名称')),
                ]),
              ),
            ],
          )
      },
    })
    render(Harness)

    const trigger = screen.getByRole('combobox', { name: '排序' })
    trigger.focus()
    await user.keyboard('{Enter}')
    await waitFor(() => expect(trigger.getAttribute('aria-expanded')).toBe('true'))
    await user.keyboard('{ArrowDown}{Enter}')

    // The command ran, and its write-back is what actually moves the model.
    await waitFor(() => expect(setSortMode).toHaveBeenCalledWith('name'))
    await waitFor(() => expect(model.value).toBe('name'))
  })
})
