// BoardCard motion browser contract (computed-style assertions on rendered
// output).
//
// Reads computed styles while exercising default, hover, focus and
// reduced-motion states: hover/focus feedback on the stable card root,
// TransitionGroup appearance/removal at the list boundary, and the
// reduced-motion policy (no transform motion, final state stays visible).
//
// Mounts through plain createApp: the testing-library render wrapper keeps
// TransitionGroup transition hooks from engaging for later mounts in the
// same page, so this file renders every host directly (same i18n + icon
// setup the shared helper provides).

import '@testing-library/jest-dom/vitest'

import { page, commands, userEvent as browserUser } from 'vitest/browser'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { createApp, defineComponent, h, nextTick, ref, TransitionGroup, type App } from 'vue'
import { IconoirProvider } from '@iconoir/vue'
import { i18n } from '../../../../core/i18n'
import '../../../../app/style.css'
import BoardCard from './BoardCard.vue'

declare module 'vitest/internal/browser' {
  interface BrowserCommands {
    emulateMotion: (reducedMotion: 'reduce' | 'no-preference') => Promise<void>
  }
}

const CONTROL_EASING = 'cubic-bezier(0.2, 0.8, 0.2, 1)'

function cardProps() {
  return {
    board: { id: 'board-1', name: 'Campaign', createdAt: 1, updatedAt: 2 },
    sceneEntry: null,
    workspaceId: null,
    isRenaming: false,
    renameDraft: '',
    isMenuOpen: false,
    displayDate: 'today',
  }
}

function styleOf(element: Element): CSSStyleDeclaration {
  return window.getComputedStyle(element)
}

function durationsOf(element: Element): string[] {
  return styleOf(element)
    .transitionDuration.split(',')
    .map((entry) => entry.trim())
    .filter((entry) => entry.length > 0)
}

async function setMotion(isReduced: boolean) {
  await commands.emulateMotion(isReduced ? 'reduce' : 'no-preference')
}

function mountApp(node: () => unknown) {
  const root = document.createElement('div')
  root.style.cssText = 'width: 600px; height: 400px;'
  document.body.append(root)
  const app: App = createApp({
    setup() {
      return () =>
        h(
          IconoirProvider,
          { iconProps: { color: 'currentColor', width: '1.5em', height: '1.5em' } },
          { default: () => node() },
        )
    },
  })
  app.use(i18n)
  app.mount(root)
  return {
    root,
    article: () => root.querySelector('article') as HTMLElement | null,
    unmount() {
      app.unmount()
      root.remove()
    },
  }
}

function renderCard() {
  return mountApp(() => h(BoardCard, cardProps()))
}

/** Host exercising the gallery list boundary: appear + removal. */
function renderListCard(isInitialShow = true) {
  const shouldShow = ref(isInitialShow)
  const Host = defineComponent({
    setup() {
      return () =>
        h(
          TransitionGroup,
          { name: 'board-card', appear: true, tag: 'div' },
          {
            default: () =>
              shouldShow.value ? [h(BoardCard, { ...cardProps(), key: 'board-1' })] : [],
          },
        )
    },
  })
  return { shouldShow, ...mountApp(() => h(Host)) }
}

describe('BoardCard motion browser contract', () => {
  beforeEach(async () => {
    await page.viewport(1400, 800)
    i18n.global.locale.value = 'en'
    await setMotion(false)
  })

  afterEach(async () => {
    document.body.innerHTML = ''
    await setMotion(false)
  })

  it('keeps the workspace owner avatar compact and free of a decorative ring', () => {
    const props = cardProps()
    const { root, unmount } = mountApp(() =>
      h(BoardCard, {
        ...props,
        board: { ...props.board, accessState: 'workspace', creatorUserId: 'usr_alice' },
      }),
    )
    try {
      const avatar = root.querySelector('.size-5')!
      expect(avatar).not.toBeNull()
      expect(avatar.getBoundingClientRect().width).toBe(20)
      expect(avatar.getBoundingClientRect().height).toBe(20)
      expect(styleOf(avatar).boxShadow).toBe('none')
    } finally {
      unmount()
    }
  })

  it('keeps hover and focus feedback on the stable card root', async () => {
    const { root, article, unmount } = renderCard()
    try {
      const card = article()!
      expect(card.classList.contains('board-card')).toBe(true)
      expect(card.getBoundingClientRect().height).toBeCloseTo(244, 0)
      const resting = styleOf(card)
      // border-color + transform both ride the control token (160ms).
      expect(durationsOf(card)).toEqual(['0.16s', '0.16s'])
      expect(resting.transitionTimingFunction).toContain(CONTROL_EASING)

      await browserUser.hover(card)
      expect(card.matches(':hover')).toBe(true)
      expect(styleOf(card).transform).not.toBe('none')

      card.focus()
      expect(card.matches(':focus-visible')).toBe(true)
      expect(styleOf(card).transform).not.toBe('none')

      // The feedback lands on the same stable root, not a replacement node.
      expect(root.querySelector('article')).toBe(card)

      card.blur()
      await browserUser.hover(document.body)
    } finally {
      unmount()
    }
  })

  it('animates card appearance and removal from the board list boundary', async () => {
    const { shouldShow, article, unmount } = renderListCard()
    try {
      // Appear converges to the visible final state.
      await expect
        .poll(() => {
          const card = article()
          if (!card) throw new Error('card missing during appear')
          if (card.className.includes('-from') || card.className.includes('-active')) {
            throw new Error('appear still running')
          }
          return styleOf(card).opacity
        })
        .toBe('1')

      shouldShow.value = false
      await nextTick()
      // Removal runs through the leave transition at the boundary.
      await expect
        .poll(() => article()?.classList.contains('board-card-leave-active') ?? false, {
          interval: 10,
        })
        .toBe(true)
      const leaving = article()!
      expect(leaving.classList.contains('board-card-leave-from')).toBe(true)
      const leavingStyle = styleOf(leaving)
      expect(leavingStyle.transitionProperty).toContain('opacity')
      expect(leavingStyle.transitionProperty).toContain('transform')
      expect(durationsOf(leaving)).toEqual(['0.28s', '0.28s'])
      expect(leavingStyle.transitionTimingFunction).toContain('cubic-bezier(0.2, 0, 0.2, 1)')

      await expect.poll(() => article()).toBeNull()
    } finally {
      unmount()
    }
  })

  it('removes transform motion but keeps the final card state visible when reduced-motion is preferred', async () => {
    await setMotion(true)
    expect(matchMedia('(prefers-reduced-motion: reduce)').matches).toBe(true)
    const { article, unmount } = renderCard()
    try {
      const card = article()!
      const resting = styleOf(card)
      expect(resting.transitionProperty).toContain('border-color')
      expect(resting.transitionProperty).not.toContain('transform')

      await browserUser.hover(card)
      expect(styleOf(card).transform).toBe('none')
      card.focus()
      expect(styleOf(card).transform).toBe('none')
    } finally {
      unmount()
    }

    // Under reduced motion Vue skips enter classes entirely (no transition
    // to run), so the reduced from/to rules are asserted by applying them
    // to the real card; removal is still exercised for real below.
    const { article: listArticle, shouldShow, unmount: unmountList } = renderListCard()
    const probe = listArticle()
    try {
      expect(probe).not.toBeNull()
      probe!.classList.add('board-card-appear-from')
      expect(styleOf(probe!).transform).toBe('none')
      expect(styleOf(probe!).opacity).toBe('1')
      probe!.classList.remove('board-card-appear-from')
      probe!.classList.add('board-card-leave-active', 'board-card-leave-from')
      expect(styleOf(probe!).transitionProperty).toBe('none')
      expect(durationsOf(probe!)).toEqual(['0s'])
      probe!.classList.remove('board-card-leave-active', 'board-card-leave-from')
      probe!.classList.add('board-card-leave-to')
      expect(styleOf(probe!).opacity).toBe('1')
      expect(styleOf(probe!).transform).toBe('none')
      probe!.classList.remove('board-card-leave-to')

      // Real removal completes without the exit transition.
      await expect.poll(() => styleOf(listArticle()!).opacity).toBe('1')
      shouldShow.value = false
      await expect.poll(() => listArticle()).toBeNull()
    } finally {
      unmountList()
    }
  })
})
