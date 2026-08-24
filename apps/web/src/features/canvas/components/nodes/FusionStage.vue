<template>
  <NodeStageFrame
    ref="stageFrameRef"
    class="fusion-node-stage node-tool-surface node-input-thumbnail-contract variant-standard"
    variant="square"
    :active="active"
    :connection-drop-target="connectionDropTarget"
    :drop-target="addSlotDragOver"
    :class="{
      'is-blend-dragging': blendDragging,
    }"
    @dragenter.stop.prevent="onAddSlotDragOver"
    @dragover.stop.prevent="onAddSlotDragOver"
    @dragleave.stop="onAddSlotDragLeave"
    @drop.stop.prevent="onAddSlotDrop"
    @contextmenu.prevent.stop="emit('contextmenu', $event)"
  >
    <TransitionGroup
      v-if="fusionRays.length"
      name="fusion-node-ray"
      tag="div"
      class="fusion-node-connection-layer"
      aria-hidden="true"
    >
      <span v-for="ray in fusionRays" :key="ray.key" class="fusion-node-ray" :style="ray.style" />
    </TransitionGroup>
    <NodeEmptyStateIcon v-if="showEmptyFusionMark" class="fusion-empty-mark" variant="fusion" />
    <div ref="thumbsRef" class="fusion-node-slots">
      <TransitionGroup name="fusion-node-slot">
        <template v-for="(slot, index) in fusionSlotItems">
          <NodeInputThumbnail
            v-if="slot.image"
            :key="slot.key"
            class="fusion-node-slot fusion-node-slot-item"
            :src="slot.image.url"
            :media-asset-id="slot.image.mediaAssetId"
            removable
            :style="getSlotStyle(index)"
            :data-image-id="slot.image.id"
            @mousedown.stop
            @remove="removeInputImage(slot.image.id)"
          />
          <div
            v-else
            :key="slot.key"
            class="fusion-node-slot fusion-node-slot-placeholder"
            :style="getSlotStyle(index)"
            aria-hidden="true"
          />
        </template>
      </TransitionGroup>
    </div>
    <div v-if="showBlendPad" class="fusion-blend-layer" aria-label="融合权重控制">
      <Transition name="fusion-blend-handle" appear>
        <button
          class="fusion-blend-handle"
          :class="{ 'is-hover': blendHandleHover, 'is-dragging': blendDragging }"
          type="button"
          :style="blendHandleStyle"
          aria-label="拖动调整融合权重，双击重置"
          @pointerdown.stop.prevent="onBlendHandlePointerDown"
          @pointerenter="onBlendHandlePointerEnter"
          @pointerleave="onBlendHandlePointerLeave"
          @dblclick.stop.prevent="onBlendHandleDoubleClick"
        >
          <span class="fusion-blend-handle-dot" />
        </button>
      </Transition>
      <TransitionGroup
        v-if="showBlendWeightBadges"
        name="fusion-blend-weight-badge"
        tag="div"
        class="fusion-blend-weight-layer"
      >
        <div
          v-for="badge in blendWeightBadges"
          :key="badge.key"
          class="fusion-blend-weight-badge"
          :style="getBlendWeightBadgeStyle(badge.index)"
        >
          {{ badge.percent }}%
        </div>
      </TransitionGroup>
    </div>
  </NodeStageFrame>
</template>

<script setup lang="ts">
import { computed, ref, watchPostEffect } from 'vue'
import type { FusionNodeItem } from '../../../canvas-board/domain'
import {
  FUSION_SLOT_LAYOUT,
  buildFusionDisplayPercentages,
  computeFusionBlendWeights,
  getFusionLineAnchors,
  getFusionSlotAnchor,
  getFusionSlotAnchors,
  type FusionBlendPoint,
} from '../../../canvas-board/domain'
import { useFusionBlendControl } from './useFusionBlendControl'
import NodeEmptyStateIcon from './NodeEmptyStateIcon.vue'
import NodeInputThumbnail from './NodeInputThumbnail.vue'
import NodeStageFrame from './NodeStageFrame.vue'
import { useImageInputSlots } from './useImageInputSlots'

const props = defineProps<{
  active: boolean
  connectionDropTarget?: boolean
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
type NodeStageFrameExpose = { stageElement: HTMLElement | null }
const stageFrameRef = ref<NodeStageFrameExpose | null>(null)
const stageRef = ref<HTMLElement | null>(null)
const maxInputImages = FUSION_SLOT_LAYOUT.maxSlots

const {
  addSlotDragOver,
  onAddSlotDragLeave,
  onAddSlotDragOver,
  onAddSlotDrop,
  removeInputImage,
  sortableInputImages,
  visibleInputImages,
} = useImageInputSlots({
  containerRef: thumbsRef,
  inputImages: () => props.inputImages,
  maxImages: maxInputImages,
  draggableSelector: '.node-input-thumbnail-item',
  filterSelector: '.node-input-thumbnail-remove',
  onUpdate: (next) => emit('update:input-images', next),
  onDrop: (event) => emit('input-image-drop', event),
  onRemove: (item) => emit('input-image-remove', item),
})

watchPostEffect(() => {
  stageRef.value = stageFrameRef.value?.stageElement ?? null
})

const imageCount = computed(() => Math.min(visibleInputImages.value.length || 0, maxInputImages))
const showBlendPad = computed(() => imageCount.value >= 2)
const showEmptyFusionMark = computed(() => imageCount.value === 0)
const nodeWidth = computed(() => props.width)
const sourceBlendPoint = computed(() => props.blendPoint)
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
    left: `calc(${anchor.x * 100}% - (var(--fusion-slot-size) / 2))`,
    top: `calc(${anchor.y * 100}% - (var(--fusion-slot-size) / 2))`,
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
</script>

<style scoped>
.fusion-node-stage {
  --fusion-slot-size: var(--node-input-thumbnail-size);
}

.fusion-node-connection-layer {
  position: absolute;
  inset: 0;
  z-index: 1;
  pointer-events: none;
}

.fusion-node-ray {
  --fusion-node-ray-color: var(--flow-edge-color);
  position: absolute;
  height: calc(var(--flow-edge-width) * 1px);
  border-radius: 999px;
  background-image: repeating-linear-gradient(
    90deg,
    var(--fusion-node-ray-color) 0 5px,
    transparent 5px 10px
  );
  background-size: 10px 100%;
  transform-origin: left center;
  translate: 0 -50%;
  animation: fusion-node-ray-flow var(--flow-edge-running-duration) linear infinite;
  transition:
    left var(--motion-feedback-duration) var(--motion-feedback-easing),
    top var(--motion-feedback-duration) var(--motion-feedback-easing),
    width var(--motion-feedback-duration) var(--motion-feedback-easing),
    transform var(--motion-feedback-duration) var(--motion-feedback-easing),
    background-image var(--motion-feedback-duration) var(--motion-feedback-easing);
  will-change: left, top, width, transform;
}

.fusion-node-slots {
  position: absolute;
  inset: 0;
  z-index: 2;
}

.fusion-node-stage.is-active .fusion-node-ray,
.fusion-node-stage.is-drop-target .fusion-node-ray {
  --fusion-node-ray-color: var(--flow-edge-running-color);
}

.fusion-empty-mark {
  position: absolute;
  left: 50%;
  top: 50%;
  width: 112px;
  height: 112px;
  stroke-width: 0.6;
  translate: -50% -50%;
  z-index: 1;
  pointer-events: none;
  color: var(--canvas-text-subtle);
}

.fusion-node-slot {
  position: absolute;
  width: var(--fusion-slot-size);
  height: auto;
  aspect-ratio: 1 / 1;
  border: 0;
  border-radius: 8px;
  background: transparent;
  overflow: hidden;
  padding: 0;
  transition:
    left var(--motion-feedback-duration) var(--motion-feedback-easing),
    top var(--motion-feedback-duration) var(--motion-feedback-easing),
    opacity var(--motion-feedback-duration) var(--motion-feedback-easing);
  will-change: left, top;
}

.fusion-node-slot-item {
  cursor: grab;
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
    left var(--motion-feedback-duration) var(--motion-feedback-easing),
    top var(--motion-feedback-duration) var(--motion-feedback-easing),
    scale var(--motion-control-duration) var(--motion-control-easing),
    background-color var(--motion-control-duration) var(--motion-control-easing);
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
  background: var(--canvas-accent);
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
  color: var(--canvas-text-body);
  font-size: 11px;
  line-height: 1.3;
  font-weight: 600;
  text-align: center;
  box-shadow: none;
  transition:
    left var(--motion-feedback-duration) var(--motion-feedback-easing),
    top var(--motion-feedback-duration) var(--motion-feedback-easing);
  will-change: left, top, translate;
}

.fusion-node-stage.is-blend-dragging .fusion-node-ray {
  transition: background-image var(--motion-feedback-duration) var(--motion-feedback-easing);
}

.fusion-node-stage.is-blend-dragging .fusion-blend-handle {
  transition:
    scale var(--motion-control-duration) var(--motion-control-easing),
    background-color var(--motion-control-duration) var(--motion-control-easing);
}

.fusion-node-stage.is-blend-dragging .fusion-blend-weight-badge {
  transition: none;
}

@keyframes fusion-node-ray-flow {
  to {
    background-position: 10px 0;
  }
}

.fusion-node-ray-enter-active,
.fusion-node-slot-enter-active,
.fusion-blend-handle-enter-active,
.fusion-blend-handle-appear-active,
.fusion-blend-weight-badge-enter-active {
  transition:
    opacity var(--motion-enter-duration) var(--motion-enter-easing),
    scale var(--motion-enter-duration) var(--motion-enter-easing);
}

.fusion-node-ray-leave-active,
.fusion-node-slot-leave-active,
.fusion-blend-handle-leave-active,
.fusion-blend-weight-badge-leave-active {
  transition:
    opacity var(--motion-exit-duration) var(--motion-exit-easing),
    scale var(--motion-exit-duration) var(--motion-exit-easing);
}

.fusion-node-ray-enter-from,
.fusion-node-ray-leave-to,
.fusion-node-slot-enter-from,
.fusion-node-slot-leave-to,
.fusion-blend-handle-enter-from,
.fusion-blend-handle-leave-to,
.fusion-blend-handle-appear-from {
  opacity: 0;
}

.fusion-node-ray-enter-to,
.fusion-node-ray-leave-from,
.fusion-node-slot-enter-to,
.fusion-node-slot-leave-from,
.fusion-blend-handle-enter-to,
.fusion-blend-handle-leave-from,
.fusion-blend-handle-appear-to {
  opacity: 1;
}

.fusion-blend-weight-badge-enter-from,
.fusion-blend-weight-badge-leave-to {
  opacity: 0;
  scale: 0.92;
}

.fusion-blend-weight-badge-enter-to,
.fusion-blend-weight-badge-leave-from {
  opacity: 1;
  scale: 1;
}

@media (prefers-reduced-motion: reduce) {
  .fusion-node-ray {
    animation: none;
  }

  .fusion-node-ray-enter-active,
  .fusion-node-ray-leave-active,
  .fusion-node-slot-enter-active,
  .fusion-node-slot-leave-active,
  .fusion-blend-handle-enter-active,
  .fusion-blend-handle-leave-active,
  .fusion-blend-handle-appear-active,
  .fusion-blend-weight-badge-enter-active,
  .fusion-blend-weight-badge-leave-active {
    transition: none;
  }
}
</style>
