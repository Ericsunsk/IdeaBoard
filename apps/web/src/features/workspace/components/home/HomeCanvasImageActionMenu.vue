<template>
  <div
    class="image-action-menu node-tool-surface"
    :class="{ 'strength-mode': showStyleStrengthControl }"
    :style="showStyleStrengthControl ? styleStrengthMenuStyle : undefined"
    @mousedown.stop
    @click.stop
  >
    <div
      v-motion="{
        initial: { opacity: 0, y: 6, scale: 0.985 },
        enter: {
          opacity: 1,
          y: 0,
          scale: 1,
          transition: { duration: 160, easing: 'ease-out' },
        },
      }"
      class="image-action-menu-inner"
    >
      <template v-if="!showStyleStrengthControl">
      <button
        type="button"
        class="image-action-icon-btn node-tool-button"
        :disabled="directGenerating"
        title="抠图"
        @click="emit('action-remove-bg')"
      >
        <span class="action-logo">
          <svg
            width="20"
            height="20"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="1.5"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <path d="M4 5h10v10H4z" />
            <path d="M10 9h10v10H10z" />
          </svg>
        </span>
        <span class="action-tooltip">抠图</span>
      </button>
      <button
        type="button"
        class="image-action-icon-btn node-tool-button"
        :disabled="directGenerating"
        title="款式创新"
        @mousedown.stop="emit('style-innovate-mousedown', $event)"
        @click="emit('style-innovate-click')"
      >
        <span class="action-logo">
          <svg
            width="20"
            height="20"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="1.5"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <path
              d="M12 3v4M12 17v4M3 12h4M17 12h4M5.6 5.6l2.8 2.8M15.6 15.6l2.8 2.8M5.6 18.4l2.8-2.8M15.6 8.4l2.8-2.8"
            />
          </svg>
        </span>
        <span class="action-tooltip">款式创新</span>
      </button>
      <button
        type="button"
        class="image-action-icon-btn node-tool-button"
        :disabled="directGenerating"
        title="转矢量图"
        @click="emit('action-to-vector')"
      >
        <span class="action-logo">
          <svg
            width="20"
            height="20"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="1.5"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <circle cx="5" cy="19" r="2" />
            <circle cx="19" cy="5" r="2" />
            <path d="M5 17C5 10 10 5 19 5" />
          </svg>
        </span>
        <span class="action-tooltip">转矢量图</span>
      </button>
      <button
        type="button"
        class="image-action-icon-btn node-tool-button"
        :disabled="directGenerating"
        title="线稿成款"
        @click="emit('action-sketch-to-design')"
      >
        <span class="action-logo">
          <svg
            width="20"
            height="20"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="1.5"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <path d="M18 2l-6.3 6.3M12 14l-4 4" />
            <path d="M3 21l3.5-3.5" />
            <path d="M15 6l4 4" />
            <path d="M20 7l-2-2" />
          </svg>
        </span>
        <span class="action-tooltip">线稿成款</span>
      </button>
      <button
        type="button"
        class="image-action-icon-btn image-action-icon-btn--placeholder node-tool-button"
        disabled
        title="局部重绘（占位）"
      >
        <span class="action-logo">
          <svg
            width="20"
            height="20"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="1.5"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <path d="M4 15s2-3 5-3 4 3 7 3 4-3 4-3" />
            <path d="M4 19s2-3 5-3 4 3 7 3 4-3 4-3" />
            <path d="M8 5h8" />
            <path d="M12 5v5" />
          </svg>
        </span>
        <span class="action-tooltip">局部重绘</span>
      </button>
      <button
        type="button"
        class="image-action-icon-btn image-action-icon-btn--placeholder node-tool-button"
        disabled
        title="扩图（占位）"
      >
        <span class="action-logo">
          <svg
            width="20"
            height="20"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="1.5"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <path d="M9 4H5a1 1 0 0 0-1 1v4" />
            <path d="M15 4h4a1 1 0 0 1 1 1v4" />
            <path d="M9 20H5a1 1 0 0 1-1-1v-4" />
            <path d="M15 20h4a1 1 0 0 0 1-1v-4" />
            <path d="M8 12h8" />
            <path d="M12 8v8" />
          </svg>
        </span>
        <span class="action-tooltip">扩图</span>
      </button>
      <button
        type="button"
        class="image-action-icon-btn image-action-icon-btn--placeholder node-tool-button"
        disabled
        title="调色（占位）"
      >
        <span class="action-logo">
          <svg
            width="20"
            height="20"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="1.5"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <circle cx="12" cy="12" r="8" />
            <path d="M12 4v16" />
            <path d="M4 12h16" />
            <path d="M7.5 7.5l9 9" />
            <path d="M16.5 7.5l-9 9" />
          </svg>
        </span>
        <span class="action-tooltip">调色</span>
      </button>
      </template>
      <div
        v-else
        class="style-strength-pop"
        @mousedown.stop
      >
        <div
          :ref="setStyleStrengthTrackRef"
          class="style-strength-track"
          @mousedown.stop.prevent="emit('style-strength-track-mousedown', $event)"
        >
          <div class="style-strength-fill" :style="{ width: `${styleInnovateStrengthPct}%` }"></div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { ComponentPublicInstance } from 'vue'

defineProps<{
  showStyleStrengthControl: boolean
  styleStrengthMenuStyle?: string | Record<string, string>
  directGenerating: boolean
  displayHeight: number
  styleInnovateStrengthPct: number
  setStyleStrengthTrackRef: (el: Element | ComponentPublicInstance | null) => void
}>()

const emit = defineEmits<{
  (event: 'style-innovate-mousedown', value: MouseEvent): void
  (event: 'style-innovate-click'): void
  (event: 'action-remove-bg'): void
  (event: 'action-to-vector'): void
  (event: 'action-sketch-to-design'): void
  (event: 'style-strength-track-mousedown', value: MouseEvent): void
}>()
</script>

<style scoped src="../nodes/styles/node-tool-buttons.css"></style>
<style scoped>
.image-action-menu {
  position: absolute;
  top: calc(100% + 10px);
  left: 50%;
  width: max-content;
  max-width: min(100vw - 32px, 320px);
  transform: translateX(-50%);
  padding: 0;
  border-radius: 0;
  background: transparent;
  border: none;
  backdrop-filter: none;
  box-shadow: none;
  z-index: 14;
  display: flex;
  flex-direction: row;
  flex-wrap: wrap;
  justify-content: center;
  gap: var(--node-tool-button-gap);
  box-sizing: border-box;
}

.image-action-menu.strength-mode {
  top: calc(100% + 10px);
  left: 0;
  transform: none;
  width: 100%;
  max-width: none;
  gap: 0;
}

.image-action-menu-inner {
  display: flex;
  flex-direction: row;
  flex-wrap: wrap;
  justify-content: inherit;
  gap: inherit;
  width: 100%;
}

.image-action-icon-btn {
  flex: 0 0 var(--node-tool-button-size);
  border: 1px solid var(--node-tool-button-border);
  background: #ffffff;
  position: relative;
  overflow: visible;
  transition:
    transform 0.18s ease,
    color 0.18s ease,
    opacity 0.18s ease,
    border-color 0.18s ease,
    background 0.18s ease;
  color: #6f61c6;
}

.image-action-icon-btn:hover:not(:disabled) {
  border-color: #7a6bd5;
  background: #ffffff;
  color: #7a6bd5;
  transform: scale(1.06);
}

.image-action-icon-btn:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}

.image-action-icon-btn--placeholder:disabled {
  cursor: default;
  opacity: 0.58;
}

.action-logo {
  display: flex;
  align-items: center;
  justify-content: center;
}

.action-logo svg {
  width: 21px;
  height: 21px;
}

.action-tooltip {
  position: absolute;
  left: 50%;
  bottom: calc(100% + 8px);
  transform: translate(-50%, 2px);
  white-space: nowrap;
  background: rgba(24, 24, 27, 0.94);
  color: #f4f4f5;
  border: 1px solid rgba(122, 107, 213, 0.26);
  border-radius: 6px;
  padding: 4px 8px;
  font-size: 11px;
  line-height: 1.2;
  letter-spacing: 0.01em;
  opacity: 0;
  pointer-events: none;
  transition:
    opacity 0.16s ease,
    transform 0.16s ease;
}

.image-action-icon-btn:hover:not(:disabled) .action-tooltip {
  opacity: 1;
  transform: translate(-50%, 0);
}

.style-strength-pop {
  position: relative;
  width: 100%;
  height: var(--node-tool-button-size);
  display: flex;
  align-items: center;
  padding: 0;
}

.style-strength-track {
  position: relative;
  width: 100%;
  height: 6px;
  border-radius: 999px;
  overflow: hidden;
  border: none;
  background: rgba(122, 107, 213, 0.2);
  cursor: ew-resize;
}

.style-strength-fill {
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  background: #7a6bd5;
  border-radius: inherit;
  transition: width 0.06s linear;
}
</style>
