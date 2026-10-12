import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { Button, buttonVariants } from './button'
import { Input } from './input'
import { Badge, badgeVariants } from './badge'
import { Avatar, AvatarFallback, AvatarImage } from './avatar'
import { Skeleton } from './skeleton'

describe('Standard shadcn-vue primitives', () => {
  it('generates expected button variant classes', () => {
    expect(buttonVariants({ variant: 'default' })).toContain('bg-primary')
    expect(buttonVariants({ variant: 'outline' })).toContain('border-input')
    expect(buttonVariants({ variant: 'destructive' })).toContain('bg-destructive')
    expect(buttonVariants({ size: 'sm' })).toContain('h-8')
    expect(buttonVariants({ size: 'compact' })).toContain('h-8')
    expect(buttonVariants({ size: 'icon' })).toContain('h-9 w-9')
  })

  it('renders standard Button component with loading and compact size', () => {
    const wrapper = mount(Button, {
      props: { variant: 'outline', size: 'compact', isLoading: true },
      slots: { default: 'Click me' },
    })
    expect(wrapper.text()).toBe('Click me')
    expect(wrapper.classes()).toContain('border-input')
    expect(wrapper.classes()).toContain('h-8')
    expect(wrapper.attributes('aria-busy')).toBe('true')
    expect(wrapper.attributes('disabled')).toBeDefined()
  })

  it('renders standard Input component with model value, compact size and invalid aria state', async () => {
    const wrapper = mount(Input, {
      props: { modelValue: 'hello', size: 'compact', invalid: true },
    })
    const input = wrapper.find('input')
    expect(input.element.value).toBe('hello')
    expect(input.classes()).toContain('h-8')
    expect(input.attributes('aria-invalid')).toBe('true')
  })

  it('renders standard Badge component with variants', () => {
    expect(badgeVariants({ variant: 'destructive' })).toContain('bg-destructive')
    expect(badgeVariants({ variant: 'success' })).toContain('text-emerald-700')
    const wrapper = mount(Badge, {
      props: { variant: 'success', rounded: true },
      slots: { default: 'Active' },
    })
    expect(wrapper.text()).toBe('Active')
    expect(wrapper.classes()).toContain('rounded-full')
    expect(wrapper.classes()).toContain('text-emerald-700')
  })

  it('renders standard Avatar compound component', () => {
    const wrapper = mount({
      components: { Avatar, AvatarFallback, AvatarImage },
      template: `
        <Avatar>
          <AvatarImage src="https://example.com/avatar.png" alt="@user" />
          <AvatarFallback>JD</AvatarFallback>
        </Avatar>
      `,
    })
    expect(wrapper.classes()).toContain('rounded-full')
  })

  it('renders standard Skeleton component', () => {
    const wrapper = mount(Skeleton, {
      props: { class: 'h-4 w-20' },
    })
    expect(wrapper.classes()).toContain('animate-pulse')
    expect(wrapper.classes()).toContain('h-4')
  })
})
