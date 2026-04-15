<template>
  <div 
    class="canvas-layout" 
    :class="{ 'copy-mode': isDuplicateModifierPressed && activeTool === 'select' }"
    ref="canvasLayoutRef"
    :style="canvasStyle" 
    @mousedown="onCanvasMouseDown"
    @contextmenu="onCanvasContextMenu"
    @dragover.prevent="isDraggingFile = true"
    @dragleave="isDraggingFile = false"
    @drop="handleDrop"
  >
    <!-- ① 无限画布背景层 -->
    <div class="canvas-grid" :class="{ 'grid-dragging': isDraggingFile }"></div>

    <!-- ② 顶部悬浮栏 -->
    <header class="floating-header">
      <div class="header-left">
        <div class="file-name-chip" @mousedown.stop>
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/></svg>
          <input
            v-if="isRenamingBoard"
            ref="boardNameInputRef"
            v-model="boardNameDraft"
            class="board-name-input"
            type="text"
            maxlength="60"
            @keydown.enter.prevent="confirmBoardRename"
            @keydown.esc.prevent="cancelBoardRename"
            @blur="confirmBoardRename"
          />
          <button
            v-else
            type="button"
            class="file-name-btn"
            title="点击重命名画布"
            @click.stop="startBoardRename"
          >
            {{ currentBoardName }}
          </button>
          <button
            type="button"
            class="board-switch-btn"
            title="选择画布"
            @click.stop="toggleBoardMenu"
          >
            <svg class="chevron-down" :class="{ open: isBoardMenuOpen }" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3"><polyline points="6 9 12 15 18 9"/></svg>
          </button>
          <div v-if="isBoardMenuOpen" class="board-switch-menu" @mousedown.stop @click.stop>
            <button
              v-for="board in boardOptions"
              :key="board.id"
              type="button"
              class="board-switch-item"
              :class="{ active: board.id === currentBoardId }"
              @click="handleBoardSelect(board.id)"
            >
              {{ board.name }}
            </button>
            <div v-if="boardOptions.length === 0" class="board-switch-empty">No canvas</div>
          </div>
        </div>
      </div>
      <div class="header-right">
        <button class="btn-ghost share-btn" @mousedown.stop>Share</button>
        <button class="btn-primary export-btn" :disabled="selectedIds.length === 0" @mousedown.stop @click="handleExport">Export</button>
        <div class="user-avatar-wrap" @click="router.push('/user/center')" title="个人中心">
          <img v-if="userStore.user?.avatar" :src="userStore.user.avatar" class="user-avatar" />
          <div v-else class="user-avatar-placeholder">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/>
            </svg>
          </div>
        </div>
      </div>
    </header>

    <!-- 框选视觉矩形 -->
    <div 
      v-if="isSelecting" 
      class="selection-marquee"
      :style="{
        left: Math.min(selectionRect.x1, selectionRect.x2) + 'px',
        top: Math.min(selectionRect.y1, selectionRect.y2) + 'px',
        width: Math.abs(selectionRect.x2 - selectionRect.x1) + 'px',
        height: Math.abs(selectionRect.y2 - selectionRect.y1) + 'px'
      }"
    ></div>

    <!-- ③ 左侧工具栏 -->
    <aside class="floating-toolbar">
      <div class="tool-item" :class="{ active: activeTool === 'select' }" title="Select (V)" @click="activeTool = 'select'">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 3l7.07 16.97 2.51-7.39 7.39-2.51L3 3z"/><path d="M13 13l6 6"/></svg>
      </div>
      <div class="tool-item" :class="{ active: activeTool === 'hand' }" title="Hand (H)" @click="activeTool = 'hand'">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 11V6a2 2 0 0 0-2-2v0a2 2 0 0 0-2 2v0"/><path d="M14 10V4a2 2 0 0 0-2-2v0a2 2 0 0 0-2 2v10"/><path d="M10 10.5V6a2 2 0 0 0-2-2v0a2 2 0 0 0-2 2v8"/><path d="M18 8a2 2 0 1 1 4 0v6a8 8 0 0 1-8 8h-2c-2.8 0-4.5-.86-5.99-2.34l-3.6-3.6a2 2 0 0 1 2.83-2.82L7 15"/></svg>
      </div>
      <div class="tool-item" :class="{ active: activeTool === 'text' }" title="Text (T)" @click="activeTool = 'text'">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><polyline points="4 7 4 4 20 4 20 7"/><line x1="9" y1="20" x2="15" y2="20"/><line x1="12" y1="4" x2="12" y2="20"/></svg>
      </div>
      <div class="toolbar-divider"></div>
      <!-- 业务入口 -->
      <div 
        class="tool-item" 
        v-for="t in tools" 
        :key="t.title" 
        :class="{ active: route.path === t.path }" 
        :title="t.title" 
        @click="router.push(t.path)"
      >
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" v-html="t.icon"></svg>
      </div>
    </aside>

    <!-- ④ 底部指令栏 -->
    <footer class="floating-bottom-bar">
      <!-- 设置面板 -->
      <transition name="slide-up">
        <div v-if="showSettings" class="settings-panel">
          <div class="settings-section">
            <div class="section-header">
              <span class="section-title">Aspect Ratio</span>
              <div class="ratio-group">
                <button 
                  v-for="r in ['1:1', '3:4', '4:3', '9:16', '16:9']" 
                  :key="r"
                  class="ratio-btn"
                  :class="{ active: selectedRatio === r }"
                  @click="selectedRatio = r"
                >
                  {{ r }}
                </button>
              </div>
            </div>
          </div>

          <div class="settings-section">
            <div class="section-header">
              <span class="section-title">Resolution</span>
              <div class="ratio-group">
                <button 
                  v-for="res in ['1K', '2K', '4K']" 
                  :key="res"
                  class="ratio-btn"
                  :class="{ active: selectedResolution === res }"
                  @click="selectedResolution = res"
                >
                  {{ res }}
                </button>
              </div>
            </div>
          </div>
          
          <div class="settings-section">
            <div class="section-header">
              <span class="section-title">Style Model</span>
              <span class="section-subtitle">{{ selectedCardIds.length }} selected</span>
            </div>
            <div class="style-cards-grid">
              <div 
                v-for="card in styleCards" 
                :key="card.id"
                class="style-card-item"
                :class="{ active: selectedCardIds.includes(card.id) }"
                @click="toggleCard(card.id)"
              >
                <img :src="card.cover_image" class="card-preview" />
                <div class="card-info">
                  <span class="card-name">{{ card.name }}</span>
                </div>
                <div class="card-check">
                  <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="4"><polyline points="20 6 9 17 4 12"/></svg>
                </div>
              </div>
            </div>
          </div>
        </div>
      </transition>

      <div v-if="selectedPreviewItems.length > 0" class="selected-preview-bar" @mousedown.stop>
        <div class="selected-preview-header">
          <span class="selected-preview-count">{{ selectedPreviewItems.length }} selected</span>
          <button class="clear-selected-btn" type="button" @click="clearSelectedIds">Clear</button>
        </div>
        <div class="selected-preview-list">
          <button
            v-for="item in visiblePreviewItems"
            :key="item.id"
            type="button"
            class="selected-preview-item"
            @click="focusSelectedItem(item.id)"
          >
            <img :src="item.url" class="selected-preview-image" />
            <span class="selected-preview-remove" @click.stop="removeSelectedId(item.id)">×</span>
          </button>
          <div v-if="previewOverflowCount > 0" class="selected-preview-overflow">+{{ previewOverflowCount }}</div>
        </div>
      </div>

      <div class="prompt-container" @mousedown.stop>
        <button class="add-btn" :class="{ active: showSettings }" @click="showSettings = !showSettings">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
        </button>
        <input v-model="userPrompt" type="text" :placeholder="promptPlaceholder" @keyup.enter="handleGenerate" />
        <button class="send-btn" :disabled="!userPrompt.trim() || generating" @click="handleGenerate">
          <svg v-if="!generating" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><line x1="5" y1="12" x2="19" y2="12"/><polyline points="12 5 19 12 12 19"/></svg>
          <n-spin v-else size="small" />
        </button>
      </div>
    </footer>

    <!-- ⑤ 右下角控件 -->
    <div class="floating-status">
      <div class="zoom-controls">
        <button class="zoom-btn" @click="updateZoom(-10)">−</button>
        <span class="zoom-level">{{ zoomLevel }}%</span>
        <button class="zoom-btn" @click="updateZoom(10)">+</button>
      </div>
      <button class="logo-btn" title="返回画布列表" @click="router.push('/user/boards')">
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
          <line x1="16" y1="12" x2="6" y2="12" />
          <polyline points="10 8 6 12 10 16" />
        </svg>
      </button>
    </div>

    <div
      v-if="leaveConfirmVisible"
      class="leave-confirm-mask"
      @mousedown.stop.prevent
      @click.self="resolveLeaveConfirm('cancel')"
    >
      <div class="leave-confirm-card" @mousedown.stop @click.stop>
        <div class="leave-confirm-title">离开前是否保存？</div>
        <div class="leave-confirm-desc">当前画布有未保存更改。</div>
        <div class="leave-confirm-actions">
          <button type="button" class="leave-btn ghost" @click="resolveLeaveConfirm('cancel')">继续编辑</button>
          <button type="button" class="leave-btn danger" @click="resolveLeaveConfirm('discard')">不保存</button>
          <button type="button" class="leave-btn primary" @click="resolveLeaveConfirm('save')">保存并离开</button>
        </div>
      </div>
    </div>

    <div
      v-if="nodeContextMenu.visible"
      class="node-context-menu"
      :style="{ left: `${nodeContextMenu.x}px`, top: `${nodeContextMenu.y}px` }"
      @mousedown.stop
      @click.stop
    >
      <button type="button" class="node-context-item" @click="createImageNode">
        + Image 节点
      </button>
      <button type="button" class="node-context-item" @click="createPromptNode">
        + Prompt 节点
      </button>
      <button type="button" class="node-context-item" @click="createModelNode">
        + 模特节点
      </button>
      <button type="button" class="node-context-item" @click="createMaskNode">
        + Mask 节点
      </button>
      <button type="button" class="node-context-item" @click="createColorNode">
        + Color 节点
      </button>
    </div>

    <!-- ⑥ 内容展示区 -->
    <div class="canvas-content">
      <ImageNode
        v-for="node in imageNodes"
        :key="node.id"
        :node="node"
        :active="isNodeSelected('image', node.id)"
        :dragging="isNodeDragging('image', node.id)"
        :copy-mode="isDuplicateModifierPressed && activeTool === 'select'"
        :stage-height="getImageNodeStageHeight(node)"
        :display-image="getImageNodeDisplayImage(node)"
        @node-mousedown="onImageNodeMouseDown($event, node)"
        @delete="deleteImageNode(node.id)"
        @stage-drop="onImageNodeStageDrop($event, node)"
        @open-file="openImageNodeFilePicker(node)"
        @append-to-canvas="appendImageNodeDisplayToCanvas(node)"
        @reset="resetImageNode(node)"
        @display-dragstart="onImageNodeDisplayDragStart($event, node)"
      />

      <PromptNode
        v-for="node in promptNodes"
        :key="node.id"
        :node="node"
        :active="isNodeSelected('prompt', node.id)"
        :dragging="isNodeDragging('prompt', node.id)"
        :copy-mode="isDuplicateModifierPressed && activeTool === 'select'"
        @node-mousedown="onPromptNodeMouseDown($event, node)"
        @delete="deletePromptNode(node.id)"
        @focus="activatePromptNode(node.id)"
        @update:text="node.text = $event"
      />

      <ModelNode
        v-for="node in modelNodes"
        :key="node.id"
        :node="node"
        :active="isNodeSelected('model', node.id)"
        :dragging="isNodeDragging('model', node.id)"
        :copy-mode="isDuplicateModifierPressed && activeTool === 'select'"
        :generating="generating"
        :stage-height="getModelStageHeight(node)"
        :angle-options="modelAngleOptions"
        :background-options="modelBackgroundOptions"
        @node-mousedown="onModelNodeMouseDown($event, node)"
        @delete="deleteModelNode(node.id)"
        @open-file-picker="openModelNodeFilePicker(node)"
        @clear-image="clearModelNodeImage(node)"
        @stage-drop="onModelStageDrop($event, node)"
        @toggle-angle="toggleModelAngle(node, $event)"
        @clear-angles="clearModelAngles(node)"
        @select-background="node.background = $event"
        @generate="handleModelNodeGenerate(node)"
      />

      <MaskNode
        v-for="node in maskNodes"
        :key="node.id"
        :node="node"
        :active="isNodeSelected('mask', node.id)"
        :dragging="isNodeDragging('mask', node.id)"
        :copy-mode="isDuplicateModifierPressed && activeTool === 'select'"
        :stage-height="getMaskStageHeight(node)"
        :canvas-width="getMaskCanvasWidth(node)"
        @node-mousedown="onMaskNodeMouseDown($event, node)"
        @delete="deleteMaskNode(node.id)"
        @open-file-picker="openMaskNodeFilePicker(node)"
        @clear-image="clearMaskNodeImage(node)"
        @stage-drop="onMaskStageDrop($event, node)"
        @canvas-mousedown="onMaskCanvasMouseDown($event, node)"
        @set-draw-mode="node.drawMode = $event"
        @update:brush-size="node.brushSize = $event"
        @update:refine-amount="node.refineAmount = clampMaskRefineAmount($event)"
        @toggle-invert="node.invertMask = !node.invertMask"
        @clear-mask="clearMaskNodeMask(node)"
        @set-canvas-ref="setMaskCanvasRef(node.id, $event)"
      />

      <ColorNode
        v-for="node in colorNodes"
        :key="node.id"
        :node="node"
        :active="isNodeSelected('color', node.id)"
        :dragging="isNodeDragging('color', node.id)"
        :copy-mode="isDuplicateModifierPressed && activeTool === 'select'"
        :generating="generating"
        :is-popover-open="activeColorPopoverNodeId === node.id"
        :popover-hex="colorPopoverHex"
        :popover-hue="colorPopoverHue"
        :popover-saturation="colorPopoverSaturation"
        :popover-lightness="colorPopoverLightness"
        :spectrum-style="colorSpectrumStyle"
        :cursor-style="colorSpectrumCursorStyle"
        :quick-swatches="colorQuickSwatches"
        @node-mousedown="onColorNodeMouseDown($event, node)"
        @delete="deleteColorNode(node.id)"
        @toggle-popover="toggleColorPopover(node)"
        @target-color-change="onColorNodeTargetColorChange(node, $event)"
        @strength-input="node.strength = Math.max(0, Math.min(100, Math.round($event)))"
        @popover-hex-input="onColorPopoverHexInput($event)"
        @popover-hex-commit="commitColorPopoverHex"
        @popover-hue-input="colorPopoverHue = $event; applyColorPopoverToNode()"
        @popover-saturation-input="colorPopoverSaturation = $event; applyColorPopoverToNode()"
        @popover-lightness-input="colorPopoverLightness = $event; applyColorPopoverToNode()"
        @spectrum-pointerdown="onColorSpectrumPointerDown($event)"
        @select-swatch="setColorPopoverHex($event)"
        @eyedropper="onColorNodeEyedropperClick(node)"
        @generate="handleColorNodeGenerate(node)"
        @set-spectrum-ref="setColorSpectrumElement($event)"
      />

       <div 
          v-for="msg in messages" 
          :key="msg.id" 
          class="canvas-item"
          :class="{ active: selectedIds.includes(msg.id) }"
          :style="{ 
            left: '50%',
            top: '50%',
            transform: `translate(${msg.x}px, ${msg.y}px)`,
            width: msg.width ? `${msg.width}px` : 'auto',
          }"
          @mousedown.stop="onItemMouseDown($event, msg)"
          @contextmenu.prevent.stop="onItemContextMenu($event, msg)"
       >
          <img
            v-if="msg.type === 'image'"
            :src="msg.url"
            class="canvas-img"
            draggable="false"
            :ref="(el) => setCanvasImageRef(msg.id, el)"
            @load="onImageLoad(msg, $event)"
          />
          <div
            v-if="activeImageActionId === msg.id"
            class="image-action-menu"
            :class="{ 'strength-mode': showStyleStrengthControl }"
            :style="showStyleStrengthControl ? getStyleStrengthMenuStyle(msg) : undefined"
            @mousedown.stop
            @click.stop
          >
            <template v-if="!showStyleStrengthControl">
              <button
                type="button"
                class="image-action-icon-btn"
                :disabled="generating"
                title="抠图退底"
                @click="runCanvasAction('remove-bg', msg)"
              >
                <span class="action-logo">
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
                    <rect x="3" y="3" width="18" height="18" rx="2" />
                    <path d="M12 8v8M8 12h8" />
                  </svg>
                </span>
                <span class="action-tooltip">抠图退底</span>
              </button>
              <button
                type="button"
                class="image-action-icon-btn"
                :disabled="generating"
                title="款式创新"
                @mousedown.stop="onStyleInnovateMouseDown($event)"
                @click="onStyleInnovateClick(msg)"
              >
                <span class="action-logo">
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
                    <path d="M12 3v4M12 17v4M3 12h4M17 12h4M5.6 5.6l2.8 2.8M15.6 15.6l2.8 2.8M5.6 18.4l2.8-2.8M15.6 8.4l2.8-2.8" />
                  </svg>
                </span>
                <span class="action-tooltip">款式创新</span>
              </button>
              <button
                type="button"
                class="image-action-icon-btn"
                :disabled="generating"
                title="转矢量图"
                @click="runCanvasAction('to-vector', msg)"
              >
                <span class="action-logo">
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
                    <circle cx="5" cy="19" r="2" />
                    <circle cx="19" cy="5" r="2" />
                    <path d="M5 17C5 10 10 5 19 5" />
                  </svg>
                </span>
                <span class="action-tooltip">转矢量图</span>
              </button>
              <button
                type="button"
                class="image-action-icon-btn"
                :disabled="generating"
                title="线稿成款"
                @click="runCanvasAction('sketch-to-design', msg)"
              >
                <span class="action-logo">
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
                    <path d="M18 2l-6.3 6.3M12 14l-4 4" />
                    <path d="M3 21l3.5-3.5" />
                    <path d="M15 6l4 4" />
                    <path d="M20 7l-2-2" />
                  </svg>
                </span>
                <span class="action-tooltip">线稿成款</span>
              </button>
            </template>
            <div
              v-else
              class="style-strength-pop"
              :style="{ height: `${getImageDisplayHeight(msg)}px` }"
              @mousedown.stop
            >
              <div
                ref="styleStrengthTrackRef"
                class="style-strength-track"
                @mousedown.stop.prevent="onStyleStrengthTrackMouseDown($event)"
              >
                <div class="style-strength-fill" :style="{ height: `${styleInnovateStrengthPct}%` }"></div>
              </div>
            </div>
          </div>
          
          <!-- 全方位缩放手柄 (仅在单选时显示) -->
          <template v-if="selectedIds.length === 1 && selectedIds[0] === msg.id">
            <div class="resize-edge n" @mousedown.stop="onResizeMouseDown($event, msg, 'n')"></div>
            <div class="resize-edge s" @mousedown.stop="onResizeMouseDown($event, msg, 's')"></div>
            <div class="resize-edge e" @mousedown.stop="onResizeMouseDown($event, msg, 'e')"></div>
            <div class="resize-edge w" @mousedown.stop="onResizeMouseDown($event, msg, 'w')"></div>
            <div class="resize-corner nw" @mousedown.stop="onResizeMouseDown($event, msg, 'nw')"></div>
            <div class="resize-corner ne" @mousedown.stop="onResizeMouseDown($event, msg, 'ne')"></div>
            <div class="resize-corner sw" @mousedown.stop="onResizeMouseDown($event, msg, 'sw')"></div>
            <div class="resize-corner se" @mousedown.stop="onResizeMouseDown($event, msg, 'se')"></div>
          </template>
       </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick, watch } from 'vue'
import type { ComponentPublicInstance } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useMessage, NSpin } from 'naive-ui'
import { segApi, taskApi, styleCardApi, uploadApi, vectorizeApi } from '../../api/index'
import type { StyleCard } from '../../api/index'
import { useUserStore } from '../../stores/user'
import { useCanvasViewport } from '../../composables/useCanvasViewport'
import { useCanvasBoard } from '../../composables/useCanvasBoard'
import { useCanvasEditor } from '../../composables/useCanvasEditor'
import { useCanvasHistory } from '../../composables/useCanvasHistory'
import { useColorNode } from '../../composables/useColorNode'
import { useGenerationPipeline } from '../../composables/useGenerationPipeline'
import { useImageNode } from '../../composables/useImageNode'
import { clampMaskRefineAmount, useMaskNode } from '../../composables/useMaskNode'
import { useModelNode } from '../../composables/useModelNode'
import ColorNode from '../../components/canvas/nodes/ColorNode.vue'
import ImageNode from '../../components/canvas/nodes/ImageNode.vue'
import MaskNode from '../../components/canvas/nodes/MaskNode.vue'
import ModelNode from '../../components/canvas/nodes/ModelNode.vue'
import PromptNode from '../../components/canvas/nodes/PromptNode.vue'
import {
  createCanvasImageItemAt,
  createColorNodeAt,
  createImageNodeAt,
  createMaskNodeAt,
  createModelNodeAt,
  createPromptNodeAt,
} from '../../utils/canvasNodeFactory'
import type {
  CanvasBoardSnapshot,
  CanvasImageItem,
  ColorNodeItem,
  ImageNodeItem,
  MaskNodeItem,
  ModelNodeItem,
  PromptNodeItem,
} from '../../types/canvasNodes'

const router = useRouter()
const route = useRoute()
const message = useMessage()
const userStore = useUserStore()
const defaultCanvasName = 'Untitled'

type SegType = 'common' | 'commodity' | 'body'
type CanvasActionKey = 'remove-bg' | 'style-innovate' | 'sketch-to-design' | 'to-vector'

const multiAngleLabelMap: Record<string, string> = {
  front_full: '正面全身',
  front_left_45: '前左 45°',
  front_right_45: '前右 45°',
  left_90: '左侧 90°',
  right_90: '右侧 90°',
  back_left_45: '后左 45°',
  back_right_45: '后右 45°',
  back_full: '背面全身',
  front_half: '正面半身',
  side_half: '侧面半身',
  portrait: '肖像特写',
  side_portrait: '侧颜特写',
}

const modelBackgroundOptions = [
  { key: 'original', label: '原图背景' },
  { key: 'studio', label: '影棚白底' },
]

const modelAngleOptions = [
  { key: 'front_full', label: '正面全身' },
  { key: 'front_left_45', label: '前左 45°' },
  { key: 'front_right_45', label: '前右 45°' },
  { key: 'left_90', label: '左侧 90°' },
  { key: 'right_90', label: '右侧 90°' },
  { key: 'back_left_45', label: '后左 45°' },
  { key: 'back_right_45', label: '后右 45°' },
  { key: 'back_full', label: '背面全身' },
  { key: 'front_half', label: '正面半身' },
  { key: 'side_half', label: '侧面半身' },
  { key: 'portrait', label: '肖像特写' },
  { key: 'side_portrait', label: '侧颜特写' },
]

const modelDefaultPose = 'runway_stand'
const modelDefaultExpression = 'cool'
const nodeImageDragMime = 'application/x-feng-image-url'

// 状态
const userPrompt = ref('')
const generating = ref(false)
const messages = ref<CanvasImageItem[]>([])
const {
  zoomLevel,
  canvasX,
  canvasY,
  canvasLayoutRef,
  canvasStyle,
  updateZoom,
  getCanvasPointFromClient,
  setViewport,
  resetViewport,
  createViewportSnapshot,
} = useCanvasViewport()
const activeTool = ref('select')
const isDraggingFile = ref(false)
const showSettings = ref(false)
const imageNodes = ref<ImageNodeItem[]>([])
const promptNodes = ref<PromptNodeItem[]>([])
const modelNodes = ref<ModelNodeItem[]>([])
const maskNodes = ref<MaskNodeItem[]>([])
const colorNodes = ref<ColorNodeItem[]>([])
const defaultSegType: SegType = 'common'
const styleInnovateDefaultStrength = 5
const styleInnovateStrength = ref(styleInnovateDefaultStrength)
const styleStrengthTrackRef = ref<HTMLElement | HTMLElement[] | null>(null)
const showStyleStrengthControl = ref(false)
const styleInnovateStrengthPct = computed(() => (styleInnovateStrength.value / 10) * 100)
const suppressStyleInnovateClick = ref(false)
const canvasImageRefs = new Map<number, HTMLImageElement>()
const styleStrengthMenuGap = 16
const colorQuickSwatches = ['#A3E635', '#7A6BD5', '#22C55E', '#3B82F6', '#F59E0B', '#EF4444', '#111827', '#F3F4F6']

let exportLoading: { destroy: () => void } | null = null
let styleStrengthLongPressTimer: number | null = null
let styleStrengthHideTimer: number | null = null
let styleStrengthDragging = false
let styleStrengthPointerY = 0

const tools = [
  { title: '图・文生图', path: '/user/picture', icon: '<rect x="3" y="3" width="18" height="18" rx="2"/><path d="M3 14l4-4 5 5 5-5 4 4"/><circle cx="8.5" cy="8.5" r="1.5"/><path d="M19 3l-1 2-2 1 2 1 1 2 1-2 2-1-2-1-1-2z" fill="currentColor" stroke="none"/>' },
  { title: '图案工作室', path: '/user/pattern-gen', icon: '<rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/>' }
]

const {
  selectedIds,
  activeImageActionId,
  isDuplicateModifierPressed,
  activeImageNodeId,
  activePromptNodeId,
  activeModelNodeId,
  activeMaskNodeId,
  activeColorNodeId,
  nodeContextMenu,
  isSelecting,
  selectionRect,
  activateImageNode,
  activatePromptNode,
  activateModelNode,
  activateMaskNode,
  activateColorNode,
  isNodeSelected,
  isNodeDragging,
  closeNodeContextMenu,
  resetEditorUiState,
  onCanvasContextMenu,
  onCanvasMouseDown,
  onItemMouseDown,
  onItemContextMenu,
  onImageNodeMouseDown,
  onPromptNodeMouseDown,
  onModelNodeMouseDown,
  onMaskNodeMouseDown,
  onColorNodeMouseDown,
  onResizeMouseDown,
  readNodeImageDropPayload,
  handleDrop: handleCanvasDropCore,
  clearSelectedIds,
  removeSelectedNodeKey,
  removeSelectedId,
  focusSelectedItem,
} = useCanvasEditor({
  messages,
  imageNodes,
  promptNodes,
  modelNodes,
  maskNodes,
  colorNodes,
  activeTool,
  showStyleStrengthControl,
  canvasX,
  canvasY,
  zoomLevel,
  canvasLayoutRef,
  isBoardMenuOpen: () => isBoardMenuOpen.value,
  setBoardMenuOpen: (visible) => {
    isBoardMenuOpen.value = visible
  },
  isBoardRenaming: () => isRenamingBoard.value,
  isLeaveConfirmVisible: () => leaveConfirmVisible.value,
  activeColorPopoverNodeId: () => activeColorPopoverNodeId.value,
  confirmBoardRename: () => confirmBoardRename(),
  resolveLeaveConfirm: (decision) => resolveLeaveConfirm(decision),
  closeColorPopover: () => closeColorPopover(),
  updateZoom,
  getCanvasPointFromClient,
  createCanvasImageAt: createCanvasImageItemAt,
  uploadImage: uploadApi.upload,
  importCanvasImageToImageNodeAtPoint: (clientX, clientY, item) => importCanvasImageToImageNodeAtPoint(clientX, clientY, item),
  importCanvasImageToMaskNodeAtPoint: (clientX, clientY, item) => importCanvasImageToMaskNodeAtPoint(clientX, clientY, item),
  importCanvasImageToModelNodeAtPoint: (clientX, clientY, item) => importCanvasImageToModelNodeAtPoint(clientX, clientY, item),
  deleteImageNode,
  deletePromptNode,
  deleteModelNode,
  deleteMaskNode,
  deleteColorNode,
  beginHistoryBatch: () => beginCanvasHistoryBatch(),
  endHistoryBatch: (commit) => endCanvasHistoryBatch(commit),
  undo: () => undoCanvasSnapshot(),
  redo: () => redoCanvasSnapshot(),
  message,
  nodeImageDragMime,
})

const {
  activeColorPopoverNodeId,
  colorPopoverHex,
  colorPopoverHue,
  colorPopoverSaturation,
  colorPopoverLightness,
  colorSpectrumRef,
  colorSpectrumStyle,
  colorSpectrumCursorStyle,
  normalizeHexColor,
  setColorPopoverHex,
  applyColorPopoverToNode,
  closeColorPopover,
  toggleColorPopover,
  onColorSpectrumPointerDown,
  onColorPopoverHexInput,
  commitColorPopoverHex,
  onColorNodeTargetColorChange,
  onColorNodeEyedropperClick,
} = useColorNode({
  colorNodes,
  message,
})

const {
  getMaskCanvasWidth,
  getMaskStageHeight,
  setMaskCanvasRef,
  renderMaskNodeMask,
  onMaskCanvasMouseDown,
  dataUrlToFile,
  buildFullMaskDataUrl,
  refineMaskDataUrl,
  openMaskNodeFilePicker,
  clearMaskNodeImage,
  clearMaskNodeMask,
  onMaskStageDrop: handleMaskStageDrop,
  importCanvasImageToMaskNodeAtPoint,
  removeMaskRuntimeFor,
  resetMaskRuntime,
} = useMaskNode({
  maskNodes,
  activeImageActionId,
  showStyleStrengthControl,
  message,
  uploadImage: uploadApi.upload,
  loadImageRatioFromFile,
  loadImageRatioFromUrl,
  loadImageSizeFromUrl,
  activateMaskNode,
  closeNodeContextMenu,
  readNodeImageDropPayload,
})

const {
  getImageNodeStageHeight,
  getImageNodePreferredImage,
  getImageNodeDisplayImage,
  openImageNodeFilePicker,
  resetImageNode,
  onImageNodeStageDrop: handleImageNodeStageDrop,
  appendImageNodeDisplayToCanvas,
  onImageNodeDisplayDragStart,
  importCanvasImageToImageNodeAtPoint,
} = useImageNode({
  imageNodes,
  messages,
  selectedIds,
  message,
  uploadImage: uploadApi.upload,
  loadImageRatioFromFile,
  loadImageRatioFromUrl,
  readNodeImageDropPayload,
  createCanvasImageAt: createCanvasImageItemAt,
  nodeImageDragMime,
})

const {
  getModelStageHeight,
  toggleModelAngle,
  clearModelAngles,
  openModelNodeFilePicker,
  clearModelNodeImage,
  onModelStageDrop: handleModelStageDrop,
  importCanvasImageToModelNodeAtPoint,
} = useModelNode({
  modelNodes,
  message,
  uploadImage: uploadApi.upload,
  loadImageRatioFromFile,
  loadImageRatioFromUrl,
  readNodeImageDropPayload,
})

const {
  stopTaskPolling,
  resolvePreferredImageOutputNode,
  appendGeneratedImages,
  submitTaskAndPoll,
} = useGenerationPipeline({
  generating,
  messages,
  selectedIds,
  imageNodes,
  activeImageNodeId,
  message,
  loadImageRatioFromUrl,
  createCanvasImageAt: createCanvasImageItemAt,
  submitTask: taskApi.submit,
  getTask: taskApi.get,
})

function getModelAngleLabel(key: string): string {
  return multiAngleLabelMap[key] || key
}

function loadImageRatioFromUrl(url: string): Promise<number | null> {
  return new Promise((resolve) => {
    const img = new Image()
    img.onload = () => {
      if (!img.naturalWidth || !img.naturalHeight) {
        resolve(null)
        return
      }
      resolve(img.naturalHeight / img.naturalWidth)
    }
    img.onerror = () => resolve(null)
    img.src = url
  })
}

function loadImageSizeFromUrl(url: string): Promise<{ width: number; height: number } | null> {
  return new Promise((resolve) => {
    const img = new Image()
    img.onload = () => {
      if (!img.naturalWidth || !img.naturalHeight) {
        resolve(null)
        return
      }
      resolve({ width: img.naturalWidth, height: img.naturalHeight })
    }
    img.onerror = () => resolve(null)
    img.src = url
  })
}

function loadImageRatioFromFile(file: File): Promise<number | null> {
  return new Promise((resolve) => {
    const localUrl = URL.createObjectURL(file)
    const img = new Image()
    img.onload = () => {
      const ratio = img.naturalWidth && img.naturalHeight
        ? img.naturalHeight / img.naturalWidth
        : null
      URL.revokeObjectURL(localUrl)
      resolve(ratio)
    }
    img.onerror = () => {
      URL.revokeObjectURL(localUrl)
      resolve(null)
    }
    img.src = localUrl
  })
}

function resetEditorState() {
  messages.value = []
  resetEditorUiState()
  resetMaskRuntime()
  imageNodes.value = []
  promptNodes.value = []
  modelNodes.value = []
  maskNodes.value = []
  colorNodes.value = []
  userPrompt.value = ''
  resetViewport()
}

let resetHistoryBaseline: ((snapshot?: CanvasBoardSnapshot | null) => void) | null = null

async function applyBoardSnapshot(state: CanvasBoardSnapshot | null) {
  if (!state) {
    resetEditorState()
    await nextTick()
    resetHistoryBaseline?.(createBoardSnapshot())
    return
  }

  messages.value = state.messages.map(item => ({ ...item }))
  resetEditorUiState()

  imageNodes.value = state.imageNodes.map(node => ({
    ...node,
    outputImages: node.outputImages.map(output => ({ ...output })),
  }))

  promptNodes.value = state.promptNodes.map(node => ({ ...node }))

  modelNodes.value = state.modelNodes.map(node => ({
    ...node,
    selectedAngles: [...node.selectedAngles],
  }))
  maskNodes.value = state.maskNodes.map(node => ({
    ...node,
    refineAmount: clampMaskRefineAmount(node.refineAmount ?? 0),
    invertMask: !!node.invertMask,
  }))
  colorNodes.value = state.colorNodes.map(node => ({
    ...node,
    strength: Math.max(0, Math.min(100, Math.round(node.strength ?? 70))),
  }))
  activeColorPopoverNodeId.value = null
  setViewport(state.viewport)

  await nextTick()
  maskNodes.value.forEach(node => renderMaskNodeMask(node))
  resetHistoryBaseline?.(state)
}

function createBoardSnapshot(): CanvasBoardSnapshot {
  return {
    messages: messages.value.map(item => ({ ...item })),
    imageNodes: imageNodes.value.map(node => ({
      ...node,
      outputImages: node.outputImages.map(output => ({ ...output })),
    })),
    promptNodes: promptNodes.value.map(node => ({ ...node })),
    modelNodes: modelNodes.value.map(node => ({
      ...node,
      selectedAngles: [...node.selectedAngles],
    })),
    maskNodes: maskNodes.value.map(node => ({ ...node })),
    colorNodes: colorNodes.value.map(node => ({ ...node })),
    viewport: createViewportSnapshot(),
  }
}

const {
  notifySnapshotChanged,
  beginBatch: beginCanvasHistoryBatch,
  endBatch: endCanvasHistoryBatch,
  resetHistoryBaseline: syncCanvasHistoryBaseline,
  undo: undoCanvasSnapshot,
  redo: redoCanvasSnapshot,
} = useCanvasHistory({
  createSnapshot: createBoardSnapshot,
  applySnapshot: applyBoardSnapshot,
})

resetHistoryBaseline = syncCanvasHistoryBaseline

const {
  currentBoardId,
  currentBoardName,
  boardOptions,
  isBoardMenuOpen,
  isRenamingBoard,
  boardNameDraft,
  boardNameInputRef,
  isBoardDirty,
  leaveConfirmVisible,
  toggleBoardMenu,
  startBoardRename,
  cancelBoardRename,
  confirmBoardRename,
  handleBoardSelect,
  resolveLeaveConfirm,
  confirmSaveBeforeLeave,
  markBoardDirty,
} = useCanvasBoard({
  defaultBoardName: defaultCanvasName,
  message,
  createSnapshot: createBoardSnapshot,
  applySnapshot: applyBoardSnapshot,
})

watch(
  [messages, imageNodes, promptNodes, modelNodes, maskNodes, colorNodes, canvasX, canvasY, zoomLevel],
  () => {
    markBoardDirty()
    notifySnapshotChanged()
  },
  { deep: true }
)

function deletePromptNode(id: number) {
  promptNodes.value = promptNodes.value.filter(n => n.id !== id)
  removeSelectedNodeKey('prompt', id)
  if (activePromptNodeId.value === id) {
    activePromptNodeId.value = null
  }
}

function deleteModelNode(id: number) {
  modelNodes.value = modelNodes.value.filter(n => n.id !== id)
  removeSelectedNodeKey('model', id)
  if (activeModelNodeId.value === id) {
    activeModelNodeId.value = null
  }
}

function deleteMaskNode(id: number) {
  maskNodes.value = maskNodes.value.filter(n => n.id !== id)
  removeMaskRuntimeFor(id)
  removeSelectedNodeKey('mask', id)
  if (activeMaskNodeId.value === id) {
    activeMaskNodeId.value = null
  }
}

function deleteImageNode(id: number) {
  imageNodes.value = imageNodes.value.filter(n => n.id !== id)
  removeSelectedNodeKey('image', id)
  if (activeImageNodeId.value === id) {
    activeImageNodeId.value = null
  }
}

function deleteColorNode(id: number) {
  colorNodes.value = colorNodes.value.filter(n => n.id !== id)
  removeSelectedNodeKey('color', id)
  if (activeColorPopoverNodeId.value === id) {
    activeColorPopoverNodeId.value = null
  }
  if (activeColorNodeId.value === id) {
    activeColorNodeId.value = null
  }
}

function createImageNode() {
  const { clientX, clientY } = nodeContextMenu.value
  const point = getCanvasPointFromClient(clientX, clientY)
  const newNode = createImageNodeAt(point)
  imageNodes.value.push(newNode)
  activateImageNode(newNode.id)
  closeNodeContextMenu()
}

function createPromptNode() {
  const { clientX, clientY } = nodeContextMenu.value
  const point = getCanvasPointFromClient(clientX, clientY)
  const newNode = createPromptNodeAt(point)
  promptNodes.value.push(newNode)
  activatePromptNode(newNode.id)
  closeNodeContextMenu()
}

function createModelNode() {
  const { clientX, clientY } = nodeContextMenu.value
  const point = getCanvasPointFromClient(clientX, clientY)
  const newNode = createModelNodeAt(point)
  modelNodes.value.push(newNode)
  activateModelNode(newNode.id)
  closeNodeContextMenu()
}

function createMaskNode() {
  const { clientX, clientY } = nodeContextMenu.value
  const point = getCanvasPointFromClient(clientX, clientY)
  const newNode = createMaskNodeAt(point)
  maskNodes.value.push(newNode)
  activateMaskNode(newNode.id)
  closeNodeContextMenu()
}

function createColorNode() {
  const { clientX, clientY } = nodeContextMenu.value
  const point = getCanvasPointFromClient(clientX, clientY)
  const newNode = createColorNodeAt(point)
  colorNodes.value.push(newNode)
  activateColorNode(newNode.id)
  closeNodeContextMenu()
}

function setColorSpectrumElement(el: HTMLElement | null) {
  colorSpectrumRef.value = el
}

function onModelStageDrop(e: DragEvent, node: ModelNodeItem) {
  isDraggingFile.value = false
  handleModelStageDrop(e, node)
}

function onMaskStageDrop(e: DragEvent, node: MaskNodeItem) {
  isDraggingFile.value = false
  handleMaskStageDrop(e, node)
}

function onImageNodeStageDrop(e: DragEvent, node: ImageNodeItem) {
  isDraggingFile.value = false
  handleImageNodeStageDrop(e, node)
}

async function handleDrop(e: DragEvent) {
  isDraggingFile.value = false
  await handleCanvasDropCore(e)
}

function clearStyleStrengthLongPressTimer() {
  if (!styleStrengthLongPressTimer) return
  clearTimeout(styleStrengthLongPressTimer)
  styleStrengthLongPressTimer = null
}

function clearStyleStrengthHideTimer() {
  if (!styleStrengthHideTimer) return
  clearTimeout(styleStrengthHideTimer)
  styleStrengthHideTimer = null
}

function removeStyleStrengthGlobalListeners() {
  window.removeEventListener('mousemove', onStyleInnovateMouseMove, true)
  window.removeEventListener('mouseup', onStyleInnovateMouseUp, true)
}

function addStyleStrengthGlobalListeners() {
  window.addEventListener('mousemove', onStyleInnovateMouseMove, true)
  window.addEventListener('mouseup', onStyleInnovateMouseUp, true)
}

function getStyleStrengthTrackEl(): HTMLElement | null {
  if (Array.isArray(styleStrengthTrackRef.value)) {
    return styleStrengthTrackRef.value[0] ?? null
  }
  return styleStrengthTrackRef.value
}

function setCanvasImageRef(id: number, el: Element | ComponentPublicInstance | null) {
  const imageEl =
    el instanceof HTMLImageElement
      ? el
      : el && '$el' in el && (el as ComponentPublicInstance).$el instanceof HTMLImageElement
        ? (el as ComponentPublicInstance).$el
        : null

  if (imageEl) {
    canvasImageRefs.set(id, imageEl)
    return
  }
  canvasImageRefs.delete(id)
}

function getStyleStrengthMenuStyle(item: CanvasImageItem): { top: string; left: string } {
  const imageEl = canvasImageRefs.get(item.id)
  if (!imageEl) {
    return { top: '8px', left: `calc(100% + ${styleStrengthMenuGap}px)` }
  }
  const top = Math.max(0, imageEl.offsetTop)
  const left = Math.max(0, imageEl.offsetLeft + imageEl.clientWidth + styleStrengthMenuGap)
  return {
    top: `${top}px`,
    left: `${left}px`,
  }
}

function getActiveCanvasImageItem(): CanvasImageItem | null {
  const id = activeImageActionId.value ?? selectedIds.value[0] ?? null
  if (id === null) return null
  return messages.value.find(item => item.id === id) ?? null
}

function updateStyleStrengthFromClientY(clientY: number) {
  const track = getStyleStrengthTrackEl()
  if (!track) return
  const rect = track.getBoundingClientRect()
  if (rect.height <= 0) return
  const ratio = 1 - (clientY - rect.top) / rect.height
  const clamped = Math.max(0, Math.min(1, ratio))
  styleInnovateStrength.value = Math.max(1, Math.min(10, Math.round(clamped * 9 + 1)))
}

function beginStyleStrengthDragging(clientY: number) {
  showStyleStrengthControl.value = true
  styleStrengthDragging = true
  styleStrengthPointerY = clientY
  addStyleStrengthGlobalListeners()
  void nextTick(() => updateStyleStrengthFromClientY(styleStrengthPointerY))
}

function onStyleInnovateMouseDown(e: MouseEvent) {
  if (generating.value) return
  clearStyleStrengthHideTimer()
  clearStyleStrengthLongPressTimer()
  suppressStyleInnovateClick.value = false
  styleStrengthDragging = false
  styleStrengthPointerY = e.clientY

  styleStrengthLongPressTimer = window.setTimeout(() => {
    suppressStyleInnovateClick.value = true
    beginStyleStrengthDragging(styleStrengthPointerY)
  }, 260)

  addStyleStrengthGlobalListeners()
}

function onStyleInnovateMouseMove(e: MouseEvent) {
  styleStrengthPointerY = e.clientY
  if (!styleStrengthDragging || !showStyleStrengthControl.value) return
  updateStyleStrengthFromClientY(e.clientY)
}

function onStyleInnovateMouseUp() {
  clearStyleStrengthLongPressTimer()
  removeStyleStrengthGlobalListeners()
  if (!styleStrengthDragging) return
  styleStrengthDragging = false
  clearStyleStrengthHideTimer()
  showStyleStrengthControl.value = false

  const target = getActiveCanvasImageItem()
  if (!target) return
  void runCanvasAction('style-innovate', target)
}

function onStyleStrengthTrackMouseDown(e: MouseEvent) {
  clearStyleStrengthHideTimer()
  beginStyleStrengthDragging(e.clientY)
}

function onStyleInnovateClick(item: CanvasImageItem) {
  if (suppressStyleInnovateClick.value) {
    suppressStyleInnovateClick.value = false
    return
  }
  void runCanvasAction('style-innovate', item)
}

const selectedPreviewItems = computed(() => {
  const messageMap = new Map(messages.value.map(item => [item.id, item]))
  return selectedIds.value
    .map(id => messageMap.get(id))
    .filter((item): item is any => !!item && item.type === 'image' && !!item.url)
})
const visiblePreviewItems = computed(() => selectedPreviewItems.value.slice(0, 6))
const previewOverflowCount = computed(() => Math.max(0, selectedPreviewItems.value.length - visiblePreviewItems.value.length))
const promptPlaceholder = computed(() =>
  selectedIds.value.length === 1
    ? 'Type an idea for the selected image, or right-click image for quick tools'
    : 'What do you want to create?'
)
const styleCards = ref<StyleCard[]>([])
const selectedCardIds = ref<number[]>([])
const selectedRatio = ref('3:4')
const selectedResolution = ref('1K')

function toggleCard(id: number) {
  const idx = selectedCardIds.value.indexOf(id)
  if (idx >= 0) selectedCardIds.value.splice(idx, 1)
  else selectedCardIds.value.push(id)
}

async function runCanvasAction(action: CanvasActionKey, item: CanvasImageItem) {
  if (generating.value) return
  selectedIds.value = [item.id]
  activeImageActionId.value = null
  showStyleStrengthControl.value = false
  if (action === 'remove-bg') {
    await handleRemoveBgGenerate(item)
    return
  }
  if (action === 'style-innovate') {
    await handleStyleInnovateGenerate(item)
    return
  }
  if (action === 'to-vector') {
    await handleToVectorGenerate(item)
    return
  }
  await handleSketchToDesignGenerate(item)
}

async function handleGenerate() {
  if (generating.value) return
  if (!userPrompt.value.trim()) return
  await handlePictureGenerate()
}

async function handlePictureGenerate() {
  generating.value = true
  await submitTaskAndPoll({
    request: { 
      prompt: userPrompt.value.trim(), 
      card_ids: selectedCardIds.value, 
      params: { 
        ratio: selectedRatio.value,
        resolution: selectedResolution.value
      } 
    },
    successMessage: '任务已提交',
    failureMessage: '提交失败',
    onSubmitted: () => {
      userPrompt.value = ''
      showSettings.value = false
    },
  })
}

async function handleRemoveBgGenerate(source: CanvasImageItem) {
  generating.value = true
  const startTime = Date.now()
  try {
    const segmentRes: any = await segApi.segment({
      image_url: source.url,
      seg_type: defaultSegType,
    })
    if (segmentRes.code !== 0 || !segmentRes.data?.result_url) {
      message.error(segmentRes.msg || 'Segmentation failed')
      return
    }

    const saveRes: any = await segApi.save({
      result_url: segmentRes.data.result_url,
      seg_type: defaultSegType,
      feature: 'remove-bg',
    })
    if (saveRes.code !== 0 || !saveRes.data?.image_url) {
      message.error(saveRes.msg || 'Save result failed')
      return
    }

    appendGeneratedImages([saveRes.data.image_url], source)
    message.success(`Background removed in ${((Date.now() - startTime) / 1000).toFixed(1)}s`)
  } catch (error: any) {
    message.error(error?.msg || error?.message || 'Failed to remove background')
  } finally {
    generating.value = false
  }
}

async function handleSketchToDesignGenerate(source: CanvasImageItem) {
  generating.value = true
  const desc = userPrompt.value.trim() || 'Sketch to design'
  await submitTaskAndPoll({
    request: {
      prompt: `[sketch-to-design] ${desc}`,
      params: {
        image_urls: [source.url],
        feature: 'sketch-to-design',
      },
    },
    source,
    successMessage: 'Task submitted',
    failureMessage: 'Failed to submit task',
    onSubmitted: () => {
      userPrompt.value = ''
    },
  })
}

async function handleStyleInnovateGenerate(source: CanvasImageItem) {
  generating.value = true
  const desc = userPrompt.value.trim() || 'Style innovate'
  const strength = styleInnovateStrength.value
  await submitTaskAndPoll({
    request: {
      prompt: `[style-innovate] 改变幅度：${strength}/10，${desc}`,
      params: {
        image_urls: [source.url],
        feature: 'style-innovate',
        variation_strength: strength,
      },
    },
    source,
    successMessage: 'Task submitted',
    failureMessage: 'Failed to submit task',
    onSubmitted: () => {
      userPrompt.value = ''
    },
  })
}

async function handleToVectorGenerate(source: CanvasImageItem) {
  generating.value = true
  const startTime = Date.now()
  try {
    const res: any = await vectorizeApi.vectorize({
      image_url: source.url,
      threshold: 0.45,
      turd_size: 2,
      alpha_max: 1.0,
      opt_tolerance: 0.2,
      turn_policy: 'minority',
      invert: false,
      color_count: 0,
    })

    if (res.code !== 0 || !res.data?.svg_url) {
      message.error(res.msg || 'Vectorize failed')
      return
    }

    appendGeneratedImages([res.data.svg_url], source)
    message.success(`Vectorized in ${((Date.now() - startTime) / 1000).toFixed(1)}s`)
  } catch (error: any) {
    message.error(error?.msg || error?.message || 'Failed to vectorize image')
  } finally {
    generating.value = false
  }
}

async function handleModelNodeGenerate(node: ModelNodeItem) {
  if (generating.value) return
  const sourceUrl = node.sourceImageUrl
  if (!sourceUrl) {
    message.warning('请先上传模特图')
    return
  }

  const angles = [...node.selectedAngles]
  if (angles.length === 0) {
    message.warning('请至少选择一个角度')
    return
  }

  generating.value = true
  const angleLabel = angles.map(getModelAngleLabel).join('、')
  const promptText = userPrompt.value.trim() || `多角度：${angleLabel}`
  const outputNodeId = resolvePreferredImageOutputNode()?.id ?? null
  const placementSource: CanvasImageItem = {
    id: -Date.now(),
    type: 'image',
    url: sourceUrl,
    x: node.x + 340,
    y: node.y + 18,
    width: 300,
  }

  await submitTaskAndPoll({
    request: {
      prompt: `[multi-angle] ${promptText}`,
      params: {
        image_urls: [sourceUrl],
        feature: 'multi-angle',
        angles: angles.join(','),
        pose: modelDefaultPose,
        expression: modelDefaultExpression,
        background: node.background,
      },
    },
    source: placementSource,
    outputNodeId,
    successMessage: `Task submitted · ${angleLabel}`,
    failureMessage: 'Failed to submit task',
  })
}

async function handleColorNodeGenerate(node: ColorNodeItem) {
  if (generating.value) return

  const targetColor = normalizeHexColor(node.targetColor)
  const strength = Math.max(0, Math.min(100, Math.round(node.strength ?? 70)))
  node.targetColor = targetColor
  node.strength = strength
  let originalImageUrl = ''
  let maskDataUrl = ''
  let placementX = node.x + 340
  let placementY = node.y + 18
  const selectedCanvasImage =
    selectedIds.value.length === 1
      ? messages.value.find(item => item.id === selectedIds.value[0])
      : null
  const activeMaskNode =
    activeMaskNodeId.value !== null
      ? maskNodes.value.find(item => item.id === activeMaskNodeId.value)
      : null
  const latestMaskNode = [...maskNodes.value]
    .reverse()
    .find(item => !!item.sourceImageUrl && !!item.maskDataUrl)
  const activeModelNode =
    activeModelNodeId.value !== null
      ? modelNodes.value.find(item => item.id === activeModelNodeId.value)
      : null
  const latestModelNode = [...modelNodes.value]
    .reverse()
    .find(item => !!item.sourceImageUrl)
  const activeImageNode =
    activeImageNodeId.value !== null
      ? imageNodes.value.find(item => item.id === activeImageNodeId.value)
      : null
  const latestImageNode = [...imageNodes.value]
    .reverse()
    .find(item => !!item.sourceImageUrl || item.outputImages.length > 0)
  const latestCanvasImage = [...messages.value].reverse()[0] ?? null

  if (selectedCanvasImage) {
    originalImageUrl = selectedCanvasImage.url
    placementX = selectedCanvasImage.x - 340
    placementY = selectedCanvasImage.y + 18
    const autoMask = await buildFullMaskDataUrl(originalImageUrl)
    if (!autoMask) {
      message.error('无法生成全图蒙版，请重试')
      return
    }
    maskDataUrl = autoMask
  } else {
    const pickedMaskNode =
      activeMaskNode && activeMaskNode.sourceImageUrl && activeMaskNode.maskDataUrl
        ? activeMaskNode
        : latestMaskNode
    if (pickedMaskNode) {
      originalImageUrl = pickedMaskNode.sourceImageUrl
      maskDataUrl = await refineMaskDataUrl(
        pickedMaskNode.maskDataUrl,
        pickedMaskNode.refineAmount ?? 0,
        !!pickedMaskNode.invertMask,
      )
      placementX = pickedMaskNode.x + 340
      placementY = pickedMaskNode.y + 18
    } else {
      const pickedImageNode = getImageNodePreferredImage(activeImageNode ?? latestImageNode ?? null)
      const pickedModelNode =
        activeModelNode && activeModelNode.sourceImageUrl
          ? activeModelNode
          : latestModelNode
      const pickedCanvasImage = latestCanvasImage

      if (pickedImageNode) {
        originalImageUrl = pickedImageNode.url
        placementX = (activeImageNode ?? latestImageNode)?.x ?? node.x
        placementY = ((activeImageNode ?? latestImageNode)?.y ?? node.y) + 18
      } else if (pickedModelNode) {
        originalImageUrl = pickedModelNode.sourceImageUrl
        placementX = pickedModelNode.x - 340
        placementY = pickedModelNode.y + 18
      } else if (pickedCanvasImage) {
        originalImageUrl = pickedCanvasImage.url
        placementX = pickedCanvasImage.x - 340
        placementY = pickedCanvasImage.y + 18
      } else {
        message.warning('暂无可用输入，请先准备图片或 Mask 节点')
        return
      }

      const autoMask = await buildFullMaskDataUrl(originalImageUrl)
      if (!autoMask) {
        message.error('无法生成全图蒙版，请重试')
        return
      }
      maskDataUrl = autoMask
    }
  }

  if (!maskDataUrl) {
    message.warning('请先准备可用蒙版')
    return
  }

  generating.value = true
  const outputNodeId = resolvePreferredImageOutputNode()?.id ?? null
  try {
    const maskFile = await dataUrlToFile(maskDataUrl, `mask-${Date.now()}.png`)
    const uploadRes: any = await uploadApi.upload(maskFile)
    if (uploadRes.code !== 0 || !uploadRes.data?.url) {
      message.error(uploadRes.msg || '蒙版上传失败')
      generating.value = false
      return
    }

    const placementSource: CanvasImageItem = {
      id: -Date.now(),
      type: 'image',
      url: originalImageUrl,
      x: placementX,
      y: placementY,
      width: 300,
    }

    await submitTaskAndPoll({
      request: {
      prompt: `[directed-recolor] 目标颜色：${targetColor}，强度：${strength}`,
      params: {
        image_urls: [uploadRes.data.url],
        original_image_url: originalImageUrl,
        feature: 'directed-recolor',
        target_color: targetColor,
        strength,
      },
      },
      source: placementSource,
      outputNodeId,
      successMessage: `Task submitted · ${targetColor} · ${strength}%`,
      failureMessage: 'Failed to submit task',
    })
  } catch (error: any) {
    message.error(error?.msg || error?.message || 'Failed to submit task')
    generating.value = false
  }
}

function toSafeFileName(rawName: string): string {
  const decoded = rawName ? decodeURIComponent(rawName) : ''
  const cleaned = decoded.replace(/[\\/:*?"<>|]+/g, '_').trim()
  if (!cleaned) return `export-${Date.now()}.png`
  if (/\.[a-z0-9]+$/i.test(cleaned)) return cleaned
  return `${cleaned}.png`
}

function buildExportFileName(url: string, index: number, total: number, timestamp: number): string {
  const fromUrl = String(url).split('/').pop()?.split('?')[0] || ''
  const safeName = toSafeFileName(fromUrl)
  if (total > 1) {
    return `${String(index + 1).padStart(2, '0')}-${timestamp}-${safeName}`
  }
  return safeName
}

async function handleExport() {
  if (selectedIds.value.length === 0) return

  const exportableItems = messages.value.filter(
    m => selectedIds.value.includes(m.id) && m.type === 'image' && !!m.url
  )

  if (exportableItems.length === 0) {
    message.warning('No exportable images selected')
    return
  }

  const timestamp = Date.now()
  let failed = 0

  if (exportLoading) {
    exportLoading.destroy()
    exportLoading = null
  }

  const loading = message.loading(`Exporting ${exportableItems.length} images...`, {
    duration: 1500,
    keepAliveOnHover: false
  })
  exportLoading = loading

  try {
    for (let i = 0; i < exportableItems.length; i++) {
      const item = exportableItems[i]
      const fileName = buildExportFileName(item.url, i, exportableItems.length, timestamp)

      try {
        const link = document.createElement('a')
        link.href = item.url
        link.download = fileName
        link.rel = 'noopener'
        link.style.display = 'none'
        document.body.appendChild(link)
        link.click()
        document.body.removeChild(link)
      } catch (error) {
        failed += 1
        console.error('Export failed:', error)
      }
    }
  } catch (err) {
    failed += 1
    console.error('Export failed:', err)
  } finally {
    loading.destroy()
    if (exportLoading === loading) exportLoading = null
  }

  if (failed === 0) {
    message.success(`Export queued: ${exportableItems.length} image${exportableItems.length > 1 ? 's' : ''}`)
  } else {
    const success = exportableItems.length - failed
    message.warning(`Export queued ${success}/${exportableItems.length}, failed ${failed}`)
  }
}

function onImageLoad(msg: any, event: Event) {
  const target = event.target as HTMLImageElement | null
  if (!target || !target.naturalWidth || !target.naturalHeight) return
  msg.height_ratio = target.naturalHeight / target.naturalWidth
}

function getImageDisplayHeight(item: CanvasImageItem): number {
  const imageEl = canvasImageRefs.get(item.id)
  if (imageEl && imageEl.clientHeight > 0) {
    return Math.round(imageEl.clientHeight)
  }
  const width = item.width || 300
  const ratio = item.height_ratio || 1
  return Math.max(72, Math.round(width * ratio))
}

onMounted(async () => {
  try {
    const res: any = await styleCardApi.getEnabled()
    if (res.code === 0) styleCards.value = res.data.list
  } catch (e) {}
})

onUnmounted(() => {
  stopTaskPolling()
  clearStyleStrengthLongPressTimer()
  clearStyleStrengthHideTimer()
  removeStyleStrengthGlobalListeners()
})
</script>

<style scoped>
.canvas-layout { width: 100%; height: 100%; background: #ffffff; position: relative; overflow: hidden; }
.canvas-layout.copy-mode .canvas-item {
  cursor: copy;
}
.canvas-layout.copy-mode :deep(.workflow-node.copy-mode .workflow-node-header) {
  cursor: copy !important;
}
.canvas-grid { position: absolute; inset: 0; background-image: radial-gradient(circle, #e2e4e8 calc(1.5px * var(--zoom, 1)), transparent calc(1.5px * var(--zoom, 1))); background-size: calc(24px * var(--zoom, 1)) calc(24px * var(--zoom, 1)); background-position: calc(50% + var(--offset-x, 0px)) calc(50% + var(--offset-y, 0px)); z-index: 0; opacity: var(--grid-opacity, 1); }
.canvas-grid.grid-dragging { background: rgba(146, 120, 230, 0.05); }

.floating-header { position: absolute; top: 16px; left: 16px; right: 16px; display: flex; justify-content: space-between; z-index: 100; pointer-events: none; }
.header-left, .header-right { display: flex; gap: 12px; pointer-events: auto; }
.file-name-chip { 
  background: #f1f3f9; 
  padding: 8px 16px; 
  border-radius: 12px; 
  font-size: 14px; 
  font-weight: 500; 
  color: #202124;
  cursor: pointer; 
  display: flex; 
  gap: 8px; 
  align-items: center; 
  transition: background 0.2s;
  position: relative;
}
.file-name-chip:hover { background: #e8eaed; }
.file-name-btn {
  border: none;
  background: transparent;
  padding: 0;
  margin: 0;
  color: #202124;
  font-size: 14px;
  font-weight: 500;
  cursor: pointer;
  line-height: 1.2;
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.file-name-btn:hover {
  color: #6757c8;
}
.board-name-input {
  height: 24px;
  min-width: 140px;
  max-width: 220px;
  border-radius: 6px;
  border: 1px solid #cfd5e3;
  padding: 0 8px;
  font-size: 14px;
  outline: none;
  color: #202124;
  background: #fff;
}
.board-name-input:focus {
  border-color: #9278e6;
}
.board-switch-btn {
  width: 20px;
  height: 20px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: #5f6368;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: background-color 0.2s ease, color 0.2s ease;
}
.board-switch-btn:hover {
  background: #fff;
  color: #6757c8;
}
.chevron-down {
  transition: transform 0.2s ease;
}
.chevron-down.open {
  transform: rotate(180deg);
}
.board-switch-menu {
  position: absolute;
  top: calc(100% + 8px);
  left: 0;
  min-width: 220px;
  max-width: 280px;
  max-height: 280px;
  overflow-y: auto;
  padding: 6px;
  border-radius: 10px;
  border: 1px solid #d8ddea;
  background: rgba(255, 255, 255, 0.96);
  backdrop-filter: blur(10px);
  box-shadow: 0 10px 30px rgba(25, 30, 45, 0.12);
  z-index: 140;
}
.board-switch-item {
  width: 100%;
  height: 34px;
  border: none;
  border-radius: 8px;
  background: transparent;
  text-align: left;
  padding: 0 10px;
  font-size: 13px;
  font-weight: 600;
  color: #4b5565;
  cursor: pointer;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.board-switch-item:hover {
  background: rgba(146, 120, 230, 0.12);
  color: #6757c8;
}
.board-switch-item.active {
  background: rgba(146, 120, 230, 0.16);
  color: #6757c8;
}
.board-switch-empty {
  height: 34px;
  display: flex;
  align-items: center;
  padding: 0 10px;
  font-size: 12px;
  color: #8a91a3;
}
.btn-ghost, .btn-primary { height: 40px; padding: 0 16px; border-radius: 10px; font-size: 14px; border: none; cursor: pointer; }
.btn-ghost { background: #f1f3f9; }
.btn-ghost:hover { background: #f1f3f9; }
.share-btn { background: #f1f3f9 !important; }
.share-btn:hover { background: #f1f3f9 !important; transform: none; }
.btn-primary { background: #9278e6; color: white; }
.btn-primary:disabled { background: #e2e4e8; color: #a8adb7; cursor: not-allowed; }

.selection-marquee {
  position: absolute;
  background: rgba(146, 120, 230, 0.1);
  border: 1.5px solid #9278e6;
  border-radius: 4px;
  z-index: 1000;
  pointer-events: none;
}

.leave-confirm-mask {
  position: absolute;
  inset: 0;
  z-index: 220;
  background: rgba(17, 24, 39, 0.24);
  display: flex;
  align-items: center;
  justify-content: center;
}

.leave-confirm-card {
  width: min(420px, calc(100% - 28px));
  border-radius: 12px;
  border: 1px solid #d9deea;
  background: #fff;
  box-shadow: 0 12px 28px rgba(15, 23, 42, 0.16);
  padding: 16px;
}

.leave-confirm-title {
  font-size: 15px;
  line-height: 1.3;
  font-weight: 600;
  color: #2a3142;
}

.leave-confirm-desc {
  margin-top: 6px;
  font-size: 13px;
  line-height: 1.45;
  color: #6b7488;
}

.leave-confirm-actions {
  margin-top: 14px;
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.leave-btn {
  height: 34px;
  border-radius: 8px;
  border: 1px solid #d5dbe8;
  background: #fff;
  color: #4f576a;
  font-size: 13px;
  font-weight: 500;
  padding: 0 12px;
  cursor: pointer;
  transition: border-color 0.15s ease, background-color 0.15s ease, color 0.15s ease;
}

.leave-btn:hover {
  border-color: #c2cae0;
}

.leave-btn.primary {
  border-color: #9278e6;
  background: #9278e6;
  color: #fff;
}

.leave-btn.primary:hover {
  background: #8166df;
  border-color: #8166df;
}

.leave-btn.danger {
  border-color: #e7ced4;
  color: #bf5a74;
  background: #fff;
}

.leave-btn.danger:hover {
  border-color: #ddb3bf;
  background: #fff6f8;
}

.node-context-menu {
  position: absolute;
  z-index: 160;
  width: 160px;
  min-width: 160px;
  max-width: 160px;
  padding: 6px;
  border-radius: 10px;
  border: 1px solid #d8ddea;
  background: rgba(255, 255, 255, 0.96);
  backdrop-filter: blur(10px);
  box-shadow: 0 10px 30px rgba(25, 30, 45, 0.12);
}

.node-context-item {
  width: 100%;
  height: 34px;
  border: none;
  border-radius: 8px;
  background: transparent;
  text-align: left;
  padding: 0 10px;
  font-size: 13px;
  font-weight: 600;
  color: #4b5565;
  cursor: pointer;
}

.node-context-item:hover {
  background: rgba(146, 120, 230, 0.12);
  color: #6757c8;
}

.user-avatar-wrap {
  width: 40px;
  height: 40px;
  border-radius: 50%;
  overflow: hidden;
  cursor: pointer;
  border: 1px solid #e2e4e8;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #f1f3f9;
  transition: all 0.2s;
  flex-shrink: 0;
  color: #5f6368;
}

.user-avatar-wrap:hover {
  border-color: #9278e6;
  background: #fff;
  color: #9278e6;
  transform: scale(1.05);
}

.user-avatar {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.user-avatar-placeholder {
  display: flex;
  align-items: center;
  justify-content: center;
}

.floating-toolbar { position: absolute; left: 16px; top: 50%; transform: translateY(-50%); background: #fff; padding: 8px; border-radius: 16px; display: flex; flex-direction: column; gap: 8px; border: 1px solid #e2e4e8; z-index: 100; }
.tool-item { width: 40px; height: 40px; border-radius: 10px; display: flex; align-items: center; justify-content: center; color: #5f6368; cursor: pointer; }
.tool-item.active { background: #f1f3f9; color: #9278e6; }
.toolbar-divider { width: 24px; height: 1px; background: #f1f3f9; margin: 4px auto; }

.floating-bottom-bar { position: absolute; bottom: 32px; left: 50%; transform: translateX(-50%); width: 100%; max-width: 600px; z-index: 100; }
.settings-panel {
  background: rgba(255, 255, 255, 0.9);
  backdrop-filter: blur(20px);
  border: 1px solid rgba(226, 228, 232, 0.8);
  border-radius: 24px;
  padding: 20px;
  margin-bottom: 16px;
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.08);
  max-height: 480px;
  overflow-y: auto;
}
.settings-section { margin-bottom: 20px; }
.settings-section:last-child { margin-bottom: 0; }
.section-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 12px; }
.section-title { font-size: 14px; font-weight: 600; color: #202124; }
.section-subtitle { font-size: 12px; color: #5f6368; }

.ratio-group { display: flex; gap: 8px; }
.ratio-btn {
  padding: 6px 12px;
  border-radius: 8px;
  border: 1px solid #e2e4e8;
  background: #fff;
  font-size: 12px;
  cursor: pointer;
  transition: all 0.2s;
  color: #5f6368;
}
.ratio-btn:hover { border-color: #9278e6; color: #9278e6; }
.ratio-btn.active { background: #9278e6; color: white; border-color: #9278e6; }

.style-cards-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(100px, 1fr));
  gap: 12px;
}
.style-card-item {
  position: relative;
  border-radius: 12px;
  overflow: hidden;
  cursor: pointer;
  border: 2px solid transparent;
  transition: all 0.2s;
  background: #f1f3f9;
}
.style-card-item.active { border-color: #9278e6; }
.card-preview { width: 100%; aspect-ratio: 1; object-fit: cover; display: block; }
.card-info {
  padding: 20px 8px 6px;
  background: linear-gradient(to top, rgba(0,0,0,0.7), transparent);
  position: absolute;
  bottom: 0;
  left: 0;
  right: 0;
}
.card-name { color: white; font-size: 11px; font-weight: 500; display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.card-check {
  position: absolute;
  top: 6px;
  right: 6px;
  width: 18px;
  height: 18px;
  background: #9278e6;
  color: white;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  opacity: 0;
  transform: scale(0.8);
  transition: all 0.2s;
}
.style-card-item.active .card-check { opacity: 1; transform: scale(1); }

.slide-up-enter-active, .slide-up-leave-active { transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1); }
.slide-up-enter-from, .slide-up-leave-to { opacity: 0; transform: translateY(20px); }

.selected-preview-bar {
  margin-bottom: 10px;
  border-radius: 16px;
  border: 1px solid rgba(226, 228, 232, 0.95);
  background: rgba(255, 255, 255, 0.94);
  backdrop-filter: blur(10px);
  padding: 10px 12px;
}

.selected-preview-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
}

.selected-preview-count {
  font-size: 12px;
  font-weight: 600;
  color: #5f6368;
}

.clear-selected-btn {
  border: none;
  background: transparent;
  font-size: 12px;
  font-weight: 600;
  color: #9278e6;
  cursor: pointer;
  padding: 2px 4px;
}

.selected-preview-list {
  display: flex;
  align-items: center;
  gap: 8px;
  overflow-x: auto;
  padding-bottom: 2px;
}

.selected-preview-list::-webkit-scrollbar {
  height: 4px;
}

.selected-preview-list::-webkit-scrollbar-thumb {
  background: #d7dbe3;
  border-radius: 999px;
}

.selected-preview-item {
  position: relative;
  flex: 0 0 38px;
  width: 38px;
  height: 38px;
  border-radius: 10px;
  border: 2px solid #9278e6;
  padding: 0;
  background: #fff;
  overflow: hidden;
  cursor: pointer;
}

.selected-preview-image {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}

.selected-preview-remove {
  position: absolute;
  top: 0;
  right: 0;
  width: 14px;
  height: 14px;
  border-radius: 50%;
  background: rgba(0, 0, 0, 0.68);
  color: #fff;
  font-size: 11px;
  line-height: 14px;
  text-align: center;
}

.selected-preview-overflow {
  flex: 0 0 38px;
  width: 38px;
  height: 38px;
  border-radius: 10px;
  border: 1px dashed #cfd5e3;
  color: #6f7787;
  font-size: 12px;
  font-weight: 600;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #f7f8fb;
}

.prompt-container { background: #fff; height: 56px; border-radius: 28px; padding: 0 8px; display: flex; align-items: center; gap: 12px; border: 1px solid #e2e4e8; }
.prompt-container input { flex: 1; border: none; outline: none; font-size: 16px; }
.add-btn, .send-btn { width: 40px; height: 40px; border-radius: 50%; border: none; cursor: pointer; display: flex; align-items: center; justify-content: center; }
.send-btn { background: #f1f3f9; color: #9278e6; }

.floating-status { position: absolute; bottom: 24px; right: 24px; display: flex; gap: 16px; z-index: 100; }
.zoom-controls { background: #f1f3f9; padding: 4px; border-radius: 12px; display: flex; align-items: center; gap: 8px; }
.zoom-btn { 
  width: 28px; 
  height: 28px; 
  border: none; 
  background: transparent; 
  cursor: pointer; 
  color: #5f6368;
  font-size: 18px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  transition: all 0.2s;
}
.zoom-btn:hover { background: #fff; color: #202124; }
.zoom-level { 
  font-size: 13px; 
  font-weight: 600;
  color: #5f6368;
  min-width: 40px; 
  text-align: center; 
}
.logo-btn { 
  width: 40px; 
  height: 40px; 
  border-radius: 50%; 
  background: #fff; 
  border: 1px solid #e2e4e8; 
  cursor: pointer; 
  display: flex; 
  align-items: center; 
  justify-content: center; 
  transition: all 0.3s ease;
  color: #5f6368;
}

.logo-btn:hover {
  background: #f8fafc;
  border-color: #9278e6;
  color: #9278e6;
}

.logo-btn:active {
  transform: scale(0.9);
}

.logo-btn svg {
  transition: transform 0.6s cubic-bezier(0.34, 1.56, 0.64, 1);
}

.logo-btn:hover svg {
  transform: rotate(360deg);
}

.canvas-content { position: absolute; inset: 0; z-index: 1; transform: translate(var(--offset-x, 0px), var(--offset-y, 0px)) scale(var(--zoom, 1)); transform-origin: center center; }
.workflow-node {
  position: absolute;
  z-index: 12;
  border: 1.5px solid #e2e8f0;
  border-radius: 14px;
  background: #ffffff;
  overflow: hidden;
  transition: border-color 0.2s ease;
  will-change: transform;
}

.workflow-node.active {
  border-color: #9278e6;
}

.workflow-node.dragging {
  transition: none;
}

.canvas-item {
  position: absolute;
  background: #fff;
  padding: 8px;
  border-radius: 12px;
  border: 1.5px solid #e2e4e8;
  user-select: none;
  -webkit-user-select: none;
}
.canvas-item.active { border-color: #9278e6; z-index: 10; }
.canvas-img {
  width: 100%;
  display: block;
  border-radius: 8px;
  pointer-events: none;
  user-select: none;
  -webkit-user-select: none;
  -webkit-user-drag: none;
}
.image-action-menu {
  position: absolute;
  top: 0;
  left: calc(100% + 12px);
  padding: 0;
  border-radius: 0;
  background: transparent;
  border: none;
  backdrop-filter: none;
  box-shadow: none;
  z-index: 14;
  display: flex;
  flex-direction: column;
  gap: 5px;
}
.image-action-menu.strength-mode {
  top: 8px;
  gap: 0;
}
.image-action-icon-btn {
  width: 34px;
  height: 34px;
  border: 1px solid rgba(122, 107, 213, 0.28);
  background: rgba(255, 255, 255, 0.92);
  border-radius: 10px;
  padding: 0;
  position: relative;
  overflow: visible;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: transform 0.18s ease, color 0.18s ease, opacity 0.18s ease, border-color 0.18s ease, background 0.18s ease;
  color: #6f61c6;
}
.image-action-icon-btn:hover:not(:disabled) {
  border-color: #7a6bd5;
  background: rgba(122, 107, 213, 0.12);
  color: #7a6bd5;
  transform: scale(1.06);
}
.image-action-icon-btn:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}
.action-logo {
  display: flex;
  align-items: center;
  justify-content: center;
}
.action-logo svg {
  width: 21.3px;
  height: 21.3px;
}
.action-tooltip {
  position: absolute;
  left: calc(100% + 8px);
  top: 50%;
  transform: translateY(-50%);
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
  transition: opacity 0.16s ease, transform 0.16s ease;
}
.image-action-icon-btn:hover:not(:disabled) .action-tooltip {
  opacity: 1;
  transform: translate(2px, -50%);
}
.style-strength-pop {
  position: relative;
  width: 5px;
  min-height: 72px;
}
.style-strength-track {
  position: relative;
  width: 100%;
  height: 100%;
  border-radius: 999px;
  overflow: hidden;
  border: none;
  background: rgba(122, 107, 213, 0.2);
  cursor: ns-resize;
}
.style-strength-fill {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  background: #7a6bd5;
  transition: height 0.06s linear;
}

/* 8方位缩放手柄 */
.resize-edge, .resize-corner { position: absolute; z-index: 11; }
.resize-edge.n { top: -4px; left: 10px; right: 10px; height: 8px; cursor: ns-resize; }
.resize-edge.s { bottom: -4px; left: 10px; right: 10px; height: 8px; cursor: ns-resize; }
.resize-edge.e { right: -4px; top: 10px; bottom: 10px; width: 8px; cursor: ew-resize; }
.resize-edge.w { left: -4px; top: 10px; bottom: 10px; width: 8px; cursor: ew-resize; }

.resize-corner { width: 16px; height: 16px; background: transparent; border: none; border-radius: 50%; }
.resize-corner.nw { top: -8px; left: -8px; cursor: nwse-resize; }
.resize-corner.ne { top: -8px; right: -8px; cursor: nesw-resize; }
.resize-corner.sw { bottom: -8px; left: -8px; cursor: nesw-resize; }
.resize-corner.se { bottom: -8px; right: -8px; cursor: nwse-resize; }
</style>
