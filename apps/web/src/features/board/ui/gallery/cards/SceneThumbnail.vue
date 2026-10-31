<template>
  <div ref="rootEl" class="scene-thumbnail">
    <div
      v-if="!presentation.viewBox"
      class="scene-empty"
      :class="blankBoardPreviewSurfaceClass"
      aria-hidden="true"
    ></div>
    <svg
      v-else
      class="scene-canvas"
      :viewBox="presentation.viewBox"
      preserveAspectRatio="xMidYMid meet"
      aria-hidden="true"
      focusable="false"
    >
      <defs>
        <clipPath v-for="clip in clips" :id="clip.id" :key="clip.id">
          <rect
            :x="clip.frame.x"
            :y="clip.frame.y"
            :width="clip.frame.width"
            :height="clip.frame.height"
            :rx="clip.radius"
          />
        </clipPath>
      </defs>
      <g>
        <rect
          v-for="group in presentation.groups"
          :key="group.id"
          class="scene-group"
          :x="group.x"
          :y="group.y"
          :width="group.width"
          :height="group.height"
          :rx="group.radius"
          vector-effect="non-scaling-stroke"
          stroke-width="1"
        />
      </g>
      <g>
        <path
          v-for="edge in presentation.edges"
          :key="edge.key"
          class="scene-edge"
          :d="edge.d"
          vector-effect="non-scaling-stroke"
          stroke-width="0.5"
        />
      </g>
      <g v-for="node in presentation.nodes" :key="node.id" :data-scene-node="node.id">
        <rect
          class="scene-frame"
          :x="node.x"
          :y="node.y"
          :width="node.width"
          :height="node.height"
          :rx="node.radius"
          vector-effect="non-scaling-stroke"
          stroke-width="1.25"
        />
        <g v-if="node.kind === 'prompt'">
          <rect
            v-for="(bar, barIndex) in node.bars"
            :key="barIndex"
            class="scene-bar"
            :x="bar.x"
            :y="bar.y"
            :width="bar.width"
            :height="bar.height"
            :rx="bar.height / 2"
          />
        </g>
        <g v-if="node.kind === 'fusion'">
          <rect
            v-for="(slot, slotIndex) in node.slots"
            :key="slotIndex"
            class="scene-slot"
            :x="slot.x"
            :y="slot.y"
            :width="slot.width"
            :height="slot.height"
            :rx="slot.radius"
            vector-effect="non-scaling-stroke"
            stroke-width="0.75"
          />
        </g>
        <g
          v-if="node.glyphTransform"
          class="scene-glyph"
          vector-effect="non-scaling-stroke"
          stroke-width="1"
          :transform="node.glyphTransform"
        >
          <rect x="-0.5" y="-0.5" width="1" height="1" rx="0.12" />
          <circle cx="0.22" cy="-0.2" r="0.07" />
          <path d="M -0.34 0.34 L -0.12 0.1 L 0.04 0.26 L 0.16 0.16 L 0.36 0.34" />
        </g>
        <SceneRasterImage
          v-for="raster in node.rasters"
          :key="raster.key"
          :workspace-id="workspaceId"
          :board-id="boardId"
          :content-version="contentVersion"
          :asset-id="raster.assetId"
          :x="raster.frame.x"
          :y="raster.frame.y"
          :width="raster.frame.width"
          :height="raster.frame.height"
          :clip-id="clipIdFor(raster.key)"
          :is-enabled="isCardVisible"
        />
      </g>
    </svg>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useResizeObserver } from '@vueuse/core'
import type { PreviewScene } from '../../../core/preview/scene'
import { composeSceneThumbnail } from '../../../core/preview/renderer'
import SceneRasterImage from './SceneRasterImage.vue'
import { blankBoardPreviewSurfaceClass } from '../styling/skin'

const props = defineProps<{
  scene: PreviewScene
  workspaceId: string
  boardId: string
  contentVersion: string
}>()

const rootEl = ref<HTMLElement | null>(null)
const viewport = ref({ width: 0, height: 0 })
useResizeObserver(rootEl, ([entry]) => {
  if (entry) viewport.value = { width: entry.contentRect.width, height: entry.contentRect.height }
})

const presentation = computed(() => composeSceneThumbnail(props.scene, viewport.value))
const clips = computed(() =>
  presentation.value.nodes.flatMap((node) =>
    node.rasters.map((raster) => ({
      id: clipIdFor(raster.key),
      frame: raster.frame,
      radius: raster.radius,
    })),
  ),
)

function clipIdFor(rasterKey: string): string {
  return `thumb-${props.boardId}-${rasterKey}`
}

// Decide at setup: when intersection observation exists, cards start hidden
// so below-fold mounts never start media reads before the observer reports.
// Environments without IntersectionObserver keep media enabled.
const isCardVisible = ref(typeof IntersectionObserver === 'undefined')

let observer: IntersectionObserver | null = null

onMounted(() => {
  if (rootEl.value) {
    viewport.value = { width: rootEl.value.clientWidth, height: rootEl.value.clientHeight }
  }
  // Cards below the fold never start media reads: rasters mount only while
  // the card intersects (with a preload margin). Environments without
  // IntersectionObserver keep media enabled.
  if (typeof IntersectionObserver === 'undefined' || !rootEl.value) return
  observer = new IntersectionObserver(
    (entries) => {
      const entry = entries[0]
      if (entry) isCardVisible.value = entry.isIntersecting
    },
    { rootMargin: '200px' },
  )
  observer.observe(rootEl.value)
})

onUnmounted(() => {
  observer?.disconnect()
  observer = null
})
</script>

<style scoped>
.scene-thumbnail {
  display: block;
  width: 100%;
  height: 100%;
}

.scene-empty {
  width: 100%;
  height: 100%;
}

.scene-canvas {
  display: block;
  width: 100%;
  height: 100%;
}

.scene-frame {
  fill: var(--card);
  stroke: var(--border);
}

.scene-group {
  fill: none;
  stroke: var(--border);
  stroke-dasharray: 3 2;
}

.scene-edge {
  fill: none;
  stroke: var(--muted-foreground);
  opacity: 0.3;
}

.scene-bar {
  fill: var(--muted-foreground);
  opacity: 0.28;
}

.scene-slot {
  fill: var(--muted);
  stroke: var(--border);
  opacity: 0.35;
}

.scene-glyph {
  fill: none;
  stroke: var(--muted-foreground);
  opacity: 0.7;
}
</style>
