<template>
  <div
    class="image-tool-chrome-frame"
    :style="chromeStyle"
    :data-retouch="mode === 'adjust' ? '' : undefined"
    :data-inpaint="mode === 'inpaint' ? '' : undefined"
    @pointerdown.stop
  >
    <div v-if="$slots.tools" class="image-tool-chrome-frame__tool-rail node-tool-surface">
      <slot name="tools" />
    </div>
    <div class="image-tool-chrome-frame__right-dock">
      <slot name="dock" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { IMAGE_TOOL_CHROME_LAYOUT } from '../../config/imageToolChrome'

type ImageToolChromeMode = 'adjust' | 'inpaint'

const props = defineProps<{
  anchorRect: {
    top: number
    left: number
    width: number
    height: number
  }
  mode: ImageToolChromeMode
}>()

const chromeStyle = computed(() => ({
  top: `${props.anchorRect.top}px`,
  left: `${props.anchorRect.left}px`,
  width: `${props.anchorRect.width}px`,
  height: `${props.anchorRect.height}px`,
  '--image-tool-image-gap': `${IMAGE_TOOL_CHROME_LAYOUT.imageGap}px`,
  '--image-tool-stack-gap': `${IMAGE_TOOL_CHROME_LAYOUT.stackGap}px`,
  '--image-tool-right-panel-width': `${IMAGE_TOOL_CHROME_LAYOUT.rightPanelWidth}px`,
  '--image-tool-button-size': `${IMAGE_TOOL_CHROME_LAYOUT.toolButtonSize}px`,
}))
</script>

<style scoped>
.image-tool-chrome-frame {
  --image-tool-image-gap: 12px;
  --image-tool-stack-gap: 8px;
  --image-tool-right-panel-width: 203px;
  --image-tool-border-width: var(--canvas-control-border-width);
  --image-tool-button-size: 35px;
  --image-tool-button-radius: 10px;
  --image-tool-button-gap: var(--canvas-tool-button-gap);
  --image-tool-icon-size: 21px;
  --node-tool-border-width: var(--image-tool-border-width);
  --node-tool-button-size: var(--image-tool-button-size);
  --node-tool-button-radius: var(--image-tool-button-radius);
  --node-tool-button-gap: var(--image-tool-button-gap);
  --node-tool-icon-size: var(--image-tool-icon-size);
  position: absolute;
  z-index: 21;
  overflow: visible;
  pointer-events: none;
}

.image-tool-chrome-frame__tool-rail {
  position: absolute;
  top: 0;
  left: calc((var(--image-tool-button-size) + var(--image-tool-image-gap)) * -1);
  width: var(--image-tool-button-size);
  padding: 0;
  border: none;
  border-radius: 0;
  background: transparent;
  box-shadow: none;
  display: flex;
  flex-direction: column;
  gap: var(--image-tool-button-gap);
  pointer-events: auto;
  animation: image-tool-chrome-slide-left-in var(--motion-enter-duration) var(--motion-enter-easing)
    both;
}

.image-tool-chrome-frame__right-dock {
  position: absolute;
  top: 0;
  right: calc((var(--image-tool-right-panel-width) + var(--image-tool-image-gap)) * -1);
  width: var(--image-tool-right-panel-width);
  height: 100%;
  display: flex;
  flex-direction: column;
  gap: var(--image-tool-stack-gap);
  pointer-events: auto;
  animation: image-tool-chrome-slide-right-in var(--motion-enter-duration)
    var(--motion-enter-easing) both;
}

@keyframes image-tool-chrome-slide-left-in {
  from {
    opacity: 0;
    transform: translateX(8px);
  }
  to {
    opacity: 1;
    transform: translateX(0);
  }
}

@keyframes image-tool-chrome-slide-right-in {
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
  .image-tool-chrome-frame__tool-rail,
  .image-tool-chrome-frame__right-dock {
    animation: none;
  }
}
</style>
