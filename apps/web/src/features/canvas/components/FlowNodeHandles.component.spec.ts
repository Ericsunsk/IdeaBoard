import { defineComponent, h } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { Position } from '@vue-flow/core'
import { render } from '../../../test/render'
import type { FlowNodeHandlesPresentation } from '../composables/useFlowNodeHandlesPresentation'
import FlowNodeHandles from './FlowNodeHandles.vue'

vi.mock('@vue-flow/core', () => ({
  Handle: defineComponent({
    inheritAttrs: false,
    setup(_, { attrs, slots }) {
      return () => h('div', attrs, slots.default?.())
    },
  }),
  Position: { Left: 'left', Right: 'right' },
}))

describe('FlowNodeHandles', () => {
  it('renders accessibility and visibility directly from its presentation', () => {
    const setInputElement = vi.fn()
    const setOutputElement = vi.fn()
    const presentation: FlowNodeHandlesPresentation = {
      anchors: [
        {
          id: 'in:image',
          direction: 'input',
          handleType: 'target',
          position: Position.Left,
          style: { left: '0' },
        },
        {
          id: 'out:secondary',
          direction: 'output',
          handleType: 'source',
          position: Position.Right,
          style: { right: '0' },
        },
      ],
      aggregates: [
        {
          id: 'in:__auto',
          direction: 'input',
          handleType: 'target',
          position: Position.Left,
          connectable: false,
          visible: false,
          proximityActive: false,
          returning: false,
          disabled: false,
          style: { left: '0' },
          ariaLabel: '图片节点输入连接点，拖拽创建连接',
          title: '拖拽创建输入连接',
          setElement: setInputElement,
          onPointerDown: vi.fn(),
        },
        {
          id: 'out:image',
          direction: 'output',
          handleType: 'source',
          position: Position.Right,
          connectable: true,
          visible: true,
          proximityActive: true,
          returning: false,
          disabled: false,
          style: { right: '0', '--handle-plus-x': '20px' },
          ariaLabel: '图片节点输出连接点，拖拽创建连接',
          title: '拖拽创建输出连接',
          setElement: setOutputElement,
          onPointerDown: vi.fn(),
        },
      ],
    }

    const view = render(FlowNodeHandles, { props: { presentation } })
    const input = view.getByLabelText('图片节点输入连接点，拖拽创建连接')
    const output = view.getByLabelText('图片节点输出连接点，拖拽创建连接')

    expect(input).toHaveAttribute('aria-hidden', 'true')
    expect(input).toHaveAttribute('tabindex', '-1')
    expect(output).not.toHaveAttribute('aria-hidden')
    expect(output).toHaveAttribute('tabindex', '0')
    expect(output).toHaveAttribute('style', expect.stringContaining('--handle-plus-x: 20px'))
    expect(setInputElement).toHaveBeenCalled()
    expect(setOutputElement).toHaveBeenCalled()
  })
})
