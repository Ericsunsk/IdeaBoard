<template>
  <div class="image-editor-layer-actions node-tool-surface">
    <div
      v-if="showProgressPanel"
      class="image-editor-layer-actions__progress"
      role="status"
      aria-live="polite"
    >
      <div class="image-editor-layer-actions__progress-copy">
        <span class="image-editor-layer-actions__progress-phase">
          {{ progressLabel }}
        </span>
        <span class="image-editor-layer-actions__progress-value">
          {{ progressValueLabel }}
        </span>
      </div>
      <div
        class="image-editor-layer-actions__progress-track"
        :class="{ indeterminate: progressPercent == null }"
        aria-hidden="true"
      >
        <span class="image-editor-layer-actions__progress-bar" :style="progressBarStyle" />
      </div>
      <NodeToolIconButton
        class="image-editor-layer-actions__cancel"
        aria-label="取消应用"
        tooltip="取消应用"
        @click="emit('cancel-apply')"
      >
        <Xmark />
      </NodeToolIconButton>
    </div>
    <div class="image-editor-layer-actions__buttons">
      <NodeToolIconButton
        class="image-editor-layer-actions__button"
        aria-label="添加图层"
        tooltip="添加图层"
        @click="emit('add-layer')"
      >
        <EmptyPage />
      </NodeToolIconButton>
      <NodeToolIconButton
        class="image-editor-layer-actions__button"
        :disabled="!canCopyLayer"
        aria-label="复制图层"
        tooltip="复制图层"
        @click="emit('copy-layer')"
      >
        <Copy />
      </NodeToolIconButton>
      <NodeToolIconButton
        class="image-editor-layer-actions__button"
        :disabled="!canMergeLayer"
        aria-label="合并图层"
        tooltip="合并图层"
        @click="emit('merge-layer')"
      >
        <VerticalMerge />
      </NodeToolIconButton>
      <NodeToolIconButton
        class="image-editor-layer-actions__button"
        :disabled="!canRemoveLayer"
        aria-label="删除图层"
        tooltip="删除图层"
        @click="emit('remove-layer')"
      >
        <Trash />
      </NodeToolIconButton>
      <NodeToolIconButton
        class="image-editor-layer-actions__button image-editor-layer-actions__button--apply"
        primary
        :disabled="applyDisabled"
        :aria-label="applying ? '应用中' : '应用为新节点'"
        :tooltip="applying ? '应用中' : '应用为新节点'"
        @click="emit('apply-adjustments')"
      >
        <SendDiagonalSolid />
      </NodeToolIconButton>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { Copy, EmptyPage, SendDiagonalSolid, Trash, VerticalMerge, Xmark } from '@iconoir/vue'
import { NodeToolIconButton } from '../../components/nodes/primitives'
import type { ImageAdjustmentFinalApplyProgress } from '../runtime/imageAdjustmentFinalApplyContext'

const props = defineProps<{
  canCopyLayer: boolean
  canMergeLayer: boolean
  canRemoveLayer: boolean
  applying: boolean
  applyDisabled: boolean
  applyProgress?: ImageAdjustmentFinalApplyProgress | null
}>()

const emit = defineEmits<{
  (e: 'add-layer'): void
  (e: 'copy-layer'): void
  (e: 'merge-layer'): void
  (e: 'remove-layer'): void
  (e: 'apply-adjustments'): void
  (e: 'cancel-apply'): void
}>()

const APPLY_PROGRESS_DISCLOSURE_DELAY_MS = 200

const progressPhaseLabels: Record<ImageAdjustmentFinalApplyProgress['phase'], string> = {
  queued: '准备应用',
  decode: '读取图像',
  render: '渲染调整',
  encode: '生成文件',
  complete: '收尾',
}

const progressPercent = computed(() => {
  const progress = props.applyProgress
  if (!progress || progress.total == null || progress.total <= 0 || progress.loaded == null) {
    return null
  }
  return Math.min(100, Math.max(0, Math.round((progress.loaded / progress.total) * 100)))
})
const progressLabel = computed(() => progressPhaseLabels[props.applyProgress?.phase ?? 'queued'])
const progressValueLabel = computed(() =>
  progressPercent.value == null ? '处理中' : `${progressPercent.value}%`,
)
const progressBarStyle = computed(() =>
  progressPercent.value == null ? {} : { width: `${progressPercent.value}%` },
)
const showProgressPanel = ref(false)
// The delayed disclosure is browser-only, so retain the DOM timer handle type.
let progressPanelDelayId: number | null = null

function clearProgressPanelDelay() {
  if (progressPanelDelayId == null) return
  window.clearTimeout(progressPanelDelayId)
  progressPanelDelayId = null
}

watch(
  () => props.applying,
  (applying) => {
    clearProgressPanelDelay()
    if (!applying) {
      showProgressPanel.value = false
      return
    }
    progressPanelDelayId = window.setTimeout(() => {
      progressPanelDelayId = null
      if (props.applying) showProgressPanel.value = true
    }, APPLY_PROGRESS_DISCLOSURE_DELAY_MS)
  },
  { immediate: true },
)

onBeforeUnmount(clearProgressPanelDelay)
</script>

<style scoped>
.image-editor-layer-actions {
  display: flex;
  flex-direction: column;
  gap: var(--node-tool-button-gap);
}

.image-editor-layer-actions__buttons {
  display: grid;
  grid-template-columns: repeat(5, var(--node-tool-button-size));
  justify-content: space-between;
  gap: var(--node-tool-button-gap);
}

.image-editor-layer-actions__button {
  width: var(--node-tool-button-size);
  flex: 0 0 var(--node-tool-button-size);
  --node-tool-icon-button-size: var(--node-tool-button-size);
  --node-tool-icon-button-radius: var(--node-tool-button-radius);
  --node-tool-icon-button-icon-size: 18px;
}

.image-editor-layer-actions__button--apply {
  --node-tool-icon-button-primary-border: var(--canvas-accent-hover);
  --node-tool-icon-button-primary-bg: var(--canvas-accent-hover);
  --node-tool-icon-button-primary-color: #ffffff;
  --node-tool-icon-button-primary-hover-border: var(--canvas-accent-strong);
  --node-tool-icon-button-primary-hover-bg: var(--canvas-accent-strong);
  --node-tool-icon-button-primary-hover-color: #ffffff;
}

.image-editor-layer-actions__progress {
  min-height: 46px;
  border: var(--node-tool-border-width) solid var(--canvas-accent-outline);
  border-radius: var(--canvas-radius-xl);
  padding: 7px 39px 7px 8px;
  position: relative;
  display: grid;
  align-content: center;
  gap: 6px;
  background: var(--canvas-surface-bg);
  color: var(--canvas-text-body);
}

.image-editor-layer-actions__progress-copy {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  min-width: 0;
  font-size: 11px;
  line-height: 1.1;
}

.image-editor-layer-actions__progress-phase,
.image-editor-layer-actions__progress-value {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.image-editor-layer-actions__progress-phase {
  color: var(--canvas-text-strong);
  font-weight: 700;
}

.image-editor-layer-actions__progress-value {
  flex: 0 0 auto;
  color: var(--canvas-text-muted);
  font-variant-numeric: tabular-nums;
}

.image-editor-layer-actions__progress-track {
  height: 4px;
  border-radius: 999px;
  overflow: hidden;
  background: var(--canvas-surface-muted);
}

.image-editor-layer-actions__progress-bar {
  width: 0;
  height: 100%;
  display: block;
  border-radius: inherit;
  background: var(--canvas-accent-hover);
  transition: width var(--motion-feedback-duration) var(--motion-feedback-easing);
}

.image-editor-layer-actions__progress-track.indeterminate
  .image-editor-layer-actions__progress-bar {
  /* Indeterminate progress needs a cycle because completion time is unknown. */
  --image-editor-progress-cycle: 1.1s;
  width: 42%;
  animation: image-editor-final-apply-progress var(--image-editor-progress-cycle) ease-in-out
    infinite;
}

.image-editor-layer-actions__cancel {
  position: absolute;
  top: 7px;
  right: 7px;
  width: 25px;
  height: 25px;
  --node-tool-icon-button-size: 25px;
  --node-tool-icon-button-radius: 7px;
  --node-tool-icon-button-icon-size: 14px;
  --node-tool-icon-button-border: var(--canvas-surface-border-soft);
  --node-tool-icon-button-color: var(--canvas-text-muted);
  --node-tool-icon-button-hover-border: var(--canvas-accent-outline);
  --node-tool-icon-button-hover-color: var(--canvas-text-strong);
  --node-tool-icon-button-hover-bg: var(--canvas-accent-wash);
}

@keyframes image-editor-final-apply-progress {
  from {
    transform: translateX(-120%);
  }
  to {
    transform: translateX(260%);
  }
}

@media (prefers-reduced-motion: reduce) {
  .image-editor-layer-actions__progress-track.indeterminate
    .image-editor-layer-actions__progress-bar {
    animation: none;
    transform: translateX(70%);
  }
}
</style>
