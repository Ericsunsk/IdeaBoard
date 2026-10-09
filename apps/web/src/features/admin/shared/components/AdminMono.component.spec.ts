import { describe, expect, it } from 'vitest'
import { render } from '../../../../test/render'
import AdminMono from './AdminMono.vue'

/**
 * AdminMono 行为契约：只收口 font-mono 的透传容器。
 * class 几何不断言（见 checkTestGuardrails），只 pin 渲染行为。
 */
describe('AdminMono', () => {
  it('默认渲染 span 并透出 slot 文本', () => {
    const { container } = render(AdminMono, { slots: { default: '2026-01-01' } })
    const el = container.firstElementChild
    expect(el?.tagName).toBe('SPAN')
    expect(el?.textContent).toBe('2026-01-01')
  })

  it('as=time 时渲染 time 并透传 datetime/title', () => {
    const { container } = render(AdminMono, {
      props: {
        as: 'time',
        datetime: '2026-01-01',
        title: '2026-01-01',
      } as Record<string, unknown>,
      slots: { default: '2026-01-01' },
    })
    const el = container.firstElementChild
    expect(el?.tagName).toBe('TIME')
    expect(el?.getAttribute('datetime')).toBe('2026-01-01')
    expect(el?.getAttribute('title')).toBe('2026-01-01')
  })

  it('as=div 时渲染 div 容器', () => {
    const { container } = render(AdminMono, {
      props: { as: 'div' },
      slots: { default: 'x' },
    })
    expect(container.firstElementChild?.tagName).toBe('DIV')
  })
})
