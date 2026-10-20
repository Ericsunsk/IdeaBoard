import { describe, expect, it } from 'vitest'
import { defineComponent, nextTick, ref } from 'vue'
import { renderWithUser, screen } from '../../../../../test/render'
import SwitchRow from './SwitchRow.vue'

describe('SwitchRow', () => {
  it('locks the status slot and pill to the configured min width without a switch', () => {
    renderWithUser(SwitchRow, {
      props: { pill: 'Active', pillVariant: 'success', statusMinWidth: '84px' },
    })

    const slot = screen.getByTestId('switch-row-status-slot')
    expect(slot.style.minWidth).toBe('84px')
    const pill = screen.getByText('Active').parentElement
    expect(pill?.style.minWidth).toBe('84px')
    expect(pill).toHaveClass('justify-center')
    expect(screen.queryByRole('switch')).not.toBeInTheDocument()
  })

  it('renders a same-width empty placeholder when the pill is missing', () => {
    renderWithUser(SwitchRow, {
      props: { pill: undefined, statusMinWidth: '104px', pillSize: 'medium' },
    })

    expect(screen.getByTestId('switch-row-status-slot').style.minWidth).toBe('104px')
    const placeholder = screen.getByTestId('switch-row-status-placeholder')
    expect(placeholder.style.minWidth).toBe('104px')
    expect(placeholder).toHaveAttribute('aria-hidden', 'true')
    expect(placeholder.textContent).toBe('')
    expect(placeholder.className).toContain('h-6')
  })

  it('keeps pulse off by default so variant flips never mount extra DOM', () => {
    const { container } = renderWithUser(SwitchRow, {
      props: { pill: 'Active', pillVariant: 'success', statusMinWidth: '84px' },
    })

    expect(container.querySelector('.status-badge-pulse')).toBeNull()
  })

  it('only adds the pulse layer when pulse is explicitly enabled', () => {
    const { container } = renderWithUser(SwitchRow, {
      props: { pill: 'Active', pillVariant: 'success', pulse: true, statusMinWidth: '84px' },
    })

    const pulse = container.querySelector('.status-badge-pulse')
    expect(pulse).not.toBeNull()
    expect(pulse?.className).toContain('bg-success')
    expect(screen.getByTestId('switch-row-status-slot').style.minWidth).toBe('84px')
  })

  it('forwards switch attrs and the v-model state to a fixed-position switch', async () => {
    const { user, emitted } = renderWithUser(SwitchRow, {
      props: {
        pill: 'On',
        statusMinWidth: '68px',
        modelValue: false,
        switchAttrs: { disabled: false, 'aria-label': 'row enabled' },
      },
    })

    const toggle = screen.getByRole('switch', { name: 'row enabled' })
    expect(toggle).not.toBeChecked()
    expect(toggle.className).toContain('shrink-0')

    await user.click(toggle)
    expect(emitted()['update:modelValue']).toEqual([[true]])
  })

  it('keeps the same switch node across pill label and variant changes', async () => {
    const pill = ref('Active')
    const variant = ref<'success' | 'neutral'>('success')
    const on = ref(false)
    const Wrapper = defineComponent({
      components: { SwitchRow },
      setup: () => ({ pill, variant, on }),
      template: `<SwitchRow :pill="pill" :pill-variant="variant" status-min-width="68px" v-model="on" :switch-attrs="{ 'aria-label': 'row enabled' }" />`,
    })
    renderWithUser(Wrapper)

    const toggle = screen.getByRole('switch', { name: 'row enabled' })
    on.value = true
    await nextTick()
    expect(toggle).toBeChecked()

    // 状态翻转只换色：同一 DOM 节点保留 checked，不绑状态 key。
    pill.value = 'Disabled'
    variant.value = 'neutral'
    await nextTick()

    expect(screen.getByText('Disabled')).toBeVisible()
    expect(screen.getByRole('switch', { name: 'row enabled' })).toBe(toggle)
    expect(toggle).toBeChecked()
  })

  it('uses gap-3 by default and honors the mail header gap-2', async () => {
    const { rerender } = renderWithUser(SwitchRow, { props: { pill: 'A', statusMinWidth: '68px' } })
    expect(screen.getByTestId('switch-row-status-slot').parentElement).toHaveClass('gap-3')

    await rerender({ pill: 'A', statusMinWidth: '104px', gapClass: 'gap-2' })
    expect(screen.getByTestId('switch-row-status-slot').parentElement).toHaveClass('gap-2')
  })
})
