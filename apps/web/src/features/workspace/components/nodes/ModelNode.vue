<template>
  <div
    class="workflow-node model-node"
    data-node-kind="model"
    :data-node-id="node.id"
    :class="{ active, dragging, 'copy-mode': copyMode, running: uiState?.isRunning }"
    :style="nodeStyle"
    @mousedown.stop
  >
    <NodeHeader
      :title="node.title || 'Model'"
      @node-mousedown="emit('node-mousedown', $event)"
      @update:title="emit('update:title', $event)"
      @delete="emit('delete')"
    >
      <template #icon>
        <Planet />
      </template>
    </NodeHeader>

    <div class="model-node-body node-execution-body" @mousedown.stop @contextmenu.stop>
      <div class="model-mode-card node-mode-card">
        <div class="model-summary-row">
          <span class="model-summary-value">{{ describeModelAngleSpec(node.angleSpec) }}</span>
        </div>

        <div class="model-angle-row">
          <span class="model-angle-anchor">L</span>
          <input
            class="model-angle-range node-inline-range"
            :style="{ '--slider-pct': `${((node.angleSpec.yaw + 180) / 360) * 100}%` }"
            :value="node.angleSpec.yaw"
            type="range"
            min="-180"
            max="180"
            step="1"
            @input="
              emit(
                'update-angle-yaw',
                Number(($event.target as HTMLInputElement).value || node.angleSpec.yaw),
              )
            "
          />
          <span class="model-angle-anchor">R</span>
        </div>

        <div class="node-option-row">
          <NodeActionButton
            v-for="opt in framingOptions"
            :key="`${node.id}-framing-${opt.key}`"
            variant="chip"
            :active="node.angleSpec.framing === opt.key"
            @click="emit('update-angle-framing', opt.key)"
          >
            {{ opt.label }}
          </NodeActionButton>
        </div>

        <div class="node-option-row">
          <NodeActionButton
            v-for="opt in backgroundOptions"
            :key="`${node.id}-bg-${opt.key}`"
            variant="chip"
            :active="node.background === opt.key"
            @click="emit('select-background', opt.key)"
          >
            {{ opt.label }}
          </NodeActionButton>
        </div>
      </div>

      <div v-if="showModelStrength" class="model-control-group node-control-group">
        <div class="node-control-head">
          <span class="model-control-label node-control-label">Model</span>
          <span class="model-strength-value">{{ node.modelStrength }}%</span>
        </div>
        <input
          class="model-strength-range node-inline-range"
          :value="node.modelStrength"
          :style="{ '--slider-pct': `${node.modelStrength}%` }"
          type="range"
          min="0"
          max="100"
          step="1"
          @input="
            emit(
              'update-model-strength',
              Number(($event.target as HTMLInputElement).value || node.modelStrength),
            )
          "
        />
      </div>

      <div v-if="showPoseStrength" class="model-control-group node-control-group">
        <div class="node-control-head">
          <span class="model-control-label node-control-label">Pose</span>
          <span class="model-strength-value">{{ node.poseStrength }}%</span>
        </div>
        <input
          class="model-strength-range node-inline-range"
          :value="node.poseStrength"
          :style="{ '--slider-pct': `${node.poseStrength}%` }"
          type="range"
          min="0"
          max="100"
          step="1"
          @input="
            emit(
              'update-pose-strength',
              Number(($event.target as HTMLInputElement).value || node.poseStrength),
            )
          "
        />
      </div>

      <NodeExecutionFooter
        :disabled="uiState?.primaryAction?.disabled"
        :label="uiState?.primaryAction?.label || '生成'"
        @generate="emit('generate')"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { Planet } from '@iconoir/vue'
import { MODEL_ANGLE_FRAMING_OPTIONS, describeModelAngleSpec } from './config/nodeOptionConfig'
import type { ModelAngleFraming, ModelNodeItem } from '../../types/canvasNodes'
import type { NodeUiState } from '../../types/canvasNodeUi'

import { useCanvasNodeStyle } from './composables/useCanvasNodeStyle'

const props = defineProps<{
  node: ModelNodeItem
  active: boolean
  dragging: boolean
  copyMode: boolean
  uiState?: NodeUiState
  backgroundOptions: Array<{ key: string; label: string }>
}>()

const emit = defineEmits<{
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'update:title', value: string): void
  (e: 'delete'): void
  (e: 'update-angle-yaw', value: number): void
  (e: 'update-angle-framing', value: ModelAngleFraming): void
  (e: 'update-model-strength', value: number): void
  (e: 'update-pose-strength', value: number): void
  (e: 'select-background', key: string): void
  (e: 'generate'): void
}>()

const nodeStyle = useCanvasNodeStyle(() => props.node)
const showModelStrength = computed(
  () =>
    !!(props.uiState?.connection?.connected?.imageA || props.uiState?.connection?.pending?.imageA),
)
const showPoseStrength = computed(
  () =>
    !!(props.uiState?.connection?.connected?.imageB || props.uiState?.connection?.pending?.imageB),
)
const framingOptions = MODEL_ANGLE_FRAMING_OPTIONS
</script>

<style scoped src="./styles/node-shared.css"></style>
<style scoped>
.model-node-body {
  gap: 8px;
}

.model-mode-card {
  gap: 8px;
}

.model-summary-row,
.model-angle-row {
  display: flex;
  align-items: center;
  gap: 6px;
}

.model-summary-row {
  justify-content: space-between;
}

.model-summary-value {
  font-size: 12px;
  font-weight: 600;
  color: #475569;
}

.model-angle-anchor {
  width: 16px;
  flex: 0 0 16px;
  text-align: center;
  font-size: 10px;
  font-weight: 700;
  color: #9aa4b2;
  letter-spacing: 0.06em;
}

.model-angle-range {
  flex: 1;
}

.model-strength-value {
  font-size: 11px;
  color: #667085;
  font-weight: 700;
}

.model-strength-range {
  width: 100%;
}
</style>
