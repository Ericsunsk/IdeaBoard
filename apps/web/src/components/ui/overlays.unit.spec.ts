import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it } from 'vitest'
import { h, nextTick, ref } from 'vue'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from './dialog'
import { Popover, PopoverContent, PopoverTrigger } from './popover'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from './dropdown-menu'

describe('Overlay compound components', () => {
  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('renders Dialog primitives with open state', async () => {
    const open = ref(true)
    const wrapper = mount(
      {
        setup() {
          return () =>
            h(
              Dialog,
              {
                open: open.value,
                'onUpdate:open': (val: boolean) => {
                  open.value = val
                },
              },
              () => [
                h(DialogTrigger, () => h('button', 'Open Dialog')),
                h(DialogContent, { class: 'custom-dialog-class' }, () => [
                  h(DialogHeader, () => [
                    h(DialogTitle, () => 'Dialog Heading'),
                    h(DialogDescription, () => 'Dialog description info'),
                  ]),
                  h('p', 'Dialog Body Text'),
                  h(DialogFooter, () => [h(DialogClose, () => h('button', 'Cancel'))]),
                ]),
              ],
            )
        },
      },
      {
        attachTo: document.body,
      },
    )

    await nextTick()

    // Dialog portal attaches to body
    expect(document.body.innerHTML).toContain('Dialog Heading')
    expect(document.body.innerHTML).toContain('Dialog description info')
    expect(document.body.innerHTML).toContain('Dialog Body Text')

    wrapper.unmount()
  })

  it('renders Popover primitives with open state', async () => {
    const open = ref(true)
    const wrapper = mount(
      {
        setup() {
          return () =>
            h(
              Popover,
              {
                open: open.value,
                'onUpdate:open': (val: boolean) => {
                  open.value = val
                },
              },
              () => [
                h(PopoverTrigger, () => h('button', 'Toggle Popover')),
                h(PopoverContent, { class: 'custom-popover' }, () => 'Popover Content Text'),
              ],
            )
        },
      },
      {
        attachTo: document.body,
      },
    )

    await nextTick()

    expect(document.body.innerHTML).toContain('Popover Content Text')
    wrapper.unmount()
  })

  it('renders DropdownMenu primitives with open state', async () => {
    const open = ref(true)
    const wrapper = mount(
      {
        setup() {
          return () =>
            h(
              DropdownMenu,
              {
                open: open.value,
                'onUpdate:open': (val: boolean) => {
                  open.value = val
                },
              },
              () => [
                h(DropdownMenuTrigger, () => h('button', 'Menu')),
                h(DropdownMenuContent, () => [
                  h(DropdownMenuItem, () => 'Action 1'),
                  h(DropdownMenuSeparator),
                  h(DropdownMenuItem, () => 'Action 2'),
                ]),
              ],
            )
        },
      },
      {
        attachTo: document.body,
      },
    )

    await nextTick()

    expect(document.body.innerHTML).toContain('Action 1')
    expect(document.body.innerHTML).toContain('Action 2')
    wrapper.unmount()
  })
})
