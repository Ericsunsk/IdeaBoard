<template>
  <div
    class="canvas-layout"
    :class="[
      `tool-${presentation.layout.activeTool}`,
      {
        'interaction-lite': presentation.layout.isInteractionActive,
        'viewport-pan-lite': presentation.layout.isViewportPanActive,
      },
    ]"
    @mousedown.self="presentation.layout.onBackgroundMouseDown"
  >
    <CanvasChrome
      v-bind="presentation.chrome.props"
      @update:board-name-draft="presentation.chrome.commands.updateBoardNameDraft"
      @toggle-board-menu="presentation.chrome.commands.toggleBoardMenu"
      @start-board-rename="presentation.chrome.commands.startBoardRename"
      @cancel-board-rename="presentation.chrome.commands.cancelBoardRename"
      @confirm-board-rename="presentation.chrome.commands.confirmBoardRename"
      @reload-board-catalog="presentation.chrome.commands.reloadBoardCatalog"
      @select-board="presentation.chrome.commands.selectBoard"
      @export="presentation.chrome.commands.exportSelectedImages"
      @navigate-back="presentation.chrome.commands.navigateBack"
      @select-tool="presentation.chrome.commands.selectTool"
      @toggle-minimap="presentation.chrome.commands.toggleMinimap"
      @zoom-out="presentation.chrome.commands.zoomOut"
      @zoom-in="presentation.chrome.commands.zoomIn"
    />

    <div
      :ref="presentation.canvas.containerRef"
      class="canvas-flow-wrap ib-page-dot-bg"
      @dragover.prevent="presentation.canvas.surface.onDragOver"
      @drop.prevent="presentation.canvas.surface.onDrop"
      @pointerdown.capture="presentation.canvas.surface.onPointerDownCapture"
      @contextmenu.capture="presentation.canvas.surface.onContextMenuCapture"
      @wheel="presentation.canvas.surface.onWheel"
    >
      <VueFlow
        :nodes="presentation.canvas.nodes.value"
        :edges="presentation.canvas.edges.value"
        v-bind="presentation.canvas.props"
        @nodes-change="presentation.canvas.flow.onNodesChange"
        @edges-change="presentation.canvas.flow.onEdgesChange"
        @connect-end="presentation.canvas.flow.onConnectEnd"
        @move-start="presentation.canvas.flow.onMoveStart"
        @move="presentation.canvas.flow.onMove"
        @move-end="presentation.canvas.flow.onMoveEnd"
        @viewport-change-end="presentation.canvas.flow.onViewportChangeEnd"
        @node-click="presentation.canvas.flow.onNodeClick"
        @node-drag-start="presentation.canvas.flow.onNodeDragStart"
        @node-drag-stop="presentation.canvas.flow.onNodeDragStop"
        @selection-start="presentation.canvas.flow.onSelectionStart"
        @selection-end="presentation.canvas.flow.onSelectionEnd"
        @pane-click="presentation.canvas.flow.onPaneClick"
        @pane-context-menu="presentation.canvas.flow.onPaneContextMenu"
      >
        <template #zoom-pane>
          <CanvasGroupPresentation v-bind="presentation.canvas.group.props" />
        </template>
        <template #node-canvas="nodeProps">
          <FlowCanvasNode
            v-bind="presentation.canvas.node.propsFor(nodeProps)"
            @node-intent="presentation.canvas.node.listeners.intent"
            @node-mousedown="presentation.canvas.node.listeners.nodeMousedown"
            @request-internals-update="presentation.canvas.node.listeners.requestInternalsUpdate"
          />
        </template>
        <CanvasChromeMiniMap
          v-bind="presentation.canvas.minimap.props"
          @center-requested="presentation.canvas.minimap.commands.centerRequested"
          @drag-start="presentation.canvas.minimap.commands.dragStart"
          @drag-move="presentation.canvas.minimap.commands.dragMove"
          @drag-end="presentation.canvas.minimap.commands.dragEnd"
        />
      </VueFlow>
      <CanvasOverlays
        v-bind="presentation.canvas.overlays.props"
        @create-node="presentation.canvas.overlays.commands.createNode"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { VueFlow } from '@vue-flow/core'
import FlowCanvasNode from '../components/FlowCanvasNode.vue'
import CanvasOverlays from '../components/CanvasOverlays.vue'
import { CanvasGroupPresentation } from '../group-presentation'
import { CanvasChrome, CanvasChromeMiniMap } from '../components/chrome'
import type { CanvasScreenPresentationBinding } from './screenPresentationContract'

defineProps<{
  presentation: CanvasScreenPresentationBinding
}>()
</script>
<style>
@import '../components/styles/canvas-vue-flow-adapter-skin.css';
@import '../components/styles/canvas-style-contracts.css';
</style>

<style scoped>
:global(html.canvas-overscroll-guard),
:global(body.canvas-overscroll-guard) {
  overscroll-behavior: none;
  overscroll-behavior-x: none;
}

.canvas-layout {
  --canvas-control-border-width: 1.2px;
  --canvas-tool-button-gap: 7px;
  --canvas-surface-bg: rgba(255, 255, 255, 0.98);
  --canvas-surface-bg-soft: rgba(255, 255, 255, 0.94);
  --canvas-surface-muted: #f4f7fb;
  --canvas-surface-border: #d8ddea;
  --canvas-surface-border-soft: rgba(34, 42, 54, 0.08);
  --canvas-minimap-node: #c9ced8;
  --canvas-minimap-viewport: #c9ced8;
  --canvas-minimap-mask: rgb(201 206 216 / 0.18);
  --canvas-surface-shadow: 0 14px 32px rgba(15, 23, 42, 0.13);
  --canvas-surface-blur: 10px;
  --canvas-surface-radius: 10px;
  --canvas-floating-radius: 13px;
  --canvas-panel-radius: 8px;
  --canvas-panel-border-width: 1px;
  --canvas-panel-content-gap: 10px;
  --canvas-panel-border: rgba(34, 42, 54, 0.14);
  --canvas-panel-border-soft: rgba(34, 42, 54, 0.1);
  --canvas-panel-header-from: #f8fafc;
  --canvas-panel-header-to: #eef2f7;
  --canvas-text-strong: #111827;
  --canvas-text-body: #334155;
  --canvas-text-muted: #64748b;
  --canvas-text-subtle: #94a3b8;
  --canvas-accent: #9278e6;
  --canvas-accent-hover: #7a6bd5;
  --canvas-accent-strong: #6757c8;
  --canvas-accent-soft: rgba(146, 120, 230, 0.12);
  --canvas-accent-soft-strong: rgba(146, 120, 230, 0.16);
  --canvas-accent-wash: #f8f7ff;
  --canvas-accent-outline: rgba(122, 107, 213, 0.22);
  --canvas-motion-fast: 160ms;
  --canvas-motion-standard: 200ms;
  --canvas-motion-panel: 220ms;
  --canvas-motion-dock: 240ms;
  --canvas-motion-ease: cubic-bezier(0.2, 0.8, 0.2, 1);
  --flow-edge-color: rgb(177 177 183 / 50%);
  --flow-edge-active-color: rgb(146 120 230 / 50%);
  --flow-edge-selected-color: rgb(123 135 152 / 50%);
  --flow-edge-width: 2;
  --flow-edge-dasharray: 5;
  --flow-edge-running-duration: 0.5s;
  width: 100%;
  height: 100vh;
  background: #ffffff;
  position: relative;
  overflow: hidden;
  overscroll-behavior: none;
  overscroll-behavior-x: none;
}

.canvas-flow-wrap {
  position: absolute;
  inset: 0;
  z-index: 2;
  overscroll-behavior: none;
  overscroll-behavior-x: none;
}
</style>
