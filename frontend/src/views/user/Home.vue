<template>
  <div 
    class="canvas-layout" 
    :style="canvasStyle" 
    @mousedown="onCanvasMouseDown"
    @dragover.prevent="isDraggingFile = true"
    @dragleave="isDraggingFile = false"
    @drop="handleDrop"
  >
    <!-- ① 无限画布背景层 -->
    <div class="canvas-grid" :class="{ 'grid-dragging': isDraggingFile }"></div>

    <!-- ② 顶部悬浮栏 -->
    <header class="floating-header">
      <div class="header-left">
        <div class="file-name-chip">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/></svg>
          <span class="file-name">Untitled</span>
          <svg class="chevron-down" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3"><polyline points="6 9 12 15 18 9"/></svg>
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
        <input v-model="userPrompt" type="text" placeholder="What do you want to create?" @keyup.enter="handleGenerate" />
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
      <button class="logo-btn" title="返回首页" @click="router.push('/user/home')">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <rect x="3" y="3" width="14" height="14" rx="2" stroke-width="2.5" />
          <path d="M7 7h14v14" opacity="0.4" />
        </svg>
      </button>
    </div>

    <!-- ⑥ 内容展示区 -->
    <div class="canvas-content">
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
       >
          <img v-if="msg.type === 'image'" :src="msg.url" class="canvas-img" @load="onImageLoad(msg, $event)" />
          
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
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useMessage, NSpin } from 'naive-ui'
import { taskApi, styleCardApi, uploadApi } from '../../api/index'
import type { StyleCard } from '../../api/index'

import { useUserStore } from '../../stores/user'

const router = useRouter()
const route = useRoute()
const message = useMessage()
const userStore = useUserStore()

// 状态
const userPrompt = ref('')
const generating = ref(false)
const messages = ref<any[]>([])
const selectedIds = ref<number[]>([])
const zoomLevel = ref(100)
const canvasX = ref(0)
const canvasY = ref(0)
const activeTool = ref('select')
const isPanning = ref(false)
const isSelecting = ref(false)
const selectionRect = ref({ x1: 0, y1: 0, x2: 0, y2: 0 })
const isDraggingItem = ref(false)
const isResizingItem = ref(false)
const isSpacePressed = ref(false)
const isDraggingFile = ref(false)
const showSettings = ref(false)

// 拖拽/缩放变量
let startX = 0, startY = 0
let initialX = 0, initialY = 0, initialW = 0, initialH = 0
let dragItem: any = null
let resizeDir = ''
let exportLoading: { destroy: () => void } | null = null

const tools = [
  { title: '图・文生图', path: '/user/picture', icon: '<rect x="3" y="3" width="18" height="18" rx="2"/><path d="M3 14l4-4 5 5 5-5 4 4"/><circle cx="8.5" cy="8.5" r="1.5"/><path d="M19 3l-1 2-2 1 2 1 1 2 1-2 2-1-2-1-1-2z" fill="currentColor" stroke="none"/>' },
  { title: '服装实验室', path: '/user/sketch-to-design', icon: '<path d="M20.38 3.46L16 2a4 4 0 0 1-8 0L3.62 3.46a2 2 0 0 0-1.34 2.23l.58 3.47a1 1 0 0 0 .99.84H6v10c0 1.1.9 2 2 2h8a2 2 0 0 0 2-2V10h2.15a1 1 0 0 0 .99-.84l.58-3.47a2 2 0 0 0-1.34-2.23z"/>' },
  { title: '摄影商拍', path: '/user/multi-angle', icon: '<path d="M14.5 4h-5L7 7H4a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2V9a2 2 0 0 0-2-2h-3l-2.5-3z"/><circle cx="12" cy="13" r="3"/>' },
  { title: '图案工作室', path: '/user/pattern-gen', icon: '<rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/>' },
  { title: 'AI 工具箱', path: '/user/to-vector', icon: '<path d="M15 4V2"/><path d="M15 16v-2"/><path d="M8 9h2"/><path d="M20 9h2"/><path d="M17.8 11.8L19 13"/><path d="M15 9h0"/><path d="M17.8 6.2L19 5"/><path d="M3 21l9-9"/><path d="M12.2 6.2L11 5"/>' }
]

function updateZoom(delta: number) {
  const newZoom = zoomLevel.value + delta
  if (newZoom >= 10 && newZoom <= 200) zoomLevel.value = newZoom
}

function onCanvasMouseDown(e: MouseEvent) {
  if (e.button === 1 || isSpacePressed.value || activeTool.value === 'hand') {
    isPanning.value = true
    startX = e.clientX; startY = e.clientY
    initialX = canvasX.value; initialY = canvasY.value
    document.body.style.cursor = 'grabbing'
    e.preventDefault()
  } else if (e.button === 0 && activeTool.value === 'select') {
    isSelecting.value = true
    startX = e.clientX; startY = e.clientY
    selectionRect.value = { x1: startX, y1: startY, x2: startX, y2: startY }
    if (!e.shiftKey) selectedIds.value = []
  }
}

function onItemMouseDown(e: MouseEvent, msg: any) {
  if (activeTool.value !== 'select' || e.button !== 0) return
  
  if (e.shiftKey) {
    const idx = selectedIds.value.indexOf(msg.id)
    if (idx >= 0) selectedIds.value.splice(idx, 1)
    else selectedIds.value.push(msg.id)
  } else {
    if (!selectedIds.value.includes(msg.id)) {
      selectedIds.value = [msg.id]
    }
  }

  isDraggingItem.value = true
  dragItem = msg
  startX = e.clientX; startY = e.clientY
  initialX = msg.x; initialY = msg.y
  e.preventDefault()
}

function onResizeMouseDown(e: MouseEvent, msg: any, dir: string) {
  isResizingItem.value = true
  dragItem = msg
  resizeDir = dir
  startX = e.clientX; startY = e.clientY
  
  // 获取父容器即被选中的 canvas-item 宽高
  const el = e.currentTarget as HTMLElement
  const parent = el.closest('.canvas-item') as HTMLElement
  if (parent) {
    initialW = parent.offsetWidth
    initialH = parent.offsetHeight
  } else {
    initialW = msg.width || 300
    initialH = initialW
  }
  
  initialX = msg.x; initialY = msg.y
  e.preventDefault()
}

function onGlobalMouseMove(e: MouseEvent) {
  if (isPanning.value) {
    canvasX.value = initialX + (e.clientX - startX)
    canvasY.value = initialY + (e.clientY - startY)
  } else if (isSelecting.value) {
    selectionRect.value.x2 = e.clientX
    selectionRect.value.y2 = e.clientY
  } else if (isDraggingItem.value && dragItem) {
    const zoom = zoomLevel.value / 100
    const dx = (e.clientX - startX) / zoom
    const dy = (e.clientY - startY) / zoom
    dragItem.x = initialX + dx
    dragItem.y = initialY + dy
  } else if (isResizingItem.value && dragItem) {
    const zoom = zoomLevel.value / 100
    const dx = (e.clientX - startX) / zoom
    const dy = (e.clientY - startY) / zoom
    const ratio = initialH / initialW
    let newW = initialW


    // 1. 计算新的宽度
    if (resizeDir.includes('e')) {
      newW = Math.max(50, initialW + dx)
    } else if (resizeDir.includes('w')) {
      newW = Math.max(50, initialW - dx)
      dragItem.x = initialX + (initialW - newW)
    } else if (resizeDir.includes('n')) {
      // 只有顶部手柄，垂直方向控制大小
      newW = Math.max(50, initialW - dy / ratio)
    } else if (resizeDir.includes('s')) {
      newW = Math.max(50, initialW + dy / ratio)
    }

    // 2. 对于顶部缩放 (n, nw, ne)，由于锚点在左上，我们需要通过 y 来抵消高度的变化，让底部保持固定
    if (resizeDir.includes('n')) {
      const dH = (newW - initialW) * ratio
      dragItem.y = initialY - dH
    }

    dragItem.width = newW
  }
}

function onGlobalMouseUp() {
  if (isSelecting.value) {
    const xMin = Math.min(selectionRect.value.x1, selectionRect.value.x2)
    const xMax = Math.max(selectionRect.value.x1, selectionRect.value.x2)
    const yMin = Math.min(selectionRect.value.y1, selectionRect.value.y2)
    const yMax = Math.max(selectionRect.value.y1, selectionRect.value.y2)

    const rect = (document.querySelector('.canvas-layout') as HTMLElement).getBoundingClientRect()
    const zoom = zoomLevel.value / 100

    messages.value.forEach(msg => {
      // 计算图片在屏幕上的实际坐标
      // 算法: (50% + offset + msg_pos) * zoom
      const screenX = rect.left + rect.width / 2 + (canvasX.value + msg.x * zoom)
      const screenY = rect.top + rect.height / 2 + (canvasY.value + msg.y * zoom)
      const screenW = (msg.width || 300) * zoom
      const screenH = screenW * (msg.height_ratio || 1)

      // 简单的中心点碰撞检测或矩形重叠检测
      if (screenX + screenW > xMin && screenX < xMax && screenY + screenH > yMin && screenY < yMax) {
        if (!selectedIds.value.includes(msg.id)) {
          selectedIds.value.push(msg.id)
        }
      }
    })
  }

  isPanning.value = false; isDraggingItem.value = false; isResizingItem.value = false; isSelecting.value = false
  dragItem = null; document.body.style.cursor = ''
}

function handleKey(e: KeyboardEvent) {
  if (e.code === 'Space') isSpacePressed.value = (e.type === 'keydown')
  if ((e.key === 'Delete' || e.key === 'Backspace') && selectedIds.value.length > 0) {
    if (e.target instanceof HTMLInputElement) return
    messages.value = messages.value.filter(m => !selectedIds.value.includes(m.id))
    selectedIds.value = []
  }
}

function handleWheel(e: WheelEvent) {
  const target = e.target as HTMLElement | null
  if (!target || !target.closest('.canvas-layout')) return
  const wheelPanSpeed = 1.5

  // 保留设置面板和缩略图条自己的滚动行为
  if (target.closest('.settings-panel') || target.closest('.selected-preview-list')) return

  // Option(Mac)/Alt(Win) + 滚轮：缩放
  if (e.altKey) {
    e.preventDefault()
    updateZoom(e.deltaY > 0 ? -5 : 5)
    return
  }

  if (e.ctrlKey || e.metaKey) {
    e.preventDefault()
    // Ctrl/Cmd + 滚轮：强制横向平移
    const horizontalDelta = (Math.abs(e.deltaX) > 0 ? e.deltaX : e.deltaY) * wheelPanSpeed
    canvasX.value -= horizontalDelta
    return
  }

  e.preventDefault()
  canvasX.value -= e.deltaX * wheelPanSpeed
  canvasY.value -= e.deltaY * wheelPanSpeed
}

async function handleDrop(e: DragEvent) {
  e.preventDefault(); isDraggingFile.value = false
  const files = e.dataTransfer?.files
  if (!files || !files[0]) return
  const rect = (document.querySelector('.canvas-layout') as HTMLElement).getBoundingClientRect()
  const zoom = zoomLevel.value / 100
  const x = (e.clientX - rect.left - rect.width/2 - canvasX.value) / zoom
  const y = (e.clientY - rect.top - rect.height/2 - canvasY.value) / zoom
  
  for (const file of Array.from(files)) {
    if (!file.type.startsWith('image/')) continue
    try {
      const res: any = await uploadApi.upload(file)
      if (res.code === 0) messages.value.push({ id: Date.now() + Math.random(), type: 'image', url: res.data.url, x, y, width: 300 })
    } catch (err) {}
  }
}

const canvasStyle = computed(() => ({
  '--zoom': zoomLevel.value / 100,
  '--offset-x': `${canvasX.value}px`,
  '--offset-y': `${canvasY.value}px`,
  '--grid-opacity': zoomLevel.value > 80 ? 1 : 0,
}))

const selectedPreviewItems = computed(() => {
  const messageMap = new Map(messages.value.map(item => [item.id, item]))
  return selectedIds.value
    .map(id => messageMap.get(id))
    .filter((item): item is any => !!item && item.type === 'image' && !!item.url)
})
const visiblePreviewItems = computed(() => selectedPreviewItems.value.slice(0, 6))
const previewOverflowCount = computed(() => Math.max(0, selectedPreviewItems.value.length - visiblePreviewItems.value.length))

const styleCards = ref<StyleCard[]>([])
const selectedCardIds = ref<number[]>([])
const selectedRatio = ref('3:4')
const selectedResolution = ref('1K')
const currentTaskId = ref<string | null>(null)
let pollTimer: number | null = null

function clearSelectedIds() {
  selectedIds.value = []
}

function removeSelectedId(id: number) {
  selectedIds.value = selectedIds.value.filter(selectedId => selectedId !== id)
}

function focusSelectedItem(id: number) {
  const target = messages.value.find(item => item.id === id)
  if (!target) return
  const zoom = zoomLevel.value / 100
  canvasX.value = -(target.x * zoom)
  canvasY.value = -(target.y * zoom)
}

function toggleCard(id: number) {
  const idx = selectedCardIds.value.indexOf(id)
  if (idx >= 0) selectedCardIds.value.splice(idx, 1)
  else selectedCardIds.value.push(id)
}
async function handleGenerate() {
  if (!userPrompt.value.trim() || generating.value) return
  generating.value = true
  try {
    const res: any = await taskApi.submit({ 
      prompt: userPrompt.value.trim(), 
      card_ids: selectedCardIds.value, 
      params: { 
        ratio: selectedRatio.value,
        resolution: selectedResolution.value
      } 
    })
    if (res.code === 0 && res.data?.task_id) {
      currentTaskId.value = res.data.task_id
      message.success('任务已提交')
      userPrompt.value = ''
      showSettings.value = false
      pollTaskStatus(res.data.task_id)
    } else {
      message.error(res.msg || '提交失败')
      generating.value = false
    }
  } catch (e: any) { message.error('Failed') } finally { generating.value = false }
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

function appendGeneratedImages(urls: string[]) {
  if (!urls.length) return
  const baseX = -((urls.length - 1) * 170) / 2
  const y = 0
  for (let i = 0; i < urls.length; i++) {
    messages.value.push({
      id: Date.now() + Math.random(),
      type: 'image',
      url: urls[i],
      x: baseX + i * 170,
      y,
      width: 300
    })
  }
}

function extractImageUrls(task: any): string[] {
  if (!task?.result) return []
  try {
    const parsed = JSON.parse(task.result)
    if (Array.isArray(parsed?.image_urls)) {
      return parsed.image_urls.filter((u: unknown) => typeof u === 'string' && u.length > 0)
    }
    if (typeof parsed?.image_url === 'string' && parsed.image_url) {
      return [parsed.image_url]
    }
    return []
  } catch {
    return []
  }
}

function pollTaskStatus(taskId: string) {
  if (pollTimer) {
    clearTimeout(pollTimer)
    pollTimer = null
  }

  const check = async () => {
    try {
      const res: any = await taskApi.get(taskId)
      if (res.code === 0 && res.data?.task) {
        const task = res.data.task
        if (task.status === 'completed') {
          const urls = extractImageUrls(task)
          if (urls.length > 0) {
            appendGeneratedImages(urls)
            message.success('生成完成')
          } else {
            message.warning('任务完成，但未返回图片')
          }
          currentTaskId.value = null
          return
        }
        if (task.status === 'failed') {
          message.error(task.error_msg || '生成失败')
          currentTaskId.value = null
          return
        }
      }
      pollTimer = window.setTimeout(check, 2000)
    } catch {
      pollTimer = window.setTimeout(check, 3000)
    }
  }

  void check()
}

onMounted(async () => {
  window.addEventListener('wheel', handleWheel, { passive: false })
  window.addEventListener('keydown', handleKey); window.addEventListener('keyup', handleKey)
  window.addEventListener('mousemove', onGlobalMouseMove); window.addEventListener('mouseup', onGlobalMouseUp)
  try {
    const res: any = await styleCardApi.getEnabled()
    if (res.code === 0) styleCards.value = res.data.list
  } catch (e) {}
})

onUnmounted(() => {
  if (pollTimer) {
    clearTimeout(pollTimer)
    pollTimer = null
  }
  window.removeEventListener('wheel', handleWheel)
  window.removeEventListener('keydown', handleKey); window.removeEventListener('keyup', handleKey)
  window.removeEventListener('mousemove', onGlobalMouseMove); window.removeEventListener('mouseup', onGlobalMouseUp)
})
</script>

<style scoped>
.canvas-layout { width: 100%; height: 100%; background: #ffffff; position: relative; overflow: hidden; }
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
}
.file-name-chip:hover { background: #e8eaed; }
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
.canvas-item { position: absolute; background: #fff; padding: 8px; border-radius: 12px; border: 2px solid #e2e4e8; }
.canvas-item.active { border-color: #9278e6; z-index: 10; }
.canvas-img { width: 100%; display: block; border-radius: 8px; pointer-events: none; }

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
