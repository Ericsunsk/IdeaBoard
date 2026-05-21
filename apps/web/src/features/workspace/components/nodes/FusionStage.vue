<template>
  <div
    ref="stageRef"
    class="fusion-node-stage node-tool-surface"
    data-flow-handle-frame
    :style="fusionStageStyle"
    :class="{
      'is-active': active,
      'is-drop-target': addSlotDragOver,
      'is-blend-dragging': blendDragging,
    }"
    @dragenter.stop.prevent="onAddSlotDragOver"
    @dragover.stop.prevent="onAddSlotDragOver"
    @dragleave.stop="onAddSlotDragLeave"
    @drop.stop.prevent="onAddSlotDrop"
    @contextmenu.prevent.stop="emit('contextmenu', $event)"
  >
    <div v-if="fusionRays.length" class="fusion-node-connection-layer" aria-hidden="true">
      <span
        v-for="ray in fusionRays"
        :key="ray.key"
        v-motion="fusionRayMotion"
        class="fusion-node-ray"
        :style="ray.style"
      />
    </div>
    <div ref="thumbsRef" class="fusion-node-slots">
      <template v-for="(slot, index) in fusionSlotItems" :key="slot.key">
        <div
          v-if="slot.image"
          v-motion="fusionSlotMotion"
          class="fusion-node-slot fusion-node-slot-item"
          :style="getSlotStyle(index)"
          role="button"
          tabindex="0"
          :data-image-id="slot.image.id"
          @mousedown.stop
        >
          <img :src="slot.image.url" alt="" draggable="false" />
          <button
            class="fusion-node-slot-remove"
            type="button"
            title="移除图片输入"
            aria-label="移除图片输入"
            @mousedown.stop
            @pointerdown.stop
            @click.stop="removeInputImage(slot.image.id)"
          >
            <svg
              width="10"
              height="10"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2.4"
              stroke-linecap="round"
            >
              <path d="M6 6l12 12" />
              <path d="M18 6L6 18" />
            </svg>
          </button>
        </div>
        <div
          v-else
          class="fusion-node-slot fusion-node-slot-placeholder"
          :style="getSlotStyle(index)"
          aria-hidden="true"
        />
      </template>
    </div>
    <div v-if="showBlendPad" class="fusion-blend-layer" aria-label="融合权重控制">
      <button
        v-motion="fusionHandleMotion"
        class="fusion-blend-handle"
        :class="{ 'is-hover': blendHandleHover, 'is-dragging': blendDragging }"
        type="button"
        :style="blendHandleStyle"
        title="拖动调整融合权重，双击重置"
        @pointerdown.stop.prevent="onBlendHandlePointerDown"
        @pointerenter="onBlendHandlePointerEnter"
        @pointerleave="onBlendHandlePointerLeave"
        @dblclick.stop.prevent="onBlendHandleDoubleClick"
      >
        <span class="fusion-blend-handle-dot" />
      </button>
      <div v-if="showBlendWeightBadges" class="fusion-blend-weight-layer">
        <div
          v-for="badge in blendWeightBadges"
          :key="badge.key"
          v-motion="fusionBadgeMotion"
          class="fusion-blend-weight-badge"
          :style="getBlendWeightBadgeStyle(badge.index)"
        >
          {{ badge.percent }}%
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import type { FusionNodeItem } from '../../types/canvasNodes'
import {
  FUSION_SLOT_LAYOUT,
  buildFusionDisplayPercentages,
  computeFusionBlendWeights,
  getFusionLineAnchors,
  getFusionSlotAnchor,
  getFusionSlotAnchors,
  type FusionBlendPoint,
} from '../../utils/fusionBlend'
import { useFusionBlendControl } from './composables/useFusionBlendControl'
import { useSortableMediaInputs } from './composables/useSortableMediaInputs'

const props = defineProps<{
  active: boolean
  blendPoint: FusionBlendPoint
  inputImages: FusionNodeItem['inputImages']
  width: number
}>()

const emit = defineEmits<{
  (e: 'update:blend-point', value: FusionBlendPoint): void
  (e: 'update:input-images', value: FusionNodeItem['inputImages']): void
  (e: 'input-image-drop', event: DragEvent): void
  (e: 'input-image-remove', value: FusionNodeItem['inputImages'][number]): void
  (e: 'contextmenu', event: MouseEvent): void
}>()

const thumbsRef = ref<HTMLElement | null>(null)
const stageRef = ref<HTMLElement | null>(null)
const addSlotDragOver = ref(false)
const maxInputImages = FUSION_SLOT_LAYOUT.maxSlots
const fusionSlotMotion = {
  initial: { opacity: 0, scale: 0.9 },
  enter: {
    opacity: 1,
    scale: 1,
    transition: { type: 'spring', stiffness: 420, damping: 30 },
  },
}
const fusionRayMotion = {
  initial: { opacity: 0 },
  enter: {
    opacity: 1,
    transition: { duration: 160, easing: 'ease-out' },
  },
}
const fusionBadgeMotion = {
  initial: { opacity: 0, scale: 0.92 },
  enter: {
    opacity: 1,
    scale: 1,
    transition: { duration: 140, easing: 'ease-out' },
  },
}
const fusionHandleMotion = {
  initial: { opacity: 0 },
  enter: {
    opacity: 1,
    transition: { duration: 140, easing: 'ease-out' },
  },
}

const visibleInputImages = computed(() => props.inputImages.slice(0, maxInputImages))
const canAcceptMoreImages = computed(() => visibleInputImages.value.length < maxInputImages)
const imageCount = computed(() => Math.min(visibleInputImages.value.length || 0, maxInputImages))
const showBlendPad = computed(() => imageCount.value >= 2)
const nodeWidth = computed(() => props.width)
const sourceBlendPoint = computed(() => props.blendPoint)
const { sortableItems: sortableInputImages } = useSortableMediaInputs({
  containerRef: thumbsRef,
  sourceItems: visibleInputImages,
  draggableSelector: '.fusion-node-slot-item',
  filterSelector: '.fusion-node-slot-remove',
  fallbackClass: 'fusion-node-sortable-fallback',
  onUpdate: (next) => emit('update:input-images', next),
})
const {
  activeBlendPoint,
  blendDragging,
  blendHandleHover,
  blendHandleStyle,
  onBlendHandleDoubleClick,
  onBlendHandlePointerDown,
  onBlendHandlePointerEnter,
  onBlendHandlePointerLeave,
} = useFusionBlendControl({
  stageRef,
  enabled: showBlendPad,
  imageCount,
  blendPoint: sourceBlendPoint,
  nodeWidth,
  onCommit: (point) => emit('update:blend-point', point),
})

const fusionSlotItems = computed(() =>
  Array.from({ length: maxInputImages }, (_, index) => {
    const image = sortableInputImages.value[index] ?? null
    return {
      key: image?.id ?? `empty-${index}`,
      image,
    }
  }),
)
const blendWeights = computed(() =>
  computeFusionBlendWeights(imageCount.value, activeBlendPoint.value, props.width),
)
const blendWeightPercents = computed(() => buildFusionDisplayPercentages(blendWeights.value))
const fusionStageStyle = computed(() => ({
  '--fusion-slot-inset': `${FUSION_SLOT_LAYOUT.inset}px`,
  '--fusion-slot-gap': `${FUSION_SLOT_LAYOUT.gap}px`,
  '--fusion-slot-min-size': `${FUSION_SLOT_LAYOUT.minSize}px`,
  '--fusion-slot-max-size': `${FUSION_SLOT_LAYOUT.maxSize}px`,
}))
const visualSlotAnchors = computed(() => getFusionSlotAnchors(imageCount.value, props.width))
const lineSlotAnchors = computed(() => getFusionLineAnchors(imageCount.value, props.width))
const blendWeightBadges = computed(() =>
  fusionSlotItems.value
    .map((slot, index) =>
      slot.image && index < blendWeightPercents.value.length
        ? { key: `${slot.key}-weight`, index, percent: blendWeightPercents.value[index] ?? 0 }
        : null,
    )
    .filter(
      (
        badge,
      ): badge is {
        key: string
        index: number
        percent: number
      } => !!badge,
    ),
)
const showBlendWeightBadges = computed(
  () => showBlendPad.value && (blendDragging.value || blendHandleHover.value),
)
const fusionRays = computed(() => {
  if (!showBlendPad.value) return []
  return lineSlotAnchors.value.map((anchor, index) => {
    const dx = anchor.x - activeBlendPoint.value.x
    const dy = anchor.y - activeBlendPoint.value.y
    return {
      key: visibleInputImages.value[index]?.id ?? `ray-${index}`,
      style: {
        left: `${activeBlendPoint.value.x * 100}%`,
        top: `${activeBlendPoint.value.y * 100}%`,
        width: `${Math.hypot(dx, dy) * 100}%`,
        transform: `rotate(${Math.atan2(dy, dx)}rad)`,
      },
    }
  })
})

function getSlotStyle(index: number) {
  const anchor = getFusionSlotAnchor(imageCount.value || maxInputImages, index, props.width)
  return {
    left: `${anchor.x * 100}%`,
    top: `${anchor.y * 100}%`,
  }
}

function getBlendWeightBadgeStyle(index: number) {
  const anchor = visualSlotAnchors.value[index]
  if (!anchor) {
    return {
      left: '50%',
      top: '50%',
    }
  }
  return {
    left: `${anchor.x * 100}%`,
    top: `${anchor.y * 100}%`,
  }
}

function removeInputImage(imageId: string) {
  const item = visibleInputImages.value.find((inputImage) => inputImage.id === imageId)
  if (!item) return
  emit('input-image-remove', item)
}

function onAddSlotDragOver(event: DragEvent) {
  if (!canAcceptMoreImages.value) return
  addSlotDragOver.value = true
  if (event.dataTransfer) event.dataTransfer.dropEffect = 'copy'
}

function onAddSlotDragLeave() {
  addSlotDragOver.value = false
}

function onAddSlotDrop(event: DragEvent) {
  addSlotDragOver.value = false
  if (!canAcceptMoreImages.value) return
  emit('input-image-drop', event)
}
</script>

<style scoped src="./styles/node-tool-buttons.css"></style>
<style scoped>
.fusion-node-stage {
  --fusion-slot-inset: 20px;
  --fusion-slot-gap: 6px;
  --fusion-slot-min-size: 46px;
  --fusion-slot-max-size: 58px;
  --fusion-slot-size: clamp(
    var(--fusion-slot-min-size),
    calc((100% - (var(--fusion-slot-inset) * 2) - (var(--fusion-slot-gap) * 3)) / 4),
    var(--fusion-slot-max-size)
  );
  position: relative;
  width: 100%;
  aspect-ratio: 1;
  min-height: 280px;
  border: 1.5px solid #e2e8f0;
  border-radius: 11px;
  background: #ffffff;
  box-sizing: border-box;
  transition: border-color 0.2s ease;
  overflow: hidden;
}

.fusion-node-connection-layer {
  position: absolute;
  inset: 0;
  z-index: 1;
  pointer-events: none;
}

.fusion-node-ray {
  --fusion-node-ray-color: var(--flow-edge-color, #b1b1b7);
  position: absolute;
  height: calc(var(--flow-edge-width, 2) * 1px);
  border-radius: 999px;
  background-image: repeating-linear-gradient(
    90deg,
    var(--fusion-node-ray-color) 0 5px,
    transparent 5px 10px
  );
  background-size: 10px 100%;
  transform-origin: left center;
  translate: 0 -50%;
  animation: fusion-node-ray-flow var(--flow-edge-running-duration, 0.5s) linear infinite;
  transition:
    left 0.24s cubic-bezier(0.22, 1, 0.36, 1),
    top 0.24s cubic-bezier(0.22, 1, 0.36, 1),
    width 0.24s cubic-bezier(0.22, 1, 0.36, 1),
    transform 0.24s cubic-bezier(0.22, 1, 0.36, 1),
    background-image 0.16s ease;
  will-change: left, top, width, transform;
}

.fusion-node-slots {
  position: absolute;
  inset: 0;
  z-index: 2;
}

.fusion-node-stage.is-active,
.fusion-node-stage.is-drop-target {
  border-color: #9278e6;
}

.fusion-node-stage.is-active .fusion-node-ray,
.fusion-node-stage.is-drop-target .fusion-node-ray {
  --fusion-node-ray-color: var(--flow-edge-active-color, #9278e6);
}

.fusion-node-slot {
  position: absolute;
  width: var(--fusion-slot-size);
  height: auto;
  aspect-ratio: 1 / 1;
  border: 1px solid var(--node-tool-button-border);
  border-radius: 8px;
  background: #ffffff;
  overflow: hidden;
  padding: 0;
  translate: -50% -50%;
  transition:
    left 0.24s cubic-bezier(0.22, 1, 0.36, 1),
    top 0.24s cubic-bezier(0.22, 1, 0.36, 1),
    background-color 0.16s ease,
    border-color 0.16s ease,
    color 0.16s ease,
    opacity 0.16s ease;
  will-change: left, top, transform;
}

.fusion-node-slot-item {
  cursor: grab;
}

.fusion-node-slot:hover,
.fusion-node-slot:focus-visible,
.fusion-node-slot.is-sortable-chosen {
  border-color: var(--node-tool-button-accent);
  background: #f8f7ff;
  color: #272932;
}

.fusion-node-slot.is-sortable-chosen,
.fusion-node-slot.is-sortable-drag {
  cursor: grabbing;
}

.fusion-node-slot.is-sortable-ghost {
  opacity: 0.45;
}

.fusion-node-slot img {
  width: 100%;
  height: 100%;
  display: block;
  border-radius: calc(8px - 1px);
  object-fit: cover;
  pointer-events: none;
}

.fusion-node-slot-remove {
  position: absolute;
  top: 3px;
  right: 3px;
  width: 16px;
  height: 16px;
  border: 1px solid rgba(226, 232, 240, 0.92);
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.94);
  color: #64748b;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  opacity: 0;
  transform: scale(0.92);
  transition:
    opacity 0.14s ease,
    transform 0.14s ease,
    color 0.14s ease,
    border-color 0.14s ease;
}

.fusion-node-slot:hover .fusion-node-slot-remove,
.fusion-node-slot:focus-within .fusion-node-slot-remove {
  opacity: 1;
  transform: scale(1);
}

:global(.fusion-node-sortable-fallback) {
  opacity: 0 !important;
}

.fusion-node-slot-placeholder {
  opacity: 0;
  pointer-events: none;
}

.fusion-blend-layer {
  position: absolute;
  inset: 0;
  z-index: 3;
  pointer-events: none;
}

.fusion-blend-handle {
  position: absolute;
  width: 28px;
  height: 28px;
  translate: -50% -50%;
  scale: 1;
  border: 0;
  border-radius: 999px;
  background: transparent;
  box-shadow: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  pointer-events: auto;
  cursor: grab;
  transition:
    left 0.24s cubic-bezier(0.22, 1, 0.36, 1),
    top 0.24s cubic-bezier(0.22, 1, 0.36, 1),
    scale 0.12s ease,
    background-color 0.12s ease;
  will-change: left, top, scale;
}

.fusion-blend-handle.is-hover,
.fusion-blend-handle.is-dragging,
.fusion-node-stage.is-active .fusion-blend-handle {
  scale: 1.08;
  box-shadow: none;
}

.fusion-blend-handle.is-dragging {
  cursor: grabbing;
}

.fusion-blend-handle-dot {
  width: 16px;
  height: 16px;
  border-radius: 999px;
  background: #9278e6;
}

.fusion-blend-weight-layer {
  position: absolute;
  inset: 0;
  pointer-events: none;
}

.fusion-blend-weight-badge {
  position: absolute;
  translate: -50% -50%;
  min-width: 34px;
  padding: 2px 7px;
  border-radius: 999px;
  border: 0;
  background: rgba(255, 255, 255, 0.8);
  color: #475569;
  font-size: 11px;
  line-height: 1.3;
  font-weight: 600;
  text-align: center;
  box-shadow: none;
  transition:
    left 0.24s cubic-bezier(0.22, 1, 0.36, 1),
    top 0.24s cubic-bezier(0.22, 1, 0.36, 1);
  will-change: left, top, translate;
}

.fusion-node-stage.is-blend-dragging .fusion-node-ray {
  transition: background-image 0.16s ease;
}

.fusion-node-stage.is-blend-dragging .fusion-blend-handle {
  transition:
    scale 0.12s ease,
    background-color 0.12s ease;
}

.fusion-node-stage.is-blend-dragging .fusion-blend-weight-badge {
  transition: none;
}

@keyframes fusion-node-ray-flow {
  to {
    background-position: 10px 0;
  }
}
</style>
