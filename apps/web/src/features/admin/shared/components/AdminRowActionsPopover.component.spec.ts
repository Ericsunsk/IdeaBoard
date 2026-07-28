import { defineComponent, ref } from 'vue'
import PrimeVue from 'primevue/config'
import { describe, expect, it } from 'vitest'
import { primeVueTestOptions } from '../../../../test/primeVue'
import { renderWithUser, screen } from '../../../../test/render'
import AdminRowActionsPopover from './AdminRowActionsPopover.vue'

type RowActionsPopoverRef = {
  toggle: (event: Event) => void
}

const Harness = defineComponent({
  components: { AdminRowActionsPopover },
  setup() {
    const popoverRef = ref<RowActionsPopoverRef | null>(null)
    const selectedKey = ref('')

    function open(event: Event) {
      popoverRef.value?.toggle(event)
    }

    return { open, popoverRef, selectedKey }
  },
  template: `
    <button type="button" @click="open">打开操作</button>
    <AdminRowActionsPopover
      ref="popoverRef"
      :items="[
        { key: 'edit', label: '编辑', disabled: true },
        { key: 'delete', label: '删除', danger: true, divided: true },
      ]"
      @select="selectedKey = $event"
    />
    <output aria-label="已选操作">{{ selectedKey }}</output>
  `,
})

describe('AdminRowActionsPopover', () => {
  it('keeps disabled and destructive row actions stable through the shared Popover', async () => {
    const { user } = renderWithUser(Harness, {
      global: { plugins: [[PrimeVue, primeVueTestOptions]] },
    })

    await user.click(screen.getByRole('button', { name: '打开操作' }))

    expect(await screen.findByRole('button', { name: '编辑' })).toBeDisabled()
    await user.click(screen.getByRole('button', { name: '删除' }))
    expect(screen.getByRole('status', { name: '已选操作' })).toHaveTextContent('delete')
  })
})
