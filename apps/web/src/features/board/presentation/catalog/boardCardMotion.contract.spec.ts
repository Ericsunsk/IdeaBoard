import { describe, expect, it } from 'vitest'
import boardCard from './BoardCard.vue?raw'
import boardsPage from './BoardsPage.vue?raw'

describe('BoardCard motion contract', () => {
  it('keeps hover and focus feedback on the stable card root', () => {
    expect(boardCard).toContain('class="board-card relative h-[244px]')
    expect(boardCard).toContain('.board-card:hover,\n.board-card:focus-visible')
    expect(boardCard).toContain('var(--motion-control-duration)')
    expect(boardCard).toContain('var(--motion-control-easing)')
  })

  it('animates card appearance and removal from the board list boundary', () => {
    expect(boardsPage).toContain('<TransitionGroup name="board-card" appear>')
    expect(boardsPage).toContain('</TransitionGroup>')
    expect(boardCard).toContain('.board-card-leave-active')
    expect(boardCard).toContain('.board-card-leave-from')
    expect(boardCard).toContain('.board-card-leave-to')
  })

  it('removes transform motion but keeps the final card state visible when reduced-motion is preferred', () => {
    expect(boardCard).toContain('@media (prefers-reduced-motion: reduce)')
    expect(boardCard).toContain('.board-card-leave-active')
    expect(boardCard).toContain('transition: none;')
    expect(boardCard).toContain('transform: none;')
  })
})
