import { defineComponent, ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { page } from 'vitest/browser'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
import '../../../../app/style.css'
import AccountDialog from './AccountDialog.vue'

const InteractiveAccountContentStub = defineComponent({
  emits: ['close', 'update:busy'],
  setup() {
    return { activeTab: ref('profile') }
  },
  template: `
    <section style="height: 1100px">
      <button type="button" @click="activeTab = 'security'">Security</button>
      <p data-testid="active-account-tab">{{ activeTab }}</p>
      <button type="button" aria-label="Close account" @click="$emit('close')">Close</button>
    </section>
  `,
})

describe('AccountDialog browser contract', () => {
  async function renderInteractiveDialog() {
    await page.viewport(1000, 600)
    const onVisibleUpdate = vi.fn()
    const rendered = renderWithUser(AccountDialog, {
      props: {
        visible: true,
        'onUpdate:visible': onVisibleUpdate,
      },
      global: {
        stubs: {
          AccountDialogContent: InteractiveAccountContentStub,
        },
      },
    })
    await screen.findByRole('dialog', { name: 'Account' })
    return { ...rendered, onVisibleUpdate }
  }

  it('keeps account navigation interactive above the mask', async () => {
    const { user } = await renderInteractiveDialog()

    const securityButton = screen.getByRole('button', { name: 'Security' })
    await user.click(securityButton)
    expect(screen.getByTestId('active-account-tab')).toHaveTextContent('security')
  })

  it('closes from the account close control', async () => {
    const { user, onVisibleUpdate } = await renderInteractiveDialog()

    await user.click(screen.getByRole('button', { name: 'Close account' }))
    await waitFor(() => expect(onVisibleUpdate).toHaveBeenCalledWith(false))
  })

  it('provides a vertical scroll container inside the viewport', async () => {
    await renderInteractiveDialog()

    const dialog = screen.getByRole('dialog', { name: 'Account' })
    const scrollContainer = dialog.firstElementChild as HTMLElement | null
    expect(scrollContainer).not.toBeNull()
    expect(scrollContainer!.scrollHeight).toBeGreaterThan(scrollContainer!.clientHeight)

    scrollContainer!.scrollTop = 240
    scrollContainer!.dispatchEvent(new Event('scroll', { bubbles: true }))
    expect(scrollContainer!.scrollTop).toBeGreaterThan(0)
  })
})
