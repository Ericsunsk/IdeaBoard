<template>
  <Transition v-bind="transientOverlayMotion">
    <div
      v-if="visible"
      ref="menuRef"
      class="canvas-node-context-menu absolute z-[160] w-40 min-w-40 max-w-40 rounded-[var(--canvas-radius-xl)] bg-[var(--canvas-glass-bg)] p-1.5 shadow-[var(--canvas-shadow-floating)] backdrop-blur-[var(--canvas-glass-blur)] [border:var(--canvas-control-border-width)_solid_var(--canvas-glass-border)]"
      :style="menuStyle"
      role="menu"
      :aria-label="t('canvas.contextMenu.label')"
      tabindex="-1"
      @mousedown.stop
      @click.stop
      @keydown="handleMenuKeydown"
    >
      <button
        v-for="item in items"
        :key="item.key"
        type="button"
        role="menuitem"
        class="h-[34px] w-full cursor-pointer rounded-[var(--canvas-radius-md)] border-0 bg-transparent px-2.5 text-left text-ib-body font-semibold text-[var(--canvas-text-body)] hover:bg-[var(--canvas-accent-soft)] hover:text-[var(--canvas-accent-strong)]"
        @pointerdown.stop.prevent="emit('create', item.key)"
        @keydown.enter.stop.prevent="emit('create', item.key)"
        @keydown.space.stop.prevent="emit('create', item.key)"
      >
        {{ item.label }}
      </button>
    </div>
  </Transition>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { CanvasCreatableNodeKind } from '../../../board/ports/canvas'
import { createPopoverMotionTransition } from '../../../../shared/motion/popoverMotion'
import { handleMenuKeyboardNavigation } from '../../../../shared/interaction/menuKeyboardNavigation'

const props = defineProps<{
  visible: boolean
  x: number
  y: number
  items: ReadonlyArray<{ key: CanvasCreatableNodeKind; label: string }>
}>()

const emit = defineEmits<{
  create: [kind: CanvasCreatableNodeKind]
}>()

const { t } = useI18n()

const transientOverlayMotion = createPopoverMotionTransition()
const menuRef = ref<HTMLElement | null>(null)
const position = ref({ x: props.x, y: props.y })
const menuStyle = computed(() => ({ left: `${position.value.x}px`, top: `${position.value.y}px` }))
let focusReturnTarget: HTMLElement | null = null
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

function resolvePositionContainer(): HTMLElement | null {
  const menu = menuRef.value
  if (menu?.offsetParent instanceof HTMLElement) return menu.offsetParent
  return null
}

function startPositionTracking() {
  stopPositionTracking()
  window.addEventListener('resize', updatePosition)
  window.addEventListener('scroll', updatePosition, true)
  // Re-resolve on every show: the menu is v-if'd, so a mount-time snapshot
  // can miss the container or hold a detached one.
  positionContainer = resolvePositionContainer()
  positionContainer?.addEventListener('scroll', updatePosition)
  if (positionContainer && typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(updatePosition)
    resizeObserver.observe(positionContainer)
  }
}

function stopPositionTracking() {
  window.removeEventListener('resize', updatePosition)
  window.removeEventListener('scroll', updatePosition, true)
  positionContainer?.removeEventListener('scroll', updatePosition)
  positionContainer = null
  resizeObserver?.disconnect()
  resizeObserver = null
}

watch(
  () => [props.visible, props.x, props.y, props.items.length] as const,
  () => void nextTick(updatePosition),
  { immediate: true },
)

watch(
  () => props.visible,
  (visible) => {
    if (visible) {
      focusReturnTarget = resolveFocusReturnTarget()
      void nextTick(() => {
        startPositionTracking()
        menuRef.value?.querySelector<HTMLElement>('[role="menuitem"]')?.focus()
      })
    } else {
      stopPositionTracking()
      const target = focusReturnTarget
      focusReturnTarget = null
      if (target?.isConnected) target.focus()
    }
  },
  { immediate: true },
)

onMounted(() => {
  if (props.visible) void nextTick(startPositionTracking)
})
onBeforeUnmount(stopPositionTracking)

void props

function handleMenuKeydown(event: KeyboardEvent) {
  handleMenuKeyboardNavigation(event, menuRef.value)
}

function resolveFocusReturnTarget() {
  const activeElement = document.activeElement
  return activeElement instanceof HTMLElement && activeElement !== document.body
    ? activeElement
    : null
}
</script>
