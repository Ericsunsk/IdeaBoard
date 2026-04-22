<template>
  <div class="canvas-connections-shell">
    <svg class="canvas-connections-layer">
      <path
        v-for="connection in visibleConnections"
        :key="connection.id"
        class="canvas-connection-hit"
        :d="connection.path"
        @mousedown.stop="emit('path-mousedown', $event, connection.id)"
      />
      <path
        v-for="connection in visibleConnections"
        :key="`visible-${connection.id}`"
        class="canvas-connection-path"
        :class="[selectedConnectionId === connection.id ? 'selected' : '', `tone-${connection.tone}`]"
        :d="connection.path"
      />
      <path
        v-if="draftConnectionPath"
        class="canvas-connection-path draft"
        :class="[ `tone-${draftConnectionKind}`, `is-${draftConnectionTone}` ]"
        :d="draftConnectionPath"
      />
      <template v-for="connection in visibleConnections" :key="`endpoints-${connection.id}`">
        <circle
          v-if="selectedConnectionId === connection.id"
          class="canvas-connection-endpoint"
          :class="`tone-${connection.tone}`"
          :cx="connection.startX"
          :cy="connection.startY"
          r="4"
        />
      </template>
    </svg>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { CanvasConnectionTone } from '../../config/canvasConnections'
import type { RenderedConnection } from '../../composables/useCanvasConnections'

const props = defineProps<{
  renderedConnections: RenderedConnection[]
  selectedConnectionId: number | null
  hiddenConnectionId: number | null
  draftConnectionPath: string
  draftConnectionTone: 'default' | 'valid' | 'invalid'
  draftConnectionKind: CanvasConnectionTone
}>()

const visibleConnections = computed(() =>
  props.hiddenConnectionId === null
    ? props.renderedConnections
    : props.renderedConnections.filter(connection => connection.id !== props.hiddenConnectionId),
)

const emit = defineEmits<{
  (e: 'path-mousedown', event: MouseEvent, connectionId: number): void
}>()
</script>

<style scoped>
.canvas-connections-shell {
  position: absolute;
  inset: 0;
  z-index: 1;
  overflow: visible;
  transform: translate(var(--offset-x, 0px), var(--offset-y, 0px)) scale(var(--zoom, 1));
  transform-origin: center center;
}

.canvas-connections-layer {
  position: absolute;
  inset: 0;
  overflow: visible;
}

.canvas-connection-path,
.canvas-connection-hit {
  fill: none;
}

.canvas-connection-path {
  stroke: var(--connection-color, #5b8def);
  stroke-width: 1.8;
  opacity: 0.78;
  stroke-linecap: round;
  stroke-linejoin: round;
  pointer-events: none;
  transition:
    stroke-width 0.16s ease,
    opacity 0.16s ease,
    filter 0.16s ease;
}

.canvas-connection-path.tone-image,
.canvas-connection-endpoint.tone-image {
  --connection-color: #5b8def;
  --connection-selected: #3f79e6;
  --connection-glow: rgba(91, 141, 239, 0.18);
}

.canvas-connection-path.tone-mask,
.canvas-connection-endpoint.tone-mask {
  --connection-color: #34c38f;
  --connection-selected: #22a876;
  --connection-glow: rgba(52, 195, 143, 0.18);
}

.canvas-connection-path.tone-prompt,
.canvas-connection-endpoint.tone-prompt {
  --connection-color: #f59e0b;
  --connection-selected: #d68608;
  --connection-glow: rgba(245, 158, 11, 0.18);
}

.canvas-connection-path.selected {
  stroke: var(--connection-selected, var(--connection-color, #3f79e6));
  stroke-width: 2.8;
  opacity: 1;
  filter: drop-shadow(0 0 10px var(--connection-glow, rgba(91, 141, 239, 0.18)));
}

.canvas-connection-path.draft {
  stroke: var(--connection-color, #5b8def);
  opacity: 0.92;
  stroke-dasharray: 6 6;
}

.canvas-connection-path.draft.is-valid {
  stroke: var(--connection-selected, var(--connection-color, #3f79e6));
}

.canvas-connection-path.draft.is-invalid {
  stroke: #e06a82;
}

.canvas-connection-hit {
  stroke: transparent;
  stroke-width: 14;
  pointer-events: stroke;
  cursor: pointer;
}

.canvas-connection-endpoint {
  fill: var(--connection-color, #5b8def);
  stroke: #fff;
  stroke-width: 2;
  filter: drop-shadow(0 0 6px var(--connection-glow, rgba(91, 141, 239, 0.18)));
}
</style>
