import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import AdminButton from './AdminButton.vue'
import AdminInput from './AdminInput.vue'
import AdminBadge from './AdminBadge.vue'

describe('Admin base control adapters', () => {
  describe('AdminButton', () => {
    it('renders as button with default admin theme classes', () => {
      const wrapper = mount(AdminButton, {
        props: { label: 'Submit' },
      })
      expect(wrapper.element.tagName).toBe('BUTTON')
      expect(wrapper.text()).toContain('Submit')
      expect(wrapper.classes()).toContain('bg-primary')
    })

    it('renders as anchor when href is provided', () => {
      const wrapper = mount(AdminButton, {
        props: { href: 'https://example.com', label: 'Link' },
      })
      expect(wrapper.element.tagName).toBe('A')
      expect(wrapper.attributes('href')).toBe('https://example.com')
    })

    it('handles severity and danger variants with disabled/loading states', () => {
      const wrapper = mount(AdminButton, {
        props: { severity: 'danger', loading: true },
        slots: { default: 'Delete' },
      })
      expect(wrapper.attributes('aria-busy')).toBe('true')
      expect(wrapper.attributes('disabled')).toBeDefined()
      expect(wrapper.classes()).toContain('bg-destructive')
    })

    it('maps small size to compact typography', () => {
      const wrapper = mount(AdminButton, {
        props: { size: 'small', label: 'Small' },
      })
      expect(wrapper.classes()).toContain('text-admin-meta')
    })
  })

  describe('AdminInput', () => {
    it('renders Input primitive and handles v-model update', async () => {
      const wrapper = mount(AdminInput, {
        props: {
          modelValue: 'test',
          placeholder: 'Search...',
          'onUpdate:modelValue': (e: string | number | null) => wrapper.setProps({ modelValue: e }),
        },
      })
      const input = wrapper.find('input')
      expect(input.element.value).toBe('test')
      expect(input.attributes('placeholder')).toBe('Search...')

      await input.setValue('updated')
      expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['updated'])
    })

    it('handles invalid status and small size', () => {
      const wrapper = mount(AdminInput, {
        props: { invalid: true, size: 'small' },
      })
      const input = wrapper.find('input')
      expect(input.classes()).toContain('!border-destructive')
      expect(input.classes()).toContain('text-admin-meta')
      expect(input.attributes('aria-invalid')).toBe('true')
    })

    it('emits enter event on enter keydown', async () => {
      const wrapper = mount(AdminInput)
      const input = wrapper.find('input')
      await input.trigger('keydown.enter')
      expect(wrapper.emitted('enter')).toBeTruthy()
    })
  })

  describe('AdminBadge', () => {
    it('renders Badge with severity and rounded-full by default', () => {
      const wrapper = mount(AdminBadge, {
        props: { severity: 'success', value: 'Active' },
      })
      expect(wrapper.text()).toBe('Active')
      expect(wrapper.classes()).toContain('rounded-full')
      expect(wrapper.classes()).toContain('text-success')
    })

    it('supports rounded false for rounded-md', () => {
      const wrapper = mount(AdminBadge, {
        props: { severity: 'danger', rounded: false },
        slots: { default: 'Banned' },
      })
      expect(wrapper.text()).toBe('Banned')
      expect(wrapper.classes()).toContain('rounded-md')
      expect(wrapper.classes()).toContain('bg-destructive/15')
    })
  })
})
