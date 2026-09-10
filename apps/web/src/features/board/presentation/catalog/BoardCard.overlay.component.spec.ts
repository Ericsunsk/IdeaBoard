import { describe, expect, it } from 'vitest'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
import BoardCard from './BoardCard.vue'

function renderCard() {
  return renderWithUser(BoardCard, {
    props: {
      board: {
        id: 'board-1',
        name: 'Campaign',
        createdAt: 1,
        updatedAt: 2,
      },
      previewSnapshotDataUrl: '',
      isRenaming: false,
      renameDraft: '',
      isMenuOpen: true,
      displayDate: 'today',
    },
  })
}

describe('BoardCard overlay lifecycle', () => {
  it('cleans up on Escape, restores focus, and can reopen without stale state', async () => {
    const view = renderCard()
    const trigger = screen.getByRole('button', { name: 'More actions' })

    await view.user.click(trigger)
    expect(await screen.findByRole('menuitem', { name: 'Rename' })).toBeVisible()

    await view.user.keyboard('{Escape}')
    await waitFor(() => expect(view.emitted()['menu-hidden']).toHaveLength(1))
    expect(trigger).toHaveFocus()

    await view.rerender({
      board: {
        id: 'board-1',
        name: 'Campaign',
        createdAt: 1,
        updatedAt: 2,
      },
      previewSnapshotDataUrl: '',
      isRenaming: false,
      renameDraft: '',
      isMenuOpen: true,
      displayDate: 'today',
    })
    await view.user.click(trigger)
    expect(await screen.findByRole('menuitem', { name: 'Delete' })).toBeVisible()
  })

  it('runs a selected command after overlay cleanup', async () => {
    const view = renderCard()

    await view.user.click(screen.getByRole('button', { name: 'More actions' }))
    await view.user.click((await screen.findByText('Delete')).closest('a')!)

    await waitFor(() => expect(view.emitted()['menu-hidden']).toHaveLength(1))
    await waitFor(() => expect(view.emitted()['delete-action']).toHaveLength(1))
  })
})
