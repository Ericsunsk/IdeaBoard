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
    <HomeCanvasHeader
      :current-board-name="currentBoardName"
      :board-options="boardOptions"
      :current-board-id="currentBoardId"
      :is-board-menu-open="isBoardMenuOpen"
      :is-renaming-board="isRenamingBoard"
      :board-name-draft="boardNameDraft"
      :set-board-name-input-ref="setBoardNameInputRef"
      :export-disabled="selectedIds.length === 0"
      :avatar-url="userStore.user?.avatar"
      @update:board-name-draft="boardNameDraft = $event"
      @toggle-board-menu="toggleBoardMenu"
      @start-board-rename="startBoardRename"
      @cancel-board-rename="cancelBoardRename"
      @confirm-board-rename="confirmBoardRename"
      @select-board="handleBoardSelect"
      @export="handleExport"
      @open-center="router.push('/user/center')"
    />

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

    <HomeCanvasConnectionLayer
      :rendered-connections="renderedConnections"
      :selected-connection-id="selectedConnectionId"
      :hidden-connection-id="reconnectingConnectionId"
      :draft-connection-path="draftConnectionPath"
      :draft-connection-kind="draftConnectionKind"
      :draft-connection-tone="draftConnectionTone"
      @path-mousedown="onConnectionPathMouseDown"
    />

    <!-- ⑥ 内容展示区 -->
    <div class="canvas-content">
      <HomeCanvasNodesBasic
        :image-nodes="imageNodes"
        :prompt-nodes="promptNodes"
        :copy-mode="nodeCopyMode"
        :is-node-selected="isNodeSelected"
        :is-node-dragging="isNodeDragging"
        :get-image-node-ui-state="getImageNodeUiState"
        :get-prompt-node-ui-state="getPromptNodeUiState"
        :get-image-node-stage-height="getImageNodeStageHeight"
        :get-resolved-image-node-display="getResolvedImageNodeDisplay"
        @image-node-mousedown="(event, node) => onImageNodeMouseDown(event, node)"
        @image-port-mousedown="onImageNodePortMouseDown"
        @image-delete="deleteImageNode"
        @image-stage-drop="(event, node) => onImageNodeStageDrop(event, node)"
        @image-open-file="openImageNodeFilePicker"
        @image-append-to-canvas="appendImageNodeDisplayToCanvas"
        @image-reset="resetImageNode"
        @image-display-dragstart="(event, node) => onImageNodeDisplayDragStart(event, node)"
        @prompt-node-mousedown="(event, node) => onPromptNodeMouseDown(event, node)"
        @prompt-port-mousedown="onPromptNodePortMouseDown"
        @prompt-delete="deletePromptNode"
        @prompt-focus="activatePromptNode"
        @prompt-update-text="onPromptNodeTextUpdate"
      />

      <HomeCanvasNodesStructured
        :model-nodes="modelNodes"
        :mannequin-nodes="mannequinNodes"
        :viewpoint-nodes="viewpointNodes"
        :mask-nodes="maskNodes"
        :generating="generating"
        :copy-mode="nodeCopyMode"
        :is-node-selected="isNodeSelected"
        :is-node-dragging="isNodeDragging"
        :get-model-node-ui-state="getModelNodeUiState"
        :get-mannequin-node-ui-state="getMannequinNodeUiState"
        :get-viewpoint-node-ui-state="getViewpointNodeUiState"
        :get-mask-node-ui-state="getMaskNodeUiState"
        :get-node-connection-error-ports="getNodeConnectionErrorPorts"
        :get-model-node-stage-height="getModelNodeStageHeight"
        :get-model-node-effective-source="getModelNodeEffectiveSource"
        :get-mannequin-node-stage-height="getMannequinNodeStageHeight"
        :get-mannequin-node-effective-source="getMannequinNodeEffectiveSource"
        :get-viewpoint-node-stage-height="getViewpointNodeStageHeight"
        :get-viewpoint-node-effective-source="getViewpointNodeEffectiveSource"
        :get-viewpoint-node-effective-source-images="getViewpointNodeEffectiveSourceImages"
        :get-mask-node-stage-height="getMaskNodeStageHeight"
        :get-mask-canvas-width="getMaskCanvasWidth"
        :get-mask-node-effective-source="getMaskNodeEffectiveSource"
        :model-angle-options="modelAngleOptions"
        :model-background-options="modelBackgroundOptions"
        @model-node-mousedown="(event, node) => onModelNodeMouseDown(event, node)"
        @model-port-mousedown="onModelNodePortMouseDown"
        @model-delete="deleteModelNode"
        @model-open-file-picker="openModelNodeFilePicker"
        @model-clear-image="clearModelNodeImage"
        @model-stage-drop="(event, node) => onModelStageDrop(event, node)"
        @model-toggle-angle="(node, key) => toggleModelAngle(node, key)"
        @model-clear-angles="clearModelAngles"
        @model-select-background="onModelNodeSelectBackground"
        @model-generate="handleModelNodeGenerate"
        @mannequin-node-mousedown="(event, node) => onMannequinNodeMouseDown(event, node)"
        @mannequin-port-mousedown="onMannequinNodePortMouseDown"
        @mannequin-delete="deleteMannequinNode"
        @mannequin-open-file-picker="openMannequinNodeFilePicker"
        @mannequin-clear-image="clearMannequinNodeImage"
        @mannequin-stage-drop="(event, node) => handleMannequinStageDrop(event, node)"
        @mannequin-generate="handleMannequinNodeGenerate"
        @viewpoint-node-mousedown="(event, node) => onViewpointNodeMouseDown(event, node)"
        @viewpoint-port-mousedown="onViewpointNodePortMouseDown"
        @viewpoint-delete="deleteViewpointNode"
        @viewpoint-open-file-picker="openViewpointNodeFilePicker"
        @viewpoint-clear-images="clearViewpointNodeImages"
        @viewpoint-remove-image="(node, index) => removeViewpointNodeImage(node, index)"
        @viewpoint-stage-drop="(event, node) => handleViewpointStageDrop(event, node)"
        @viewpoint-update-category="(node, category) => setViewpointNodeCategory(node, category)"
        @viewpoint-toggle-selection="(node, key) => toggleViewpointSelection(node, key)"
        @viewpoint-update-lighting="onViewpointNodeUpdateLighting"
        @viewpoint-generate="handleViewpointNodeGenerate"
        @mask-node-mousedown="(event, node) => onMaskNodeMouseDown(event, node)"
        @mask-port-mousedown="onMaskNodePortMouseDown"
        @mask-delete="deleteMaskNode"
        @mask-open-file-picker="openMaskNodeFilePicker"
        @mask-clear-image="clearMaskNodeImage"
        @mask-stage-drop="(event, node) => onMaskStageDrop(event, node)"
        @mask-canvas-mousedown="onMaskNodeCanvasMouseDownBridge"
        @mask-set-draw-mode="onMaskNodeSetDrawMode"
        @mask-update-brush-size="onMaskNodeUpdateBrushSize"
        @mask-update-refine-amount="onMaskNodeUpdateRefineAmount"
        @mask-toggle-invert="onMaskNodeToggleInvert"
        @mask-clear-mask="clearMaskNodeMask"
        @mask-set-canvas-ref="setMaskCanvasRef"
      />

      <HomeCanvasNodesExecution
        :edit-nodes="editNodes"
        :fusion-nodes="fusionNodes"
        :enhance-nodes="enhanceNodes"
        :try-on-nodes="tryOnNodes"
        :face-swap-nodes="faceSwapNodes"
        :color-nodes="colorNodes"
        :generate-nodes="generateNodes"
        :generating="generating"
        :copy-mode="nodeCopyMode"
        :is-node-selected="isNodeSelected"
        :is-node-dragging="isNodeDragging"
        :get-edit-node-ui-state="getEditNodeUiState"
        :get-fusion-node-ui-state="getFusionNodeUiState"
        :get-enhance-node-ui-state="getEnhanceNodeUiState"
        :get-try-on-node-ui-state="getTryOnNodeUiState"
        :get-face-swap-node-ui-state="getFaceSwapNodeUiState"
        :get-color-node-ui-state="getColorNodeUiState"
        :get-generate-node-ui-state="getGenerateNodeUiState"
        :get-node-connection-error-ports="getNodeConnectionErrorPorts"
        :get-generate-style-options="getGenerateStyleOptions"
        :is-generate-style-loading="isGenerateStyleLoading"
        @edit-node-mousedown="(event, node) => onEditNodeMouseDown(event, node)"
        @edit-delete="deleteEditNode"
        @edit-update-mode="onEditNodeUpdateMode"
        @edit-generate="handleEditNodeGenerate"
        @edit-port-mousedown="onEditNodePortMouseDown"
        @fusion-node-mousedown="(event, node) => onFusionNodeMouseDown(event, node)"
        @fusion-delete="deleteFusionNode"
        @fusion-update-blend-ratio="onFusionNodeUpdateBlendRatio"
        @fusion-generate="handleFusionNodeGenerate"
        @fusion-port-mousedown="onFusionNodePortMouseDown"
        @enhance-node-mousedown="(event, node) => onEnhanceNodeMouseDown(event, node)"
        @enhance-delete="deleteEnhanceNode"
        @enhance-update-mode="onEnhanceNodeUpdateMode"
        @enhance-update-upscale-mode="onEnhanceNodeUpdateUpscaleMode"
        @enhance-update-upscale-factor="onEnhanceNodeUpdateUpscaleFactor"
        @enhance-update-output-format="onEnhanceNodeUpdateOutputFormat"
        @enhance-update-output-quality="onEnhanceNodeUpdateOutputQuality"
        @enhance-generate="handleEnhanceNodeGenerate"
        @enhance-port-mousedown="onEnhanceNodePortMouseDown"
        @tryon-node-mousedown="(event, node) => onTryOnNodeMouseDown(event, node)"
        @tryon-delete="deleteTryOnNode"
        @tryon-update-mode="onTryOnNodeUpdateMode"
        @tryon-update-tile-mode="onTryOnNodeUpdateTileMode"
        @tryon-update-rotation="onTryOnNodeUpdateRotation"
        @tryon-update-scale="onTryOnNodeUpdateScale"
        @tryon-update-opacity="onTryOnNodeUpdateOpacity"
        @tryon-generate="handleTryOnNodeGenerate"
        @tryon-port-mousedown="onTryOnNodePortMouseDown"
        @faceswap-node-mousedown="(event, node) => onFaceSwapNodeMouseDown(event, node)"
        @faceswap-delete="deleteFaceSwapNode"
        @faceswap-generate="handleFaceSwapNodeGenerate"
        @faceswap-port-mousedown="onFaceSwapNodePortMouseDown"
        @color-node-mousedown="(event, node) => onColorNodeMouseDown(event, node)"
        @color-delete="deleteColorNode"
        @color-apply-preset="onColorNodeApplyPreset"
        @color-remove-color="(node, index) => removeColorNodeColor(node, index)"
        @color-add-color="(node, value) => addColorNodeColor(node, value)"
        @color-strength-input="onColorNodeStrengthInput"
        @color-generate="handleColorNodeGenerate"
        @color-port-mousedown="onColorNodePortMouseDown"
        @generate-node-mousedown="(event, node) => onGenerateNodeMouseDown(event, node)"
        @generate-delete="deleteGenerateNode"
        @generate-update-mode="onGenerateNodeUpdateMode"
        @generate-update-style-model="onGenerateNodeUpdateStyleModel"
        @generate-update-ratio="onGenerateNodeUpdateRatio"
        @generate-update-count="onGenerateNodeUpdateCount"
        @generate-update-quality="onGenerateNodeUpdateQuality"
        @generate-generate="handleGenerateNodeGenerate"
        @generate-port-mousedown="onGenerateNodePortMouseDown"
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
import { useCanvasNodeConnections } from '../../composables/useCanvasNodeConnections'
import { useCanvasNodeUiState } from '../../composables/useCanvasNodeUiState'
import { useCanvasDirectActions, type SegType } from '../../composables/useCanvasDirectActions'
import { useGenerationPipeline } from '../../composables/useGenerationPipeline'
import { useImageNode } from '../../composables/useImageNode'
import { useMannequinNode } from '../../composables/useMannequinNode'
import { clampMaskRefineAmount, useMaskNode } from '../../composables/useMaskNode'
import { useModelNode } from '../../composables/useModelNode'
import { useNodeExecutors } from '../../composables/useNodeExecutors'
import { useViewpointNode } from '../../composables/useViewpointNode'
import HomeCanvasConnectionLayer from './home/HomeCanvasConnectionLayer.vue'
import HomeCanvasHeader from './home/HomeCanvasHeader.vue'
import HomeCanvasNodesBasic from './home/HomeCanvasNodesBasic.vue'
import HomeCanvasNodesExecution from './home/HomeCanvasNodesExecution.vue'
import HomeCanvasNodesStructured from './home/HomeCanvasNodesStructured.vue'
import {
  MODEL_ANGLE_OPTIONS,
  MODEL_BACKGROUND_OPTIONS,
  VIEWPOINT_LIGHTING_LABELS,
  VIEWPOINT_OPTIONS_BY_CATEGORY,
} from '../../components/canvas/nodes/nodeOptionConfig'
import { normalizeHexColor } from '../../components/canvas/nodes/colorPopoverUtils'
import {
  enhanceModeSupportsPrompt,
} from '../../config/canvasNodeConnectionContract'
import { isCanvasConnectionAllowed } from '../../config/canvasConnections'
import {
  createCanvasImageItemAt,
  createEnhanceNodeAt,
  createEditNodeAt,
  createFusionNodeAt,
  createFaceSwapNodeAt,
  createGenerateNodeAt,
  createImageNodeAt,
  createMannequinNodeAt,
  createMaskNodeAt,
  createModelNodeAt,
  createColorNodeAt,
  createPromptNodeAt,
  createTryOnNodeAt,
  createViewpointNodeAt,
} from '../../utils/canvasNodeFactory'
import {
  DEFAULT_COLOR_STRENGTH,
  isCanvasExecutionOutputNodeType,
  isColorNodePlaceholderState,
  MAX_COLOR_NODE_SWATCHES,
  normalizeColorNodeColors,
  resolveWorkflowNodeOutputStatus,
} from '../../types/canvasNodes'
import type {
  CanvasBoardSnapshot,
  CanvasConnection,
  CanvasExecutionOutputNodeType,
  CanvasExecutionOutputNodeItem,
  CanvasConnectionNodeType,
  CanvasConnectionPort,
  CanvasImageItem,
  CanvasNodeItem,
  ColorNodeItem,
  EnhanceNodeItem,
  EditNodeItem,
  FusionNodeItem,
  FaceSwapNodeItem,
  GenerateNodeItem,
  ImageNodeItem,
  MannequinNodeItem,
  MaskNodeItem,
  ModelNodeItem,
  PromptNodeItem,
  TryOnNodeItem,
  ViewpointNodeItem,
  WorkflowNodeOutputStatus,
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
const viewpointLightingLabels = VIEWPOINT_LIGHTING_LABELS
const viewpointLabelMap = Object.values(VIEWPOINT_OPTIONS_BY_CATEGORY).reduce<Record<string, string>>((acc, options) => {
  options.forEach((option) => {
    acc[option.key] = option.label
  })
  return acc
}, {})

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
const mannequinNodes = ref<MannequinNodeItem[]>([])
const viewpointNodes = ref<ViewpointNodeItem[]>([])
const maskNodes = ref<MaskNodeItem[]>([])
const colorNodes = ref<ColorNodeItem[]>([])
const enhanceNodes = ref<EnhanceNodeItem[]>([])
const editNodes = ref<EditNodeItem[]>([])
const fusionNodes = ref<FusionNodeItem[]>([])
const tryOnNodes = ref<TryOnNodeItem[]>([])
const faceSwapNodes = ref<FaceSwapNodeItem[]>([])
const generateNodes = ref<GenerateNodeItem[]>([])
const executionNodeWatchSources = [
  modelNodes,
  mannequinNodes,
  viewpointNodes,
  colorNodes,
  enhanceNodes,
  editNodes,
  fusionNodes,
  tryOnNodes,
  faceSwapNodes,
  generateNodes,
] as const
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
  'bg-swap': {
    feature: 'bg-swap',
    promptPrefix: '[bg-swap]',
    fallbackPrompt: '换背景',
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
  reconnectingConnectionId,
  hasSelectedConnection,
  clearSelectedConnection,
  selectConnection,
  deleteSelectedConnection,
  findIncomingConnection,
  getIncomingConnections,
  hasIncomingPortConnection,
  getOutgoingConnections,
  hasOutgoingPortConnection,
  removeConnectionsForNode,
  removeIncomingConnectionsForPort,
  clearNodeConnectionError,
  clearAllNodeConnectionErrors,
  getNodeConnectionErrorPorts,
  triggerNodeConnectionError,
  clearConnectionDraft,
  beginConnectionReconnect,
  refreshRenderedConnections,
  scheduleRenderedConnectionsRefresh,
  startConnectionDrag,
  addGlobalConnectionListeners,
  removeGlobalConnectionListeners,
  disposeConnectionRuntime,
} = useCanvasConnections({
  connections,
  canvasLayoutRef,
  canvasX,
  canvasY,
  zoomLevel,
  lookupNodeByType: getCanvasConnectionNodeByType,
  isConnectionAllowed: (from, to) => isCanvasConnectionAllowed(from, to, getCanvasConnectionNodeByType),
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
  activeMannequinNodeId,
  activeViewpointNodeId,
  activeMaskNodeId,
  activeColorNodeId,
  activeEditNodeId,
  activeFusionNodeId,
  activeEnhanceNodeId,
  activeTryOnNodeId,
  activeFaceSwapNodeId,
  activeGenerateNodeId,
  nodeContextMenu,
  isSelecting,
  selectionRect,
  activateImageNode,
  activatePromptNode,
  activateModelNode,
  activateMannequinNode,
  activateViewpointNode,
  activateMaskNode,
  activateColorNode,
  activateEditNode,
  activateFusionNode,
  activateEnhanceNode,
  activateTryOnNode,
  activateFaceSwapNode,
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
  onMannequinNodeMouseDown,
  onViewpointNodeMouseDown,
  onMaskNodeMouseDown,
  onEditNodeMouseDown,
  onFusionNodeMouseDown,
  onEnhanceNodeMouseDown,
  onTryOnNodeMouseDown,
  onFaceSwapNodeMouseDown,
  onColorNodeMouseDown,
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
  mannequinNodes,
  viewpointNodes,
  maskNodes,
  colorNodes,
  enhanceNodes,
  editNodes,
  fusionNodes,
  tryOnNodes,
  faceSwapNodes,
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
  confirmBoardRename: () => confirmBoardRename(),
  resolveLeaveConfirm: (decision) => resolveLeaveConfirm(decision),
  updateZoom,
  getCanvasPointFromClient,
  createCanvasImageAt: createCanvasImageItemAt,
  uploadImage: uploadApi.upload,
  importCanvasImageToImageNodeAtPoint: (clientX, clientY, item) => importCanvasImageToImageNodeAtPoint(clientX, clientY, item),
  importCanvasImageToMaskNodeAtPoint: (clientX, clientY, item) => importCanvasImageToMaskNodeAtPoint(clientX, clientY, item),
  importCanvasImageToModelNodeAtPoint: (clientX, clientY, item) => importCanvasImageToModelNodeAtPoint(clientX, clientY, item),
  importCanvasImageToMannequinNodeAtPoint: (clientX, clientY, item) => importCanvasImageToMannequinNodeAtPoint(clientX, clientY, item),
  importCanvasImageToViewpointNodeAtPoint: (clientX, clientY, item) => importCanvasImageToViewpointNodeAtPoint(clientX, clientY, item),
  deleteImageNode,
  deletePromptNode,
  deleteModelNode,
  deleteMannequinNode,
  deleteViewpointNode,
  deleteMaskNode,
  deleteColorNode,
  deleteEnhanceNode,
  deleteEditNode,
  deleteFusionNode,
  deleteTryOnNode,
  deleteFaceSwapNode,
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
  getMannequinStageHeight,
  openMannequinNodeFilePicker,
  clearMannequinNodeImage,
  onMannequinStageDrop: handleMannequinStageDrop,
  importCanvasImageToMannequinNodeAtPoint,
} = useMannequinNode({
  mannequinNodes,
  message,
  uploadImage: uploadApi.upload,
  loadImageRatioFromFile,
  loadImageRatioFromUrl,
  readNodeImageDropPayload,
})

const {
  getViewpointStageHeight,
  openViewpointNodeFilePicker,
  clearViewpointNodeImages,
  removeViewpointNodeImage,
  onViewpointStageDrop: handleViewpointStageDrop,
  importCanvasImageToViewpointNodeAtPoint,
  toggleViewpointSelection,
} = useViewpointNode({
  viewpointNodes,
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

function getExecutionOutputNodeByType(
  nodeType: CanvasExecutionOutputNodeType,
  nodeId: number,
) {
  const node = getCanvasConnectionNodeByType(nodeType, nodeId)
  if (!node || !('outputStatus' in node)) return null
  return node as CanvasExecutionOutputNodeItem
}

function setWorkflowNodeOutputStatus(
  node: { outputImages: { url: string }[]; outputStatus?: WorkflowNodeOutputStatus },
  status?: WorkflowNodeOutputStatus,
) {
  node.outputStatus = status ?? (node.outputImages.length > 0 ? 'ready' : 'idle')
}

function cloneNodeOutputImages(outputs: { url: string; ratio: number }[]) {
  return outputs.map(output => ({ ...output }))
}

function toNormalizedColorNode(node: ColorNodeItem): ColorNodeItem {
  const colors = normalizeColorNodeColors(node.colors.map(color => normalizeHexColor(color)))
  return {
    ...node,
    outputStatus: resolveWorkflowNodeOutputStatus(node.outputStatus, node.outputImages),
    colors,
    strength: Math.max(0, Math.min(100, Math.round(node.strength ?? DEFAULT_COLOR_STRENGTH))),
    outputImages: cloneNodeOutputImages(node.outputImages),
  }
}

function markExecutionNodeDirty(
  nodeType: CanvasExecutionOutputNodeType,
  nodeId: number,
  visited = new Set<string>(),
) {
  const visitKey = `${nodeType}:${nodeId}`
  if (visited.has(visitKey)) return
  visited.add(visitKey)

  const node = getExecutionOutputNodeByType(nodeType, nodeId)
  if (!node) return
  setWorkflowNodeOutputStatus(node, node.outputImages.length > 0 ? 'stale' : 'idle')

  getOutgoingConnections(nodeType, nodeId, 'image').forEach((connection) => {
    if (isCanvasExecutionOutputNodeType(connection.toNodeType)) {
      markExecutionNodeDirty(connection.toNodeType, connection.toNodeId, visited)
      return
    }
    if (connection.toNodeType === 'mask') {
      markConnectedExecutionTargetsDirty('mask', connection.toNodeId, visited)
    }
  })
}

function markConnectedExecutionTargetsDirty(
  fromNodeType: CanvasConnectionNodeType,
  fromNodeId: number,
  visited = new Set<string>(),
) {
  const outgoingPort =
    fromNodeType === 'prompt'
      ? 'prompt'
      : fromNodeType === 'mask'
        ? 'mask'
        : 'image'
  getOutgoingConnections(fromNodeType, fromNodeId, outgoingPort).forEach((connection) => {
    if (isCanvasExecutionOutputNodeType(connection.toNodeType)) {
      markExecutionNodeDirty(connection.toNodeType, connection.toNodeId, visited)
      return
    }
    if (connection.toNodeType === 'mask') {
      markConnectedExecutionTargetsDirty('mask', connection.toNodeId, visited)
    }
  })
}

function appendWorkflowNodeOutputs(
  nodeType: CanvasExecutionOutputNodeType,
  nodeId: number,
  urls: string[],
) {
  if (urls.length === 0) return
  const node = getExecutionOutputNodeByType(nodeType, nodeId)
  if (!node) return
  setWorkflowNodeOutputStatus(node, 'ready')
  const startIndex = node.outputImages.length
  urls.forEach((url) => {
    node.outputImages.push({ url, ratio: 1 })
  })
  urls.forEach((url, offset) => {
    const outputIndex = startIndex + offset
    void loadImageRatioFromUrl(url).then((ratio) => {
      if (!ratio) return
      const targetNode = getExecutionOutputNodeByType(nodeType, nodeId)
      if (!targetNode) return
      const output = targetNode.outputImages[outputIndex]
      if (!output || output.url !== url) return
      output.ratio = ratio
    })
  })
}

function getModelAngleLabel(key: string): string {
  return multiAngleLabelMap[key] || key
}

function getViewpointLabel(key: string): string {
  return viewpointLabelMap[key] || key
}

function getViewpointLightingLabel(key: ViewpointNodeItem['lighting']): string {
  return viewpointLightingLabels[key] || key
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
  mannequinNodes.value = []
  viewpointNodes.value = []
  maskNodes.value = []
  colorNodes.value = []
  enhanceNodes.value = []
  editNodes.value = []
  fusionNodes.value = []
  tryOnNodes.value = []
  faceSwapNodes.value = []
  generateNodes.value = []
  connections.value = []
  processedMaskPreviewUrls.value = {}
  processedMaskPreviewSignatures.clear()
  clearConnectionDraft()
  userPrompt.value = ''
  resetViewport()
}

let suspendWorkflowCacheReactions = false
let previousPromptSignatureMap: Record<string, string> = {}
let previousImageSourceSignatureMap: Record<string, string> = {}
let previousMaskSignatureMap: Record<string, string> = {}
let previousExecutionInputSignatureMap: Record<string, string> = {}
let previousExecutionOutputSignatureMap: Record<string, string> = {}
let previousConnectionTargetSignatureMap: Record<string, string> = {}

function buildPromptSignatureMap(): Record<string, string> {
  return promptNodes.value.reduce<Record<string, string>>((acc, node) => {
    acc[String(node.id)] = node.text
    return acc
  }, {})
}

function buildImageSourceSignatureMap(): Record<string, string> {
  return imageNodes.value.reduce<Record<string, string>>((acc, node) => {
    const latestOutput = node.outputImages[node.outputImages.length - 1]
    acc[String(node.id)] = [
      node.sourceImageUrl,
      String(node.sourceImageRatio || 1),
      latestOutput?.url || '',
      String(latestOutput?.ratio || 1),
      String(node.outputImages.length),
    ].join('|')
    return acc
  }, {})
}

function buildMaskSignatureMap(): Record<string, string> {
  return maskNodes.value.reduce<Record<string, string>>((acc, node) => {
    acc[String(node.id)] = [
      node.sourceImageUrl,
      String(node.sourceImageRatio || 1),
      node.maskDataUrl,
      String(clampMaskRefineAmount(node.refineAmount ?? 0)),
      node.invertMask ? '1' : '0',
    ].join('|')
    return acc
  }, {})
}

function buildExecutionInputSignatureMap(): Record<string, string> {
  const result: Record<string, string> = {}

  modelNodes.value.forEach((node) => {
    result[`model:${node.id}`] = [
      node.sourceImageUrl,
      String(node.sourceImageRatio || 1),
      node.selectedAngles.join(','),
      node.background,
    ].join('|')
  })

  mannequinNodes.value.forEach((node) => {
    result[`mannequin:${node.id}`] = [node.sourceImageUrl, String(node.sourceImageRatio || 1)].join('|')
  })

  viewpointNodes.value.forEach((node) => {
    result[`viewpoint:${node.id}`] = [
      node.sourceImages.map(image => `${image.url}@${image.ratio}`).join(','),
      node.selectedViewpoints.join(','),
      node.lighting,
    ].join('|')
  })

  colorNodes.value.forEach((node) => {
    result[`color:${node.id}`] = [
      node.colors.join(','),
      String(node.strength),
    ].join('|')
  })

  editNodes.value.forEach((node) => {
    result[`edit:${node.id}`] = node.mode
  })

  fusionNodes.value.forEach((node) => {
    result[`fusion:${node.id}`] = String(node.blendRatio)
  })

  enhanceNodes.value.forEach((node) => {
    result[`enhance:${node.id}`] = [
      node.mode,
      node.upscaleMode,
      String(node.upscaleFactor),
      node.outputFormat,
      String(node.outputQuality),
    ].join('|')
  })

  tryOnNodes.value.forEach((node) => {
    result[`tryon:${node.id}`] = [
      node.mode,
      node.tileMode,
      String(node.rotation),
      String(node.scale),
      String(node.opacity),
    ].join('|')
  })

  faceSwapNodes.value.forEach((node) => {
    result[`faceswap:${node.id}`] = String(node.id)
  })

  generateNodes.value.forEach((node) => {
    result[`generate:${node.id}`] = [
      node.mode,
      node.styleModel,
      node.ratio,
      String(node.count),
      node.quality,
    ].join('|')
  })

  return result
}

function buildExecutionOutputSignatureMap(): Record<string, string> {
  const result: Record<string, string> = {}
  const assign = (nodeType: CanvasExecutionOutputNodeType, nodes: { id: number; outputImages: { url: string; ratio: number }[] }[]) => {
    nodes.forEach((node) => {
      const latestOutput = node.outputImages[node.outputImages.length - 1]
      result[`${nodeType}:${node.id}`] = [
        latestOutput?.url || '',
        String(latestOutput?.ratio || 1),
        String(node.outputImages.length),
      ].join('|')
    })
  }

  assign('model', modelNodes.value)
  assign('mannequin', mannequinNodes.value)
  assign('viewpoint', viewpointNodes.value)
  assign('color', colorNodes.value)
  assign('edit', editNodes.value)
  assign('fusion', fusionNodes.value)
  assign('enhance', enhanceNodes.value)
  assign('tryon', tryOnNodes.value)
  assign('faceswap', faceSwapNodes.value)
  assign('generate', generateNodes.value)

  return result
}

function buildConnectionTargetSignatureMap(): Record<string, string> {
  const grouped: Record<string, string[]> = {}
  connections.value.forEach((connection) => {
    const targetKey = `${connection.toNodeType}:${connection.toNodeId}`
    if (!grouped[targetKey]) {
      grouped[targetKey] = []
    }
    grouped[targetKey].push(`${connection.fromNodeType}:${connection.fromNodeId}:${connection.fromPort}->${connection.toPort}`)
  })
  return Object.entries(grouped).reduce<Record<string, string>>((acc, [key, values]) => {
    acc[key] = values.sort().join('|')
    return acc
  }, {})
}

function primeWorkflowCacheSignatureMaps() {
  previousPromptSignatureMap = buildPromptSignatureMap()
  previousImageSourceSignatureMap = buildImageSourceSignatureMap()
  previousMaskSignatureMap = buildMaskSignatureMap()
  previousExecutionInputSignatureMap = buildExecutionInputSignatureMap()
  previousExecutionOutputSignatureMap = buildExecutionOutputSignatureMap()
  previousConnectionTargetSignatureMap = buildConnectionTargetSignatureMap()
}

function forEachChangedSignature(
  previousMap: Record<string, string>,
  nextMap: Record<string, string>,
  onChanged: (key: string) => void,
) {
  const keys = new Set([...Object.keys(previousMap), ...Object.keys(nextMap)])
  keys.forEach((key) => {
    if (previousMap[key] !== nextMap[key]) {
      onChanged(key)
    }
  })
}

let resetHistoryBaseline: ((snapshot?: CanvasBoardSnapshot | null) => void) | null = null

async function applyBoardSnapshot(state: CanvasBoardSnapshot | null) {
  suspendWorkflowCacheReactions = true
  try {
    if (!state) {
      resetEditorState()
      await nextTick()
      primeWorkflowCacheSignatureMaps()
      resetHistoryBaseline?.(createBoardSnapshot())
      return
    }

    messages.value = state.messages.map(item => ({ ...item }))
    resetEditorUiState()
    clearAllNodeConnectionErrors()

    imageNodes.value = state.imageNodes.map(node => ({
      ...node,
      outputImages: cloneNodeOutputImages(node.outputImages),
    }))

    promptNodes.value = state.promptNodes.map(node => ({ ...node }))

    modelNodes.value = state.modelNodes.map(node => ({
      ...node,
      outputStatus: resolveWorkflowNodeOutputStatus(node.outputStatus, node.outputImages),
      selectedAngles: [...node.selectedAngles],
      outputImages: cloneNodeOutputImages(node.outputImages),
    }))
    mannequinNodes.value = state.mannequinNodes.map(node => ({
      ...node,
      outputStatus: resolveWorkflowNodeOutputStatus(node.outputStatus, node.outputImages),
      outputImages: cloneNodeOutputImages(node.outputImages),
    }))
    viewpointNodes.value = state.viewpointNodes.map(node => ({
      ...node,
      outputStatus: resolveWorkflowNodeOutputStatus(node.outputStatus, node.outputImages),
      sourceImages: cloneNodeOutputImages(node.sourceImages),
      selectedViewpoints: [...node.selectedViewpoints],
      outputImages: cloneNodeOutputImages(node.outputImages),
    }))
    maskNodes.value = state.maskNodes.map(node => ({
      ...node,
      refineAmount: clampMaskRefineAmount(node.refineAmount ?? 0),
      invertMask: !!node.invertMask,
    }))
    enhanceNodes.value = state.enhanceNodes.map(node => ({
      ...node,
      outputStatus: resolveWorkflowNodeOutputStatus(node.outputStatus, node.outputImages),
      outputQuality: Math.max(30, Math.min(100, Math.round(node.outputQuality ?? 95))),
      outputImages: cloneNodeOutputImages(node.outputImages),
    }))
    editNodes.value = state.editNodes.map(node => ({
      ...node,
      outputStatus: resolveWorkflowNodeOutputStatus(node.outputStatus, node.outputImages),
      outputImages: cloneNodeOutputImages(node.outputImages),
    }))
    fusionNodes.value = state.fusionNodes.map(node => ({
      ...node,
      blendRatio: Math.max(0, Math.min(100, Math.round(node.blendRatio ?? 50))),
      outputStatus: resolveWorkflowNodeOutputStatus(node.outputStatus, node.outputImages),
      outputImages: cloneNodeOutputImages(node.outputImages),
    }))
    tryOnNodes.value = state.tryOnNodes.map(node => ({
      ...node,
      outputStatus: resolveWorkflowNodeOutputStatus(node.outputStatus, node.outputImages),
      rotation: Math.max(0, Math.min(360, Math.round(node.rotation ?? 0))),
      scale: Math.max(10, Math.min(300, Math.round(node.scale ?? 100))),
      opacity: Math.max(0, Math.min(100, Math.round(node.opacity ?? 100))),
      outputImages: cloneNodeOutputImages(node.outputImages),
    }))
    faceSwapNodes.value = state.faceSwapNodes.map(node => ({
      ...node,
      outputStatus: resolveWorkflowNodeOutputStatus(node.outputStatus, node.outputImages),
      outputImages: cloneNodeOutputImages(node.outputImages),
    }))
    colorNodes.value = state.colorNodes.map(toNormalizedColorNode)
    generateNodes.value = state.generateNodes.map(node => ({
      ...node,
      outputStatus: resolveWorkflowNodeOutputStatus(node.outputStatus, node.outputImages),
      outputImages: cloneNodeOutputImages(node.outputImages),
    }))
    generateNodes.value.forEach((node) => {
      if (generateModeSupportsStyleModel(node.mode)) {
        void ensureGenerateStyleOptions(node.mode)
      }
    })
    connections.value = state.connections.map(connection => ({ ...connection }))
    clearSelectedConnection()
    clearConnectionDraft()
    setViewport(state.viewport)

    await nextTick()
    maskNodes.value.forEach(node => renderMaskNodeMask(node))
    scheduleRenderedConnectionsRefresh()
    primeWorkflowCacheSignatureMaps()
    resetHistoryBaseline?.(createBoardSnapshot())
  } finally {
    suspendWorkflowCacheReactions = false
  }
}

function createBoardSnapshot(): CanvasBoardSnapshot {
  return {
    messages: messages.value.map(item => ({ ...item })),
    imageNodes: imageNodes.value.map(node => ({
      ...node,
      outputImages: cloneNodeOutputImages(node.outputImages),
    })),
    promptNodes: promptNodes.value.map(node => ({ ...node })),
    modelNodes: modelNodes.value.map(node => ({
      ...node,
      selectedAngles: [...node.selectedAngles],
      outputImages: cloneNodeOutputImages(node.outputImages),
    })),
    mannequinNodes: mannequinNodes.value.map(node => ({
      ...node,
      outputImages: cloneNodeOutputImages(node.outputImages),
    })),
    viewpointNodes: viewpointNodes.value.map(node => ({
      ...node,
      sourceImages: cloneNodeOutputImages(node.sourceImages),
      selectedViewpoints: [...node.selectedViewpoints],
      outputImages: cloneNodeOutputImages(node.outputImages),
    })),
    maskNodes: maskNodes.value.map(node => ({ ...node })),
    colorNodes: colorNodes.value.map(toNormalizedColorNode),
    enhanceNodes: enhanceNodes.value.map(node => ({
      ...node,
      outputImages: cloneNodeOutputImages(node.outputImages),
    })),
    editNodes: editNodes.value.map(node => ({
      ...node,
      outputImages: cloneNodeOutputImages(node.outputImages),
    })),
    fusionNodes: fusionNodes.value.map(node => ({
      ...node,
      outputImages: cloneNodeOutputImages(node.outputImages),
    })),
    tryOnNodes: tryOnNodes.value.map(node => ({
      ...node,
      outputImages: cloneNodeOutputImages(node.outputImages),
    })),
    faceSwapNodes: faceSwapNodes.value.map(node => ({
      ...node,
      outputImages: cloneNodeOutputImages(node.outputImages),
    })),
    generateNodes: generateNodes.value.map(node => ({
      ...node,
      outputImages: cloneNodeOutputImages(node.outputImages),
    })),
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

primeWorkflowCacheSignatureMaps()

function setBoardNameInputRef(element: HTMLInputElement | null) {
  boardNameInputRef.value = element
}

watch(
  promptNodes,
  () => {
    const nextMap = buildPromptSignatureMap()
    if (suspendWorkflowCacheReactions) {
      previousPromptSignatureMap = nextMap
      return
    }
    forEachChangedSignature(previousPromptSignatureMap, nextMap, (key) => {
      const nodeId = Number(key)
      if (Number.isFinite(nodeId)) {
        markConnectedExecutionTargetsDirty('prompt', nodeId)
      }
    })
    previousPromptSignatureMap = nextMap
  },
  { deep: true },
)

watch(
  imageNodes,
  () => {
    const nextMap = buildImageSourceSignatureMap()
    if (suspendWorkflowCacheReactions) {
      previousImageSourceSignatureMap = nextMap
      return
    }
    forEachChangedSignature(previousImageSourceSignatureMap, nextMap, (key) => {
      const nodeId = Number(key)
      if (Number.isFinite(nodeId)) {
        markConnectedExecutionTargetsDirty('image', nodeId)
      }
    })
    previousImageSourceSignatureMap = nextMap
  },
  { deep: true },
)

watch(
  maskNodes,
  () => {
    const nextMap = buildMaskSignatureMap()
    if (suspendWorkflowCacheReactions) {
      previousMaskSignatureMap = nextMap
      return
    }
    forEachChangedSignature(previousMaskSignatureMap, nextMap, (key) => {
      const nodeId = Number(key)
      if (Number.isFinite(nodeId)) {
        markConnectedExecutionTargetsDirty('mask', nodeId)
      }
    })
    previousMaskSignatureMap = nextMap
  },
  { deep: true },
)

watch(
  executionNodeWatchSources,
  () => {
    const nextMap = buildExecutionInputSignatureMap()
    if (suspendWorkflowCacheReactions) {
      previousExecutionInputSignatureMap = nextMap
      return
    }
    forEachChangedSignature(previousExecutionInputSignatureMap, nextMap, (key) => {
      const [nodeType, rawNodeId] = key.split(':')
      const nodeId = Number(rawNodeId)
      if (isCanvasExecutionOutputNodeType(nodeType) && Number.isFinite(nodeId)) {
        markExecutionNodeDirty(nodeType, nodeId)
      }
    })
    previousExecutionInputSignatureMap = nextMap
  },
  { deep: true },
)

watch(
  executionNodeWatchSources,
  () => {
    const nextMap = buildExecutionOutputSignatureMap()
    if (suspendWorkflowCacheReactions) {
      previousExecutionOutputSignatureMap = nextMap
      return
    }
    forEachChangedSignature(previousExecutionOutputSignatureMap, nextMap, (key) => {
      const [nodeType, rawNodeId] = key.split(':')
      const nodeId = Number(rawNodeId)
      if (!isCanvasExecutionOutputNodeType(nodeType) || !Number.isFinite(nodeId)) return
      const node = getExecutionOutputNodeByType(nodeType, nodeId)
      if (!node) return
      setWorkflowNodeOutputStatus(node)
      markConnectedExecutionTargetsDirty(nodeType, nodeId)
    })
    previousExecutionOutputSignatureMap = nextMap
  },
  { deep: true },
)

watch(
  connections,
  () => {
    const nextMap = buildConnectionTargetSignatureMap()
    if (suspendWorkflowCacheReactions) {
      previousConnectionTargetSignatureMap = nextMap
      return
    }
    forEachChangedSignature(previousConnectionTargetSignatureMap, nextMap, (key) => {
      const [nodeType, rawNodeId] = key.split(':')
      const nodeId = Number(rawNodeId)
      if (!Number.isFinite(nodeId)) return
      if (isCanvasExecutionOutputNodeType(nodeType)) {
        markExecutionNodeDirty(nodeType, nodeId)
        return
      }
      if (nodeType === 'mask') {
        markConnectedExecutionTargetsDirty('mask', nodeId)
      }
    })
    previousConnectionTargetSignatureMap = nextMap
  },
  { deep: true },
)

watch(
  [messages, imageNodes, promptNodes, modelNodes, mannequinNodes, viewpointNodes, maskNodes, colorNodes, enhanceNodes, editNodes, fusionNodes, tryOnNodes, faceSwapNodes, generateNodes, connections, canvasX, canvasY, zoomLevel],
  () => {
    markBoardDirty()
    notifySnapshotChanged()
  },
  { deep: true }
)

watch(
  [connections, imageNodes, promptNodes, modelNodes, mannequinNodes, viewpointNodes, maskNodes, colorNodes, enhanceNodes, editNodes, fusionNodes, tryOnNodes, faceSwapNodes, generateNodes, canvasX, canvasY, zoomLevel],
  () => {
    refreshRenderedConnections()
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

function deleteMannequinNode(id: number) {
  deleteNodeFromCollection(mannequinNodes, 'mannequin', id, () => {
    if (activeMannequinNodeId.value === id) {
      activeMannequinNodeId.value = null
    }
  })
}

function deleteViewpointNode(id: number) {
  deleteNodeFromCollection(viewpointNodes, 'viewpoint', id, () => {
    if (activeViewpointNodeId.value === id) {
      activeViewpointNodeId.value = null
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

function deleteFaceSwapNode(id: number) {
  deleteNodeFromCollection(faceSwapNodes, 'faceswap', id, () => {
    if (activeFaceSwapNodeId.value === id) {
      activeFaceSwapNodeId.value = null
    }
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
  createMannequinNode: buildNodeCreateHandler(createMannequinNodeAt, mannequinNodes, activateMannequinNode),
  createViewpointNode: buildNodeCreateHandler(createViewpointNodeAt, viewpointNodes, activateViewpointNode),
  createMaskNode: buildNodeCreateHandler(createMaskNodeAt, maskNodes, activateMaskNode),
  createEditNode: buildNodeCreateHandler(createEditNodeAt, editNodes, activateEditNode),
  createFusionNode: buildNodeCreateHandler(createFusionNodeAt, fusionNodes, activateFusionNode),
  createEnhanceNode: buildNodeCreateHandler(createEnhanceNodeAt, enhanceNodes, activateEnhanceNode),
  createTryOnNode: buildNodeCreateHandler(createTryOnNodeAt, tryOnNodes, activateTryOnNode),
  createFaceSwapNode: buildNodeCreateHandler(createFaceSwapNodeAt, faceSwapNodes, activateFaceSwapNode),
  createColorNode: buildNodeCreateHandler(createColorNodeAt, colorNodes, activateColorNode),
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
  const target = event.currentTarget as HTMLElement | null
  const direction = target?.dataset.portDirection
  if (direction === 'input') {
    const existingConnection = findIncomingConnection(nodeType, nodeId, port)
    if (!existingConnection) return
    beginConnectionReconnect(existingConnection.id)
    return
  }
  startConnectionDrag(event, nodeType, nodeId, port)
}

function onConnectionPathMouseDown(event: MouseEvent, connectionId: number) {
  event.preventDefault()
  event.stopPropagation()
  clearSelectedIds()
  closeNodeContextMenu()
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
const hasDirectPrompt = computed(() => !!userPrompt.value.trim())
const styleCards = ref<StyleCard[]>([])
const selectedCardIds = ref<number[]>([])
const selectedRatio = ref('3:4')
const selectedResolution = ref('1K')

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

function getImageNodeConnectedInput(node: ImageNodeItem): { url: string; ratio: number; mode: 'mask' | 'output' } | null {
  const connection = findIncomingConnection('image', node.id, 'image')
  if (!connection) return null
  if (connection.fromNodeType === 'mask') {
    const sourceNode = maskNodes.value.find(item => item.id === connection.fromNodeId) ?? null
    if (!sourceNode?.maskDataUrl) return null
    const source = getMaskNodeEffectiveSource(sourceNode)
    return {
      url: getMaskNodeProcessedOutputUrl(sourceNode),
      ratio: source.ratio,
      mode: 'mask',
    }
  }
  const resolvedImage = getConnectedImageInput('image', node.id, 'image')
  if (!resolvedImage) return null
  return {
    ...resolvedImage,
    mode: 'output',
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
  return getConnectedImageInput('mask', node.id, 'image')
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
  return getConnectedImageInput('model', node.id, 'image')
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

function getMannequinNodeConnectedImageInput(node: MannequinNodeItem): { url: string; ratio: number } | null {
  return getConnectedImageInput('mannequin', node.id, 'image')
}

function getMannequinNodeEffectiveSource(node: MannequinNodeItem): { url: string; ratio: number; connected: boolean } {
  const connectedImage = getMannequinNodeConnectedImageInput(node)
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

function getMannequinNodeStageHeight(node: MannequinNodeItem): number {
  const source = getMannequinNodeEffectiveSource(node)
  return getMannequinStageHeight(node, source.url, source.ratio)
}

function getViewpointNodeConnectedImageInput(node: ViewpointNodeItem): { url: string; ratio: number } | null {
  return getConnectedImageInput('viewpoint', node.id, 'image')
}

function getViewpointNodeEffectiveSource(node: ViewpointNodeItem): { images: { url: string; ratio: number }[]; connected: boolean } {
  const connectedImage = getViewpointNodeConnectedImageInput(node)
  if (connectedImage) {
    return {
      images: [connectedImage],
      connected: true,
    }
  }
  return {
    images: node.sourceImages.map(image => ({
      url: image.url,
      ratio: image.ratio,
    })),
    connected: false,
  }
}

function getViewpointNodeEffectiveSourceImages(node: ViewpointNodeItem) {
  return getViewpointNodeEffectiveSource(node).images
}

function getViewpointNodeStageHeight(node: ViewpointNodeItem): number {
  const source = getViewpointNodeEffectiveSource(node).images[0]
  return getViewpointStageHeight(node, source?.url ?? '', source?.ratio ?? 1)
}

function setViewpointNodeCategory(node: ViewpointNodeItem, category: ViewpointNodeItem['activeCategory']) {
  node.activeCategory = category
  const allowedKeys = new Set((VIEWPOINT_OPTIONS_BY_CATEGORY[category] ?? []).map(option => option.key))
  node.selectedViewpoints = node.selectedViewpoints.filter(key => allowedKeys.has(key))
}

function getCanvasConnectionNodeByType(
  nodeType: CanvasConnectionNodeType,
  nodeId: number,
): CanvasNodeItem | null {
  switch (nodeType) {
    case 'prompt':
      return promptNodes.value.find(item => item.id === nodeId) ?? null
    case 'image':
      return imageNodes.value.find(item => item.id === nodeId) ?? null
    case 'mask':
      return maskNodes.value.find(item => item.id === nodeId) ?? null
    case 'color':
      return colorNodes.value.find(item => item.id === nodeId) ?? null
    case 'model':
      return modelNodes.value.find(item => item.id === nodeId) ?? null
    case 'mannequin':
      return mannequinNodes.value.find(item => item.id === nodeId) ?? null
    case 'viewpoint':
      return viewpointNodes.value.find(item => item.id === nodeId) ?? null
    case 'edit':
      return editNodes.value.find(item => item.id === nodeId) ?? null
    case 'fusion':
      return fusionNodes.value.find(item => item.id === nodeId) ?? null
    case 'enhance':
      return enhanceNodes.value.find(item => item.id === nodeId) ?? null
    case 'tryon':
      return tryOnNodes.value.find(item => item.id === nodeId) ?? null
    case 'faceswap':
      return faceSwapNodes.value.find(item => item.id === nodeId) ?? null
    case 'generate':
      return generateNodes.value.find(item => item.id === nodeId) ?? null
    default:
      return null
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

function generateModeSupportsStyleModel(mode: GenerateNodeItem['mode']) {
  return generateModeMeta[mode].supportsStyleModel
}

function warnMissingNodeInputs(
  nodeType: CanvasConnectionNodeType,
  nodeId: number,
  missingPorts: string[],
  formatMessage: (state: { disconnectedPorts: string[]; pendingPorts: string[] }) => string,
): boolean {
  if (missingPorts.length === 0) return false
  const { disconnectedPorts, pendingPorts } = getNodeRequiredPortIssues(nodeType, nodeId, missingPorts)
  triggerNodeConnectionError(nodeType, nodeId, missingPorts)
  message.warning(formatMessage({ disconnectedPorts, pendingPorts }))
  return true
}

function setEnhanceNodeMode(node: EnhanceNodeItem, mode: EnhanceNodeItem['mode']) {
  node.mode = mode
  if (enhanceModeSupportsPrompt(mode)) return
  removeIncomingConnectionsForPort('enhance', node.id, 'prompt')
}

function getGenerateStyleOptions(mode: GenerateNodeItem['mode']): string[] {
  return generateStyleOptions.value[mode] ?? []
}

function isGenerateStyleLoading(mode: GenerateNodeItem['mode']): boolean {
  return !!generateStyleLoading.value[mode]
}

async function ensureGenerateStyleOptions(mode: GenerateNodeItem['mode']) {
  if (!generateModeSupportsStyleModel(mode)) return
  if (!userStore.isLoggedIn) return
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

function removeColorNodeColor(node: ColorNodeItem, index: number) {
  if (index < 0 || index >= node.colors.length) return
  node.colors.splice(index, 1)
}

function addColorNodeColor(node: ColorNodeItem, rawColor: string) {
  if (node.colors.length >= MAX_COLOR_NODE_SWATCHES) {
    message.warning('最多保留 8 种颜色')
    return
  }
  const nextColor = normalizeHexColor(rawColor)
  if (node.colors.includes(nextColor)) return
  node.colors.push(nextColor)
}

const {
  getNodeConnectionStatus,
  getConnectedImageInput,
  getConnectedPromptInput,
  getConnectedOutputImageTarget,
  resolveConnectedPlacedImageInput,
  resolveConnectedPlacedMaskInput,
  getMissingRequiredNodeInputs,
} = useCanvasNodeConnections({
  imageNodes,
  promptNodes,
  maskNodes,
  getIncomingConnections,
  hasIncomingPortConnection,
  hasOutgoingPortConnection,
  findIncomingConnection,
  getOutgoingConnections,
  getImageNodePreferredImage,
  lookupNodeByType: getCanvasConnectionNodeByType,
  refineMaskDataUrl,
})

const {
  getNodeRequiredPortIssues,
  getNodeUiState,
} = useCanvasNodeUiState({
  getNodeConnectionStatus,
  lookupNodeByType: getCanvasConnectionNodeByType,
  generateModeSupportsStyleModel,
})

function resolveWorkflowPromptText(
  nodeType: CanvasConnectionNodeType,
  nodeId: number,
  fallback = '',
): string {
  return getConnectedPromptInput(nodeType, nodeId, 'prompt') || fallback
}

const nodeCopyMode = computed(() => isDuplicateModifierPressed.value && activeTool.value === 'select')

const getImageNodeUiState = (node: ImageNodeItem) => getNodeUiState('image', node)
const getPromptNodeUiState = (node: PromptNodeItem) => getNodeUiState('prompt', node)
const getModelNodeUiState = (node: ModelNodeItem) => getNodeUiState('model', node)
const getMannequinNodeUiState = (node: MannequinNodeItem) => getNodeUiState('mannequin', node)
const getViewpointNodeUiState = (node: ViewpointNodeItem) => getNodeUiState('viewpoint', node)
const getMaskNodeUiState = (node: MaskNodeItem) => getNodeUiState('mask', node)
const getEditNodeUiState = (node: EditNodeItem) => getNodeUiState('edit', node)
const getFusionNodeUiState = (node: FusionNodeItem) => getNodeUiState('fusion', node)
const getEnhanceNodeUiState = (node: EnhanceNodeItem) => getNodeUiState('enhance', node)
const getTryOnNodeUiState = (node: TryOnNodeItem) => getNodeUiState('tryon', node)
const getFaceSwapNodeUiState = (node: FaceSwapNodeItem) => getNodeUiState('faceswap', node)
const getColorNodeUiState = (node: ColorNodeItem) => getNodeUiState('color', node)
const getGenerateNodeUiState = (node: GenerateNodeItem) => getNodeUiState('generate', node)

const onImageNodePortMouseDown = (event: MouseEvent, nodeId: number, port: CanvasConnectionPort) =>
  handleConnectionPortMouseDown(event, 'image', nodeId, port)
const onPromptNodePortMouseDown = (event: MouseEvent, nodeId: number, port: CanvasConnectionPort) =>
  handleConnectionPortMouseDown(event, 'prompt', nodeId, port)
const onModelNodePortMouseDown = (event: MouseEvent, nodeId: number, port: CanvasConnectionPort) =>
  handleConnectionPortMouseDown(event, 'model', nodeId, port)
const onMannequinNodePortMouseDown = (event: MouseEvent, nodeId: number, port: CanvasConnectionPort) =>
  handleConnectionPortMouseDown(event, 'mannequin', nodeId, port)
const onViewpointNodePortMouseDown = (event: MouseEvent, nodeId: number, port: CanvasConnectionPort) =>
  handleConnectionPortMouseDown(event, 'viewpoint', nodeId, port)
const onMaskNodePortMouseDown = (event: MouseEvent, nodeId: number, port: CanvasConnectionPort) =>
  handleConnectionPortMouseDown(event, 'mask', nodeId, port)
const onEditNodePortMouseDown = (event: MouseEvent, nodeId: number, port: CanvasConnectionPort) =>
  handleConnectionPortMouseDown(event, 'edit', nodeId, port)
const onFusionNodePortMouseDown = (event: MouseEvent, nodeId: number, port: CanvasConnectionPort) =>
  handleConnectionPortMouseDown(event, 'fusion', nodeId, port)
const onEnhanceNodePortMouseDown = (event: MouseEvent, nodeId: number, port: CanvasConnectionPort) =>
  handleConnectionPortMouseDown(event, 'enhance', nodeId, port)
const onTryOnNodePortMouseDown = (event: MouseEvent, nodeId: number, port: CanvasConnectionPort) =>
  handleConnectionPortMouseDown(event, 'tryon', nodeId, port)
const onFaceSwapNodePortMouseDown = (event: MouseEvent, nodeId: number, port: CanvasConnectionPort) =>
  handleConnectionPortMouseDown(event, 'faceswap', nodeId, port)
const onColorNodePortMouseDown = (event: MouseEvent, nodeId: number, port: CanvasConnectionPort) =>
  handleConnectionPortMouseDown(event, 'color', nodeId, port)
const onGenerateNodePortMouseDown = (event: MouseEvent, nodeId: number, port: CanvasConnectionPort) =>
  handleConnectionPortMouseDown(event, 'generate', nodeId, port)

function onPromptNodeTextUpdate(node: PromptNodeItem, value: string) {
  node.text = value
}

function onModelNodeSelectBackground(node: ModelNodeItem, key: string) {
  node.background = key
}

function onViewpointNodeUpdateLighting(node: ViewpointNodeItem, value: ViewpointNodeItem['lighting']) {
  node.lighting = value
}

function onMaskNodeCanvasMouseDownBridge(event: MouseEvent, node: MaskNodeItem) {
  onMaskCanvasMouseDown(event, getMaskNodeRuntimeNode(node))
}

function onMaskNodeSetDrawMode(node: MaskNodeItem, value: 'paint' | 'erase') {
  node.drawMode = value
}

function onMaskNodeUpdateBrushSize(node: MaskNodeItem, value: number) {
  node.brushSize = value
}

function onMaskNodeUpdateRefineAmount(node: MaskNodeItem, value: number) {
  node.refineAmount = clampMaskRefineAmount(value)
}

function onMaskNodeToggleInvert(node: MaskNodeItem) {
  node.invertMask = !node.invertMask
}

function onEditNodeUpdateMode(node: EditNodeItem, value: EditNodeItem['mode']) {
  node.mode = value
}

function onFusionNodeUpdateBlendRatio(node: FusionNodeItem, value: number) {
  node.blendRatio = Math.max(0, Math.min(100, Math.round(value)))
}

function onEnhanceNodeUpdateMode(node: EnhanceNodeItem, value: EnhanceNodeItem['mode']) {
  setEnhanceNodeMode(node, value)
}

function onEnhanceNodeUpdateUpscaleMode(node: EnhanceNodeItem, value: EnhanceNodeItem['upscaleMode']) {
  node.upscaleMode = value
}

function onEnhanceNodeUpdateUpscaleFactor(node: EnhanceNodeItem, value: EnhanceNodeItem['upscaleFactor']) {
  node.upscaleFactor = value
}

function onEnhanceNodeUpdateOutputFormat(node: EnhanceNodeItem, value: EnhanceNodeItem['outputFormat']) {
  node.outputFormat = value
}

function onEnhanceNodeUpdateOutputQuality(node: EnhanceNodeItem, value: number) {
  node.outputQuality = Math.max(30, Math.min(100, Math.round(value)))
}

function onTryOnNodeUpdateMode(node: TryOnNodeItem, value: TryOnNodeItem['mode']) {
  node.mode = value
}

function onTryOnNodeUpdateTileMode(node: TryOnNodeItem, value: TryOnNodeItem['tileMode']) {
  node.tileMode = value
}

function onTryOnNodeUpdateRotation(node: TryOnNodeItem, value: number) {
  node.rotation = Math.max(0, Math.min(360, Math.round(value)))
}

function onTryOnNodeUpdateScale(node: TryOnNodeItem, value: number) {
  node.scale = Math.max(10, Math.min(300, Math.round(value)))
}

function onTryOnNodeUpdateOpacity(node: TryOnNodeItem, value: number) {
  node.opacity = Math.max(0, Math.min(100, Math.round(value)))
}

function onColorNodeApplyPreset(node: ColorNodeItem, value: string[]) {
  const baseColors = isColorNodePlaceholderState(node) ? [] : node.colors
  node.colors = normalizeColorNodeColors([...baseColors, ...value])
}

function onColorNodeStrengthInput(node: ColorNodeItem, value: number) {
  node.strength = Math.max(0, Math.min(100, Math.round(value)))
}

function onGenerateNodeUpdateMode(node: GenerateNodeItem, value: GenerateNodeItem['mode']) {
  setGenerateNodeMode(node, value)
}

function onGenerateNodeUpdateStyleModel(node: GenerateNodeItem, value: string) {
  node.styleModel = value
}

function onGenerateNodeUpdateRatio(node: GenerateNodeItem, value: GenerateNodeItem['ratio']) {
  node.ratio = value
}

function onGenerateNodeUpdateCount(node: GenerateNodeItem, value: GenerateNodeItem['count']) {
  node.count = value
}

function onGenerateNodeUpdateQuality(node: GenerateNodeItem, value: GenerateNodeItem['quality']) {
  node.quality = value
}

const {
  handleModelNodeGenerate,
  handleMannequinNodeGenerate,
  handleViewpointNodeGenerate,
  handleEditNodeGenerate,
  handleFusionNodeGenerate,
  handleEnhanceNodeGenerate,
  handleTryOnNodeGenerate,
  handleFaceSwapNodeGenerate,
  handleColorNodeGenerate,
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
  modelDefaultPose,
  modelDefaultExpression,
  editModeMeta,
  tryOnModeMeta,
  generateModeMeta,
  getModelAngleLabel,
  getViewpointLabel,
  getViewpointLightingLabel,
  getNodeConnectionStatus,
  triggerNodeConnectionError,
  getModelNodeEffectiveSource,
  resolveNodePromptText: resolveWorkflowPromptText,
  getConnectedOutputImageTarget,
  resolvePreferredOutputNodeId,
  buildPlacementSource,
  getMissingRequiredNodeInputs,
  warnMissingNodeInputs,
  submitTaskAndPoll,
  appendGeneratedImages,
  appendWorkflowNodeOutputs,
  normalizeHexColor,
  getImageNodePreferredImage,
  resolveConnectedPlacedImageInput,
  resolveConnectedPlacedMaskInput,
  buildFullMaskDataUrl,
  refineMaskDataUrl,
  dataUrlToFile,
  uploadImage: uploadApi.upload,
  upscaleImage: upscaleApi.upscale,
  segmentImage: segApi.segment,
  saveSegmentResult: segApi.save,
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
  if (!userStore.isLoggedIn) {
    addGlobalConnectionListeners()
    nextTick(() => {
      scheduleRenderedConnectionsRefresh()
    })
    return
  }
  try {
    const res: any = await styleCardApi.getEnabled()
    if (res.code === 0) styleCards.value = res.data.list
  } catch (error) {
    console.error('[Home] Failed to load enabled style cards', error)
  }
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

.canvas-content { position: absolute; inset: 0; z-index: 2; pointer-events: none; transform: translate(var(--offset-x, 0px), var(--offset-y, 0px)) scale(var(--zoom, 1)); transform-origin: center center; }
:deep(.workflow-node) {
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

:deep(.workflow-node .workflow-node-header) {
  border-top-left-radius: 12px;
  border-top-right-radius: 12px;
}

:deep(.workflow-node.active) {
  border-color: #9278e6;
}

:deep(.workflow-node.dragging) {
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
