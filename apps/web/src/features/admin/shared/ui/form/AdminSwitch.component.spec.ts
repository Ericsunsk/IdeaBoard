import { describe, expect, it } from 'vitest'
import { renderWithUser, screen } from '@/test/render'
import AdminSwitch from './AdminSwitch.vue'

describe('AdminSwitch', () => {
  it('runs the track on the control motion token including opacity', () => {
    renderWithUser(AdminSwitch, { props: { modelValue: false } })

    const track = screen.getByRole('switch')
    // `control-state` supplies duration/easing from --motion-control-*;
    // the property list stays explicit and includes opacity, while the bare
    // `transition-colors` utility would leave disabled opacity hard-cut.
    // (No transition/duration/ease utility may sit on this track: its
    // utilities-layer default timing would shadow the token, and arbitrary
    // duration-[var(...)]/ease-[var(...)] need a style-governance allowance.)
    expect(track.className).toContain('control-state')
    expect(track.className).toContain(
      '[transition-property:background-color,border-color,color,opacity]',
    )
    expect(track.className).toContain('disabled:opacity-50')
    expect(track.className).not.toContain('transition-colors')
    expect(track.className).not.toContain('duration-[var(')
    expect(track.className).not.toContain('ease-[var(')
  })

  it('keeps the knob translate mechanism untouched', () => {
    renderWithUser(AdminSwitch, { props: { modelValue: true } })

    const thumb = screen.getByRole('switch').firstElementChild
    expect(thumb).not.toBeNull()
    expect(thumb?.className).toContain('transition-transform')
    expect(thumb?.className).toContain('data-[state=checked]:translate-x-4')
    expect(thumb?.className).toContain('data-[state=unchecked]:translate-x-0')
  })

  it('exposes the disabled state for the opacity fade', () => {
    renderWithUser(AdminSwitch, { props: { modelValue: false, disabled: true } })

    expect(screen.getByRole('switch')).toBeDisabled()
  })
})
