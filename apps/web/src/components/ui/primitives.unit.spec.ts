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
    expect(buttonVariants({ size: 'icon' })).toContain('h-9 w-9')
  })

  it('renders standard Button component', () => {
    const wrapper = mount(Button, {
      props: { variant: 'outline', size: 'sm' },
      slots: { default: 'Click me' },
    })
    expect(wrapper.text()).toBe('Click me')
    expect(wrapper.classes()).toContain('border-input')
  })

  it('renders standard Input component with model value', async () => {
    const wrapper = mount(Input, {
      props: { modelValue: 'hello' },
    })
    const input = wrapper.find('input')
    expect(input.element.value).toBe('hello')
  })

  it('renders standard Badge component with variants', () => {
    expect(badgeVariants({ variant: 'destructive' })).toContain('bg-destructive')
    const wrapper = mount(Badge, {
      props: { variant: 'destructive' },
      slots: { default: 'Alert' },
    })
    expect(wrapper.text()).toBe('Alert')
    expect(wrapper.classes()).toContain('bg-destructive')
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
