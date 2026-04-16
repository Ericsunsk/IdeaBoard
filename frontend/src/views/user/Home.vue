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
        <button class="send-btn" :disabled="!hasDirectPrompt || generating" @click="handleGenerate">
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
      <button
        v-for="item in canvasNodeMenuItems"
        :key="item.key"
        type="button"
        class="node-context-item"
        @click="handleNodeMenuAction(item.actionKey)"
      >
        {{ item.label }}
      </button>
    </div>

    <svg class="canvas-connections-layer">
      <path
        v-for="connection in renderedConnections"
        :key="connection.id"
        class="canvas-connection-hit"
        :d="connection.path"
        @mousedown.stop="onConnectionPathMouseDown($event, connection.id)"
      />
      <path
        v-for="connection in renderedConnections"
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
      <template v-for="connection in renderedConnections" :key="`endpoints-${connection.id}`">
        <circle
          v-if="selectedConnectionId === connection.id"
          class="canvas-connection-endpoint"
          :class="`tone-${connection.tone}`"
          :cx="connection.startX"
          :cy="connection.startY"
          r="4"
        />
        <circle
          v-if="selectedConnectionId === connection.id"
          class="canvas-connection-endpoint"
          :class="`tone-${connection.tone}`"
          :cx="connection.endX"
          :cy="connection.endY"
          r="4"
        />
      </template>
    </svg>

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
        :display-image="getResolvedImageNodeDisplay(node).image"
        :display-mode="getResolvedImageNodeDisplay(node).mode"
        :input-connected="hasIncomingPortConnection('image', node.id, 'image')"
        :output-connected="hasOutgoingPortConnection('image', node.id, 'image')"
        @node-mousedown="onImageNodeMouseDown($event, node)"
        @port-mousedown="(event, port) => handleConnectionPortMouseDown(event, 'image', node.id, port)"
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
        :output-connected="hasOutgoingPortConnection('prompt', node.id, 'prompt')"
        @node-mousedown="onPromptNodeMouseDown($event, node)"
        @port-mousedown="(event, port) => handleConnectionPortMouseDown(event, 'prompt', node.id, port)"
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
        :stage-height="getModelNodeStageHeight(node)"
        :source-image-url="getModelNodeEffectiveSource(node).url"
        :has-connected-image-input="getModelNodeEffectiveSource(node).connected"
        :image-input-connected="getModelNodeConnectionStatus(node).imageConnected"
        :prompt-input-connected="getModelNodeConnectionStatus(node).promptConnected"
        :output-connected="getModelNodeConnectionStatus(node).outputConnected"
        :error-port-keys="getNodeConnectionErrorPorts('model', node.id)"
        :angle-options="modelAngleOptions"
        :background-options="modelBackgroundOptions"
        @node-mousedown="onModelNodeMouseDown($event, node)"
        @port-mousedown="(event, port) => handleConnectionPortMouseDown(event, 'model', node.id, port)"
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
        :stage-height="getMaskNodeStageHeight(node)"
        :canvas-width="getMaskCanvasWidth(node)"
        :source-image-url="getMaskNodeEffectiveSource(node).url"
        :has-connected-image-input="getMaskNodeEffectiveSource(node).connected"
        :image-input-connected="hasIncomingPortConnection('mask', node.id, 'image')"
        :output-connected="hasOutgoingPortConnection('mask', node.id, 'mask')"
        @node-mousedown="onMaskNodeMouseDown($event, node)"
        @port-mousedown="(event, port) => handleConnectionPortMouseDown(event, 'mask', node.id, port)"
        @delete="deleteMaskNode(node.id)"
        @open-file-picker="openMaskNodeFilePicker(node)"
        @clear-image="clearMaskNodeImage(node)"
        @stage-drop="onMaskStageDrop($event, node)"
        @canvas-mousedown="onMaskCanvasMouseDown($event, getMaskNodeRuntimeNode(node))"
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
        :image-input-connected="getColorNodeConnectionStatus(node).imageConnected"
        :mask-input-connected="getColorNodeConnectionStatus(node).maskConnected"
        :prompt-input-connected="getColorNodeConnectionStatus(node).promptConnected"
        :output-connected="getColorNodeConnectionStatus(node).outputConnected"
        :error-port-keys="getNodeConnectionErrorPorts('color', node.id)"
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
        @set-spectrum-ref="setColorSpectrumRef($event)"
        @port-mousedown="(event, port) => handleConnectionPortMouseDown(event, 'color', node.id, port)"
      />

      <EditNode
        v-for="node in editNodes"
        :key="node.id"
        :node="node"
        :active="isNodeSelected('edit', node.id)"
        :dragging="isNodeDragging('edit', node.id)"
        :copy-mode="isDuplicateModifierPressed && activeTool === 'select'"
        :generating="generating"
        :image-input-connected="getEditNodeConnectionStatus(node).imageConnected"
        :mask-input-connected="getEditNodeConnectionStatus(node).maskConnected"
        :prompt-input-connected="getEditNodeConnectionStatus(node).promptConnected"
        :output-connected="getEditNodeConnectionStatus(node).outputConnected"
        :error-port-keys="getNodeConnectionErrorPorts('edit', node.id)"
        @node-mousedown="onEditNodeMouseDown($event, node)"
        @delete="deleteEditNode(node.id)"
        @update:mode="node.mode = $event"
        @generate="handleEditNodeGenerate(node)"
        @port-mousedown="(event, port) => handleConnectionPortMouseDown(event, 'edit', node.id, port)"
      />

      <FusionNode
        v-for="node in fusionNodes"
        :key="node.id"
        :node="node"
        :active="isNodeSelected('fusion', node.id)"
        :dragging="isNodeDragging('fusion', node.id)"
        :copy-mode="isDuplicateModifierPressed && activeTool === 'select'"
        :generating="generating"
        :image-a-input-connected="getFusionNodeConnectionStatus(node).imageAConnected"
        :image-b-input-connected="getFusionNodeConnectionStatus(node).imageBConnected"
        :prompt-input-connected="getFusionNodeConnectionStatus(node).promptConnected"
        :output-connected="getFusionNodeConnectionStatus(node).outputConnected"
        :error-port-keys="getNodeConnectionErrorPorts('fusion', node.id)"
        @node-mousedown="onFusionNodeMouseDown($event, node)"
        @delete="deleteFusionNode(node.id)"
        @update:mode="node.mode = $event"
        @generate="handleFusionNodeGenerate(node)"
        @port-mousedown="(event, port) => handleConnectionPortMouseDown(event, 'fusion', node.id, port)"
      />

      <EnhanceNode
        v-for="node in enhanceNodes"
        :key="node.id"
        :node="node"
        :active="isNodeSelected('enhance', node.id)"
        :dragging="isNodeDragging('enhance', node.id)"
        :copy-mode="isDuplicateModifierPressed && activeTool === 'select'"
        :generating="generating"
        :image-input-connected="getEnhanceNodeConnectionStatus(node).imageConnected"
        :prompt-input-connected="getEnhanceNodeConnectionStatus(node).promptConnected"
        :output-connected="getEnhanceNodeConnectionStatus(node).outputConnected"
        :error-port-keys="getNodeConnectionErrorPorts('enhance', node.id)"
        @node-mousedown="onEnhanceNodeMouseDown($event, node)"
        @delete="deleteEnhanceNode(node.id)"
        @update:mode="setEnhanceNodeMode(node, $event)"
        @update:upscale-mode="node.upscaleMode = $event"
        @update:upscale-factor="node.upscaleFactor = $event"
        @update:output-format="node.outputFormat = $event"
        @update:output-quality="node.outputQuality = Math.max(30, Math.min(100, Math.round($event)))"
        @generate="handleEnhanceNodeGenerate(node)"
        @port-mousedown="(event, port) => handleConnectionPortMouseDown(event, 'enhance', node.id, port)"
      />

      <TryOnNode
        v-for="node in tryOnNodes"
        :key="node.id"
        :node="node"
        :active="isNodeSelected('tryon', node.id)"
        :dragging="isNodeDragging('tryon', node.id)"
        :copy-mode="isDuplicateModifierPressed && activeTool === 'select'"
        :generating="generating"
        :image-a-input-connected="getTryOnNodeConnectionStatus(node).imageAConnected"
        :image-b-input-connected="getTryOnNodeConnectionStatus(node).imageBConnected"
        :output-connected="getTryOnNodeConnectionStatus(node).outputConnected"
        :error-port-keys="getNodeConnectionErrorPorts('tryon', node.id)"
        @node-mousedown="onTryOnNodeMouseDown($event, node)"
        @delete="deleteTryOnNode(node.id)"
        @update:mode="node.mode = $event"
        @update:tile-mode="node.tileMode = $event"
        @update:rotation="node.rotation = Math.max(0, Math.min(360, Math.round($event)))"
        @update:scale="node.scale = Math.max(10, Math.min(300, Math.round($event)))"
        @update:opacity="node.opacity = Math.max(0, Math.min(100, Math.round($event)))"
        @generate="handleTryOnNodeGenerate(node)"
        @port-mousedown="(event, port) => handleConnectionPortMouseDown(event, 'tryon', node.id, port)"
      />

      <PaletteNode
        v-for="node in paletteNodes"
        :key="node.id"
        :node="node"
        :active="isNodeSelected('palette', node.id)"
        :dragging="isNodeDragging('palette', node.id)"
        :copy-mode="isDuplicateModifierPressed && activeTool === 'select'"
        :generating="generating"
        :image-input-connected="getPaletteNodeConnectionStatus(node).imageConnected"
        :reference-input-connected="getPaletteNodeConnectionStatus(node).referenceConnected"
        :output-connected="getPaletteNodeConnectionStatus(node).outputConnected"
        :error-port-keys="getNodeConnectionErrorPorts('palette', node.id)"
        @node-mousedown="onPaletteNodeMouseDown($event, node)"
        @delete="deletePaletteNode(node.id)"
        @update:mode="setPaletteNodeMode(node, $event)"
        @apply-preset="node.colors = [...$event]"
        @remove-color="removePaletteColor(node, $event)"
        @add-color="addPaletteColor(node, $event)"
        @generate="handlePaletteNodeGenerate(node)"
        @port-mousedown="(event, port) => handleConnectionPortMouseDown(event, 'palette', node.id, port)"
      />

      <GenerateNode
        v-for="node in generateNodes"
        :key="node.id"
        :node="node"
        :active="isNodeSelected('generate', node.id)"
        :dragging="isNodeDragging('generate', node.id)"
        :copy-mode="isDuplicateModifierPressed && activeTool === 'select'"
        :generating="generating"
        :image-input-connected="getGenerateNodeConnectionStatus(node).imageConnected"
        :prompt-input-connected="getGenerateNodeConnectionStatus(node).promptConnected"
        :output-connected="getGenerateNodeConnectionStatus(node).outputConnected"
        :error-port-keys="getNodeConnectionErrorPorts('generate', node.id)"
        :style-options="getGenerateStyleOptions(node.mode)"
        :styles-loading="isGenerateStyleLoading(node.mode)"
        @node-mousedown="onGenerateNodeMouseDown($event, node)"
        @delete="deleteGenerateNode(node.id)"
        @update:mode="setGenerateNodeMode(node, $event)"
        @update:style-model="node.styleModel = $event"
        @update:ratio="node.ratio = $event"
        @update:count="node.count = $event"
        @update:quality="node.quality = $event"
        @generate="handleGenerateNodeGenerate(node)"
        @port-mousedown="(event, port) => handleConnectionPortMouseDown(event, 'generate', node.id, port)"
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
import type { ComponentPublicInstance, Ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useMessage, NSpin } from 'naive-ui'
import { patternStyleApi, segApi, taskApi, styleCardApi, uploadApi, upscaleApi, vectorizeApi } from '../../api/index'
import type { StyleCard } from '../../api/index'
import { useUserStore } from '../../stores/user'
import { CANVAS_NODE_MENU_ITEMS, type CanvasNodeCreateActionKey } from '../../config/canvasNodeMenu'
import { useCanvasConnections } from '../../composables/useCanvasConnections'
import { useCanvasViewport } from '../../composables/useCanvasViewport'
import { useCanvasBoard } from '../../composables/useCanvasBoard'
import { useCanvasEditor } from '../../composables/useCanvasEditor'
import { useCanvasHistory } from '../../composables/useCanvasHistory'
import { useColorNode } from '../../composables/useColorNode'
import { useCanvasDirectActions, type SegType } from '../../composables/useCanvasDirectActions'
import { useGenerationPipeline } from '../../composables/useGenerationPipeline'
import { useImageNode } from '../../composables/useImageNode'
import { clampMaskRefineAmount, useMaskNode } from '../../composables/useMaskNode'
import { useModelNode } from '../../composables/useModelNode'
import { useNodeExecutors } from '../../composables/useNodeExecutors'
import ColorNode from '../../components/canvas/nodes/ColorNode.vue'
import EditNode from '../../components/canvas/nodes/EditNode.vue'
import EnhanceNode from '../../components/canvas/nodes/EnhanceNode.vue'
import FusionNode from '../../components/canvas/nodes/FusionNode.vue'
import GenerateNode from '../../components/canvas/nodes/GenerateNode.vue'
import ImageNode from '../../components/canvas/nodes/ImageNode.vue'
import MaskNode from '../../components/canvas/nodes/MaskNode.vue'
import ModelNode from '../../components/canvas/nodes/ModelNode.vue'
import PaletteNode from '../../components/canvas/nodes/PaletteNode.vue'
import PromptNode from '../../components/canvas/nodes/PromptNode.vue'
import TryOnNode from '../../components/canvas/nodes/TryOnNode.vue'
import { MODEL_ANGLE_OPTIONS, MODEL_BACKGROUND_OPTIONS } from '../../components/canvas/nodes/nodeOptionConfig'
import {
  createCanvasImageItemAt,
  createColorNodeAt,
  createEnhanceNodeAt,
  createEditNodeAt,
  createFusionNodeAt,
  createGenerateNodeAt,
  createImageNodeAt,
  createMaskNodeAt,
  createModelNodeAt,
  createPaletteNodeAt,
  createPromptNodeAt,
  createTryOnNodeAt,
} from '../../utils/canvasNodeFactory'
import type {
  CanvasBoardSnapshot,
  CanvasConnection,
  CanvasConnectionNodeType,
  CanvasConnectionPort,
  CanvasImageItem,
  ColorNodeItem,
  EnhanceNodeItem,
  EditNodeItem,
  FusionNodeItem,
  GenerateNodeItem,
  ImageNodeItem,
  MaskNodeItem,
  ModelNodeItem,
  PaletteNodeItem,
  PromptNodeItem,
  TryOnNodeItem,
} from '../../types/canvasNodes'

const router = useRouter()
const route = useRoute()
const message = useMessage()
const userStore = useUserStore()
const defaultCanvasName = 'Untitled'

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

const modelBackgroundOptions = MODEL_BACKGROUND_OPTIONS
const modelAngleOptions = MODEL_ANGLE_OPTIONS

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
const enhanceNodes = ref<EnhanceNodeItem[]>([])
const editNodes = ref<EditNodeItem[]>([])
const fusionNodes = ref<FusionNodeItem[]>([])
const tryOnNodes = ref<TryOnNodeItem[]>([])
const paletteNodes = ref<PaletteNodeItem[]>([])
const generateNodes = ref<GenerateNodeItem[]>([])
const connections = ref<CanvasConnection[]>([])
const processedMaskPreviewUrls = ref<Record<number, string>>({})
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
const paletteReferenceExtractingIds = ref<number[]>([])
const generateStyleOptions = ref<Record<string, string[]>>({})
const generateStyleLoading = ref<Record<string, boolean>>({})

let styleStrengthLongPressTimer: number | null = null
let styleStrengthHideTimer: number | null = null
let styleStrengthDragging = false
let styleStrengthPointerY = 0

const tools = [
  { title: '图・文生图', path: '/user/picture', icon: '<rect x="3" y="3" width="18" height="18" rx="2"/><path d="M3 14l4-4 5 5 5-5 4 4"/><circle cx="8.5" cy="8.5" r="1.5"/><path d="M19 3l-1 2-2 1 2 1 1 2 1-2 2-1-2-1-1-2z" fill="currentColor" stroke="none"/>' }
]

const canvasNodeMenuItems = CANVAS_NODE_MENU_ITEMS

const editModeMeta = {
  'partial-edit': {
    feature: 'partial-edit',
    promptPrefix: '[partial-edit]',
    fallbackPrompt: '局部改款',
  },
  'pattern-inpaint': {
    feature: 'pattern-inpaint',
    promptPrefix: '[pattern-inpaint]',
    fallbackPrompt: '图案涂改',
  },
} as const

const fusionModeMeta = {
  'style-fusion': {
    feature: 'style-fusion',
    promptPrefix: '[style-fusion]',
    fallbackPrompt: '风格融合',
    successLabel: 'Style fusion',
  },
  'pattern-fusion-creative': {
    feature: 'pattern-fusion-creative',
    promptPrefix: '[pattern-fusion-creative]',
    fallbackPrompt: '图案创意融合',
    successLabel: 'Pattern fusion',
  },
  'pattern-fusion-style': {
    feature: 'pattern-fusion-style',
    promptPrefix: '[pattern-fusion-style]',
    fallbackPrompt: '图案画风融合',
    successLabel: 'Pattern fusion',
  },
  'craft-fusion-creative': {
    feature: 'craft-fusion-creative',
    promptPrefix: '[craft-fusion-creative]',
    fallbackPrompt: '工艺创意融合',
    successLabel: 'Craft fusion',
  },
  'craft-fusion-style': {
    feature: 'craft-fusion-style',
    promptPrefix: '[craft-fusion-style]',
    fallbackPrompt: '工艺画风融合',
    successLabel: 'Craft fusion',
  },
} as const

const tryOnModeMeta = {
  'fabric-on-body': {
    feature: 'fabric-on-body',
    promptPrefix: '[fabric-on-body]',
    successLabel: 'Fabric try-on',
  },
  'pattern-on-body': {
    feature: 'pattern-on-body',
    promptPrefix: '[pattern-on-body]',
    successLabel: 'Pattern try-on',
  },
} as const

const paletteModeMeta = {
  women: {
    colorType: 'women',
    label: '女装',
  },
  men: {
    colorType: 'men',
    label: '男装',
  },
  kids: {
    colorType: 'kids',
    label: '童装',
  },
  reference: {
    colorType: 'reference',
    label: '参考图',
  },
} as const

const generateModeMeta = {
  'pattern-gen': {
    feature: 'pattern-gen',
    promptPrefix: '',
    successLabel: 'Pattern generate',
    requiresImage: false,
    supportsStyleModel: true,
  },
  'style-clone': {
    feature: 'style-clone',
    promptPrefix: '',
    successLabel: 'Style clone',
    requiresImage: true,
    supportsStyleModel: true,
  },
  'seamless-tile-classic': {
    feature: 'seamless-tile-classic',
    promptPrefix: '[seamless-tile-classic]',
    successLabel: 'Tile classic',
    requiresImage: true,
    supportsStyleModel: false,
  },
  'seamless-tile-cartoon': {
    feature: 'seamless-tile-cartoon',
    promptPrefix: '[seamless-tile-cartoon]',
    successLabel: 'Tile cartoon',
    requiresImage: true,
    supportsStyleModel: false,
  },
} as const

const processedMaskPreviewSignatures = new Map<number, string>()
const {
  selectedConnectionId,
  renderedConnections,
  draftConnectionPath,
  draftConnectionTone,
  draftConnectionKind,
  hasSelectedConnection,
  clearSelectedConnection,
  selectConnection,
  deleteSelectedConnection,
  findIncomingConnection,
  getIncomingConnections,
  hasIncomingPortConnection,
  findOutgoingConnection,
  hasOutgoingPortConnection,
  removeConnectionsForNode,
  clearNodeConnectionError,
  clearAllNodeConnectionErrors,
  getNodeConnectionErrorPorts,
  triggerNodeConnectionError,
  clearConnectionDraft,
  scheduleRenderedConnectionsRefresh,
  startConnectionDrag,
  addGlobalConnectionListeners,
  removeGlobalConnectionListeners,
  disposeConnectionRuntime,
} = useCanvasConnections({
  connections,
  canvasLayoutRef,
  onConnectionNotAllowed: () => {
    message.warning('当前这两个端口不能连接')
  },
  onInvalidDirection: () => {
    message.warning('连线只能从输出口接到输入口')
  },
})

const {
  selectedIds,
  activeImageActionId,
  isDuplicateModifierPressed,
  activeImageNodeId,
  activePromptNodeId,
  activeModelNodeId,
  activeMaskNodeId,
  activeColorNodeId,
  activeEditNodeId,
  activeFusionNodeId,
  activeEnhanceNodeId,
  activeTryOnNodeId,
  activePaletteNodeId,
  activeGenerateNodeId,
  nodeContextMenu,
  isSelecting,
  selectionRect,
  activateImageNode,
  activatePromptNode,
  activateModelNode,
  activateMaskNode,
  activateColorNode,
  activateEditNode,
  activateFusionNode,
  activateEnhanceNode,
  activateTryOnNode,
  activatePaletteNode,
  activateGenerateNode,
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
  onEditNodeMouseDown,
  onFusionNodeMouseDown,
  onEnhanceNodeMouseDown,
  onTryOnNodeMouseDown,
  onPaletteNodeMouseDown,
  onGenerateNodeMouseDown,
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
  enhanceNodes,
  editNodes,
  fusionNodes,
  tryOnNodes,
  paletteNodes,
  generateNodes,
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
  deleteEnhanceNode,
  deleteEditNode,
  deleteFusionNode,
  deleteTryOnNode,
  deletePaletteNode,
  deleteGenerateNode,
  hasSelectedConnection,
  clearSelectedConnection,
  deleteSelectedConnection,
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
  setColorSpectrumRef,
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
  clearAllNodeConnectionErrors()
  resetMaskRuntime()
  imageNodes.value = []
  promptNodes.value = []
  modelNodes.value = []
  maskNodes.value = []
  colorNodes.value = []
  enhanceNodes.value = []
  editNodes.value = []
  fusionNodes.value = []
  tryOnNodes.value = []
  paletteNodes.value = []
  generateNodes.value = []
  connections.value = []
  processedMaskPreviewUrls.value = {}
  processedMaskPreviewSignatures.clear()
  clearConnectionDraft()
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
  clearAllNodeConnectionErrors()

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
  enhanceNodes.value = state.enhanceNodes.map(node => ({
    ...node,
    outputQuality: Math.max(30, Math.min(100, Math.round(node.outputQuality ?? 95))),
  }))
  editNodes.value = state.editNodes.map(node => ({ ...node }))
  fusionNodes.value = state.fusionNodes.map(node => ({ ...node }))
  tryOnNodes.value = state.tryOnNodes.map(node => ({
    ...node,
    rotation: Math.max(0, Math.min(360, Math.round(node.rotation ?? 0))),
    scale: Math.max(10, Math.min(300, Math.round(node.scale ?? 100))),
    opacity: Math.max(0, Math.min(100, Math.round(node.opacity ?? 100))),
  }))
  paletteNodes.value = state.paletteNodes.map(node => ({
    ...node,
    colors: [...node.colors],
  }))
  generateNodes.value = state.generateNodes.map(node => ({ ...node }))
  generateNodes.value.forEach((node) => {
    if (generateModeSupportsStyleModel(node.mode)) {
      void ensureGenerateStyleOptions(node.mode)
    }
  })
  connections.value = state.connections.map(connection => ({ ...connection }))
  clearSelectedConnection()
  clearConnectionDraft()
  activeColorPopoverNodeId.value = null
  setViewport(state.viewport)

  await nextTick()
  maskNodes.value.forEach(node => renderMaskNodeMask(node))
  scheduleRenderedConnectionsRefresh()
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
    enhanceNodes: enhanceNodes.value.map(node => ({ ...node })),
    editNodes: editNodes.value.map(node => ({ ...node })),
    fusionNodes: fusionNodes.value.map(node => ({ ...node })),
    tryOnNodes: tryOnNodes.value.map(node => ({ ...node })),
    paletteNodes: paletteNodes.value.map(node => ({
      ...node,
      colors: [...node.colors],
    })),
    generateNodes: generateNodes.value.map(node => ({ ...node })),
    connections: connections.value.map(connection => ({ ...connection })),
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
  [messages, imageNodes, promptNodes, modelNodes, maskNodes, colorNodes, enhanceNodes, editNodes, fusionNodes, tryOnNodes, paletteNodes, generateNodes, connections, canvasX, canvasY, zoomLevel],
  () => {
    markBoardDirty()
    notifySnapshotChanged()
  },
  { deep: true }
)

watch(
  [connections, imageNodes, promptNodes, modelNodes, maskNodes, colorNodes, enhanceNodes, editNodes, fusionNodes, tryOnNodes, paletteNodes, generateNodes, canvasX, canvasY, zoomLevel],
  () => {
    scheduleRenderedConnectionsRefresh()
  },
  { deep: true, flush: 'post' }
)

watch(
  maskNodes,
  (nodes) => {
    const nextIds = new Set(nodes.map(node => node.id))
    Object.keys(processedMaskPreviewUrls.value).forEach((rawId) => {
      const id = Number(rawId)
      if (!nextIds.has(id)) {
        clearProcessedMaskPreview(id)
      }
    })
    nodes.forEach((node) => {
      void refreshProcessedMaskPreview(node)
    })
  },
  { deep: true, immediate: true }
)

function deletePromptNode(id: number) {
  deleteNodeFromCollection(promptNodes, 'prompt', id, () => {
    if (activePromptNodeId.value === id) {
      activePromptNodeId.value = null
    }
  })
}

function deleteModelNode(id: number) {
  deleteNodeFromCollection(modelNodes, 'model', id, () => {
    if (activeModelNodeId.value === id) {
      activeModelNodeId.value = null
    }
  })
}

function deleteMaskNode(id: number) {
  deleteNodeFromCollection(maskNodes, 'mask', id, () => {
    removeMaskRuntimeFor(id)
    clearProcessedMaskPreview(id)
    if (activeMaskNodeId.value === id) {
      activeMaskNodeId.value = null
    }
  })
}

function deleteImageNode(id: number) {
  deleteNodeFromCollection(imageNodes, 'image', id, () => {
    if (activeImageNodeId.value === id) {
      activeImageNodeId.value = null
    }
  })
}

function deleteColorNode(id: number) {
  deleteNodeFromCollection(colorNodes, 'color', id, () => {
    if (activeColorPopoverNodeId.value === id) {
      activeColorPopoverNodeId.value = null
    }
    if (activeColorNodeId.value === id) {
      activeColorNodeId.value = null
    }
  })
}

function deleteEnhanceNode(id: number) {
  deleteNodeFromCollection(enhanceNodes, 'enhance', id, () => {
    if (activeEnhanceNodeId.value === id) {
      activeEnhanceNodeId.value = null
    }
  })
}

function deleteEditNode(id: number) {
  deleteNodeFromCollection(editNodes, 'edit', id, () => {
    if (activeEditNodeId.value === id) {
      activeEditNodeId.value = null
    }
  })
}

function deleteFusionNode(id: number) {
  deleteNodeFromCollection(fusionNodes, 'fusion', id, () => {
    if (activeFusionNodeId.value === id) {
      activeFusionNodeId.value = null
    }
  })
}

function deleteTryOnNode(id: number) {
  deleteNodeFromCollection(tryOnNodes, 'tryon', id, () => {
    if (activeTryOnNodeId.value === id) {
      activeTryOnNodeId.value = null
    }
  })
}

function deletePaletteNode(id: number) {
  deleteNodeFromCollection(paletteNodes, 'palette', id, () => {
    if (activePaletteNodeId.value === id) {
      activePaletteNodeId.value = null
    }
    paletteReferenceExtractingIds.value = paletteReferenceExtractingIds.value.filter(nodeId => nodeId !== id)
  })
}

function deleteGenerateNode(id: number) {
  deleteNodeFromCollection(generateNodes, 'generate', id, () => {
    if (activeGenerateNodeId.value === id) {
      activeGenerateNodeId.value = null
    }
  })
}

function createNodeFromContextMenu<T extends { id: number }>(
  createAt: (point: { x: number; y: number }) => T,
  append: (node: T) => void,
  activate: (id: number) => void,
  afterCreate?: (node: T) => void,
) {
  const { clientX, clientY } = nodeContextMenu.value
  const point = getCanvasPointFromClient(clientX, clientY)
  const newNode = createAt(point)
  append(newNode)
  activate(newNode.id)
  afterCreate?.(newNode)
  closeNodeContextMenu()
}

function deleteNodeFromCollection<T extends { id: number }>(
  collection: Ref<T[]>,
  nodeType: CanvasConnectionNodeType,
  id: number,
  afterDelete?: () => void,
) {
  collection.value = collection.value.filter(node => node.id !== id)
  clearNodeConnectionError(nodeType, id)
  removeConnectionsForNode(nodeType, id)
  removeSelectedNodeKey(nodeType, id)
  afterDelete?.()
}

function buildNodeCreateHandler<T extends { id: number }>(
  createAt: (point: { x: number; y: number }) => T,
  collection: Ref<T[]>,
  activate: (id: number) => void,
  afterCreate?: (node: T) => void,
) {
  return () => createNodeFromContextMenu(
    createAt,
    (newNode) => collection.value.push(newNode),
    activate,
    afterCreate,
  )
}

const nodeContextMenuHandlers: Record<CanvasNodeCreateActionKey, () => void> = {
  createImageNode: buildNodeCreateHandler(createImageNodeAt, imageNodes, activateImageNode),
  createPromptNode: buildNodeCreateHandler(createPromptNodeAt, promptNodes, activatePromptNode),
  createModelNode: buildNodeCreateHandler(createModelNodeAt, modelNodes, activateModelNode),
  createMaskNode: buildNodeCreateHandler(createMaskNodeAt, maskNodes, activateMaskNode),
  createColorNode: buildNodeCreateHandler(createColorNodeAt, colorNodes, activateColorNode),
  createEditNode: buildNodeCreateHandler(createEditNodeAt, editNodes, activateEditNode),
  createFusionNode: buildNodeCreateHandler(createFusionNodeAt, fusionNodes, activateFusionNode),
  createEnhanceNode: buildNodeCreateHandler(createEnhanceNodeAt, enhanceNodes, activateEnhanceNode),
  createTryOnNode: buildNodeCreateHandler(createTryOnNodeAt, tryOnNodes, activateTryOnNode),
  createPaletteNode: buildNodeCreateHandler(createPaletteNodeAt, paletteNodes, activatePaletteNode),
  createGenerateNode: buildNodeCreateHandler(createGenerateNodeAt, generateNodes, activateGenerateNode, (newNode) => {
    void ensureGenerateStyleOptions(newNode.mode)
  }),
}

function handleNodeMenuAction(actionKey: CanvasNodeCreateActionKey) {
  nodeContextMenuHandlers[actionKey]?.()
}

function handleConnectionPortMouseDown(
  event: MouseEvent,
  nodeType: CanvasConnectionNodeType,
  nodeId: number,
  port: CanvasConnectionPort,
) {
  clearSelectedIds()
  clearSelectedConnection()
  closeNodeContextMenu()
  closeColorPopover()
  startConnectionDrag(event, nodeType, nodeId, port)
}

function onConnectionPathMouseDown(event: MouseEvent, connectionId: number) {
  event.preventDefault()
  event.stopPropagation()
  clearSelectedIds()
  closeNodeContextMenu()
  closeColorPopover()
  clearSelectedConnection()
  selectConnection(connectionId)
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
  selectedIds.value.length > 0
    ? 'Direct prompt for the selected image'
    : 'Direct text-to-image prompt'
)
const preferredWorkflowPromptNode = computed(() => {
  const activePromptNode =
    activePromptNodeId.value !== null
      ? promptNodes.value.find(item => item.id === activePromptNodeId.value) ?? null
      : null
  if (activePromptNode?.text.trim()) return activePromptNode
  return [...promptNodes.value].reverse().find(item => item.text.trim()) ?? null
})
const hasDirectPrompt = computed(() => !!userPrompt.value.trim())
const styleCards = ref<StyleCard[]>([])
const selectedCardIds = ref<number[]>([])
const selectedRatio = ref('3:4')
const selectedResolution = ref('1K')

function resolveWorkflowPromptText(fallback = ''): string {
  return preferredWorkflowPromptNode.value?.text.trim() || fallback
}

function extractDominantColors(imageUrl: string, maxColors: number): Promise<string[]> {
  return new Promise((resolve, reject) => {
    const img = new Image()
    img.crossOrigin = 'anonymous'
    img.onload = () => {
      const canvas = document.createElement('canvas')
      const size = 150
      canvas.width = size
      canvas.height = size
      const ctx = canvas.getContext('2d')
      if (!ctx) {
        reject(new Error('Canvas not available'))
        return
      }
      ctx.drawImage(img, 0, 0, size, size)
      const data = ctx.getImageData(0, 0, size, size).data
      const buckets = new Map<string, number>()
      for (let index = 0; index < data.length; index += 4) {
        const r = data[index]
        const g = data[index + 1]
        const b = data[index + 2]
        const alpha = data[index + 3]
        if (alpha < 128) continue
        if (r > 240 && g > 240 && b > 240) continue
        if (r < 15 && g < 15 && b < 15) continue
        const qr = Math.round(r / 32) * 32
        const qg = Math.round(g / 32) * 32
        const qb = Math.round(b / 32) * 32
        const key = `${qr},${qg},${qb}`
        buckets.set(key, (buckets.get(key) || 0) + 1)
      }
      const result: string[] = []
      const sorted = [...buckets.entries()].sort((left, right) => right[1] - left[1])
      for (const [key] of sorted) {
        if (result.length >= maxColors) break
        const [r, g, b] = key.split(',').map(Number)
        const hex = `#${[r, g, b].map(value => value.toString(16).padStart(2, '0')).join('')}`.toUpperCase()
        const tooClose = result.some((existing) => {
          const er = parseInt(existing.slice(1, 3), 16)
          const eg = parseInt(existing.slice(3, 5), 16)
          const eb = parseInt(existing.slice(5, 7), 16)
          return Math.abs(r - er) + Math.abs(g - eg) + Math.abs(b - eb) < 80
        })
        if (!tooClose) {
          result.push(hex)
        }
      }
      resolve(result)
    }
    img.onerror = () => reject(new Error('Failed to load image'))
    img.src = imageUrl
  })
}

function getImageNodeConnectionOutput(node: ImageNodeItem | null): { url: string; ratio: number } | null {
  if (!node) return null
  return getImageNodePreferredImage(node)
}

function clearProcessedMaskPreview(id: number) {
  if (!(id in processedMaskPreviewUrls.value)) {
    processedMaskPreviewSignatures.delete(id)
    return
  }
  const next = { ...processedMaskPreviewUrls.value }
  delete next[id]
  processedMaskPreviewUrls.value = next
  processedMaskPreviewSignatures.delete(id)
}

function getMaskPreviewSignature(node: MaskNodeItem): string {
  return `${node.maskDataUrl}|${clampMaskRefineAmount(node.refineAmount ?? 0)}|${node.invertMask ? 1 : 0}`
}

async function refreshProcessedMaskPreview(node: MaskNodeItem) {
  if (!node.maskDataUrl) {
    clearProcessedMaskPreview(node.id)
    return
  }

  const refineAmount = clampMaskRefineAmount(node.refineAmount ?? 0)
  const signature = getMaskPreviewSignature(node)
  processedMaskPreviewSignatures.set(node.id, signature)

  if (refineAmount === 0 && !node.invertMask) {
    processedMaskPreviewUrls.value = {
      ...processedMaskPreviewUrls.value,
      [node.id]: node.maskDataUrl,
    }
    return
  }

  const refinedUrl = await refineMaskDataUrl(node.maskDataUrl, refineAmount, !!node.invertMask)
  if (processedMaskPreviewSignatures.get(node.id) !== signature) return
  processedMaskPreviewUrls.value = {
    ...processedMaskPreviewUrls.value,
    [node.id]: refinedUrl || node.maskDataUrl,
  }
}

function getMaskNodeProcessedOutputUrl(node: MaskNodeItem): string {
  if (!node.maskDataUrl) return ''
  return processedMaskPreviewUrls.value[node.id] || node.maskDataUrl
}

function getImageNodeConnectedInput(node: ImageNodeItem): { url: string; ratio: number; mode: 'mask' } | null {
  const connection = findIncomingConnection('image', node.id, 'image')
  if (!connection || connection.fromNodeType !== 'mask') return null
  const sourceNode = maskNodes.value.find(item => item.id === connection.fromNodeId) ?? null
  if (!sourceNode?.maskDataUrl) return null
  const source = getMaskNodeEffectiveSource(sourceNode)
  return {
    url: getMaskNodeProcessedOutputUrl(sourceNode),
    ratio: source.ratio,
    mode: 'mask',
  }
}

function getResolvedImageNodeDisplay(node: ImageNodeItem): { image: { url: string; ratio: number } | null; mode: 'source' | 'output' | 'mask' } {
  const connectedInput = getImageNodeConnectedInput(node)
  if (connectedInput) {
    return {
      image: {
        url: connectedInput.url,
        ratio: connectedInput.ratio,
      },
      mode: connectedInput.mode,
    }
  }
  return {
    image: getImageNodeDisplayImage(node),
    mode: node.outputImages.length > 0 ? 'output' : 'source',
  }
}

function getMaskNodeConnectedImageInput(node: MaskNodeItem): { url: string; ratio: number } | null {
  return getConnectedImageNodeOutput('mask', node.id, 'image')
}

function getMaskNodeEffectiveSource(node: MaskNodeItem): { url: string; ratio: number; connected: boolean } {
  const connectedImage = getMaskNodeConnectedImageInput(node)
  if (connectedImage) {
    return {
      url: connectedImage.url,
      ratio: connectedImage.ratio,
      connected: true,
    }
  }
  return {
    url: node.sourceImageUrl,
    ratio: node.sourceImageRatio,
    connected: false,
  }
}

function getModelNodeConnectedImageInput(node: ModelNodeItem): { url: string; ratio: number } | null {
  return getConnectedImageNodeOutput('model', node.id, 'image')
}

function getModelNodeConnectedPromptInput(node: ModelNodeItem): string {
  return getConnectedPromptText('model', node.id, 'prompt')
}

function getModelNodeEffectiveSource(node: ModelNodeItem): { url: string; ratio: number; connected: boolean } {
  const connectedImage = getModelNodeConnectedImageInput(node)
  if (connectedImage) {
    return {
      url: connectedImage.url,
      ratio: connectedImage.ratio,
      connected: true,
    }
  }
  return {
    url: node.sourceImageUrl,
    ratio: node.sourceImageRatio,
    connected: false,
  }
}

function getModelNodeConnectedOutputTarget(node: ModelNodeItem): ImageNodeItem | null {
  return getConnectedOutputImageTarget('model', node.id, 'image')
}

function getColorNodeConnectedOutputTarget(node: ColorNodeItem): ImageNodeItem | null {
  return getConnectedOutputImageTarget('color', node.id, 'image')
}

function getEditNodeConnectedOutputTarget(node: EditNodeItem): ImageNodeItem | null {
  return getConnectedOutputImageTarget('edit', node.id, 'image')
}

function getFusionNodeConnectedOutputTarget(node: FusionNodeItem): ImageNodeItem | null {
  return getConnectedOutputImageTarget('fusion', node.id, 'image')
}

function getEnhanceNodeConnectedOutputTarget(node: EnhanceNodeItem): ImageNodeItem | null {
  return getConnectedOutputImageTarget('enhance', node.id, 'image')
}

function getTryOnNodeConnectedOutputTarget(node: TryOnNodeItem): ImageNodeItem | null {
  return getConnectedOutputImageTarget('tryon', node.id, 'image')
}

function getPaletteNodeConnectedOutputTarget(node: PaletteNodeItem): ImageNodeItem | null {
  return getConnectedOutputImageTarget('palette', node.id, 'image')
}

function getGenerateNodeConnectedOutputTarget(node: GenerateNodeItem): ImageNodeItem | null {
  return getConnectedOutputImageTarget('generate', node.id, 'image')
}

function getColorNodeConnectionStatus(node: ColorNodeItem) {
  const status = createNodeConnectionStatusBuilder('color', node.id)
  const hasAnyConnection = status.hasAnyConnection('image')
  const imageConnected = status.incoming('image')
  const maskConnected = status.incoming('mask')
  const promptConnected = status.incoming('prompt')
  const outputConnected = status.outgoing('image')
  return {
    hasAnyConnection,
    imageConnected,
    maskConnected,
    promptConnected,
    outputConnected,
  }
}

function getConnectedImageNodeOutput(
  toNodeType: CanvasConnectionNodeType,
  toNodeId: number,
  toPort: CanvasConnectionPort,
): { url: string; ratio: number } | null {
  const connection = findIncomingConnection(toNodeType, toNodeId, toPort)
  if (!connection || connection.fromNodeType !== 'image') return null
  const sourceNode = imageNodes.value.find(item => item.id === connection.fromNodeId) ?? null
  const preferredImage = getImageNodeConnectionOutput(sourceNode)
  if (!preferredImage?.url) return null
  return preferredImage
}

function getConnectedPromptText(
  toNodeType: CanvasConnectionNodeType,
  toNodeId: number,
  toPort: CanvasConnectionPort,
): string {
  const connection = findIncomingConnection(toNodeType, toNodeId, toPort)
  if (!connection || connection.fromNodeType !== 'prompt') return ''
  const sourceNode = promptNodes.value.find(item => item.id === connection.fromNodeId) ?? null
  return sourceNode?.text.trim() || ''
}

function getConnectedOutputImageTarget(
  fromNodeType: CanvasConnectionNodeType,
  fromNodeId: number,
  fromPort: CanvasConnectionPort,
): ImageNodeItem | null {
  const connection = findOutgoingConnection(fromNodeType, fromNodeId, fromPort)
  if (!connection || connection.toNodeType !== 'image') return null
  return imageNodes.value.find(item => item.id === connection.toNodeId) ?? null
}

function resolveConnectedPlacedImageInput(
  toNodeType: CanvasConnectionNodeType,
  toNodeId: number,
  toPort: CanvasConnectionPort,
): { url: string; placementX: number; placementY: number } | null {
  const connection = findIncomingConnection(toNodeType, toNodeId, toPort)
  if (!connection || connection.fromNodeType !== 'image') return null
  const sourceNode = imageNodes.value.find(item => item.id === connection.fromNodeId) ?? null
  const preferredImage = getImageNodeConnectionOutput(sourceNode)
  if (!preferredImage?.url || !sourceNode) return null
  return {
    url: preferredImage.url,
    placementX: sourceNode.x - 340,
    placementY: sourceNode.y + 18,
  }
}

async function resolveConnectedPlacedMaskInput(
  toNodeType: CanvasConnectionNodeType,
  toNodeId: number,
  toPort: CanvasConnectionPort,
): Promise<{ dataUrl: string; placementX: number; placementY: number } | null> {
  const connection = findIncomingConnection(toNodeType, toNodeId, toPort)
  if (!connection || connection.fromNodeType !== 'mask') return null
  const sourceNode = maskNodes.value.find(item => item.id === connection.fromNodeId) ?? null
  if (!sourceNode?.maskDataUrl) return null
  const dataUrl = await refineMaskDataUrl(
    sourceNode.maskDataUrl,
    sourceNode.refineAmount ?? 0,
    !!sourceNode.invertMask,
  )
  if (!dataUrl) return null
  return {
    dataUrl,
    placementX: sourceNode.x + 340,
    placementY: sourceNode.y + 18,
  }
}

function resolvePreferredOutputNodeId(connectedOutputNodeId: number | null): number | null {
  return connectedOutputNodeId ?? resolvePreferredImageOutputNode()?.id ?? null
}

function buildPlacementSource(url: string, x: number, y: number): CanvasImageItem {
  return {
    id: -Date.now(),
    type: 'image',
    url,
    x,
    y,
    width: 300,
  }
}

function warnMissingNodeInputs(
  nodeType: CanvasConnectionNodeType,
  nodeId: number,
  missingPorts: string[],
  formatMessage: (ports: string[]) => string,
): boolean {
  if (missingPorts.length === 0) return false
  triggerNodeConnectionError(nodeType, nodeId, missingPorts)
  message.warning(formatMessage(missingPorts))
  return true
}

function createNodeConnectionStatusBuilder(nodeType: CanvasConnectionNodeType, nodeId: number) {
  return {
    incoming: (port: CanvasConnectionPort) => hasIncomingPortConnection(nodeType, nodeId, port),
    outgoing: (port: CanvasConnectionPort) => hasOutgoingPortConnection(nodeType, nodeId, port),
    hasAnyConnection: (outputPort: CanvasConnectionPort) =>
      getIncomingConnections(nodeType, nodeId).length > 0 || hasOutgoingPortConnection(nodeType, nodeId, outputPort),
  }
}

function getModelNodeConnectionStatus(node: ModelNodeItem) {
  const status = createNodeConnectionStatusBuilder('model', node.id)
  const hasAnyConnection = status.hasAnyConnection('image')
  const imageConnected = status.incoming('image')
  const promptConnected = status.incoming('prompt')
  const outputConnected = status.outgoing('image')
  return {
    hasAnyConnection,
    imageConnected,
    promptConnected,
    outputConnected,
  }
}

function getEditNodeConnectionStatus(node: EditNodeItem) {
  const status = createNodeConnectionStatusBuilder('edit', node.id)
  const hasAnyConnection = status.hasAnyConnection('image')
  const imageConnected = status.incoming('image')
  const maskConnected = status.incoming('mask')
  const promptConnected = status.incoming('prompt')
  const outputConnected = status.outgoing('image')
  return {
    hasAnyConnection,
    imageConnected,
    maskConnected,
    promptConnected,
    outputConnected,
  }
}

function getFusionNodeConnectionStatus(node: FusionNodeItem) {
  const status = createNodeConnectionStatusBuilder('fusion', node.id)
  const hasAnyConnection = status.hasAnyConnection('image')
  const imageAConnected = status.incoming('imageA')
  const imageBConnected = status.incoming('imageB')
  const promptConnected = status.incoming('prompt')
  const outputConnected = status.outgoing('image')
  return {
    hasAnyConnection,
    imageAConnected,
    imageBConnected,
    promptConnected,
    outputConnected,
  }
}

function enhanceModeSupportsPrompt(mode: EnhanceNodeItem['mode']) {
  return mode === 'deblur' || mode === 'canvas-extend'
}

function getEnhanceNodeConnectionStatus(node: EnhanceNodeItem) {
  const status = createNodeConnectionStatusBuilder('enhance', node.id)
  const hasAnyConnection = status.hasAnyConnection('image')
  const imageConnected = status.incoming('image')
  const promptConnected = enhanceModeSupportsPrompt(node.mode)
    ? status.incoming('prompt')
    : false
  const outputConnected = status.outgoing('image')
  return {
    hasAnyConnection,
    imageConnected,
    promptConnected,
    outputConnected,
  }
}

function getTryOnNodeConnectionStatus(node: TryOnNodeItem) {
  const status = createNodeConnectionStatusBuilder('tryon', node.id)
  const imageAConnected = status.incoming('imageA')
  const imageBConnected = status.incoming('imageB')
  const outputConnected = status.outgoing('image')
  return {
    imageAConnected,
    imageBConnected,
    outputConnected,
  }
}

function paletteModeUsesReference(mode: PaletteNodeItem['mode']) {
  return mode === 'reference'
}

function generateModeRequiresImage(mode: GenerateNodeItem['mode']) {
  return generateModeMeta[mode].requiresImage
}

function generateModeSupportsStyleModel(mode: GenerateNodeItem['mode']) {
  return generateModeMeta[mode].supportsStyleModel
}

function getPaletteNodeConnectionStatus(node: PaletteNodeItem) {
  const status = createNodeConnectionStatusBuilder('palette', node.id)
  const imageConnected = status.incoming('image')
  const referenceConnected = paletteModeUsesReference(node.mode)
    ? status.incoming('reference')
    : false
  const outputConnected = status.outgoing('image')
  return {
    imageConnected,
    referenceConnected,
    outputConnected,
  }
}

function getGenerateNodeConnectionStatus(node: GenerateNodeItem) {
  const status = createNodeConnectionStatusBuilder('generate', node.id)
  const imageConnected = status.incoming('image')
  const promptConnected = status.incoming('prompt')
  const outputConnected = status.outgoing('image')
  return {
    imageConnected,
    promptConnected,
    outputConnected,
  }
}

function setEnhanceNodeMode(node: EnhanceNodeItem, mode: EnhanceNodeItem['mode']) {
  node.mode = mode
  if (enhanceModeSupportsPrompt(mode)) return
  const removedSelected = connections.value.some(connection =>
    connection.id === selectedConnectionId.value &&
    connection.toNodeType === 'enhance' &&
    connection.toNodeId === node.id &&
    connection.toPort === 'prompt',
  )
  connections.value = connections.value.filter(connection =>
    !(
      connection.toNodeType === 'enhance' &&
      connection.toNodeId === node.id &&
      connection.toPort === 'prompt'
    ),
  )
  if (removedSelected) {
    clearSelectedConnection()
  }
}

function setPaletteNodeMode(node: PaletteNodeItem, mode: PaletteNodeItem['mode']) {
  const usingReferenceBefore = paletteModeUsesReference(node.mode)
  node.mode = mode
  if (paletteModeUsesReference(mode)) return

  const removedSelected = connections.value.some(connection =>
    connection.id === selectedConnectionId.value &&
    connection.toNodeType === 'palette' &&
    connection.toNodeId === node.id &&
    connection.toPort === 'reference',
  )
  connections.value = connections.value.filter(connection =>
    !(
      connection.toNodeType === 'palette' &&
      connection.toNodeId === node.id &&
      connection.toPort === 'reference'
    ),
  )
  if (removedSelected) {
    clearSelectedConnection()
  }
  if (usingReferenceBefore) {
    paletteReferenceExtractingIds.value = paletteReferenceExtractingIds.value.filter(item => item !== node.id)
  }
}

function getGenerateStyleOptions(mode: GenerateNodeItem['mode']): string[] {
  return generateStyleOptions.value[mode] ?? []
}

function isGenerateStyleLoading(mode: GenerateNodeItem['mode']): boolean {
  return !!generateStyleLoading.value[mode]
}

async function ensureGenerateStyleOptions(mode: GenerateNodeItem['mode']) {
  if (!generateModeSupportsStyleModel(mode)) return
  if (generateStyleLoading.value[mode]) return
  if ((generateStyleOptions.value[mode] ?? []).length > 0) return
  generateStyleLoading.value = { ...generateStyleLoading.value, [mode]: true }
  try {
    const res: any = await patternStyleApi.listEnabled(mode)
    if (res.code === 0) {
      const names = (res.data?.list || [])
        .map((item: any) => item?.name)
        .filter((item: unknown): item is string => typeof item === 'string' && item.trim().length > 0)
      const uniqueNames = Array.from(new Set<string>(names))
      generateStyleOptions.value = {
        ...generateStyleOptions.value,
        [mode]: uniqueNames,
      }
    }
  } catch {
    // ignore and keep current options empty
  } finally {
    generateStyleLoading.value = { ...generateStyleLoading.value, [mode]: false }
  }
}

function setGenerateNodeMode(node: GenerateNodeItem, mode: GenerateNodeItem['mode']) {
  node.mode = mode
  if (!generateModeSupportsStyleModel(mode)) {
    node.styleModel = ''
  } else {
    void ensureGenerateStyleOptions(mode)
  }
}

function getMaskNodeStageHeight(node: MaskNodeItem): number {
  const source = getMaskNodeEffectiveSource(node)
  return getMaskStageHeight({
    ...node,
    sourceImageUrl: source.url,
    sourceImageRatio: source.ratio,
  })
}

function getMaskNodeRuntimeNode(node: MaskNodeItem): MaskNodeItem {
  const source = getMaskNodeEffectiveSource(node)
  return {
    ...node,
    sourceImageUrl: source.url,
    sourceImageRatio: source.ratio,
  }
}

function getModelNodeStageHeight(node: ModelNodeItem): number {
  const source = getModelNodeEffectiveSource(node)
  return getModelStageHeight({
    ...node,
    sourceImageUrl: source.url,
    sourceImageRatio: source.ratio,
  })
}

function toggleCard(id: number) {
  const idx = selectedCardIds.value.indexOf(id)
  if (idx >= 0) selectedCardIds.value.splice(idx, 1)
  else selectedCardIds.value.push(id)
}

function removePaletteColor(node: PaletteNodeItem, index: number) {
  if (index < 0 || index >= node.colors.length) return
  node.colors.splice(index, 1)
}

function addPaletteColor(node: PaletteNodeItem, rawColor: string) {
  if (node.colors.length >= 8) {
    message.warning('最多保留 8 种颜色')
    return
  }
  const nextColor = normalizeHexColor(rawColor)
  if (node.colors.includes(nextColor)) return
  node.colors.push(nextColor)
}

const {
  handleModelNodeGenerate,
  handleColorNodeGenerate,
  handleEditNodeGenerate,
  handleFusionNodeGenerate,
  handleEnhanceNodeGenerate,
  handleTryOnNodeGenerate,
  handlePaletteNodeGenerate,
  handleGenerateNodeGenerate,
} = useNodeExecutors({
  generating,
  message,
  messages,
  selectedIds,
  imageNodes,
  modelNodes,
  maskNodes,
  activeImageNodeId,
  activeModelNodeId,
  activeMaskNodeId,
  paletteReferenceExtractingIds,
  modelDefaultPose,
  modelDefaultExpression,
  editModeMeta,
  fusionModeMeta,
  tryOnModeMeta,
  paletteModeMeta,
  generateModeMeta,
  getModelAngleLabel,
  getIncomingConnections,
  triggerNodeConnectionError,
  getModelNodeEffectiveSource,
  getModelNodeConnectedPromptInput,
  resolveWorkflowPromptText,
  getModelNodeConnectedOutputTarget,
  getColorNodeConnectedOutputTarget,
  getEditNodeConnectedOutputTarget,
  getFusionNodeConnectedOutputTarget,
  getEnhanceNodeConnectedOutputTarget,
  getTryOnNodeConnectedOutputTarget,
  getPaletteNodeConnectedOutputTarget,
  getGenerateNodeConnectedOutputTarget,
  resolvePreferredOutputNodeId,
  buildPlacementSource,
  warnMissingNodeInputs,
  submitTaskAndPoll,
  appendGeneratedImages,
  normalizeHexColor,
  getImageNodePreferredImage,
  resolveConnectedPlacedImageInput,
  resolveConnectedPlacedMaskInput,
  getConnectedPromptText,
  buildFullMaskDataUrl,
  refineMaskDataUrl,
  dataUrlToFile,
  uploadImage: uploadApi.upload,
  upscaleImage: upscaleApi.upscale,
  segmentImage: segApi.segment,
  extractDominantColors,
  paletteModeUsesReference,
  generateModeRequiresImage,
  generateModeSupportsStyleModel,
})

const {
  runCanvasAction,
  handleGenerate,
  handleExport,
} = useCanvasDirectActions({
  generating,
  selectedIds,
  activeImageActionId,
  showStyleStrengthControl,
  hasDirectPrompt,
  showSettings,
  selectedCardIds,
  selectedRatio,
  selectedResolution,
  userPrompt,
  messages,
  styleInnovateStrength,
  defaultSegType,
  message,
  submitTaskAndPoll,
  appendGeneratedImages,
  segmentImage: segApi.segment,
  saveSegmentResult: segApi.save,
  vectorizeImage: vectorizeApi.vectorize,
})

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
  addGlobalConnectionListeners()
  nextTick(() => {
    scheduleRenderedConnectionsRefresh()
  })
})

onUnmounted(() => {
  stopTaskPolling()
  clearStyleStrengthLongPressTimer()
  clearStyleStrengthHideTimer()
  removeStyleStrengthGlobalListeners()
  removeGlobalConnectionListeners()
  disposeConnectionRuntime()
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

.canvas-connections-layer {
  position: absolute;
  inset: 0;
  z-index: 1;
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
  pointer-events: none;
  filter: drop-shadow(0 0 6px var(--connection-glow, rgba(91, 141, 239, 0.18)));
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

.canvas-content { position: absolute; inset: 0; z-index: 2; pointer-events: none; transform: translate(var(--offset-x, 0px), var(--offset-y, 0px)) scale(var(--zoom, 1)); transform-origin: center center; }
.workflow-node {
  position: absolute;
  z-index: 12;
  border: 1.5px solid #e2e8f0;
  border-radius: 14px;
  background: #ffffff;
  overflow: visible;
  transition: border-color 0.2s ease;
  will-change: transform;
  pointer-events: auto;
}

.workflow-node :deep(.workflow-node-header) {
  border-top-left-radius: 12px;
  border-top-right-radius: 12px;
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
  pointer-events: auto;
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
