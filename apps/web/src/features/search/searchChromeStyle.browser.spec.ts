import '@testing-library/jest-dom/vitest'

// Search chrome computed-style browser contract.
//
// Observable replacement for the retired `searchChrome.contract.spec.ts`
// source-string assertions (P3 re-scope reversal, plan-owner 2026-10-11:
// every retired constraint needs an effective computed-style replacement
// passing first). Follows the board precedent
// (`boardCardMotion.browser.spec.ts`): assert rendered output — computed
// background/shadow/blur/border, token application, gutter layout and
// overlay-button fills — never file text.
//
// Coverage map (ledger `src/features/search/searchChrome.contract.spec.ts`):
// - chrome surface without a Search-only layer (scenario 1): dock rendered
//   on the bare shared surface (bg equality + no shadow + shared blur).
// - SearchPage hover border (scenario 1): the library trigger rests on the
//   shared chrome border and hovers to the shared hover-border token,
//   proven by moving the token live (retired 2026-10-11, second pass, once
//   the account lane landed and the page mounted again).
// - popover treatment (scenario 2): primitive-driven open state, Search
//   classes on the rendered node, winning padding, shared blur, stable
//   6px radius, intact pseudo layers.
// - overlay gutter (scenario 3): 10px CSS margin, top-side flip, no inline
//   JS positioning.
// - overlay-btn fills (scenario 4): runtime alias values plus rest/hover
//   paint on both card buttons.

import { page, userEvent as browserUser } from 'vitest/browser'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h, ref } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { renderWithUser, screen, waitFor } from '../../test/render'
import { i18n } from '../../core/i18n'
import { createAuthOverlay, provideAuthOverlay } from '../auth'
import '../../app/style.css'
import './ui/search.css'
import SearchBatchActionDock from './ui/components/SearchBatchActionDock.vue'
import SearchResultCard from './ui/components/SearchResultCard.vue'
import SearchSavePopover from './ui/components/SearchSavePopover.vue'
import SearchPage from './ui/pages/SearchPage.vue'
import type { SearchSavePopoverController } from './ui/browser/savePopoverContract'
import type { SearchResult } from './core/models'

vi.mock('../../core/composables/useAppToast', () => ({
  useAppToast: () => ({ success: vi.fn(), error: vi.fn(), info: vi.fn(), warning: vi.fn() }),
}))
vi.mock('./session/adapters/libraryCoordinator', () => ({
  useSearchLibraryCoordinator: () => ({
    folderFacts: {
      folders: ref([{ id: 'folder-a', name: 'Folder A' }]),
      load: async () => ({ status: 'current' }),
    },
    saveReference: vi.fn(),
    removeReference: vi.fn(),
  }),
}))

// SearchPage mounts the full screen runtime (session, workspace selection,
// library island). The hover-border contract lives in the page template
// itself, so the test mounts the real page with a quiet data runtime: no
// session, no results, island parked — the library trigger still renders.
vi.mock('./session/screenRuntime', () => ({
  screenRuntime: () => ({
    view: {
      search: {
        draftQuery: ref(''),
        queryError: ref(''),
        heading: ref('Search results'),
        references: ref([]),
        referenceSlotCount: ref(0),
        canAddReference: ref(true),
        visionState: ref('idle'),
        fileInput: ref(null),
        buttonTitle: ref('Search'),
      },
      results: {
        items: ref([]),
        hasMore: ref(false),
        loading: ref(false),
        loadingMore: ref(false),
        hasError: ref(false),
        failedImages: ref(new Set()),
        savedKeys: ref(new Set()),
      },
      library: {
        open: ref(false),
        expanded: ref(false),
        folderFacts: ref([]),
        mutationPublisher: {},
        mounted: ref(false),
        isDropTarget: ref(false),
      },
      selection: { count: ref(0) },
      preview: { visible: ref(false), activeResult: ref(null) },
    },
    commands: {
      search: {
        clear: vi.fn(),
        submit: vi.fn(),
        openFilePicker: vi.fn(),
        handleFileInput: vi.fn(),
        removeReference: vi.fn(),
        useResultAsReference: vi.fn(),
        useLibraryReference: vi.fn(),
      },
      results: {
        reload: vi.fn(),
        loadNextPage: vi.fn(),
        recordScroll: vi.fn(),
        markImageError: vi.fn(),
        retryImage: vi.fn(),
        imageRetryCount: () => 0,
        openPreview: vi.fn(),
        closePreview: vi.fn(),
      },
      selection: {
        isSelected: () => false,
        toggle: vi.fn(),
        clear: vi.fn(),
        useAsVisionReferences: vi.fn(),
        saveAllToLibrary: vi.fn(),
      },
      library: {
        toggleForUser: vi.fn(),
        preload: vi.fn(),
        close: vi.fn(),
        openSavePopover: vi.fn(),
        handleCardDragStart: vi.fn(),
        handleCardDragEnd: vi.fn(),
        handleTriggerDragOver: vi.fn(),
        handleTriggerDragLeave: vi.fn(),
        handleTriggerDrop: vi.fn(),
        registerSavePopover: vi.fn(),
        requestSaveAuthentication: vi.fn(),
        setSavePopoverAnchor: vi.fn(),
        handleAnchorHidden: vi.fn(),
      },
    },
    savePopoverAnchor: ref(null),
    workspaceId: ref('workspace-test'),
  }),
}))

const searchResult: SearchResult = {
  id: 'photo',
  provider: 'unsplash',
  title: 'Photo',
  author: 'Author',
  author_url: 'https://example.com/author',
  source_url: 'https://example.com/photo',
  thumbnail_url: 'https://example.com/thumb',
  preview_url: 'https://example.com/preview',
  original_url: 'https://example.com/original',
  width: 100,
  height: 100,
  media_kind: 'image',
}

const PopoverHost = defineComponent({
  components: { SearchSavePopover },
  setup() {
    const popover = ref<SearchSavePopoverController>()
    function open(event: Event) {
      popover.value?.toggle(event, { result: searchResult, authenticated: true, saved: false })
    }
    return { popover, open }
  },
  template: '<button @click="open">Open save menu</button><SearchSavePopover ref="popover" />',
})

function styleOf(element: Element): CSSStyleDeclaration {
  return window.getComputedStyle(element)
}

function overlayButtons(container: Element): HTMLElement[] {
  return [...container.querySelectorAll('button')].filter((button) =>
    (button.getAttribute('class') ?? '').includes('search-overlay-btn-bg'),
  ) as HTMLElement[]
}

async function openSavePopover(user: { click: (element: Element) => Promise<void> }) {
  await user.click(screen.getByRole('button', { name: 'Open save menu' }))
  return waitFor(() => {
    const content = document.body.querySelector('.search-save-popover')
    if (!content) throw new Error('save popover content missing')
    return content as HTMLElement
  })
}

async function renderSearchPage() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/', component: { template: '<div />' } }],
  })
  await router.push('/')
  await router.isReady()
  const Harness = defineComponent({
    setup() {
      provideAuthOverlay(createAuthOverlay())
      return () => h(SearchPage)
    },
  })
  return renderWithUser(Harness, {
    global: {
      plugins: [router],
      stubs: [
        'WorkspaceHeader',
        'SearchEmptyState',
        'SearchResultViewport',
        'SearchResultCard',
        'SearchSavePopover',
        'SearchLightboxModal',
        'SearchBatchActionDock',
        'LibraryFloatingIsland',
      ],
    },
  })
}

describe('Search chrome style browser contract', () => {
  beforeEach(async () => {
    await page.viewport(1400, 800)
    document.documentElement.classList.remove('dark')
    i18n.global.locale.value = 'en'
  })

  afterEach(async () => {
    await browserUser.hover(document.body)
  })

  it('renders the batch dock on the shared chrome surface with no Search-only layer', async () => {
    const rendered = renderWithUser(SearchBatchActionDock, { props: { selectedCount: 2 } })
    try {
      const dock = await waitFor(() => {
        const node = document.body.querySelector('.search-batch-action-dock')
        if (!node) throw new Error('batch dock missing')
        return node as HTMLElement
      })
      expect(dock.classList.contains('page-chrome-surface')).toBe(true)
      const probe = document.createElement('div')
      probe.className = 'page-chrome-surface'
      document.body.append(probe)
      try {
        const dockStyle = styleOf(dock)
        // No Search-only background layer: the dock paints exactly the bare
        // shared surface background with no extra image layer.
        expect(dockStyle.backgroundImage).toBe('none')
        expect(dockStyle.backgroundColor).toBe(styleOf(probe).backgroundColor)
        // No Search-only dock shadow redeclare.
        expect(dockStyle.boxShadow).toBe('none')
        // No Search-only glass blur: only the shared surface blur applies.
        expect(dockStyle.backdropFilter).toBe('blur(12px)')
      } finally {
        probe.remove()
      }
    } finally {
      rendered.unmount()
    }
  })

  it('keeps the Search popover treatment on the shared surface through the popover primitive', async () => {
    const rendered = renderWithUser(PopoverHost)
    try {
      const content = await openSavePopover(rendered.user)
      // The popover opens through the Radix primitive (state/side owned by
      // the primitive, not Search JS).
      expect(content.getAttribute('data-state')).toBe('open')
      expect(content.getAttribute('data-side')).toBe('bottom')
      // The Search treatment classes reach the rendered node.
      expect(content.classList.contains('search-save-popover')).toBe(true)
      expect(content.classList.contains('page-chrome-surface')).toBe(true)
      expect(content.classList.contains('rounded-control')).toBe(true)
      const contentStyle = styleOf(content)
      // Search padding wins over the primitive default (p-4) and is never zeroed.
      expect(contentStyle.paddingTop).toBe('6px')
      // The shared surface class is effective, not decorative: its blur applies.
      expect(contentStyle.backdropFilter).toBe('blur(12px)')
      // Primitive default radius wins the cascade; the Search token class
      // rides along without redeclaring radius as an arbitrary value (which
      // tailwind-merge would keep alone, jumping the radius to 10px).
      expect(contentStyle.borderRadius).toBe('6px')
      // Decorative pseudo layers stay intact (never force-hidden).
      expect(window.getComputedStyle(content, '::before').display).not.toBe('none')
      expect(window.getComputedStyle(content, '::after').display).not.toBe('none')
    } finally {
      rendered.unmount()
    }
  })

  it('keeps the overlay gutter in CSS for directional alignment', async () => {
    const rendered = renderWithUser(PopoverHost)
    try {
      const content = await openSavePopover(rendered.user)
      // Gutter comes from the Search stylesheet, not inline JS positioning.
      expect(content.style.marginBlockStart).toBe('')
      expect(content.style.top).toBe('')
      expect(styleOf(content).marginBlockStart).toBe(
        styleOf(document.documentElement).getPropertyValue('--spacing-overlay-gutter').trim(),
      )
      expect(styleOf(content).marginBlockStart).toBe('10px')
      // Directional rule: flipping the primitive side flips the gutter via CSS alone.
      content.setAttribute('data-side', 'top')
      expect(styleOf(content).marginBlockStart).toBe('-10px')
      expect(styleOf(content).marginBlockEnd).toBe('10px')
    } finally {
      rendered.unmount()
    }
  })

  it('sources image overlay action fills from the app overlay-btn tokens', async () => {
    const rootStyle = styleOf(document.documentElement)
    // Search aliases resolve end to end to the canonical app token values.
    expect(rootStyle.getPropertyValue('--search-overlay-btn-bg').trim()).toBe('rgba(0, 0, 0, 0.42)')
    expect(rootStyle.getPropertyValue('--search-overlay-btn-bg-hover').trim()).toBe(
      'rgba(0, 0, 0, 0.68)',
    )
    const rendered = renderWithUser(SearchResultCard, {
      props: {
        result: searchResult,
        index: 5,
        isSelected: false,
        isUsedAsReference: false,
        isSaved: false,
        hasImageError: false,
        imageRetryCount: 0,
      },
    })
    try {
      const buttons = overlayButtons(rendered.container)
      expect(buttons.length).toBe(2)
      for (const button of buttons) {
        expect(styleOf(button).backgroundColor).toBe('rgba(0, 0, 0, 0.42)')
      }
      const selectButton = buttons.find((button) =>
        (button.getAttribute('class') ?? '').includes('search-overlay-btn-bg-hover'),
      )
      expect(selectButton).toBeDefined()
      // Provenance: the Search alias tracks the app token live — moving the
      // app token moves the card paint, so a hardcoded Search-side value (or
      // a dropped var indirection) cannot satisfy this.
      document.documentElement.style.setProperty('--color-overlay-btn-bg', 'rgb(255, 0, 0)')
      try {
        await expect.poll(() => styleOf(selectButton!).backgroundColor).toBe('rgb(255, 0, 0)')
      } finally {
        document.documentElement.style.removeProperty('--color-overlay-btn-bg')
      }
      await expect.poll(() => styleOf(selectButton!).backgroundColor).toBe('rgba(0, 0, 0, 0.42)')
      await browserUser.hover(selectButton!)
      expect(selectButton!.matches(':hover')).toBe(true)
      // Hover fill rides the 160ms control transition; poll to its end state.
      await expect.poll(() => styleOf(selectButton!).backgroundColor).toBe('rgba(0, 0, 0, 0.68)')
    } finally {
      rendered.unmount()
    }
  })

  it('keeps the SearchPage library trigger hover border on the shared chrome token', async () => {
    const rendered = await renderSearchPage()
    try {
      const trigger = screen.getByRole('button', { name: 'Toggle personal library' })
      expect(trigger.classList.contains('page-chrome-surface')).toBe(true)
      // Rest paint rides the shared chrome border, hover paint the shared
      // hover border — both resolved through live probes so a hardcoded
      // Search-side value cannot satisfy this.
      const restProbe = document.createElement('div')
      restProbe.style.borderStyle = 'solid'
      restProbe.style.borderColor = 'var(--color-page-chrome-border)'
      document.body.append(restProbe)
      const hoverProbe = document.createElement('div')
      hoverProbe.style.borderStyle = 'solid'
      hoverProbe.style.borderColor = 'var(--color-page-chrome-border-hover)'
      document.body.append(hoverProbe)
      try {
        expect(styleOf(trigger).borderStyle).toBe('solid')
        expect(styleOf(trigger).borderColor).toBe(styleOf(restProbe).borderColor)
        // Provenance: the hover rule tracks the shared token live — moving
        // the token moves the trigger paint, so a hardcoded hover value (or
        // a dropped hover rule) cannot satisfy this.
        document.documentElement.style.setProperty(
          '--color-page-chrome-border-hover',
          'rgb(255, 0, 0)',
        )
        try {
          await browserUser.hover(trigger)
          expect(trigger.matches(':hover')).toBe(true)
          // Hover border rides the control transition; poll to its end state.
          await expect.poll(() => styleOf(trigger).borderColor).toBe('rgb(255, 0, 0)')
        } finally {
          document.documentElement.style.removeProperty('--color-page-chrome-border-hover')
        }
        await expect.poll(() => styleOf(trigger).borderColor).toBe(
          styleOf(hoverProbe).borderColor,
        )
      } finally {
        restProbe.remove()
        hoverProbe.remove()
      }
    } finally {
      rendered.unmount()
    }
  })
})
