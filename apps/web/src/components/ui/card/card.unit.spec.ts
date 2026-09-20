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

    expect(wrapper.classes()).toContain('rounded-xl')
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
})
