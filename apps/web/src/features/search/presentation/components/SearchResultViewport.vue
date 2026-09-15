<template>
  <div ref="viewportRef" class="relative min-h-0 w-full" :style="{ height: `${contentHeight}px` }">
    <slot
      v-for="entry in renderedEntries"
      :key="entry.key"
      :item="entry.item"
      :index="entry.index"
      :style="entry.style"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useVirtualizer, useWindowVirtualizer, type Virtualizer } from '@tanstack/vue-virtual'
import type { SearchResult } from '../../domain/models'

const props = withDefaults(
  defineProps<{
    items: SearchResult[]
    hasMore?: boolean
    loading?: boolean
    compact?: boolean
    scrollRoot?: HTMLElement | null
    overscan?: number
    activeAnchor?: HTMLElement | null
  }>(),
  {
    compact: false,
    hasMore: false,
    loading: false,
    scrollRoot: null,
    overscan: 720,
    activeAnchor: null,
  },
)

const emit = defineEmits<{
  'active-anchor-hidden': []
  'load-next-page': []
  'scroll-position': [scrollTop: number]
}>()

type RenderedEntry = {
  item: SearchResult
  index: number
  key: string
  style: {
    position: 'absolute'
    top: string
    left: string
    width: string
  }
}

const viewportRef = ref<HTMLElement | null>(null)
const containerWidth = ref(0)
const viewportHeight = ref(0)
// Offset of this container from the start of the scroll content — the
// `scrollMargin` the virtualizers measure from. Stable across scrolls;
// only content above this container moves it.
const contentOffsetTop = ref(0)
let resizeObserver: ResizeObserver | null = null
let paginationRequestedForHeight: number | null = null

// Responsive lane count and geometry mirror the previous hand-rolled masonry:
// shortest-lane-first placement keyed off aspect-ratio estimates.
const laneCount = computed(() => {
  if (containerWidth.value < 640) return 1
  if (containerWidth.value < 768) return 2
  if (containerWidth.value < 1024) return 3
  if (containerWidth.value < 1280) return 4
  return props.compact ? 4 : 5
})

const gapPx = computed(() => (props.compact ? 14 : 16))
const laneWidth = computed(() => {
  const width = containerWidth.value
  if (width <= 0) return 0
  return (width - gapPx.value * (laneCount.value - 1)) / laneCount.value
})

function aspectRatio(item: SearchResult): number {
  return item.height > 0 && item.width > 0 ? item.height / item.width : 1
}

const FALLBACK_CARD_HEIGHT = 320

function estimateCardHeight(index: number): number {
  const item = props.items[index]
  if (!item) return FALLBACK_CARD_HEIGHT
  if (laneWidth.value <= 0) return FALLBACK_CARD_HEIGHT
  return laneWidth.value * aspectRatio(item)
}

function itemKey(index: number): string {
  const item = props.items[index]
  return item ? `${item.provider}:${item.id}` : `${index}`
}

// Window scrolling needs the window-flavored observers, so both virtualizers
// exist and exactly one is enabled. The idle one measures nothing.
const useElementScroll = computed(() => props.scrollRoot !== null)

// TanStack counts `overscan` in items, not pixels. Translate the pixel budget
// (the prop contract) into an item count from the estimated average card
// height. Must not read `getTotalSize()` here: that would feed the virtualizer
// output back into its own options and cause recompute jitter.
const indexOverscan = computed(() => {
  const count = props.items.length
  if (count === 0 || laneWidth.value <= 0) return 4
  const sample = Math.min(count, 20)
  let sum = 0
  for (let i = 0; i < sample; i += 1) {
    const item = props.items[i]
    if (item) sum += laneWidth.value * aspectRatio(item)
  }
  const avg = sum > 0 ? sum / sample : FALLBACK_CARD_HEIGHT
  if (avg <= 0) return 4
  return Math.min(count, Math.max(1, Math.ceil(((props.overscan * 2) / avg) * laneCount.value)))
})

const elementVirtualizer = useVirtualizer(
  computed(() => ({
    count: props.items.length,
    getScrollElement: () => props.scrollRoot,
    estimateSize: estimateCardHeight,
    getItemKey: itemKey,
    lanes: laneCount.value,
    laneAssignmentMode: 'estimate' as const,
    gap: gapPx.value,
    overscan: indexOverscan.value,
    scrollMargin: contentOffsetTop.value,
    enabled: useElementScroll.value,
  })),
)

const windowVirtualizer = useWindowVirtualizer(
  computed(() => ({
    count: props.items.length,
    estimateSize: estimateCardHeight,
    getItemKey: itemKey,
    lanes: laneCount.value,
    laneAssignmentMode: 'estimate' as const,
    gap: gapPx.value,
    overscan: indexOverscan.value,
    scrollMargin: contentOffsetTop.value,
    enabled: !useElementScroll.value,
  })),
)

// NOTE: this must stay a plain function, not a computed. The adapter pushes
// updates via `triggerRef` on a stable instance, and a computed returning that
// same instance reference would swallow the notification (`Object.is` equal),
// silently freezing scroll/windowing updates downstream.
function getActiveVirtualizer(): Virtualizer<HTMLElement, Element> | Virtualizer<Window, Element> {
  return useElementScroll.value ? elementVirtualizer.value : windowVirtualizer.value
}

const renderedEntries = computed<RenderedEntry[]>(() => {
  const virtualizer = getActiveVirtualizer()
  const width = laneWidth.value
  if (width <= 0) return []
  const margin = contentOffsetTop.value
  const gap = gapPx.value
  return virtualizer.getVirtualItems().map((virtual) => {
    const item = props.items[virtual.index]
    // `getVirtualItems` never yields indexes outside `count`, and `count`
    // always equals `items.length`; the guard is for type narrowing only.
    const safeItem = item ?? props.items[0]!
    return {
      item: safeItem,
      index: virtual.index,
      key: virtual.key as string,
      style: {
        position: 'absolute' as const,
        top: `${virtual.start - margin}px`,
        left: `${virtual.lane * (width + gap)}px`,
        width: `${width}px`,
      },
    }
  })
})

const contentHeight = computed(() => {
  if (laneWidth.value <= 0) return 0
  return Math.max(0, getActiveVirtualizer().getTotalSize())
})

function measureContainer() {
  const element = viewportRef.value
  if (!element) return
  containerWidth.value = element.clientWidth
  const root = props.scrollRoot
  viewportHeight.value = root?.clientHeight ?? window.innerHeight
  const scrollTop = root?.scrollTop ?? window.scrollY
  const rootTop = root?.getBoundingClientRect().top ?? 0
  contentOffsetTop.value = element.getBoundingClientRect().top - rootTop + scrollTop
}

// Lane width lives inside the `estimateSize` closure where the virtualizer
// memo cannot see it — rebuild explicitly so assignments never go stale.
watch(laneWidth, () => {
  getActiveVirtualizer().measure()
})

watch(
  () => getActiveVirtualizer().scrollOffset,
  (offset) => {
    if (offset != null) emit('scroll-position', offset)
  },
)

/** The viewport owns geometry and observation only; the feed decides whether a load is valid. */
function requestNextPageIfNeeded() {
  const virtualizer = getActiveVirtualizer()
  if (!props.hasMore || props.loading || laneWidth.value <= 0) return
  const total = virtualizer.getTotalSize()
  if (total <= 0) return
  const viewportBottom = (virtualizer.scrollOffset ?? 0) + viewportHeight.value
  if (viewportBottom + props.overscan < total) return
  if (paginationRequestedForHeight === total) return
  paginationRequestedForHeight = total
  emit('load-next-page')
}

const paginationProbe = computed(() => ({
  offset: getActiveVirtualizer().scrollOffset,
  height: contentHeight.value,
  viewport: viewportHeight.value,
  count: props.items.length,
  hasMore: props.hasMore,
  loading: props.loading,
}))

watch(paginationProbe, () => requestNextPageIfNeeded())

watch(
  () => props.items,
  () => {
    paginationRequestedForHeight = null
  },
)

watch(
  [renderedEntries, () => props.activeAnchor],
  () => {
    const anchor = props.activeAnchor
    if (!anchor) return
    void nextTick(() => {
      if (props.activeAnchor === anchor && !anchor.isConnected) emit('active-anchor-hidden')
    })
  },
  { flush: 'post' },
)

watch(
  () => props.scrollRoot,
  () => {
    measureContainer()
  },
)

onMounted(async () => {
  await nextTick()
  resizeObserver = new ResizeObserver(() => measureContainer())
  if (viewportRef.value) {
    resizeObserver.observe(viewportRef.value)
    if (viewportRef.value.parentElement) resizeObserver.observe(viewportRef.value.parentElement)
  }
  window.addEventListener('resize', measureContainer, { passive: true })
  measureContainer()
  getActiveVirtualizer().measure()
  requestNextPageIfNeeded()
})

onBeforeUnmount(() => {
  if (props.activeAnchor) emit('active-anchor-hidden')
  resizeObserver?.disconnect()
  window.removeEventListener('resize', measureContainer)
})
</script>
