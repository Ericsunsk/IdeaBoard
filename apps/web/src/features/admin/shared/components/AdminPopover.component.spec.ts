import { describe, expect, it } from 'vitest'
import { h } from 'vue'
import { renderWithUser, screen } from '../../../../test/render'
import { AdminPopover, AdminPopoverContent, AdminPopoverTrigger } from './adminPopoverAtoms'

describe('AdminPopover', () => {
  it('renders content when open is true', async () => {
    renderWithUser(
      h(
        AdminPopover,
        { open: true },
        {
          default: () => [
            h(
              AdminPopoverTrigger,
              { asChild: true },
              {
                default: () => h('button', { type: 'button' }, 'Trigger'),
              },
            ),
            h(AdminPopoverContent, null, {
              default: () => h('div', 'Popover Body Content'),
            }),
          ],
        },
      ),
    )

    expect(await screen.findByText('Popover Body Content')).toBeInTheDocument()
  })

  it('hides content when open is false', () => {
    renderWithUser(
      h(
        AdminPopover,
        { open: false },
        {
          default: () => [
            h(
              AdminPopoverTrigger,
              { asChild: true },
              {
                default: () => h('button', { type: 'button' }, 'Trigger'),
              },
            ),
            h(AdminPopoverContent, null, {
              default: () => h('div', 'Popover Body Content'),
            }),
          ],
        },
      ),
    )

    expect(screen.queryByText('Popover Body Content')).not.toBeInTheDocument()
  })
})
