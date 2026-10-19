import { describe, expect, test } from 'vitest'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
import BoardCard from './BoardCard.vue'

function cardProps(overrides: Record<string, unknown> = {}) {
  return {
    board: {
      id: 'board-1',
      name: 'Campaign',
      createdAt: 1,
      updatedAt: 2,
    },
    sceneEntry: null,
    workspaceId: null,
    isRenaming: false,
    renameDraft: '',
    isMenuOpen: true,
    displayDate: 'today',
    ...overrides,
  }
}

describe('BoardCard rename blur guard', () => {
  test('menu rename keeps the input open without a spurious commit', async () => {
    const view = renderWithUser(BoardCard, { props: cardProps() })

    await view.user.click(screen.getByRole('button', { name: 'More actions' }))
    await view.user.click(await screen.findByRole('menuitem', { name: 'Rename' }))
    expect(view.emitted()['rename-action']).toHaveLength(1)

    // Parent startRename flips isRenaming; the menu close steals focus back
    // to the trigger, which must not commit.
    await view.rerender(
      cardProps({ isRenaming: true, renameDraft: 'Campaign' }) as never,
    )
    const input = await screen.findByRole('textbox', { name: 'Rename' })
    expect(input).toBeVisible()

    const trigger = screen.getByRole('button', { name: 'More actions' })
    input.focus()
    expect(input).toHaveFocus()
    trigger.focus()
    input.dispatchEvent(new FocusEvent('blur', { relatedTarget: trigger }))

    expect(view.emitted()['commit-rename']).toBeUndefined()
    expect(screen.getByRole('textbox', { name: 'Rename' })).toBeVisible()
  })

  test('blur to the card trigger does not commit while true outside blur does', async () => {
    const view = renderWithUser(BoardCard, {
      props: cardProps({ isRenaming: true, renameDraft: 'Campaign' }) as never,
    })
    const input = await screen.findByRole('textbox', { name: 'Rename' })
    const trigger = screen.getByRole('button', { name: 'More actions' })

    input.focus()
    trigger.focus()
    input.dispatchEvent(new FocusEvent('blur', { relatedTarget: trigger }))
    expect(view.emitted()['commit-rename']).toBeUndefined()

    input.focus()
    input.dispatchEvent(new FocusEvent('blur', { relatedTarget: document.body }))
    await waitFor(() => expect(view.emitted()['commit-rename']).toHaveLength(1))
  })

  test('Enter commits and Esc cancels', async () => {
    const view = renderWithUser(BoardCard, {
      props: cardProps({ isRenaming: true, renameDraft: 'Campaign' }) as never,
    })
    const input = await screen.findByRole('textbox', { name: 'Rename' })

    input.focus()
    await view.user.keyboard('{Enter}')
    expect(view.emitted()['commit-rename']).toHaveLength(1)

    await view.user.keyboard('{Escape}')
    expect(view.emitted()['cancel-rename']).toHaveLength(1)
  })
})
