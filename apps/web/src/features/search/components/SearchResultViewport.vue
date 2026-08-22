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
import type { SearchResult } from '../searchResource'

const props = withDefaults(
  defineProps<{
    items: SearchResult[]
    compact?: boolean
    scrollRoot?: HTMLElement | null
    overscan?: number
  }>(),
  { compact: false, scrollRoot: null, overscan: 720 },
)

const viewportRef = ref<HTMLElement | null>(null)
const containerWidth = ref(0)
const scrollTop = ref(0)
const viewportHeight = ref(0)
const viewportOffsetTop = ref(0)
let resizeObserver: ResizeObserver | null = null
let frame: number | null = null

const columns = computed(() => {
  if (containerWidth.value < 640) return 1
  if (containerWidth.value < 768) return 2
  if (containerWidth.value < 1024) return 3
  if (containerWidth.value < 1280) return 4
  return props.compact ? 4 : 5
})

const gap = computed(() => (props.compact ? 14 : 16))
const entries = computed(() => {
  const width = containerWidth.value
  if (width <= 0 || props.items.length === 0) return []
  const columnWidth = (width - gap.value * (columns.value - 1)) / columns.value
  const heights = Array.from({ length: columns.value }, () => 0)
  return props.items.map((item, index) => {
    const column = heights.indexOf(Math.min(...heights))
    const height = columnWidth * (item.height > 0 && item.width > 0 ? item.height / item.width : 1)
    const top = heights[column]
    heights[column] += height + gap.value
    return {
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
    }
  })
})

const totalHeight = computed(() => {
  if (entries.value.length === 0) return 0
  return Math.max(...entries.value.map((entry) => entry.top + entry.height))
})

const visibleEntries = computed(() => {
  const start = scrollTop.value - viewportOffsetTop.value - props.overscan
  const end = start + viewportHeight.value + props.overscan * 2
  return entries.value.filter((entry) => entry.top + entry.height >= start && entry.top <= end)
})

function scheduleMeasure() {
  if (frame !== null) return
  const schedule =
    typeof requestAnimationFrame === 'function'
      ? requestAnimationFrame
      : (callback: FrameRequestCallback) => window.setTimeout(callback, 0)
  frame = schedule(() => {
    frame = null
    const element = viewportRef.value
    if (!element) return
    containerWidth.value = element.clientWidth
    const root = props.scrollRoot
    scrollTop.value = root?.scrollTop ?? window.scrollY
    viewportHeight.value = root?.clientHeight ?? window.innerHeight
    const rootTop = root?.getBoundingClientRect().top ?? 0
    viewportOffsetTop.value = element.getBoundingClientRect().top - rootTop + scrollTop.value
  })
}

function attachScrollRoot(root: HTMLElement | null) {
  root?.addEventListener('scroll', scheduleMeasure, { passive: true })
  scheduleMeasure()
  return () => root?.removeEventListener('scroll', scheduleMeasure)
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

watch(() => [props.items.length, props.compact], scheduleMeasure)

onMounted(async () => {
  await nextTick()
  resizeObserver = new ResizeObserver(scheduleMeasure)
  if (viewportRef.value) resizeObserver.observe(viewportRef.value)
  window.addEventListener('resize', scheduleMeasure, { passive: true })
  scheduleMeasure()
})

onBeforeUnmount(() => {
  detachScrollRoot()
  resizeObserver?.disconnect()
  window.removeEventListener('resize', scheduleMeasure)
  if (frame !== null) cancelAnimationFrame(frame)
})
</script>
