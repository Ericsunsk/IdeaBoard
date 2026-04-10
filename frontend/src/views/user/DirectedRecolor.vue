<template>
  <div class="home-layout">
    <div class="home-body">

      <!-- ====== 左侧：作品画廊 ====== -->
      <aside class="history-panel">
        <div class="history-header">
          <span>我的画廊</span>
          <span class="gallery-count" v-if="galleryTasks.length">{{ galleryTasks.length }}</span>
        </div>
        <div class="gallery-grid" v-if="galleryTasks.length > 0">
          <div
            v-for="ht in galleryTasks"
            :key="ht.task_id"
            class="gallery-item"
            :class="{ active: activeTaskId === ht.task_id }"
            @click="viewHistoryTask(ht)"
          >
            <img :src="getTaskImageUrl(ht)" alt="" loading="lazy" @error="handleImgError" />
            <div class="gallery-item-overlay">
              <span class="gallery-item-prompt">{{ stripPrefix(ht.prompt) }}</span>
            </div>
            <button class="gallery-delete-btn" @click.stop="showDeleteConfirm($event, ht.task_id)" title="删除">
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
            </button>
          </div>
        </div>
        <div class="history-empty" v-else>
          <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="#333" stroke-width="1.5"><rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="8.5" cy="8.5" r="1.5"/><path d="m21 15-5-5L5 21"/></svg>
          <p>还没有作品</p>
          <p style="font-size: 11px; color: #444;">生成图片后将在这里展示</p>
        </div>
      </aside>

      <!-- ====== 中间：生成区 ====== -->
      <main class="canvas-area">
        <!-- 未登录 -->
        <div v-if="!userStore.isLoggedIn" class="guest-prompt">
          <div class="smiley-icon">
            <svg width="120" height="120" viewBox="0 0 120 120" fill="none">
              <circle cx="60" cy="60" r="56" stroke="#a3e635" stroke-width="4" fill="none"/>
              <circle cx="44" cy="48" r="5" fill="#a3e635"/>
              <circle cx="76" cy="48" r="5" fill="#a3e635"/>
              <path d="M38 72 C38 72, 44 90, 60 90 C76 90, 82 72, 82 72" stroke="#a3e635" stroke-width="4" stroke-linecap="round" fill="none"/>
            </svg>
          </div>
          <p class="guest-title">温馨提示：</p>
          <p class="guest-desc">您当前是游客身份，暂无工具相关权限。</p>
          <p class="guest-action">
            请先 <router-link to="/user/register" class="link-highlight">注册</router-link>
            或 <router-link to="/user/login" class="link-highlight">登录</router-link>
          </p>
        </div>

        <!-- 已登录：聊天记录气泡 -->
        <div v-else class="gen-display" ref="chatMessagesRef">
          <!-- 空状态 -->
          <div v-if="messages.length === 0" class="chat-empty">
            <div class="empty-icon-wrap">
              <svg width="64" height="64" viewBox="0 0 24 24" fill="none" stroke="#a3e635" stroke-width="1.2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M12 2L2 7l10 5 10-5-10-5z"/><path d="M2 17l10 5 10-5"/><path d="M2 12l10 5 10-5"/>
              </svg>
            </div>
            <p class="chat-empty-text">定向换色</p>
            <p class="chat-empty-sub">上传图片，涂抹需要换色的区域，选择目标颜色</p>
          </div>

          <!-- 聊天气泡 -->
          <div v-for="msg in messages" :key="msg.id" :id="msg.taskId ? 'task-' + msg.taskId : undefined" class="chat-bubble" :class="msg.role">
            <div v-if="msg.role === 'user'" class="bubble-row user">
              <div class="bubble-body user">
                <p class="bubble-prompt">{{ msg.prompt }}</p>
                <div class="bubble-ref-image" v-if="msg.refImageUrl">
                  <img :src="msg.refImageUrl" alt="参考图" />
                </div>
              </div>
              <span class="bubble-time" v-if="msg.time">{{ msg.time }}</span>
            </div>

            <div v-else class="bubble-row ai">
              <div v-if="msg.status === 'pending'" class="skeleton-wrap">
                <div class="skeleton-shimmer"></div>
                <div class="skeleton-overlay">
                  <div class="skeleton-spinner"></div>
                  <p class="skeleton-text">AI 正在生成中...</p>
                  <p class="skeleton-sub">请耐心等待，通常需要 10-30 秒</p>
                </div>
              </div>
              <div v-else-if="msg.status === 'failed'" class="bubble-body failed">
                <div class="bubble-failed-row">
                  <div class="bubble-failed-icon">✕</div>
                  <div class="bubble-failed-info">
                    <div class="bubble-failed-title">生成失败</div>
                    <div class="bubble-failed-reason">{{ msg.errorMsg }}</div>
                  </div>
                </div>
              </div>
              <template v-else-if="msg.status === 'completed'">
                <div class="bubble-body ai-success">
                  <div v-if="msg.imageUrl" class="bubble-image-wrap" @click="lightboxUrl = msg.imageUrl!">
                    <img :src="msg.imageUrl" :alt="msg.prompt" class="bubble-image" @error="handleImgError" />
                    <div class="image-zoom-hint">
                      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/><line x1="11" y1="8" x2="11" y2="14"/><line x1="8" y1="11" x2="14" y2="11"/></svg>
                    </div>
                  </div>
                  <div class="bubble-footer">
                    <span class="bubble-meta" v-if="msg.elapsed">耗时 {{ msg.elapsed }}s</span>
                    <div class="result-actions">
                      <button class="result-action-btn" @click="downloadImage(msg.imageUrl)">
                        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
                        下载
                      </button>
                      <button class="result-action-btn primary" @click="handleGenerate">
                        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/></svg>
                        再次生成
                      </button>
                      <button class="result-action-btn danger" @click="showDeleteConfirm($event, msg.taskId)">
                        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
                        删除
                      </button>
                    </div>
                  </div>
                </div>
              </template>
            </div>
          </div>
        </div>
      </main>

      <!-- ====== 右侧：创作面板 ====== -->
      <aside class="control-panel">
        <div class="panel-top">
          <div class="panel-tabs">
            <button class="panel-tab active">定向换色</button>
          </div>
        </div>

        <div class="panel-scroll">
          <!-- ① 上传图片 -->
          <div class="section">
            <div class="section-header-row">
              <span class="section-num">①</span>
              <span class="section-label">上传图片</span>
            </div>
            <ImageUploader
              v-model="uploadImages"
              :gallery-groups="allGalleryGroups"
              :max="1"
              :max-size-m-b="20"
            />
          </div>

          <!-- ② 涂抹区域 -->
          <div class="section" v-if="uploadedImage">
            <div class="section-header-row">
              <span class="section-num">②</span>
              <span class="section-label">涂抹需要换色的区域</span>
            </div>

            <div class="paint-preview" @click="openPaintModal">
              <img :src="uploadedImage" class="paint-preview-img" />
              <canvas ref="previewCanvasRef" class="paint-preview-overlay"></canvas>
              <div class="paint-preview-mask">
                <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 19l7-7 3 3-7 7-3-3z"/><path d="M18 13l-1.5-7.5L2 2l3.5 14.5L13 18l5-5z"/></svg>
                <span>{{ hasPainted ? '重新涂抹' : '开始涂抹' }}</span>
              </div>
            </div>
            <div v-if="hasPainted" class="paint-done-tip">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="#a3e635" stroke-width="2.5"><polyline points="20 6 9 17 4 12"/></svg>
              <span>涂抹已完成</span>
            </div>
          </div>

          <!-- ③ 选择目标颜色 -->
          <div class="section" v-if="hasPainted">
            <div class="section-header-row">
              <span class="section-num">③</span>
              <span class="section-label">选择目标颜色</span>
            </div>

            <!-- 当前选中颜色 -->
            <div class="target-color-bar" v-if="targetColor">
              <span class="target-color-dot" :style="{ background: targetColor }"></span>
              <span class="target-color-hex">{{ targetColor }}</span>
              <button class="target-color-clear" @click="targetColor = ''">✕</button>
            </div>

            <!-- HSV 颜色选择器 -->
            <div class="color-picker-inline">
              <div class="sv-canvas-wrap">
                <canvas
                  ref="svCanvasRef"
                  width="260" height="160"
                  class="sv-canvas"
                  @mousedown="onSvMouseDown"
                  @touchstart.prevent="onSvTouchStart"
                ></canvas>
                <div class="sv-cursor" :style="{ left: svCursor.x + 'px', top: svCursor.y + 'px', background: pickerHex }"></div>
              </div>

              <div class="hue-bar-wrap">
                <canvas
                  ref="hueCanvasRef"
                  width="260" height="14"
                  class="hue-bar"
                  @mousedown="onHueMouseDown"
                  @touchstart.prevent="onHueTouchStart"
                ></canvas>
                <div class="hue-cursor" :style="{ left: hueCursorX + 'px' }"></div>
              </div>

              <div class="picker-row">
                <div class="picker-preview" :style="{ background: pickerHex }"></div>
                <div class="picker-inputs">
                  <div class="picker-input-group">
                    <label>HEX</label>
                    <input :value="pickerHex" @change="onHexInput" maxlength="7" class="picker-input hex" />
                  </div>
                  <div class="picker-input-group">
                    <label>R</label>
                    <input :value="pickerRgb.r" @change="e => onRgbInput(e, 'r')" type="number" min="0" max="255" class="picker-input" />
                  </div>
                  <div class="picker-input-group">
                    <label>G</label>
                    <input :value="pickerRgb.g" @change="e => onRgbInput(e, 'g')" type="number" min="0" max="255" class="picker-input" />
                  </div>
                  <div class="picker-input-group">
                    <label>B</label>
                    <input :value="pickerRgb.b" @change="e => onRgbInput(e, 'b')" type="number" min="0" max="255" class="picker-input" />
                  </div>
                </div>
                <button class="picker-confirm-btn" @click="confirmPickerColor">确定选色</button>
              </div>
            </div>
          </div>
        </div>

        <!-- 底部操作 -->
        <div class="panel-bottom">
          <div class="button-group">
            <button
              class="btn-generate"
              :disabled="!canGenerate || generating || uploading"
              @click="handleGenerate"
            >
              {{ uploading ? '上传涂抹图...' : generating ? '生成中...' : '生成图片' }}
            </button>
            <button class="btn-clear" :disabled="generating || uploading" @click="handleClear">清空</button>
          </div>
        </div>
      </aside>
    </div>

    <!-- 删除确认浮层 -->
    <div v-if="deleteConfirm.show" class="delete-popover" :style="{ left: deleteConfirm.x + 'px', top: deleteConfirm.y + 'px' }">
      <p>确认删除？</p>
      <div class="delete-popover-btns">
        <button @click="deleteConfirm.show = false">取消</button>
        <button class="confirm" @click="confirmDelete">删除</button>
      </div>
    </div>

    <!-- ====== 涂抹工作台弹窗 ====== -->
    <div v-if="showPaintModal" class="paint-modal-overlay">
      <div class="paint-modal">
        <div class="paint-modal-header">
          <span class="paint-modal-title">涂抹工作台</span>
          <span class="paint-modal-hint">在图片上涂抹需要换色的区域</span>
          <div class="header-actions">
            <button class="tool-btn-text cancel" @click="cancelPaintModal">取消</button>
            <button class="tool-btn-text confirm" @click="confirmPaintModal">完成涂抹</button>
          </div>
        </div>

        <div
          class="paint-modal-body"
          ref="viewportRef"
          :class="{ 'cursor-grab': paintTool === 'hand' && !panning, 'cursor-grabbing': panning }"
          @mousedown="onViewportMouseDown"
          @mousemove="onViewportMouseMove"
          @mouseup="onViewportMouseUp"
          @mouseleave="onViewportMouseLeave"
          @wheel.prevent="onViewportWheel"
          @touchstart.prevent="onViewportTouchStart"
          @touchmove.prevent="onViewportTouchMove"
          @touchend.prevent="onViewportTouchEnd"
        >
          <div class="paint-canvas-wrap" ref="canvasWrapRef" :style="canvasWrapStyle">
            <img ref="bgImageRef" :src="uploadedImage" class="paint-bg-image" @load="initCanvas" crossorigin="anonymous" />
            <canvas ref="paintCanvasRef" class="paint-canvas"></canvas>
            <div v-if="!hasPainted && !painting" class="paint-hint">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 19l7-7 3 3-7 7-3-3z"/><path d="M18 13l-1.5-7.5L2 2l3.5 14.5L13 18l5-5z"/><path d="M2 2l7.586 7.586"/><circle cx="11" cy="11" r="2"/></svg>
              <span>在图片上涂抹需要换色的区域</span>
            </div>
          </div>

          <div
            v-if="cursorVisible && (paintTool === 'brush' || paintTool === 'eraser')"
            class="brush-cursor"
            :style="brushCursorStyle"
          ></div>
        </div>

        <div class="paint-modal-footer">
          <div class="paint-toolbar">
            <div class="toolbar-group">
              <button class="tool-btn" :class="{ active: paintTool === 'brush' }" @click="paintTool = 'brush'" title="画笔 (B)">
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 19l7-7 3 3-7 7-3-3z"/><path d="M18 13l-1.5-7.5L2 2l3.5 14.5L13 18l5-5z"/></svg>
              </button>
              <button class="tool-btn" :class="{ active: paintTool === 'eraser' }" @click="paintTool = 'eraser'" title="橡皮擦 (E)">
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="m7 21-4.3-4.3c-1-1-1-2.5 0-3.4l9.6-9.6c1-1 2.5-1 3.4 0l5.6 5.6c1 1 1 2.5 0 3.4L13 21"/><path d="M22 21H7"/><path d="m5 11 9 9"/></svg>
              </button>
              <button class="tool-btn" :class="{ active: paintTool === 'hand' }" @click="paintTool = 'hand'" title="抓手 (H)">
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 11V6a2 2 0 0 0-4 0v1"/><path d="M14 10V4a2 2 0 0 0-4 0v2"/><path d="M10 10.5V6a2 2 0 0 0-4 0v8"/><path d="M18 8a2 2 0 0 1 4 0v6a8 8 0 0 1-8 8h-2c-2.8 0-4.5-.86-5.99-2.34l-3.6-3.6a2 2 0 0 1 2.83-2.82L7 15"/></svg>
              </button>

              <div class="toolbar-divider"></div>

              <div class="brush-size-wrap">
                <span class="brush-size-label">{{ brushSize }}px</span>
                <input type="range" class="brush-size-slider" v-model.number="brushSize" min="5" max="80" step="1" />
              </div>
            </div>

            <div class="toolbar-group">
              <button class="tool-btn" @click="undoPaint" :disabled="paintHistory.length === 0" title="撤销 (Ctrl+Z)">
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="1 4 1 10 7 10"/><path d="M3.51 15a9 9 0 1 0 2.13-9.36L1 10"/></svg>
              </button>
              <button class="tool-btn" @click="redoPaint" :disabled="redoStack.length === 0" title="重做 (Ctrl+Shift+Z)">
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/></svg>
              </button>
              <button class="tool-btn" @click="clearPaint" title="重置">
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
              </button>
            </div>

            <div class="toolbar-group">
              <button class="tool-btn" @click="zoomOut" :disabled="zoom <= 0.25" title="缩小 (-)">
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/><line x1="8" y1="11" x2="14" y2="11"/></svg>
              </button>
              <span class="zoom-label">{{ Math.round(zoom * 100) }}%</span>
              <button class="tool-btn" @click="zoomIn" :disabled="zoom >= 5" title="放大 (+)">
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/><line x1="11" y1="8" x2="11" y2="14"/><line x1="8" y1="11" x2="14" y2="11"/></svg>
              </button>
              <button class="tool-btn" @click="zoomReset" title="1:1 原始大小">
                <span style="font-size:11px;font-weight:700;">1:1</span>
              </button>
              <button class="tool-btn" @click="zoomFit" title="适应窗口">
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M8 3H5a2 2 0 0 0-2 2v3"/><path d="M21 8V5a2 2 0 0 0-2-2h-3"/><path d="M3 16v3a2 2 0 0 0 2 2h3"/><path d="M16 21h3a2 2 0 0 0 2-2v-3"/></svg>
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 图片放大 -->
    <div v-if="lightboxUrl" class="lightbox-overlay" @click.self="lightboxUrl = ''">
      <div class="lightbox-container">
        <img :src="lightboxUrl" class="lightbox-image" />
        <button class="lightbox-close" @click="lightboxUrl = ''">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
        </button>
        <div class="lightbox-actions">
          <button class="lightbox-action-btn" @click="downloadImage(lightboxUrl)">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
            下载原图
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, nextTick, onUnmounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useMessage } from 'naive-ui'
import { useUserStore } from '../../stores/user'
import { taskApi, uploadApi } from '../../api'
import ImageUploader from '../../components/ImageUploader.vue'
import { buildGalleryGroups } from '../../utils/galleryGroups'
import { downloadImageByProxy, getTaskImageUrl as extractTaskImageUrl, stripFeaturePrefix } from '../../utils/task-history'
import type { Task } from '../../api'

const FEATURE_SLUG = 'directed-recolor'
const FEATURE_TITLE = '定向换色'
const FEATURE_PREFIX = `[${FEATURE_SLUG}] `

const router = useRouter()
const message = useMessage()
const userStore = useUserStore()

function stripPrefix(str?: string) {
  return stripFeaturePrefix(str)
}

// ============ 画廊 ============
const historyTasks = ref<Task[]>([])
const activeTaskId = ref<string | null>(null)

const galleryTasks = computed(() =>
  historyTasks.value.filter(t => {
    if (t.status !== 'completed') return false
    if (!t.prompt || !t.prompt.startsWith(FEATURE_PREFIX)) return false
    try { return !!JSON.parse(t.result || '{}').image_url } catch { return false }
  })
)

function getTaskImageUrl(task: Task): string {
  return extractTaskImageUrl(task)
}

async function loadHistory() {
  try {
    const res: any = await taskApi.list({ page: 1, page_size: 200 })
    if (res.code === 0) {
      historyTasks.value = res.data.list || []
      if (messages.value.length === 0) rebuildMessagesFromHistory()
    }
  } catch {}
}

function rebuildMessagesFromHistory() {
  const tasks = historyTasks.value
    .filter(t => t.prompt && t.prompt.startsWith(FEATURE_PREFIX))
    .sort((a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime())
    .slice(-20)

  for (const task of tasks) {
    const imgUrl = getTaskImageUrl(task)
    const timeStr = task.created_at ? new Date(task.created_at).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' }) : ''
    messages.value.push({ id: 'hist_user_' + task.task_id, role: 'user', prompt: stripPrefix(task.prompt) || FEATURE_TITLE, refImageUrl: undefined, time: timeStr })
    if (task.status === 'completed' && imgUrl) {
      let elapsed: string | undefined
      if (task.completed_at && task.created_at) {
        const t = new Date(task.updated_at).getTime() - new Date(task.created_at).getTime()
        if (t > 0) elapsed = (t / 1000).toFixed(1)
      }
      messages.value.push({ id: 'hist_ai_' + task.task_id, role: 'assistant', prompt: task.prompt || '', status: 'completed', imageUrl: imgUrl, taskId: task.task_id, elapsed })
    } else if (task.status === 'failed') {
      messages.value.push({ id: 'hist_ai_' + task.task_id, role: 'assistant', prompt: task.prompt || '', status: 'failed', errorMsg: task.error_msg || '生成失败', taskId: task.task_id })
    }
  }
  if (messages.value.length > 0) scrollToBottom()
}

function viewHistoryTask(task: Task) {
  activeTaskId.value = task.task_id
  const imgUrl = getTaskImageUrl(task)
  if (imgUrl) {
    const existing = messages.value.find(m => m.taskId === task.task_id)
    if (!existing) {
      messages.value.push({ id: 'hist_user_' + task.task_id, role: 'user', prompt: stripPrefix(task.prompt) || FEATURE_TITLE })
      messages.value.push({ id: 'hist_ai_' + task.task_id, role: 'assistant', prompt: task.prompt || '', status: 'completed', imageUrl: imgUrl, taskId: task.task_id })
      scrollToBottom()
    } else {
      const el = document.getElementById('task-' + task.task_id)
      if (el) { el.scrollIntoView({ behavior: 'smooth', block: 'center' }); el.classList.add('highlight'); setTimeout(() => el.classList.remove('highlight'), 2000) }
    }
  }
}

// ============ 删除 ============
const deleteConfirm = ref<{ show: boolean; x: number; y: number; taskId: string }>({ show: false, x: 0, y: 0, taskId: '' })

function showDeleteConfirm(e: MouseEvent, taskId?: string) {
  if (!taskId) return
  deleteConfirm.value = { show: true, x: e.clientX, y: e.clientY, taskId }
  const close = (ev: MouseEvent) => {
    const el = document.querySelector('.delete-popover')
    if (el && !el.contains(ev.target as Node)) { deleteConfirm.value.show = false; document.removeEventListener('mousedown', close) }
  }
  setTimeout(() => document.addEventListener('mousedown', close), 0)
}

async function confirmDelete() {
  const taskId = deleteConfirm.value.taskId
  deleteConfirm.value.show = false
  if (!taskId) return
  try {
    const res: any = await taskApi.delete(taskId)
    if (res.code === 0) {
      historyTasks.value = historyTasks.value.filter(t => t.task_id !== taskId)
      messages.value = messages.value.filter(m => m.id !== 'hist_user_' + taskId && m.id !== 'hist_ai_' + taskId && m.taskId !== taskId)
      if (activeTaskId.value === taskId) activeTaskId.value = null
    }
  } catch {}
}

// ============ 上传图片 ============
const uploadedImage = ref('')
const uploadImages = computed({
  get: () => uploadedImage.value ? [uploadedImage.value] : [],
  set: (val: string[]) => { uploadedImage.value = val.length > 0 ? val[0] : '' }
})
const allGalleryGroups = computed(() => buildGalleryGroups(historyTasks.value))

// ============ 涂抹画布 ============
const canvasWrapRef = ref<HTMLElement | null>(null)
const bgImageRef = ref<HTMLImageElement | null>(null)
const paintCanvasRef = ref<HTMLCanvasElement | null>(null)
const previewCanvasRef = ref<HTMLCanvasElement | null>(null)
const viewportRef = ref<HTMLElement | null>(null)

const showPaintModal = ref(false)
const paintTool = ref<'brush' | 'eraser' | 'hand'>('brush')
const brushSize = ref(30)
const hasPainted = ref(false)
const painting = ref(false)

const paintHistory = ref<ImageData[]>([])
const redoStack = ref<ImageData[]>([])
let savedPaintData: ImageData | null = null

const zoom = ref(1)
const panX = ref(0)
const panY = ref(0)
const panning = ref(false)
let panStartX = 0
let panStartY = 0
let panStartPX = 0
let panStartPY = 0
let fitZoom = 1

let canvasW = 0
let canvasH = 0

const cursorX = ref(0)
const cursorY = ref(0)
const cursorVisible = ref(false)

const canvasWrapStyle = computed(() => ({
  transform: `translate(${panX.value}px, ${panY.value}px) scale(${zoom.value})`,
  transformOrigin: '0 0',
}))

const brushCursorStyle = computed(() => {
  const size = brushSize.value * zoom.value
  return {
    width: size + 'px', height: size + 'px',
    left: (cursorX.value - size / 2) + 'px', top: (cursorY.value - size / 2) + 'px',
    borderColor: paintTool.value === 'eraser' ? '#fff' : 'rgba(255,80,80,0.8)',
  }
})

function openPaintModal() {
  showPaintModal.value = true
  nextTick(() => {
    if (savedPaintData && paintCanvasRef.value) {
      const ctx = paintCanvasRef.value.getContext('2d')
      if (ctx) ctx.putImageData(savedPaintData, 0, 0)
    }
    document.addEventListener('keydown', onKeyDown)
  })
}

function confirmPaintModal() {
  const canvas = paintCanvasRef.value
  if (canvas) {
    const ctx = canvas.getContext('2d')
    if (ctx) { savedPaintData = ctx.getImageData(0, 0, canvas.width, canvas.height); hasPainted.value = isCanvasNonEmpty(canvas) }
  }
  closePaintModal()
  syncPreviewCanvas()
}

function cancelPaintModal() {
  const canvas = paintCanvasRef.value
  if (canvas) {
    const ctx = canvas.getContext('2d')
    if (ctx) { if (savedPaintData) ctx.putImageData(savedPaintData, 0, 0); else ctx.clearRect(0, 0, canvas.width, canvas.height) }
  }
  paintHistory.value = []; redoStack.value = []
  closePaintModal()
}

function closePaintModal() { showPaintModal.value = false; document.removeEventListener('keydown', onKeyDown) }

function syncPreviewCanvas() {
  const preview = previewCanvasRef.value
  if (!preview || !savedPaintData) return
  preview.width = savedPaintData.width; preview.height = savedPaintData.height
  const ctx = preview.getContext('2d')
  if (ctx) ctx.putImageData(savedPaintData, 0, 0)
}

function initCanvas() {
  const canvas = paintCanvasRef.value; const img = bgImageRef.value
  if (!canvas || !img) return
  canvasW = img.naturalWidth; canvasH = img.naturalHeight
  canvas.width = canvasW; canvas.height = canvasH
  paintHistory.value = []; redoStack.value = []
  if (savedPaintData) { const ctx = canvas.getContext('2d'); if (ctx) ctx.putImageData(savedPaintData, 0, 0) }
  nextTick(() => { zoomFit() })
}

watch(uploadedImage, (newVal, oldVal) => {
  hasPainted.value = false; paintHistory.value = []; redoStack.value = []; savedPaintData = null; targetColor.value = ''
  if (newVal && newVal !== oldVal) nextTick(() => { openPaintModal() })
})

function zoomIn() { setZoom(zoom.value * 1.25) }
function zoomOut() { setZoom(zoom.value / 1.25) }
function zoomReset() { zoom.value = 1; centerCanvas() }
function zoomFit() {
  const vp = viewportRef.value; if (!vp || !canvasW || !canvasH) return
  const pad = 40; const zx = (vp.clientWidth - pad) / canvasW; const zy = (vp.clientHeight - pad) / canvasH
  fitZoom = Math.min(zx, zy, 1); zoom.value = fitZoom; centerCanvas()
}
function setZoom(z: number) { zoom.value = Math.max(0.1, Math.min(5, z)); clampPan() }
function centerCanvas() {
  const vp = viewportRef.value; if (!vp) return
  panX.value = (vp.clientWidth - canvasW * zoom.value) / 2; panY.value = (vp.clientHeight - canvasH * zoom.value) / 2
}
function clampPan() {
  const vp = viewportRef.value; if (!vp) return
  const w = canvasW * zoom.value; const h = canvasH * zoom.value; const vpW = vp.clientWidth; const vpH = vp.clientHeight
  if (w <= vpW) panX.value = (vpW - w) / 2; else panX.value = Math.min(0, Math.max(vpW - w, panX.value))
  if (h <= vpH) panY.value = (vpH - h) / 2; else panY.value = Math.min(0, Math.max(vpH - h, panY.value))
}

function viewportToCanvas(vx: number, vy: number): { x: number; y: number } | null {
  if (!canvasW || !canvasH) return null
  const x = (vx - panX.value) / zoom.value; const y = (vy - panY.value) / zoom.value
  if (x < 0 || y < 0 || x > canvasW || y > canvasH) return null; return { x, y }
}
function getViewportPos(e: MouseEvent): { vx: number; vy: number } {
  const vp = viewportRef.value!; const rect = vp.getBoundingClientRect(); return { vx: e.clientX - rect.left, vy: e.clientY - rect.top }
}

let lastPos: { x: number; y: number } | null = null

function onViewportMouseDown(e: MouseEvent) {
  const { vx, vy } = getViewportPos(e)
  if (paintTool.value === 'hand' || e.button === 1) { panning.value = true; panStartX = e.clientX; panStartY = e.clientY; panStartPX = panX.value; panStartPY = panY.value; return }
  const pos = viewportToCanvas(vx, vy); if (!pos) return
  painting.value = true; saveCanvasState(); drawDot(pos.x, pos.y); lastPos = pos; hasPainted.value = true
}
function onViewportMouseMove(e: MouseEvent) {
  const { vx, vy } = getViewportPos(e); cursorX.value = vx; cursorY.value = vy; cursorVisible.value = true
  if (panning.value) { panX.value = panStartPX + (e.clientX - panStartX); panY.value = panStartPY + (e.clientY - panStartY); clampPan(); return }
  if (!painting.value) return; const pos = viewportToCanvas(vx, vy); if (!pos || !lastPos) return; drawLine(lastPos.x, lastPos.y, pos.x, pos.y); lastPos = pos
}
function onViewportMouseUp() { painting.value = false; panning.value = false; lastPos = null }
function onViewportMouseLeave() { painting.value = false; panning.value = false; lastPos = null; cursorVisible.value = false }
function onViewportWheel(e: WheelEvent) {
  const { vx, vy } = getViewportPos(e); const oldZ = zoom.value
  const newZ = Math.max(0.1, Math.min(5, oldZ * (e.deltaY < 0 ? 1.15 : 1 / 1.15))); zoom.value = newZ
  panX.value = vx - (vx - panX.value) * (newZ / oldZ); panY.value = vy - (vy - panY.value) * (newZ / oldZ); clampPan()
}

let lastTouchPos: { x: number; y: number } | null = null
function onViewportTouchStart(e: TouchEvent) {
  if (e.touches.length !== 1) return
  const vp = viewportRef.value!; const rect = vp.getBoundingClientRect(); const vx = e.touches[0].clientX - rect.left; const vy = e.touches[0].clientY - rect.top
  if (paintTool.value === 'hand') { panning.value = true; panStartX = e.touches[0].clientX; panStartY = e.touches[0].clientY; panStartPX = panX.value; panStartPY = panY.value; return }
  const pos = viewportToCanvas(vx, vy); if (!pos) return; painting.value = true; saveCanvasState(); drawDot(pos.x, pos.y); lastTouchPos = pos; hasPainted.value = true
}
function onViewportTouchMove(e: TouchEvent) {
  if (e.touches.length !== 1) return
  if (panning.value) { panX.value = panStartPX + (e.touches[0].clientX - panStartX); panY.value = panStartPY + (e.touches[0].clientY - panStartY); clampPan(); return }
  if (!painting.value) return
  const vp = viewportRef.value!; const rect = vp.getBoundingClientRect(); const vx = e.touches[0].clientX - rect.left; const vy = e.touches[0].clientY - rect.top
  const pos = viewportToCanvas(vx, vy); if (!pos || !lastTouchPos) return; drawLine(lastTouchPos.x, lastTouchPos.y, pos.x, pos.y); lastTouchPos = pos
}
function onViewportTouchEnd() { painting.value = false; panning.value = false; lastTouchPos = null }

function saveCanvasState() {
  const canvas = paintCanvasRef.value; if (!canvas) return; const ctx = canvas.getContext('2d'); if (!ctx) return
  paintHistory.value.push(ctx.getImageData(0, 0, canvas.width, canvas.height))
  if (paintHistory.value.length > 50) paintHistory.value.shift(); redoStack.value = []
}
function drawDot(x: number, y: number) {
  const canvas = paintCanvasRef.value; if (!canvas) return; const ctx = canvas.getContext('2d'); if (!ctx) return
  ctx.beginPath(); ctx.arc(x, y, brushSize.value / 2, 0, Math.PI * 2)
  if (paintTool.value === 'eraser') { ctx.globalCompositeOperation = 'destination-out'; ctx.fill(); ctx.globalCompositeOperation = 'source-over' }
  else { ctx.fillStyle = 'rgba(255, 80, 80, 0.55)'; ctx.fill() }
}
function drawLine(x1: number, y1: number, x2: number, y2: number) {
  const canvas = paintCanvasRef.value; if (!canvas) return; const ctx = canvas.getContext('2d'); if (!ctx) return
  ctx.lineWidth = brushSize.value; ctx.lineCap = 'round'; ctx.lineJoin = 'round'
  if (paintTool.value === 'eraser') { ctx.globalCompositeOperation = 'destination-out'; ctx.strokeStyle = 'rgba(0,0,0,1)'; ctx.beginPath(); ctx.moveTo(x1, y1); ctx.lineTo(x2, y2); ctx.stroke(); ctx.globalCompositeOperation = 'source-over' }
  else { ctx.strokeStyle = 'rgba(255, 80, 80, 0.55)'; ctx.beginPath(); ctx.moveTo(x1, y1); ctx.lineTo(x2, y2); ctx.stroke() }
}
function undoPaint() {
  const canvas = paintCanvasRef.value; if (!canvas || paintHistory.value.length === 0) return; const ctx = canvas.getContext('2d'); if (!ctx) return
  redoStack.value.push(ctx.getImageData(0, 0, canvas.width, canvas.height)); const prev = paintHistory.value.pop()!; ctx.putImageData(prev, 0, 0)
  if (paintHistory.value.length === 0) hasPainted.value = isCanvasNonEmpty(canvas)
}
function redoPaint() {
  const canvas = paintCanvasRef.value; if (!canvas || redoStack.value.length === 0) return; const ctx = canvas.getContext('2d'); if (!ctx) return
  paintHistory.value.push(ctx.getImageData(0, 0, canvas.width, canvas.height)); const next = redoStack.value.pop()!; ctx.putImageData(next, 0, 0); hasPainted.value = isCanvasNonEmpty(canvas)
}
function clearPaint() {
  const canvas = paintCanvasRef.value; if (!canvas) return; const ctx = canvas.getContext('2d'); if (!ctx) return
  saveCanvasState(); ctx.clearRect(0, 0, canvas.width, canvas.height); hasPainted.value = false
}
function isCanvasNonEmpty(canvas: HTMLCanvasElement): boolean {
  const ctx = canvas.getContext('2d'); if (!ctx) return false
  const data = ctx.getImageData(0, 0, canvas.width, canvas.height).data
  for (let i = 3; i < data.length; i += 4) { if (data[i] > 0) return true }; return false
}
function onKeyDown(e: KeyboardEvent) {
  if (!showPaintModal.value) return
  const ctrl = e.ctrlKey || e.metaKey
  if (ctrl && e.shiftKey && e.key.toLowerCase() === 'z') { e.preventDefault(); redoPaint(); return }
  if (ctrl && e.key.toLowerCase() === 'z') { e.preventDefault(); undoPaint(); return }
  if (e.key === 'b' || e.key === 'B') { paintTool.value = 'brush'; return }
  if (e.key === 'e' || e.key === 'E') { paintTool.value = 'eraser'; return }
  if (e.key === 'h' || e.key === 'H') { paintTool.value = 'hand'; return }
  if (e.key === '=' || e.key === '+') { e.preventDefault(); zoomIn(); return }
  if (e.key === '-') { e.preventDefault(); zoomOut(); return }
  if (e.key === '0') { e.preventDefault(); zoomFit(); return }
  if (e.key === '1') { e.preventDefault(); zoomReset(); return }
  if (e.key === '[') { brushSize.value = Math.max(5, brushSize.value - 5); return }
  if (e.key === ']') { brushSize.value = Math.min(80, brushSize.value + 5); return }
}

async function exportPaintedImage(): Promise<File> {
  if (!savedPaintData) throw new Error('无涂抹数据')
  const img = await new Promise<HTMLImageElement>((resolve, reject) => {
    const el = new Image(); el.crossOrigin = 'anonymous'; el.onload = () => resolve(el); el.onerror = () => reject(new Error('原图加载失败')); el.src = uploadedImage.value
  })
  const paintLayer = document.createElement('canvas'); paintLayer.width = savedPaintData.width; paintLayer.height = savedPaintData.height
  paintLayer.getContext('2d')!.putImageData(savedPaintData, 0, 0)
  const offscreen = document.createElement('canvas'); offscreen.width = img.naturalWidth; offscreen.height = img.naturalHeight
  const ctx = offscreen.getContext('2d')!
  ctx.drawImage(img, 0, 0, img.naturalWidth, img.naturalHeight)
  ctx.globalAlpha = 0.35; ctx.drawImage(paintLayer, 0, 0, paintLayer.width, paintLayer.height, 0, 0, img.naturalWidth, img.naturalHeight); ctx.globalAlpha = 1.0
  return new Promise((resolve) => { offscreen.toBlob((blob) => { resolve(new File([blob!], 'painted_' + Date.now() + '.png', { type: 'image/png' })) }, 'image/png') })
}

// ============ HSV 颜色选择器 ============
const svCanvasRef = ref<HTMLCanvasElement | null>(null)
const hueCanvasRef = ref<HTMLCanvasElement | null>(null)
const pickerHue = ref(0)
const pickerSat = ref(1)
const pickerVal = ref(1)
const targetColor = ref('')

const svCursor = computed(() => {
  const w = 260, h = 160
  return { x: pickerSat.value * w, y: (1 - pickerVal.value) * h }
})
const hueCursorX = computed(() => (pickerHue.value / 360) * 260)

function hsvToRgb(h: number, s: number, v: number): { r: number; g: number; b: number } {
  let r = 0, g = 0, b = 0
  const i = Math.floor(h / 60) % 6; const f = h / 60 - Math.floor(h / 60)
  const p = v * (1 - s); const q = v * (1 - f * s); const t = v * (1 - (1 - f) * s)
  switch (i) { case 0: r = v; g = t; b = p; break; case 1: r = q; g = v; b = p; break; case 2: r = p; g = v; b = t; break; case 3: r = p; g = q; b = v; break; case 4: r = t; g = p; b = v; break; case 5: r = v; g = p; b = q; break }
  return { r: Math.round(r * 255), g: Math.round(g * 255), b: Math.round(b * 255) }
}
function rgbToHsv(r: number, g: number, b: number): [number, number, number] {
  r /= 255; g /= 255; b /= 255
  const max = Math.max(r, g, b); const min = Math.min(r, g, b); const d = max - min
  let h = 0; const s = max === 0 ? 0 : d / max; const v = max
  if (d !== 0) { switch (max) { case r: h = ((g - b) / d + (g < b ? 6 : 0)) * 60; break; case g: h = ((b - r) / d + 2) * 60; break; case b: h = ((r - g) / d + 4) * 60; break } }
  return [h, s, v]
}
function rgbToHex(r: number, g: number, b: number): string {
  return '#' + [r, g, b].map(c => c.toString(16).padStart(2, '0')).join('')
}

const pickerRgb = computed(() => hsvToRgb(pickerHue.value, pickerSat.value, pickerVal.value))
const pickerHex = computed(() => rgbToHex(pickerRgb.value.r, pickerRgb.value.g, pickerRgb.value.b))

function drawSvCanvas() {
  const canvas = svCanvasRef.value; if (!canvas) return; const ctx = canvas.getContext('2d'); if (!ctx) return
  const w = canvas.width; const h = canvas.height
  const hueColor = hsvToRgb(pickerHue.value, 1, 1)
  const baseColor = `rgb(${hueColor.r},${hueColor.g},${hueColor.b})`
  ctx.fillStyle = baseColor; ctx.fillRect(0, 0, w, h)
  const gradW = ctx.createLinearGradient(0, 0, w, 0); gradW.addColorStop(0, '#fff'); gradW.addColorStop(1, 'rgba(255,255,255,0)'); ctx.fillStyle = gradW; ctx.fillRect(0, 0, w, h)
  const gradB = ctx.createLinearGradient(0, 0, 0, h); gradB.addColorStop(0, 'rgba(0,0,0,0)'); gradB.addColorStop(1, '#000'); ctx.fillStyle = gradB; ctx.fillRect(0, 0, w, h)
}
function drawHueCanvas() {
  const canvas = hueCanvasRef.value; if (!canvas) return; const ctx = canvas.getContext('2d'); if (!ctx) return
  const grad = ctx.createLinearGradient(0, 0, canvas.width, 0)
  for (let i = 0; i <= 6; i++) { const c = hsvToRgb(i * 60, 1, 1); grad.addColorStop(i / 6, `rgb(${c.r},${c.g},${c.b})`) }
  ctx.fillStyle = grad; ctx.fillRect(0, 0, canvas.width, canvas.height)
}

watch(pickerHue, () => { drawSvCanvas() })
watch(hasPainted, (val) => { if (val) nextTick(() => { drawSvCanvas(); drawHueCanvas() }) })

function onSvMouseDown(e: MouseEvent) { updateSv(e); const move = (ev: MouseEvent) => updateSv(ev); const up = () => { document.removeEventListener('mousemove', move); document.removeEventListener('mouseup', up) }; document.addEventListener('mousemove', move); document.addEventListener('mouseup', up) }
function onSvTouchStart(e: TouchEvent) { updateSvTouch(e); const move = (ev: TouchEvent) => { ev.preventDefault(); updateSvTouch(ev) }; const up = () => { document.removeEventListener('touchmove', move as any); document.removeEventListener('touchend', up) }; document.addEventListener('touchmove', move as any, { passive: false }); document.addEventListener('touchend', up) }
function updateSv(e: MouseEvent) { const rect = svCanvasRef.value!.getBoundingClientRect(); pickerSat.value = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width)); pickerVal.value = Math.max(0, Math.min(1, 1 - (e.clientY - rect.top) / rect.height)) }
function updateSvTouch(e: TouchEvent) { const t = e.touches[0]; const rect = svCanvasRef.value!.getBoundingClientRect(); pickerSat.value = Math.max(0, Math.min(1, (t.clientX - rect.left) / rect.width)); pickerVal.value = Math.max(0, Math.min(1, 1 - (t.clientY - rect.top) / rect.height)) }

function onHueMouseDown(e: MouseEvent) { updateHue(e); const move = (ev: MouseEvent) => updateHue(ev); const up = () => { document.removeEventListener('mousemove', move); document.removeEventListener('mouseup', up) }; document.addEventListener('mousemove', move); document.addEventListener('mouseup', up) }
function onHueTouchStart(e: TouchEvent) { updateHueTouch(e); const move = (ev: TouchEvent) => { ev.preventDefault(); updateHueTouch(ev) }; const up = () => { document.removeEventListener('touchmove', move as any); document.removeEventListener('touchend', up) }; document.addEventListener('touchmove', move as any, { passive: false }); document.addEventListener('touchend', up) }
function updateHue(e: MouseEvent) { const rect = hueCanvasRef.value!.getBoundingClientRect(); pickerHue.value = Math.max(0, Math.min(360, ((e.clientX - rect.left) / rect.width) * 360)) }
function updateHueTouch(e: TouchEvent) { const t = e.touches[0]; const rect = hueCanvasRef.value!.getBoundingClientRect(); pickerHue.value = Math.max(0, Math.min(360, ((t.clientX - rect.left) / rect.width) * 360)) }

function onHexInput(e: Event) {
  let hex = (e.target as HTMLInputElement).value.trim()
  if (!hex.startsWith('#')) hex = '#' + hex
  if (/^#[0-9a-fA-F]{6}$/.test(hex)) {
    const r = parseInt(hex.slice(1, 3), 16); const g = parseInt(hex.slice(3, 5), 16); const b = parseInt(hex.slice(5, 7), 16)
    const [h, s, v] = rgbToHsv(r, g, b); pickerHue.value = h; pickerSat.value = s; pickerVal.value = v
  }
}
function onRgbInput(e: Event, channel: 'r' | 'g' | 'b') {
  const val = Math.max(0, Math.min(255, parseInt((e.target as HTMLInputElement).value) || 0))
  const rgb = { ...pickerRgb.value, [channel]: val }; const [h, s, v] = rgbToHsv(rgb.r, rgb.g, rgb.b); pickerHue.value = h; pickerSat.value = s; pickerVal.value = v
}

function confirmPickerColor() { targetColor.value = pickerHex.value }

// ============ 消息气泡 ============
interface ChatMessage {
  id: string; role: 'user' | 'assistant'; prompt?: string; refImageUrl?: string
  status?: 'pending' | 'completed' | 'failed'; imageUrl?: string; errorMsg?: string
  taskId?: string; elapsed?: string; time?: string
}
const messages = ref<ChatMessage[]>([])
const chatMessagesRef = ref<HTMLElement | null>(null)
function scrollToBottom() { nextTick(() => { if (chatMessagesRef.value) chatMessagesRef.value.scrollTop = chatMessagesRef.value.scrollHeight }) }

// ============ 生成 ============
const generating = ref(false)
const uploading = ref(false)
const currentTaskId = ref<string | null>(null)
const lightboxUrl = ref('')

const canGenerate = computed(() => {
  if (!userStore.isLoggedIn) return false
  if (generating.value || uploading.value) return false
  if (!uploadedImage.value) return false
  if (!hasPainted.value) return false
  if (!targetColor.value) return false
  return true
})

async function handleGenerate() {
  if (!userStore.isLoggedIn) { message.warning('请先登录'); router.push('/user/login'); return }
  if (!uploadedImage.value) { message.warning('请先上传图片'); return }
  if (!hasPainted.value || !savedPaintData) { message.warning('请先涂抹需要换色的区域'); return }
  if (!targetColor.value) { message.warning('请选择目标颜色'); return }

  activeTaskId.value = null
  const now = new Date()
  const timeStr = `${now.getHours().toString().padStart(2, '0')}:${now.getMinutes().toString().padStart(2, '0')}`
  const displayPrompt = `${FEATURE_TITLE} → ${targetColor.value}`

  // Step 1: 导出涂抹后的图并上传
  uploading.value = true
  let paintedUrl: string
  try {
    const file = await exportPaintedImage()
    const uploadRes: any = await uploadApi.upload(file)
    if (uploadRes.code !== 0 || !uploadRes.data?.url) { message.error(uploadRes.msg || '涂抹图上传失败'); uploading.value = false; return }
    paintedUrl = uploadRes.data.url
  } catch (e: any) { message.error('涂抹图上传失败: ' + (e.message || '网络错误')); uploading.value = false; return }
  uploading.value = false

  // Step 2: 提交任务
  const userMsg: ChatMessage = { id: 'msg_' + Date.now(), role: 'user', prompt: displayPrompt, refImageUrl: paintedUrl, time: timeStr }
  messages.value.push(userMsg)

  const assistantMsg: ChatMessage = { id: 'loading_' + Date.now(), role: 'assistant', prompt: displayPrompt, status: 'pending' }
  messages.value.push(assistantMsg)
  const assistantRef = messages.value[messages.value.length - 1]
  scrollToBottom()

  generating.value = true; currentTaskId.value = null

  try {
    const res: any = await taskApi.submit({
      prompt: FEATURE_PREFIX + `目标颜色：${targetColor.value}`,
      params: {
        image_urls: [paintedUrl],
        original_image_url: uploadedImage.value,
        feature: FEATURE_SLUG,
        target_color: targetColor.value,
      },
    })
    if (res.code === 0) { currentTaskId.value = res.data.task_id; assistantRef.taskId = res.data.task_id; pollTaskStatus(res.data.task_id, assistantRef) }
    else { assistantRef.status = 'failed'; assistantRef.errorMsg = res.msg || '提交失败'; generating.value = false }
  } catch (e: any) { assistantRef.status = 'failed'; assistantRef.errorMsg = e.msg || '网络错误，请重试'; generating.value = false }
}

let pollTimer: number | null = null

async function pollTaskStatus(taskId: string, assistantMsg: ChatMessage) {
  if (pollTimer) clearTimeout(pollTimer)
  try {
    const res: any = await taskApi.get(taskId)
    if (res.code === 0) {
      const task = res.data.task
      if (task.status === 'completed') {
        try { const result = JSON.parse(task.result || '{}'); assistantMsg.imageUrl = result.image_url || ''; if (task.completed_at) { const t = new Date(task.updated_at).getTime() - new Date(task.created_at).getTime(); assistantMsg.elapsed = (t / 1000).toFixed(1) } } catch { assistantMsg.imageUrl = '' }
        assistantMsg.status = 'completed'; generating.value = false; scrollToBottom(); loadHistory(); return
      } else if (task.status === 'failed') {
        assistantMsg.status = 'failed'; assistantMsg.errorMsg = task.error_msg || '生成失败'; generating.value = false; scrollToBottom(); loadHistory(); return
      }
    }
  } catch {}
  pollTimer = window.setTimeout(() => pollTaskStatus(taskId, assistantMsg), 3000)
}

function handleClear() {
  uploadedImage.value = ''; targetColor.value = ''; messages.value = []; activeTaskId.value = null
  hasPainted.value = false; paintHistory.value = []; redoStack.value = []; savedPaintData = null
}

function handleImgError(e: Event) { const img = e.target as HTMLImageElement; img.style.background = '#27272a' }
function downloadImage(url?: string) {
  downloadImageByProxy(url)
}

// ============ 生命周期 ============
onMounted(() => { if (userStore.isLoggedIn) loadHistory() })
onUnmounted(() => { if (pollTimer) clearTimeout(pollTimer); document.removeEventListener('keydown', onKeyDown) })
</script>

<style scoped>
/* ============ 整体布局 ============ */
.home-layout { width: 100%; height: 100%; display: flex; flex-direction: column; background: #111113; overflow: hidden; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif; }
.home-body { flex: 1; display: grid; grid-template-columns: 220px 1fr 380px; min-height: 0; overflow: hidden; }

/* ============ 左侧画廊 ============ */
.history-panel { border-right: 1px solid #222225; background: #111113; display: flex; flex-direction: column; overflow: hidden; min-height: 0; }
.history-header { padding: 14px 16px 10px; font-size: 13px; font-weight: 600; color: #ccc; border-bottom: 1px solid #222225; flex-shrink: 0; display: flex; align-items: center; justify-content: space-between; }
.gallery-count { font-size: 11px; color: #a3e635; background: rgba(163,230,53,0.1); padding: 1px 8px; border-radius: 10px; font-weight: 500; }
.gallery-grid { flex: 1; overflow-y: auto; padding: 8px; column-count: 2; column-gap: 6px; }
.gallery-item { break-inside: avoid; margin-bottom: 6px; border-radius: 8px; overflow: hidden; position: relative; cursor: pointer; background: #1a1a1d; border: 2px solid transparent; transition: border-color 0.2s; }
.gallery-item.active { border-color: #a3e635; }
.gallery-item img { display: block; width: 100%; }
.gallery-item-overlay { position: absolute; bottom: 0; left: 0; right: 0; padding: 4px 6px; background: linear-gradient(transparent, rgba(0,0,0,0.7)); }
.gallery-item-prompt { font-size: 10px; color: #aaa; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; display: block; }
.gallery-delete-btn { position: absolute; top: 4px; right: 4px; width: 20px; height: 20px; border-radius: 50%; background: rgba(0,0,0,0.6); border: none; color: #999; cursor: pointer; display: flex; align-items: center; justify-content: center; opacity: 0; transition: opacity 0.15s; }
.gallery-item:hover .gallery-delete-btn { opacity: 1; }
.history-empty { flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; color: #555; font-size: 12px; gap: 6px; }

/* ============ 中间生成区 ============ */
.canvas-area { display: flex; flex-direction: column; overflow: hidden; background: #111113; }
.guest-prompt { flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 8px; color: #888; }
.guest-title { font-size: 15px; font-weight: 600; color: #ccc; }
.guest-desc { font-size: 13px; color: #888; }
.guest-action { font-size: 13px; color: #888; }
.link-highlight { color: #a3e635; text-decoration: none; font-weight: 600; }
.gen-display { flex: 1; overflow-y: auto; padding: 24px 32px; display: flex; flex-direction: column; gap: 16px; }
.chat-empty { flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 8px; }
.empty-icon-wrap { margin-bottom: 8px; }
.chat-empty-text { font-size: 16px; color: #888; font-weight: 500; }
.chat-empty-sub { font-size: 12px; color: #555; }

/* ============ 聊天气泡 ============ */
.chat-bubble { display: flex; flex-direction: column; }
.chat-bubble.user { align-items: flex-end; }
.chat-bubble.assistant { align-items: flex-start; }
.bubble-row { max-width: 80%; }
.bubble-row.user { display: flex; flex-direction: column; align-items: flex-end; gap: 4px; }
.bubble-row.ai { display: flex; flex-direction: column; align-items: flex-start; gap: 4px; }
.bubble-body { border-radius: 12px; padding: 12px 16px; }
.bubble-body.user { background: rgba(163,230,53,0.1); border: 1px solid rgba(163,230,53,0.2); }
.bubble-body.failed { background: rgba(248,113,113,0.08); border: 1px solid rgba(248,113,113,0.25); }
.bubble-body.ai-success { background: #1a1a1e; border: 1px solid #27272a; }
.bubble-prompt { font-size: 13px; color: #ccc; line-height: 1.6; margin: 0; word-break: break-word; }
.bubble-time { font-size: 10px; color: #444; padding: 0 4px; }
.bubble-ref-image { margin-top: 8px; max-width: 160px; border-radius: 8px; overflow: hidden; }
.bubble-ref-image img { width: 100%; display: block; }
.bubble-failed-row { display: flex; align-items: center; gap: 12px; }
.bubble-failed-icon { width: 32px; height: 32px; border-radius: 50%; background: rgba(248,113,113,0.15); color: #f87171; display: flex; align-items: center; justify-content: center; font-size: 16px; flex-shrink: 0; }
.bubble-failed-info { flex: 1; }
.bubble-failed-title { font-size: 13px; color: #f87171; font-weight: 500; }
.bubble-failed-reason { font-size: 12px; color: #888; margin-top: 3px; word-break: break-word; }
.bubble-image-wrap { border-radius: 10px; overflow: hidden; max-width: 400px; cursor: pointer; position: relative; }
.bubble-image-wrap:hover .image-zoom-hint { opacity: 1; }
.image-zoom-hint { position: absolute; top: 8px; right: 8px; width: 32px; height: 32px; border-radius: 8px; background: rgba(0,0,0,0.55); display: flex; align-items: center; justify-content: center; color: #fff; opacity: 0; transition: opacity 0.2s; pointer-events: none; }
.bubble-image { width: 100%; display: block; transition: transform 0.2s; }
.bubble-image-wrap:hover .bubble-image { transform: scale(1.02); }
.bubble-footer { display: flex; align-items: center; justify-content: space-between; margin-top: 10px; padding-top: 8px; border-top: 1px solid #27272a; }
.bubble-meta { font-size: 11px; color: #555; }
.result-actions { display: flex; align-items: center; gap: 8px; }
.result-action-btn { display: inline-flex; align-items: center; gap: 6px; padding: 5px 10px; border-radius: 6px; border: 1px solid #333; background: #27272a; color: #aaa; font-size: 12px; cursor: pointer; transition: all 0.2s; }
.result-action-btn:hover { border-color: #444; color: #ddd; }
.result-action-btn.primary { background: rgba(163,230,53,0.1); border-color: rgba(163,230,53,0.3); color: #a3e635; }
.result-action-btn.primary:hover { background: rgba(163,230,53,0.18); border-color: #a3e635; }
.result-action-btn.danger { color: #888; }
.result-action-btn.danger:hover { background: rgba(220,38,38,0.12); border-color: rgba(220,38,38,0.4); color: #ef4444; }
.chat-bubble.highlight .bubble-body, .chat-bubble.highlight .bubble-image-wrap { outline: 2px solid #a3e635; outline-offset: 2px; transition: outline 0.3s; }

/* 删除确认浮层 */
.delete-popover { position: fixed; z-index: 1000; background: #27272a; border: 1px solid #3f3f46; border-radius: 10px; padding: 12px 16px; box-shadow: 0 8px 24px rgba(0,0,0,0.4); transform: translate(-50%, -100%); margin-top: -8px; }
.delete-popover p { margin: 0 0 8px; font-size: 13px; color: #e4e4e7; }
.delete-popover-btns { display: flex; gap: 8px; justify-content: flex-end; }
.delete-popover-btns button { padding: 4px 14px; border-radius: 6px; border: 1px solid #3f3f46; background: #18181b; color: #a1a1aa; font-size: 12px; cursor: pointer; }
.delete-popover-btns button.confirm { background: rgba(220,38,38,0.2); border-color: rgba(220,38,38,0.4); color: #ef4444; }

/* 骨架占位 */
.skeleton-wrap { width: 240px; aspect-ratio: 3 / 4; border-radius: 12px; background: #1a1a1e; border: 1px solid #27272a; position: relative; overflow: hidden; }
.skeleton-shimmer { position: absolute; inset: 0; background: linear-gradient(110deg, transparent 30%, rgba(163,230,53,0.04) 50%, transparent 70%); background-size: 300% 100%; animation: shimmer 2s ease-in-out infinite; }
@keyframes shimmer { 0% { background-position: 200% 0; } 100% { background-position: -100% 0; } }
.skeleton-overlay { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 12px; }
.skeleton-spinner { width: 32px; height: 32px; border: 3px solid #27272a; border-top-color: #a3e635; border-radius: 50%; animation: spin 1s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
.skeleton-text { font-size: 13px; color: #888; font-weight: 500; }
.skeleton-sub { font-size: 11px; color: #555; }

/* ============ 右侧工作区 ============ */
.control-panel { display: flex; flex-direction: column; border-left: 1px solid #222225; background: #18181b; overflow: hidden; }
.panel-top { padding: 12px 16px 0; flex-shrink: 0; }
.panel-tabs { display: flex; gap: 0; background: #27272a; border-radius: 8px; padding: 3px; }
.panel-tab { flex: 1; padding: 7px 0; border: none; border-radius: 6px; background: transparent; color: #888; font-size: 13px; font-weight: 500; cursor: pointer; transition: all 0.2s; }
.panel-tab.active { background: #3f3f46; color: #e4e4e7; }
.panel-scroll { flex: 1; overflow-y: auto; padding: 16px; }
.section { margin-bottom: 20px; }
.section-header-row { display: flex; align-items: center; gap: 8px; margin-bottom: 10px; }
.section-num { width: 22px; height: 22px; border-radius: 50%; background: rgba(163,230,53,0.12); color: #a3e635; font-size: 12px; font-weight: 700; display: flex; align-items: center; justify-content: center; flex-shrink: 0; }
.section-label { font-size: 13px; font-weight: 600; color: #d4d4d8; }

/* ============ 涂抹预览缩略图 ============ */
.paint-preview { position: relative; border-radius: 10px; overflow: hidden; cursor: pointer; border: 1px solid #27272a; transition: border-color 0.2s; }
.paint-preview:hover { border-color: #a3e635; }
.paint-preview-img { display: block; width: 100%; height: auto; pointer-events: none; }
.paint-preview-overlay { position: absolute; top: 0; left: 0; width: 100%; height: 100%; pointer-events: none; }
.paint-preview-mask { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 6px; background: rgba(0,0,0,0.5); color: #a3e635; font-size: 13px; font-weight: 500; opacity: 0; transition: opacity 0.2s; }
.paint-preview:hover .paint-preview-mask { opacity: 1; }
.paint-done-tip { display: flex; align-items: center; gap: 6px; margin-top: 8px; font-size: 12px; color: #a3e635; }

/* ============ 涂抹工作台弹窗 ============ */
.paint-modal-overlay { position: fixed; inset: 0; z-index: 1100; background: rgba(0,0,0,0.85); display: flex; align-items: center; justify-content: center; }
.paint-modal { width: 94vw; max-width: 1100px; height: 90vh; background: #18181b; border-radius: 14px; border: 1px solid #27272a; display: flex; flex-direction: column; overflow: hidden; box-shadow: 0 20px 60px rgba(0,0,0,0.5); }
.paint-modal-header { display: flex; align-items: center; gap: 12px; padding: 10px 20px; border-bottom: 1px solid #27272a; flex-shrink: 0; }
.paint-modal-title { font-size: 14px; font-weight: 600; color: #e4e4e7; }
.paint-modal-hint { font-size: 12px; color: #71717a; flex: 1; }
.header-actions { display: flex; gap: 8px; }
.paint-modal-body { flex: 1; overflow: hidden; position: relative; background: #0a0a0c; cursor: crosshair; }
.paint-modal-body.cursor-grab { cursor: grab; }
.paint-modal-body.cursor-grabbing { cursor: grabbing; }
.paint-modal-footer { padding: 10px 16px; border-top: 1px solid #27272a; flex-shrink: 0; }
.paint-canvas-wrap { position: absolute; top: 0; left: 0; border: 1px solid #333; background: repeating-conic-gradient(#1a1a1e 0% 25%, #111113 0% 50%) 0 0 / 16px 16px; }
.paint-bg-image { display: block; width: 100%; height: auto; pointer-events: none; user-select: none; }
.paint-canvas { position: absolute; top: 0; left: 0; width: 100%; height: 100%; touch-action: none; }
.paint-hint { position: absolute; bottom: 12px; left: 50%; transform: translateX(-50%); display: flex; align-items: center; gap: 6px; padding: 6px 14px; background: rgba(0,0,0,0.75); border-radius: 8px; color: #a3e635; font-size: 12px; pointer-events: none; white-space: nowrap; }
.brush-cursor { position: absolute; pointer-events: none; border: 2px solid rgba(255,80,80,0.8); border-radius: 50%; box-sizing: border-box; z-index: 10; mix-blend-mode: difference; }

/* ============ 画笔工具栏 ============ */
.paint-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.toolbar-group { display: flex; align-items: center; gap: 5px; }
.toolbar-divider { width: 1px; height: 22px; background: #333; margin: 0 4px; }
.tool-btn { width: 34px; height: 34px; border-radius: 8px; border: 1px solid #333; background: #27272a; color: #aaa; cursor: pointer; display: flex; align-items: center; justify-content: center; transition: all 0.15s; flex-shrink: 0; }
.tool-btn:hover { border-color: #555; color: #ddd; }
.tool-btn.active { background: rgba(163,230,53,0.12); border-color: #a3e635; color: #a3e635; }
.tool-btn:disabled { opacity: 0.35; cursor: not-allowed; }
.tool-btn-text { padding: 6px 18px; border-radius: 8px; border: 1px solid #333; font-size: 13px; font-weight: 500; cursor: pointer; transition: all 0.15s; white-space: nowrap; }
.tool-btn-text.cancel { background: #27272a; color: #aaa; }
.tool-btn-text.cancel:hover { background: #3f3f46; color: #ddd; }
.tool-btn-text.confirm { background: linear-gradient(135deg, #a3e635 0%, #65a30d 100%); border-color: #65a30d; color: #111; font-weight: 600; }
.tool-btn-text.confirm:hover { opacity: 0.9; }
.zoom-label { font-size: 11px; color: #888; min-width: 38px; text-align: center; font-variant-numeric: tabular-nums; }
.brush-size-wrap { display: flex; align-items: center; gap: 6px; margin-left: 2px; }
.brush-size-label { font-size: 11px; color: #888; min-width: 34px; text-align: center; }
.brush-size-slider { width: 90px; height: 4px; -webkit-appearance: none; appearance: none; background: #333; border-radius: 2px; outline: none; }
.brush-size-slider::-webkit-slider-thumb { -webkit-appearance: none; width: 14px; height: 14px; border-radius: 50%; background: #a3e635; cursor: pointer; }

/* ============ 目标颜色选择 ============ */
.target-color-bar {
  display: flex; align-items: center; gap: 10px;
  padding: 10px 14px; border-radius: 10px;
  background: #27272a; margin-bottom: 12px;
}
.target-color-dot {
  width: 32px; height: 32px; border-radius: 50%;
  border: 2px solid rgba(255,255,255,0.15); flex-shrink: 0;
}
.target-color-hex {
  font-size: 14px; font-family: monospace; color: #e4e4e7; font-weight: 600; flex: 1;
}
.target-color-clear {
  background: none; border: none; color: #71717a; cursor: pointer; font-size: 14px;
  padding: 2px 6px; border-radius: 4px; transition: all 0.15s;
}
.target-color-clear:hover { color: #f87171; background: rgba(248,113,113,0.1); }

/* HSV 颜色选择器 */
.color-picker-inline {
  display: flex; flex-direction: column; gap: 10px;
  background: #1a1a1e;
  border: 1px solid #27272a;
  border-radius: 10px;
  padding: 12px;
}
.sv-canvas-wrap { position: relative; width: 260px; height: 160px; border-radius: 6px; overflow: hidden; cursor: crosshair; }
.sv-canvas { display: block; width: 260px; height: 160px; }
.sv-cursor {
  position: absolute; width: 14px; height: 14px; border-radius: 50%;
  border: 2px solid #fff; box-shadow: 0 0 2px rgba(0,0,0,0.6);
  pointer-events: none; transform: translate(-50%, -50%);
}
.hue-bar-wrap { position: relative; width: 260px; height: 14px; border-radius: 7px; overflow: hidden; cursor: pointer; }
.hue-bar { display: block; width: 260px; height: 14px; border-radius: 7px; }
.hue-cursor {
  position: absolute; top: -1px; width: 6px; height: 16px; border-radius: 3px;
  border: 2px solid #fff; box-shadow: 0 0 2px rgba(0,0,0,0.5);
  pointer-events: none; transform: translateX(-50%);
}
.picker-row {
  display: flex; align-items: center; gap: 10px; margin-top: 10px;
}
.picker-preview {
  width: 36px; height: 36px; border-radius: 8px;
  border: 2px solid rgba(255,255,255,0.1); flex-shrink: 0;
}
.picker-inputs { display: flex; gap: 6px; flex: 1; }
.picker-input-group { display: flex; flex-direction: column; gap: 2px; flex: 1; }
.picker-input-group label { font-size: 9px; color: #555; text-transform: uppercase; }
.picker-input {
  width: 100%; padding: 4px 6px; border-radius: 4px;
  border: 1px solid #333; background: #111; color: #ccc;
  font-size: 11px; text-align: center; outline: none;
  font-family: monospace;
}
.picker-input:focus { border-color: #a3e635; }
.picker-input.hex { font-size: 11px; }
.picker-confirm-btn {
  padding: 6px 14px; border-radius: 8px; border: none;
  background: linear-gradient(135deg, #a3e635 0%, #65a30d 100%);
  color: #111; font-size: 12px; font-weight: 600; cursor: pointer;
  white-space: nowrap; transition: opacity 0.15s; flex-shrink: 0;
}
.picker-confirm-btn:hover { opacity: 0.85; }

/* 底部 */
.panel-bottom { padding: 16px; border-top: 1px solid #27272a; flex-shrink: 0; }
.button-group { display: flex; gap: 10px; }
.btn-generate { flex: 1; padding: 12px 24px; border-radius: 10px; border: none; background: linear-gradient(135deg, #a3e635 0%, #65a30d 100%); color: #111; font-size: 14px; font-weight: 600; cursor: pointer; transition: all 0.2s; }
.btn-generate:hover:not(:disabled) { opacity: 0.9; transform: translateY(-1px); }
.btn-generate:disabled { opacity: 0.4; cursor: not-allowed; }
.btn-clear { padding: 12px 24px; border-radius: 10px; border: 1px solid #333; background: #27272a; color: #aaa; font-size: 14px; cursor: pointer; }
.btn-clear:hover:not(:disabled) { background: #3f3f46; }

/* lightbox */
.lightbox-overlay { position: fixed; inset: 0; z-index: 999; background: rgba(0,0,0,0.85); display: flex; align-items: center; justify-content: center; }
.lightbox-container { position: relative; max-width: 90vw; max-height: 90vh; }
.lightbox-image { max-width: 100%; max-height: 85vh; border-radius: 8px; }
.lightbox-close { position: absolute; top: -10px; right: -10px; width: 32px; height: 32px; border-radius: 50%; background: #333; border: none; color: #ccc; cursor: pointer; display: flex; align-items: center; justify-content: center; }
.lightbox-actions { display: flex; gap: 8px; justify-content: center; margin-top: 12px; }
.lightbox-action-btn { padding: 8px 16px; border-radius: 8px; border: 1px solid #444; background: #27272a; color: #ccc; font-size: 13px; cursor: pointer; display: flex; align-items: center; gap: 6px; }
</style>
