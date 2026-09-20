import { defineComponent, h, type Component } from 'vue'

export function defineIconStub(name: string): Component {
  return defineComponent({
    name,
    setup() {
      return () => h('span', { 'aria-hidden': 'true', 'data-testid': `${name}-stub` })
    },
  })
}

export function definePassthroughStub(name: string): Component {
  return defineComponent({
    name,
    setup(_, { slots }) {
      return () => h('div', { 'data-testid': `${name}-stub` }, slots.default?.())
    },
  })
}
