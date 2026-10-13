import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { h } from 'vue'
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from './index'

describe('Card compound components', () => {
  it('renders all Card micro-primitives properly', () => {
    const wrapper = mount(Card, {
      props: { class: 'custom-card' },
      slots: {
        default: () => [
          h(CardHeader, { class: 'custom-header' }, () => [
            h(CardTitle, { as: 'h2' }, () => 'Card Title Text'),
            h(CardDescription, () => 'Card Description Text'),
          ]),
          h(CardContent, { class: 'custom-content' }, () => 'Main Body Content'),
          h(CardFooter, { class: 'custom-footer' }, () => 'Footer Content'),
        ],
      },
    })

    expect(wrapper.classes()).toContain('rounded-panel')
    expect(wrapper.classes()).toContain('custom-card')

    const header = wrapper.findComponent(CardHeader)
    expect(header.exists()).toBe(true)
    expect(header.classes()).toContain('custom-header')

    const title = wrapper.findComponent(CardTitle)
    expect(title.exists()).toBe(true)
    expect(title.element.tagName.toLowerCase()).toBe('h2')
    expect(title.text()).toBe('Card Title Text')

    const desc = wrapper.findComponent(CardDescription)
    expect(desc.exists()).toBe(true)
    expect(desc.text()).toBe('Card Description Text')

    const content = wrapper.findComponent(CardContent)
    expect(content.exists()).toBe(true)
    expect(content.text()).toBe('Main Body Content')

    const footer = wrapper.findComponent(CardFooter)
    expect(footer.exists()).toBe(true)
    expect(footer.text()).toBe('Footer Content')
  })

  describe('elevation contract', () => {
    it('elevates by default', () => {
      const wrapper = mount(Card)
      expect(wrapper.classes()).toContain('shadow')
    })

    it('opts out when elevated is false, without needing a class override', () => {
      const wrapper = mount(Card, { props: { elevated: false } })
      expect(wrapper.classes()).not.toContain('shadow')
      // A caller must never have to cancel the default with `shadow-none`;
      // that only wins by accident of cn() class ordering.
      expect(wrapper.classes().join(' ')).not.toContain('shadow-none')
    })

    it('keeps the base surface classes when elevation is off', () => {
      const wrapper = mount(Card, { props: { elevated: false, class: 'p-6' } })
      for (const base of ['rounded-panel', 'border', 'bg-card']) {
        expect(wrapper.classes()).toContain(base)
      }
      expect(wrapper.classes()).toContain('p-6')
    })
  })
})
