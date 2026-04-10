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
              <span class="gallery-item-prompt">{{ ht.prompt }}</span>
            </div>
            <button class="gallery-delete-btn" @click.stop="deleteTask(ht.task_id)" title="删除">
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
            </button>
          </div>
        </div>
        <div class="history-empty" v-else>
          <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="#333" stroke-width="1.5"><rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="8.5" cy="8.5" r="1.5"/><path d="m21 15-5-5L5 21"/></svg>
          <p>暂无作品记录</p>
          <p style="font-size: 11px; color: #444;">生成线稿后将在这里展示</p>
        </div>
      </aside>

      <!-- ====== 中间：生成区 ====== -->
      <main class="canvas-area">
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

        <div v-else class="gen-display" ref="chatMessagesRef">
          <!-- 空状态 -->
          <div v-if="messages.length === 0" class="chat-empty">
            <div class="empty-icon-wrap">
              <svg width="64" height="64" viewBox="0 0 120 120" fill="none">
                <circle cx="60" cy="60" r="56" stroke="#a3e635" stroke-width="4" fill="none"/>
                <circle cx="44" cy="48" r="5" fill="#a3e635"/>
                <circle cx="76" cy="48" r="5" fill="#a3e635"/>
                <path d="M38 72 C38 72, 44 90, 60 90 C76 90, 82 72, 82 72" stroke="#a3e635" stroke-width="4" stroke-linecap="round" fill="none"/>
              </svg>
            </div>
            <p class="chat-empty-text">等您好久了</p>
            <p class="chat-empty-sub">在右侧工作区生成图片吧~</p>
          </div>

          <!-- 聊天气泡 -->
          <div v-for="msg in messages" :key="msg.id" :id="msg.taskId ? 'task-' + msg.taskId : undefined" class="chat-bubble" :class="msg.role">
            <!-- 用户消息 -->
            <div v-if="msg.role === 'user'" class="bubble-row user">
              <div class="bubble-body user">
                <p class="bubble-prompt">{{ msg.prompt || '线稿生成' }}</p>
                <div class="bubble-tags" v-if="msg.modeName">
                  <span class="tag">{{ msg.modeName }}</span>
                </div>
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
                  <p class="skeleton-text">AI 正在生成线稿中...</p>
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
                      <button class="result-action-btn" @click="downloadImage(msg.imageUrl, 'line-draft')">
                        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
                        下载
                      </button>
                      <button class="result-action-btn primary" @click="regenerateWithMode(msg)">
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

      <!-- ====== 右侧：工作区 ====== -->
      <aside class="control-panel">
        <div class="panel-top">
          <div class="panel-tabs">
            <button class="panel-tab active">线稿生成</button>
          </div>
        </div>

        <div class="panel-scroll">
          <!-- ① 上传图片 -->
          <div class="section-block">
            <div class="section-header">
              <span class="section-num">①</span>
              <span class="section-title">上传图片</span>
            </div>
            <ImageUploader
              v-model="uploadImages"
              :gallery-groups="allGalleryGroups"
              :max="1"
              :max-size-m-b="20"
            />
          </div>

          <!-- 示例图片 -->
          <div class="section-block" v-if="!uploadedImage">
            <div class="section-header">
              <span class="section-title">示例图片（点击可上传）</span>
              <span class="no-img-link" v-if="exampleImages.length === 0">无满意图片？<a href="#" @click.prevent>点这里&gt;</a></span>
            </div>
            <p style="color: #52525b; font-size: 12px; margin: 0 0 8px;">· 请上传背景简单的服装图片</p>
            <div class="example-grid" v-if="exampleImages.length > 0">
              <div v-for="(img, idx) in exampleImages" :key="idx" class="example-item" @click="uploadedImage = img">
                <img :src="img" alt="示例" />
              </div>
            </div>
            <button v-if="exampleImages.length > 4" class="view-all-btn" @click="showAllExamples = !showAllExamples">更多示例图</button>
          </div>

          <!-- ② 选择线稿类型（仅上传后显示） -->
          <div class="section-block" v-if="uploadedImage">
            <div class="section-header">
              <span class="section-num">②</span>
              <span class="section-title">选择生成线稿类型</span>
            </div>
            <div v-if="loadingModes" class="loading-state"><n-spin size="small" /></div>
            <div v-else-if="draftModes.length > 0" class="mode-cards-grid">
              <div
                v-for="mode in draftModes"
                :key="mode.id"
                class="mode-card"
                :class="{ selected: selectedModeId === mode.id }"
                @click="selectedModeId = mode.id"
              >
                <div class="mode-card-cover">
                  <img v-if="mode.cover_image" :src="mode.cover_image" :alt="mode.name" @error="handleImgError" />
                  <div v-else class="mode-card-placeholder">
                    <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="#555" stroke-width="1.5"><rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="8.5" cy="8.5" r="1.5"/><path d="m21 15-5-5L5 21"/></svg>
                  </div>
                  <div v-if="selectedModeId === mode.id" class="mode-check">
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="white" stroke-width="3"><polyline points="20 6 9 17 4 12"/></svg>
                  </div>
                </div>
                <div class="mode-card-name">{{ mode.name }}</div>
              </div>
            </div>
            <div v-else class="empty-state"><p>暂无可用线稿模式，请联系管理员配置</p></div>
          </div>
        </div>

        <!-- 按钮组 -->
        <div class="panel-bottom">
          <div class="button-group">
            <button class="btn-generate" :disabled="!canGenerate || generating" @click="handleGenerate">
              {{ generating ? '生成中...' : '生成图片' }}
            </button>
            <button class="btn-clear" :disabled="generating" @click="handleClear">清空</button>
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

    <!-- 图片放大 -->
    <div v-if="lightboxUrl" class="lightbox-overlay" @click.self="lightboxUrl = ''">
      <div class="lightbox-container">
        <img :src="lightboxUrl" class="lightbox-image" />
        <button class="lightbox-close" @click="lightboxUrl = ''">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
        </button>
        <div class="lightbox-actions">
          <button class="lightbox-action-btn" @click="downloadImage(lightboxUrl, 'line-draft')">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
            下载原图
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import { NModal, NSpin, useMessage } from 'naive-ui'
import ImageUploader from '../../components/ImageUploader.vue'
import { buildGalleryGroups } from '../../utils/galleryGroups'
import { taskApi, lineDraftApi } from '../../api/index'
import type { Task } from '../../api/index'
import { useUserStore } from '../../stores/user'

const router = useRouter()
const message = useMessage()
const userStore = useUserStore()

// ============ 画廊 ============
const historyTasks = ref<Task[]>([])
const activeTaskId = ref<string | null>(null)
let historyInitialized = false

const LINE_DRAFT_PREFIX = '[线稿] '

const galleryTasks = computed(() => {
  return historyTasks.value.filter(t => {
    if (t.status !== 'completed') return false
    if (!t.prompt || !t.prompt.startsWith(LINE_DRAFT_PREFIX)) return false
    try {
      const r = JSON.parse(t.result || '{}')
      return !!r.image_url
    } catch { return false }
  })
})

function getTaskImageUrl(task: Task): string {
  try {
    const r = JSON.parse(task.result || '{}')
    return r.image_url || ''
  } catch { return '' }
}

async function loadHistory() {
  try {
    const res: any = await taskApi.list({ page: 1, page_size: 100 })
    if (res.code === 0) {
      historyTasks.value = res.data.list || []
      historyInitialized = true
      // 刷新后自动从历史任务还原聊天气泡
      if (messages.value.length === 0) {
        rebuildMessagesFromHistory()
      }
    }
  } catch {}
}

function rebuildMessagesFromHistory() {
  const lineDraftTasks = historyTasks.value
    .filter(t => t.prompt && t.prompt.startsWith(LINE_DRAFT_PREFIX))
    .sort((a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime())
    .slice(-20) // 最多恢复最近 20 条

  for (const task of lineDraftTasks) {
    const imgUrl = getTaskImageUrl(task)
    const timeStr = task.created_at ? new Date(task.created_at).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' }) : ''
    const displayPrompt = task.prompt || '线稿生成'
    const modeName = displayPrompt.startsWith(LINE_DRAFT_PREFIX) ? displayPrompt.slice(LINE_DRAFT_PREFIX.length) : ''

    messages.value.push({
      id: 'hist_user_' + task.task_id,
      role: 'user',
      prompt: displayPrompt,
      modeName,
      time: timeStr,
    })

    if (task.status === 'completed' && imgUrl) {
      let elapsed: string | undefined
      if (task.completed_at && task.created_at) {
        const t = new Date(task.updated_at).getTime() - new Date(task.created_at).getTime()
        if (t > 0) elapsed = (t / 1000).toFixed(1)
      }
      messages.value.push({
        id: 'hist_ai_' + task.task_id,
        role: 'assistant',
        prompt: displayPrompt,
        status: 'completed',
        imageUrl: imgUrl,
        taskId: task.task_id,
        elapsed,
      })
    } else if (task.status === 'failed') {
      messages.value.push({
        id: 'hist_ai_' + task.task_id,
        role: 'assistant',
        prompt: displayPrompt,
        status: 'failed',
        errorMsg: task.error_msg || '生成失败',
        taskId: task.task_id,
      })
    }
  }
  if (messages.value.length > 0) {
    scrollToBottom()
  }
}

function viewHistoryTask(task: Task) {
  activeTaskId.value = task.task_id
  const imgUrl = getTaskImageUrl(task)
  if (imgUrl) {
    // 在消息中查找或添加
    const existing = messages.value.find(m => m.taskId === task.task_id)
    if (!existing) {
      messages.value.push({
        id: 'hist_user_' + task.task_id,
        role: 'user',
        prompt: task.prompt || '线稿生成',
        modeName: '',
      })
      messages.value.push({
        id: 'hist_ai_' + task.task_id,
        role: 'assistant',
        prompt: task.prompt || '',
        status: 'completed',
        imageUrl: imgUrl,
        taskId: task.task_id,
      })
      scrollToBottom()
    } else {
      // 滚动到该任务
      const el = document.getElementById('task-' + task.task_id)
      if (el) el.scrollIntoView({ behavior: 'smooth', block: 'center' })
    }
  }
}

const deleteConfirm = ref<{ show: boolean; x: number; y: number; taskId: string }>({ show: false, x: 0, y: 0, taskId: '' })

function showDeleteConfirm(e: MouseEvent, taskId?: string) {
  if (!taskId) return
  deleteConfirm.value = { show: true, x: e.clientX, y: e.clientY, taskId }
  const close = (ev: MouseEvent) => {
    const el = document.querySelector('.delete-popover')
    if (el && !el.contains(ev.target as Node)) {
      deleteConfirm.value.show = false
      document.removeEventListener('mousedown', close)
    }
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
      messages.value = messages.value.filter(m =>
        m.id !== 'hist_user_' + taskId && m.id !== 'hist_ai_' + taskId &&
        m.taskId !== taskId
      )
      if (activeTaskId.value === taskId) activeTaskId.value = null
    }
  } catch {}
}

async function deleteTask(taskId: string) {
  try {
    const res: any = await taskApi.delete(taskId)
    if (res.code === 0) {
      historyTasks.value = historyTasks.value.filter(t => t.task_id !== taskId)
      if (activeTaskId.value === taskId) activeTaskId.value = null
    }
  } catch {}
}

function regenerateWithMode(msg: ChatMessage) {
  if (!uploadedImage.value) {
    message.warning('请先上传图片')
    return
  }
  if (!selectedModeId.value) {
    message.warning('请选择线稿类型')
    return
  }
  handleGenerate()
}

// ============ 上传图片 ============
const uploadedImage = ref('')

// ImageUploader 使用 string[] 双向绑定
const uploadImages = computed({
  get: () => uploadedImage.value ? [uploadedImage.value] : [],
  set: (val: string[]) => {
    uploadedImage.value = val.length > 0 ? val[0] : ''
    if (!val.length) selectedModeId.value = null
  }
})

// 所有历史作品（分组展示供选择器用）
const allGalleryGroups = computed(() => buildGalleryGroups(historyTasks.value))

// ============ 线稿模式 ============
const draftModes = ref<any[]>([])
const loadingModes = ref(false)
const selectedModeId = ref<number | null>(null)

async function loadModes() {
  loadingModes.value = true
  try {
    const res: any = await lineDraftApi.listModes()
    if (res.code === 0) draftModes.value = res.data.list || []
  } finally {
    loadingModes.value = false
  }
}

// ============ 示例图片 (placeholder) ============
const exampleImages = ref<string[]>([])
const showAllExamples = ref(false)

// ============ 消息气泡 ============
interface ChatMessage {
  id: string
  role: 'user' | 'assistant'
  prompt?: string
  modeName?: string
  refImageUrl?: string
  status?: 'pending' | 'completed' | 'failed'
  imageUrl?: string
  errorMsg?: string
  taskId?: string
  elapsed?: string
  time?: string
}

const messages = ref<ChatMessage[]>([])
const chatMessagesRef = ref<HTMLElement | null>(null)

function scrollToBottom() {
  nextTick(() => {
    if (chatMessagesRef.value) {
      chatMessagesRef.value.scrollTop = chatMessagesRef.value.scrollHeight
    }
  })
}

// ============ 生成 ============
const generating = ref(false)
const currentTaskId = ref<string | null>(null)
const lightboxUrl = ref('')

const canGenerate = computed(() => {
  if (!userStore.isLoggedIn) return false
  if (generating.value) return false
  if (!uploadedImage.value) return false
  if (!selectedModeId.value) return false
  return true
})

const selectedModeName = computed(() => {
  const mode = draftModes.value.find(m => m.id === selectedModeId.value)
  return mode?.name || '线稿生成'
})

async function handleGenerate() {
  if (!userStore.isLoggedIn) {
    message.warning('请先登录')
    router.push('/user/login')
    return
  }
  if (!uploadedImage.value) {
    message.warning('请先上传图片')
    return
  }
  if (!selectedModeId.value) {
    message.warning('请选择线稿类型')
    return
  }

  activeTaskId.value = null

  const modeName = selectedModeName.value
  const displayPrompt = LINE_DRAFT_PREFIX + modeName
  const now = new Date()
  const timeStr = `${now.getHours().toString().padStart(2, '0')}:${now.getMinutes().toString().padStart(2, '0')}`
  const userMsg: ChatMessage = {
    id: 'msg_' + Date.now(),
    role: 'user',
    prompt: displayPrompt,
    modeName,
    refImageUrl: uploadedImage.value,
    time: timeStr,
  }
  messages.value.push(userMsg)

  const assistantMsg: ChatMessage = {
    id: 'loading_' + Date.now(), role: 'assistant', prompt: modeName, status: 'pending',
  }
  messages.value.push(assistantMsg)
  const assistantRef = messages.value[messages.value.length - 1]
  scrollToBottom()

  generating.value = true
  currentTaskId.value = null

  try {
    // 提交任务：prompt 使用模式的 prompt（后端通过 card_ids 无关），image_urls 使用上传的图片
    const res: any = await taskApi.submit({
      prompt: LINE_DRAFT_PREFIX + modeName,
      params: {
        image_urls: [uploadedImage.value],
        line_draft_mode_id: selectedModeId.value,
      },
    })
    if (res.code === 0) {
      currentTaskId.value = res.data.task_id
      assistantRef.taskId = res.data.task_id
      pollTaskStatus(res.data.task_id, assistantRef)
    } else {
      assistantRef.status = 'failed'
      assistantRef.errorMsg = res.msg || '提交失败'
      generating.value = false
    }
  } catch (e: any) {
    assistantRef.status = 'failed'
    assistantRef.errorMsg = e.msg || '网络错误，请重试'
    generating.value = false
  }
}

let pollTimer: number | null = null

async function pollTaskStatus(taskId: string, assistantMsg: ChatMessage) {
  if (pollTimer) clearTimeout(pollTimer)
  try {
    const res: any = await taskApi.get(taskId)
    if (res.code === 0) {
      const task = res.data.task
      if (task.status === 'completed') {
        try {
          const result = JSON.parse(task.result || '{}')
          assistantMsg.imageUrl = result.image_url || ''
          if (!assistantMsg.imageUrl) assistantMsg.errorMsg = '生成完成，但未获取到图片链接'
          if (task.completed_at) {
            const t = new Date(task.updated_at).getTime() - new Date(task.created_at).getTime()
            assistantMsg.elapsed = (t / 1000).toFixed(1)
          }
        } catch { assistantMsg.imageUrl = '' }
        assistantMsg.status = 'completed'
        generating.value = false
        scrollToBottom()
        loadHistory()
        return
      } else if (task.status === 'failed') {
        assistantMsg.status = 'failed'
        assistantMsg.errorMsg = task.error_msg || '生成失败'
        generating.value = false
        scrollToBottom()
        loadHistory()
        return
      }
      pollTimer = window.setTimeout(() => pollTaskStatus(taskId, assistantMsg), 2000)
    } else {
      pollTimer = window.setTimeout(() => pollTaskStatus(taskId, assistantMsg), 3000)
    }
  } catch {
    pollTimer = window.setTimeout(() => pollTaskStatus(taskId, assistantMsg), 3000)
  }
}

function handleClear() {
  uploadedImage.value = ''
  selectedModeId.value = null
  currentTaskId.value = null
  messages.value = []
  activeTaskId.value = null
  historyInitialized = false
  loadHistory()
}

function handleImgError(e: Event) {
  const img = e.target as HTMLImageElement
  img.style.background = '#27272a'
  img.src = 'data:image/svg+xml;base64,PHN2ZyB3aWR0aD0iMjQiIGhlaWdodD0iMjQiIHZpZXdCb3g9IjAgMCAyNCAyNCIgZmlsbD0ibm9uZSIgeG1sbnM9Imh0dHA6Ly93d3cudzMub3JnLzIwMDAvc3ZnIj48cmVjdCB3aWR0aD0iMjQiIGhlaWdodD0iMjQiIHJ4PSIyIiBmaWxsPSIjMzM2NjZmMSIgZmlsbC1vcGFjaXR5PSIwLjEiLz48cGF0aCBkPSJNMyA5aDE4bC0xOCA4eiIgZmlsbD0ibm9uZSIgc3Ryb2tlPSIjNTU1NTY2IiBzdHJva2Utd2lkdGg9IjIiLz48L3N2Zz4='
}

function downloadImage(url: string | undefined, prefix: string) {
  if (!url) return
  const proxyUrl = `/api/user/download?url=${encodeURIComponent(url)}`
  const a = document.createElement('a')
  a.href = proxyUrl
  a.download = `${prefix}_${Date.now()}.png`
  a.click()
}

// ============ 初始化 ============
onMounted(() => {
  if (userStore.isLoggedIn) {
    loadHistory()
    loadModes()
  }
})
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
.bubble-tags { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 6px; }
.tag { font-size: 11px; padding: 2px 8px; border-radius: 10px; background: rgba(163,230,53,0.1); color: #a3e635; }
.bubble-time { font-size: 10px; color: #444; padding: 0 4px; }
.bubble-ref-image { margin-top: 8px; max-width: 160px; border-radius: 8px; overflow: hidden; }
.bubble-ref-image img { width: 100%; display: block; }

/* 失败气泡 */
.bubble-failed-row { display: flex; align-items: center; gap: 12px; }
.bubble-failed-icon { width: 32px; height: 32px; border-radius: 50%; background: rgba(248,113,113,0.15); color: #f87171; display: flex; align-items: center; justify-content: center; font-size: 16px; flex-shrink: 0; }
.bubble-failed-info { flex: 1; }
.bubble-failed-title { font-size: 13px; color: #f87171; font-weight: 500; }
.bubble-failed-reason { font-size: 12px; color: #888; margin-top: 3px; word-break: break-word; }

/* 成功气泡 */
.bubble-image-wrap { border-radius: 10px; overflow: hidden; max-width: 400px; cursor: pointer; position: relative; }
.bubble-image-wrap:hover .image-zoom-hint { opacity: 1; }
.image-zoom-hint { position: absolute; top: 8px; right: 8px; width: 32px; height: 32px; border-radius: 8px; background: rgba(0,0,0,0.55); display: flex; align-items: center; justify-content: center; color: #fff; opacity: 0; transition: opacity 0.2s; pointer-events: none; }
.bubble-image { width: 100%; display: block; transition: transform 0.2s; }
.bubble-image-wrap:hover .bubble-image { transform: scale(1.02); }
.bubble-footer { display: flex; align-items: center; justify-content: space-between; margin-top: 10px; padding-top: 8px; border-top: 1px solid #27272a; }
.bubble-meta { font-size: 11px; color: #555; }

/* 结果操作按钮 */
.result-actions { display: flex; align-items: center; gap: 8px; }
.result-action-btn { display: inline-flex; align-items: center; gap: 6px; padding: 5px 10px; border-radius: 6px; border: 1px solid #333; background: #27272a; color: #aaa; font-size: 12px; cursor: pointer; transition: all 0.2s; }
.result-action-btn:hover { border-color: #444; color: #ddd; }
.result-action-btn.primary { background: rgba(163,230,53,0.1); border-color: rgba(163,230,53,0.3); color: #a3e635; }
.result-action-btn.primary:hover { background: rgba(163,230,53,0.18); border-color: #a3e635; }
.result-action-btn.danger { color: #888; }
.result-action-btn.danger:hover { background: rgba(220,38,38,0.12); border-color: rgba(220,38,38,0.4); color: #ef4444; }

/* 删除确认浮层 */
.delete-popover { position: fixed; z-index: 1000; background: #27272a; border: 1px solid #3f3f46; border-radius: 10px; padding: 12px 16px; box-shadow: 0 8px 24px rgba(0,0,0,0.4); transform: translate(-50%, -100%); margin-top: -8px; }
.delete-popover p { margin: 0 0 8px; font-size: 13px; color: #e4e4e7; }
.delete-popover-btns { display: flex; gap: 8px; justify-content: flex-end; }
.delete-popover-btns button { padding: 4px 14px; border-radius: 6px; border: 1px solid #3f3f46; background: #18181b; color: #a1a1aa; font-size: 12px; cursor: pointer; }
.delete-popover-btns button.confirm { background: rgba(220,38,38,0.2); border-color: rgba(220,38,38,0.4); color: #ef4444; }

/* 3:4 骨架占位 */
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
.panel-scroll { flex: 1; overflow-y: auto; padding: 16px; display: flex; flex-direction: column; gap: 18px; }
.section-block { display: flex; flex-direction: column; gap: 8px; }
.section-header { display: flex; align-items: center; gap: 6px; }
.section-num { font-size: 11px; color: #a3e635; }
.section-title { font-size: 13px; font-weight: 600; color: #ccc; }
.no-img-link { font-size: 12px; color: #a3e635; margin-left: auto; }
.no-img-link a { color: #a3e635; text-decoration: none; }


/* 示例图片 */
.example-grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 8px; }
.example-item { border-radius: 8px; overflow: hidden; cursor: pointer; background: #27272a; aspect-ratio: 3/4; }
.example-item img { width: 100%; height: 100%; object-fit: cover; display: block; }

/* 线稿模式卡片 */
.mode-cards-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: 10px; }
.mode-card { border-radius: 10px; overflow: hidden; cursor: pointer; border: 2px solid #27272a; transition: border-color 0.2s; background: #1a1a1d; }
.mode-card.selected { border-color: #a3e635; }
.mode-card-cover { position: relative; aspect-ratio: 1; overflow: hidden; background: #27272a; }
.mode-card-cover img { width: 100%; height: 100%; object-fit: cover; display: block; }
.mode-card-placeholder { width: 100%; height: 100%; display: flex; align-items: center; justify-content: center; }
.mode-check { position: absolute; top: 6px; right: 6px; width: 22px; height: 22px; border-radius: 50%; background: #a3e635; display: flex; align-items: center; justify-content: center; }
.mode-card-name { padding: 6px 8px; font-size: 12px; color: #ccc; text-align: center; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }

/* 按钮组 */
.panel-bottom { padding: 12px 16px; border-top: 1px solid #222225; flex-shrink: 0; }
.button-group { display: flex; gap: 10px; }
.btn-generate { flex: 1; padding: 12px 0; border-radius: 10px; border: none; background: #a3e635; color: #111; font-size: 15px; font-weight: 700; cursor: pointer; transition: background 0.2s; }
.btn-generate:hover:not(:disabled) { background: #bef264; }
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


.loading-state { display: flex; justify-content: center; padding: 20px; }
.empty-state { text-align: center; color: #555; font-size: 12px; padding: 20px; }
.view-all-btn { background: none; border: none; color: #71717a; font-size: 12px; cursor: pointer; padding: 4px 0; text-align: center; }
.view-all-btn:hover { color: #a3e635; }
</style>
