<template>
  <div ref="viewportRef" class="relative min-h-0 w-full" :style="{ height: `${totalHeight}px` }">
    <slot
      v-for="entry in visibleEntries"
      :key="entry.key"
      :item="entry.item"
      :index="entry.index"
      :style="entry.style"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { SearchResult } from '../../domain/searchModels'

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

const viewportRef = ref<HTMLElement | null>(null)
const containerWidth = ref(0)
const scrollTop = ref(0)
const viewportHeight = ref(0)
const viewportOffsetTop = ref(0)
let resizeObserver: ResizeObserver | null = null
let frame: number | null = null
let needsLayoutMeasure = false
let paginationRequestedForHeight: number | null = null

type LayoutEntry = {
  item: SearchResult
  index: number
  key: string
  top: number
  height: number
  style: {
    position: 'absolute'
    top: string
    left: string
    width: string
  }
}

const columns = computed(() => {
  if (containerWidth.value < 640) return 1
  if (containerWidth.value < 768) return 2
  if (containerWidth.value < 1024) return 3
  if (containerWidth.value < 1280) return 4
  return props.compact ? 4 : 5
})

const gap = computed(() => (props.compact ? 14 : 16))
const layout = computed(() => {
  const width = containerWidth.value
  if (width <= 0 || props.items.length === 0) {
    return { columns: [] as LayoutEntry[][], totalHeight: 0 }
  }
  const columnWidth = (width - gap.value * (columns.value - 1)) / columns.value
  const heights = Array.from({ length: columns.value }, () => 0)
  const columnEntries = Array.from({ length: columns.value }, () => [] as LayoutEntry[])
  props.items.forEach((item, index) => {
    const column = heights.indexOf(Math.min(...heights))
    const height = columnWidth * (item.height > 0 && item.width > 0 ? item.height / item.width : 1)
    const top = heights[column] ?? 0
    heights[column] = top + height + gap.value
    columnEntries[column]?.push({
      item,
      index,
      key: `${item.provider}:${item.id}`,
      top,
      height,
      style: {
        position: 'absolute' as const,
        top: `${top}px`,
        left: `${column * (columnWidth + gap.value)}px`,
        width: `${columnWidth}px`,
      },
    })
  })
  return {
    columns: columnEntries,
    totalHeight: Math.max(...heights) - gap.value,
  }
})

const totalHeight = computed(() => layout.value.totalHeight)

const visibleEntries = computed(() => {
  const start = scrollTop.value - viewportOffsetTop.value - props.overscan
  const end = start + viewportHeight.value + props.overscan * 2
  const visible: LayoutEntry[] = []
  for (const column of layout.value.columns) {
    let low = 0
    let high = column.length
    while (low < high) {
      const middle = (low + high) >>> 1
      const entry = column[middle]
      if (entry && entry.top + entry.height < start) low = middle + 1
      else high = middle
    }
    for (let index = low; index < column.length; index += 1) {
      const entry = column[index]
      if (!entry || entry.top > end) break
      visible.push(entry)
    }
  }
  return visible.sort((left, right) => left.index - right.index)
})

function scheduleFrame(measureLayout: boolean) {
  needsLayoutMeasure ||= measureLayout
  if (frame !== null) return
  frame = requestAnimationFrame(() => {
    frame = null
    const element = viewportRef.value
    if (!element) return
    const root = props.scrollRoot
    scrollTop.value = root?.scrollTop ?? window.scrollY
    emit('scroll-position', scrollTop.value)
    if (needsLayoutMeasure) {
      needsLayoutMeasure = false
      containerWidth.value = element.clientWidth
      viewportHeight.value = root?.clientHeight ?? window.innerHeight
      const rootTop = root?.getBoundingClientRect().top ?? 0
      viewportOffsetTop.value = element.getBoundingClientRect().top - rootTop + scrollTop.value
    }
    requestNextPageIfNeeded()
  })
}

/** The viewport owns geometry and observation only; the feed decides whether a load is valid. */
function requestNextPageIfNeeded() {
  if (!props.hasMore || props.loading || totalHeight.value <= 0) return
  const viewportBottom = scrollTop.value - viewportOffsetTop.value + viewportHeight.value
  if (viewportBottom + props.overscan < totalHeight.value) return
  if (paginationRequestedForHeight === totalHeight.value) return
  paginationRequestedForHeight = totalHeight.value
  emit('load-next-page')
}

function scheduleScrollUpdate() {
  scheduleFrame(false)
}

function scheduleLayoutMeasure() {
  scheduleFrame(true)
}

function attachScrollRoot(root: HTMLElement | null) {
  root?.addEventListener('scroll', scheduleScrollUpdate, { passive: true })
  scheduleLayoutMeasure()
  return () => root?.removeEventListener('scroll', scheduleScrollUpdate)
}

let detachScrollRoot = () => {}
watch(
  () => props.scrollRoot,
  (root) => {
    detachScrollRoot()
    detachScrollRoot = attachScrollRoot(root)
  },
  { immediate: true },
)

watch(
  () => props.items,
  () => {
    paginationRequestedForHeight = null
    scheduleLayoutMeasure()
  },
)
watch(() => [props.compact, props.hasMore, props.loading], scheduleLayoutMeasure)
watch(
  [visibleEntries, () => props.activeAnchor],
  () => {
    const anchor = props.activeAnchor
    if (!anchor) return
    void nextTick(() => {
      if (props.activeAnchor === anchor && !anchor.isConnected) emit('active-anchor-hidden')
    })
  },
  { flush: 'post' },
)

onMounted(async () => {
  await nextTick()
  resizeObserver = new ResizeObserver(scheduleLayoutMeasure)
  if (viewportRef.value) {
    resizeObserver.observe(viewportRef.value)
    if (viewportRef.value.parentElement) resizeObserver.observe(viewportRef.value.parentElement)
  }
  window.addEventListener('resize', scheduleLayoutMeasure, { passive: true })
  scheduleLayoutMeasure()
})

onBeforeUnmount(() => {
  if (props.activeAnchor) emit('active-anchor-hidden')
  detachScrollRoot()
  resizeObserver?.disconnect()
  window.removeEventListener('resize', scheduleLayoutMeasure)
  if (frame !== null) cancelAnimationFrame(frame)
})
</script>
