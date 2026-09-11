<template>
  <Transition v-bind="transientOverlayMotion">
    <div
      v-if="visible"
      ref="menuRef"
      class="canvas-node-context-menu absolute z-[160] w-40 min-w-40 max-w-40 rounded-[var(--canvas-radius-xl)] bg-[var(--canvas-glass-bg)] p-1.5 shadow-[var(--canvas-shadow-floating)] backdrop-blur-[var(--canvas-glass-blur)] [border:var(--canvas-control-border-width)_solid_var(--canvas-glass-border)]"
      :style="menuStyle"
      @mousedown.stop
      @click.stop
    >
      <button
        v-for="item in items"
        :key="item.key"
        type="button"
        class="h-[34px] w-full cursor-pointer rounded-[var(--canvas-radius-md)] border-0 bg-transparent px-2.5 text-left text-[13px] font-semibold text-[var(--canvas-text-body)] hover:bg-[var(--canvas-accent-soft)] hover:text-[var(--canvas-accent-strong)]"
        @pointerdown.stop.prevent="emit('create', item.key)"
      >
        {{ item.label }}
      </button>
    </div>
  </Transition>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { CanvasCreatableNodeKind } from '../../../board'
import { createPopoverMotionTransition } from '../../../../shared/motion/popoverMotion'

const props = defineProps<{
  visible: boolean
  x: number
  y: number
  items: ReadonlyArray<{ key: CanvasCreatableNodeKind; label: string }>
}>()

const emit = defineEmits<{
  create: [kind: CanvasCreatableNodeKind]
}>()

const transientOverlayMotion = createPopoverMotionTransition()
const menuRef = ref<HTMLElement | null>(null)
const position = ref({ x: props.x, y: props.y })
const menuStyle = computed(() => ({ left: `${position.value.x}px`, top: `${position.value.y}px` }))
let resizeObserver: ResizeObserver | null = null
let positionContainer: HTMLElement | null = null

function updatePosition() {
  if (!props.visible) return
  const menu = menuRef.value
  const container =
    menu?.offsetParent instanceof HTMLElement ? menu.offsetParent : menu?.parentElement
  const width = menu?.offsetWidth || 160
  const height = menu?.offsetHeight || props.items.length * 34 + 12
  const containerWidth = container?.clientWidth || window.innerWidth
  const containerHeight = container?.clientHeight || window.innerHeight
  const inset = 8
  position.value = {
    x: Math.max(inset, Math.min(props.x, Math.max(inset, containerWidth - width - inset))),
    y: Math.max(inset, Math.min(props.y, Math.max(inset, containerHeight - height - inset))),
  }
}

watch(
  () => [props.visible, props.x, props.y, props.items.length] as const,
  () => void nextTick(updatePosition),
  { immediate: true },
)

onMounted(() => {
  window.addEventListener('resize', updatePosition)
  window.addEventListener('scroll', updatePosition, true)
  positionContainer =
    menuRef.value?.offsetParent instanceof HTMLElement ? menuRef.value.offsetParent : null
  positionContainer?.addEventListener('scroll', updatePosition)
  if (positionContainer && typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(updatePosition)
    resizeObserver.observe(positionContainer)
  }
})
onBeforeUnmount(() => {
  window.removeEventListener('resize', updatePosition)
  window.removeEventListener('scroll', updatePosition, true)
  positionContainer?.removeEventListener('scroll', updatePosition)
  positionContainer = null
  resizeObserver?.disconnect()
  resizeObserver = null
})

void props
</script>
