<template>
  <div
    class="image-editor-chrome-frame"
    :style="chromeStyle"
    :data-image-editor="mode === 'adjust' ? '' : undefined"
    :data-image-mask-editor="mode === 'inpaint' ? '' : undefined"
    @pointerdown.stop
  >
    <div class="image-editor-chrome-frame__tool-rail node-tool-surface">
      <slot name="tools" />
    </div>
    <div class="image-editor-chrome-frame__right-dock">
      <slot name="dock" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { IMAGE_EDITOR_CHROME_LAYOUT } from '../../image-editor/config/imageEditorChrome'
import type { ImageEditorMode } from '../../image-editor/types'

const props = defineProps<{
  anchorRect: {
    top: number
    left: number
    width: number
    height: number
  }
  mode: ImageEditorMode
}>()

const chromeStyle = computed(() => ({
  top: `${props.anchorRect.top}px`,
  left: `${props.anchorRect.left}px`,
  width: `${props.anchorRect.width}px`,
  height: `${props.anchorRect.height}px`,
  '--image-editor-image-gap': `${IMAGE_EDITOR_CHROME_LAYOUT.imageGap}px`,
  '--image-editor-stack-gap': `${IMAGE_EDITOR_CHROME_LAYOUT.stackGap}px`,
  '--image-editor-right-panel-width': `${IMAGE_EDITOR_CHROME_LAYOUT.rightPanelWidth}px`,
  '--image-editor-tool-button-size': `${IMAGE_EDITOR_CHROME_LAYOUT.toolButtonSize}px`,
}))
</script>

<style scoped>
@import './styles/node-tool-buttons.css';

.image-editor-chrome-frame {
  --image-editor-image-gap: 12px;
  --image-editor-stack-gap: 8px;
  --image-editor-right-panel-width: 203px;
  --image-editor-tool-border-width: var(--workspace-control-border-width);
  --image-editor-tool-button-size: 35px;
  --image-editor-tool-button-radius: 10px;
  --image-editor-tool-button-gap: var(--workspace-tool-button-gap);
  --image-editor-tool-icon-size: 21px;
  --node-tool-border-width: var(--image-editor-tool-border-width);
  --node-tool-button-size: var(--image-editor-tool-button-size);
  --node-tool-button-radius: var(--image-editor-tool-button-radius);
  --node-tool-button-gap: var(--image-editor-tool-button-gap);
  --node-tool-icon-size: var(--image-editor-tool-icon-size);
  position: absolute;
  z-index: 21;
  overflow: visible;
  pointer-events: none;
}

.image-editor-chrome-frame__tool-rail {
  position: absolute;
  top: 0;
  left: calc((var(--image-editor-tool-button-size) + var(--image-editor-image-gap)) * -1);
  width: var(--image-editor-tool-button-size);
  padding: 0;
  border: none;
  border-radius: 0;
  background: transparent;
  box-shadow: none;
  display: flex;
  flex-direction: column;
  gap: var(--image-editor-tool-button-gap);
  pointer-events: auto;
  animation: image-editor-chrome-slide-left-in var(--workspace-motion-panel)
    var(--workspace-motion-ease) both;
}

.image-editor-chrome-frame__right-dock {
  position: absolute;
  top: 0;
  right: calc((var(--image-editor-right-panel-width) + var(--image-editor-image-gap)) * -1);
  width: var(--image-editor-right-panel-width);
  height: 100%;
  display: flex;
  flex-direction: column;
  gap: var(--image-editor-stack-gap);
  pointer-events: auto;
  animation: image-editor-chrome-slide-right-in var(--workspace-motion-dock)
    var(--workspace-motion-ease) both;
}

@keyframes image-editor-chrome-slide-left-in {
  from {
    opacity: 0;
    transform: translateX(8px);
  }
  to {
    opacity: 1;
    transform: translateX(0);
  }
}

@keyframes image-editor-chrome-slide-right-in {
  from {
    opacity: 0;
    transform: translateX(-8px);
  }
  to {
    opacity: 1;
    transform: translateX(0);
  }
}

@media (prefers-reduced-motion: reduce) {
  .image-editor-chrome-frame__tool-rail,
  .image-editor-chrome-frame__right-dock {
    animation: none;
  }
}
</style>
