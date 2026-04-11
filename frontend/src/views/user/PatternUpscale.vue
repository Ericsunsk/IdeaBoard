<template>
  <div class="home-layout">
    <div class="home-body">
      <aside class="history-panel">
        <div class="history-header">
          <span>我的画廊</span>
          <span v-if="galleryTasks.length" class="gallery-count">{{ galleryTasks.length }}</span>
        </div>
        <div v-if="galleryTasks.length > 0" class="gallery-grid">
          <div
            v-for="task in galleryTasks"
            :key="task.task_id"
            class="gallery-item"
            :class="{ active: activeTaskId === task.task_id }"
            @click="viewHistoryTask(task)"
          >
            <img :src="getTaskImageUrl(task)" alt="" loading="lazy" @error="handleImgError" />
            <div class="gallery-item-overlay">
              <span class="gallery-item-prompt">{{ stripPrefix(task.prompt) }}</span>
            </div>
            <button class="gallery-delete-btn" title="删除" @click.stop="showDeleteConfirm($event, task.task_id)">
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                <line x1="18" y1="6" x2="6" y2="18" />
                <line x1="6" y1="6" x2="18" y2="18" />
              </svg>
            </button>
          </div>
        </div>
        <div v-else class="history-empty">
          <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="#333" stroke-width="1.5">
            <rect x="3" y="3" width="18" height="18" rx="2" />
            <circle cx="8.5" cy="8.5" r="1.5" />
            <path d="m21 15-5-5L5 21" />
          </svg>
          <p>还没有作品</p>
          <p class="history-empty-tip">超分完成后会自动存入这里</p>
        </div>
      </aside>

      <main class="canvas-area">
        <div v-if="!userStore.isLoggedIn" class="guest-prompt">
          <p class="guest-title">温馨提示</p>
          <p class="guest-desc">您当前是游客身份，暂无工具相关权限。</p>
          <p class="guest-action">
            请先
            <router-link to="/user/register" class="link-highlight">注册</router-link>
            或
            <router-link to="/user/login" class="link-highlight">登录</router-link>
          </p>
        </div>

        <div v-else class="gen-display" ref="chatMessagesRef">
          <div v-if="messages.length === 0" class="chat-empty">
            <div class="empty-icon-wrap">
              <svg width="64" height="64" viewBox="0 0 24 24" fill="none" stroke="#a3e635" stroke-width="1.2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M5 12h5" />
                <path d="M14 6h5" />
                <path d="M14 18h5" />
                <path d="M10 7l4-2" />
                <path d="M10 17l4 2" />
                <rect x="3" y="6" width="8" height="12" rx="2" />
              </svg>
            </div>
            <p class="chat-empty-text">{{ featureTitle }}</p>
            <p class="chat-empty-sub">{{ featureSubtitle }}</p>
          </div>

          <div
            v-for="msg in messages"
            :key="msg.id"
            :id="msg.taskId ? 'task-' + msg.taskId : undefined"
            class="chat-bubble"
            :class="msg.role"
          >
            <div v-if="msg.role === 'user'" class="bubble-row user">
              <div class="bubble-body user">
                <p class="bubble-prompt">{{ msg.prompt }}</p>
                <div v-if="msg.refImageUrl" class="bubble-ref-image">
                  <img :src="msg.refImageUrl" alt="原图" />
                </div>
              </div>
              <span v-if="msg.time" class="bubble-time">{{ msg.time }}</span>
            </div>

            <div v-else class="bubble-row ai">
              <div v-if="msg.status === 'pending'" class="skeleton-wrap">
                <div class="skeleton-shimmer"></div>
                <div class="skeleton-overlay">
                  <div class="skeleton-spinner"></div>
                  <p class="skeleton-text">正在提升图片清晰度...</p>
                  <p class="skeleton-sub">通常需要 10-30 秒</p>
                </div>
              </div>

              <div v-else-if="msg.status === 'failed'" class="bubble-body failed">
                <div class="bubble-failed-row">
                  <div class="bubble-failed-icon">✕</div>
                  <div>
                    <div class="bubble-failed-title">处理失败</div>
                    <div class="bubble-failed-reason">{{ msg.errorMsg }}</div>
                  </div>
                </div>
              </div>

              <template v-else-if="msg.status === 'completed'">
                <div class="bubble-body ai-success">
                  <div v-if="msg.imageUrl" class="bubble-image-wrap" @click="lightboxUrl = msg.imageUrl">
                    <img :src="msg.imageUrl" alt="超分结果" class="bubble-image" @error="handleImgError" />
                    <div class="image-zoom-hint">
                      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <circle cx="11" cy="11" r="8" />
                        <line x1="21" y1="21" x2="16.65" y2="16.65" />
                      </svg>
                    </div>
                  </div>
                  <div class="bubble-footer">
                    <span v-if="msg.elapsed" class="bubble-meta">耗时 {{ msg.elapsed }}s</span>
                    <div class="result-actions">
                      <button class="result-action-btn" @click="downloadImage(msg.imageUrl)">下载</button>
                      <button class="result-action-btn primary" @click="handleGenerate">再次处理</button>
                      <button class="result-action-btn danger" @click="showDeleteConfirm($event, msg.taskId)">删除</button>
                    </div>
                  </div>
                </div>
              </template>
            </div>
          </div>
        </div>
      </main>

      <aside class="control-panel">
        <div class="panel-top">
          <div class="panel-tabs">
            <button class="panel-tab active">{{ featureTitle }}</button>
          </div>
        </div>

        <div class="panel-scroll">
          <div class="section-block">
            <div class="section-header">
              <span class="section-num">①</span>
              <span class="section-title">上传图片</span>
            </div>
            <ImageUploader v-model="uploadImages" :gallery-groups="allGalleryGroups" :max="1" :max-size-m-b="20" />
            <p class="section-tip">建议上传花型、面料纹理、印花稿等细节明确的图像</p>
          </div>

          <div class="section-block" v-if="uploadedImage">
            <div class="section-header">
              <span class="section-num">②</span>
              <span class="section-title">超分模式</span>
            </div>
            <div class="mode-grid">
              <button
                v-for="item in modeOptions"
                :key="item.value"
                class="mode-card"
                :class="{ active: selectedMode === item.value }"
                @click="selectedMode = item.value"
              >
                <span class="mode-card-title">{{ item.label }}</span>
                <span class="mode-card-desc">{{ item.desc }}</span>
              </button>
            </div>
          </div>

          <div class="section-block" v-if="uploadedImage">
            <div class="section-header">
              <span class="section-num">③</span>
              <span class="section-title">放大倍率</span>
            </div>
            <div class="factor-grid">
              <button
                v-for="factor in factorOptions"
                :key="factor"
                class="factor-btn"
                :class="{ active: upscaleFactor === factor }"
                @click="upscaleFactor = factor"
              >
                {{ factor }}x
              </button>
            </div>
          </div>

          <div class="section-block" v-if="uploadedImage">
            <div class="section-header">
              <span class="section-num">④</span>
              <span class="section-title">输出格式</span>
            </div>
            <div class="format-grid">
              <button
                v-for="item in formatOptions"
                :key="item.value"
                class="format-btn"
                :class="{ active: outputFormat === item.value }"
                @click="outputFormat = item.value"
              >
                <span class="format-label">{{ item.label }}</span>
                <span class="format-desc">{{ item.desc }}</span>
              </button>
            </div>
          </div>

          <div class="section-block" v-if="uploadedImage">
            <div class="section-header">
              <span class="section-num">⑤</span>
              <span class="section-title">输出质量</span>
            </div>
            <div class="quality-row">
              <span class="quality-label">JPG 质量</span>
              <span class="quality-value">{{ outputQuality }}</span>
            </div>
            <input
              v-model.number="outputQuality"
              class="quality-slider"
              type="range"
              min="30"
              max="100"
              step="1"
              :disabled="outputFormat === 'png'"
            />
            <div class="quality-range">
              <span>30</span>
              <span>100</span>
            </div>
            <p class="section-tip" :class="{ muted: outputFormat === 'png' }">
              {{ outputFormat === 'png' ? 'PNG 输出时该参数不会生效' : '数值越高压缩越少，文件体积也更大' }}
            </p>
          </div>

          <div class="section-block" v-if="uploadedImage">
            <div class="summary-card">
              <div class="summary-label">当前配置</div>
              <div class="summary-value">{{ currentConfigSummary }}</div>
            </div>
          </div>
        </div>

        <div class="panel-bottom">
          <div class="button-group">
            <button class="btn-generate" :disabled="!canGenerate || generating" @click="handleGenerate">
              {{ generating ? '处理中...' : '开始超分' }}
            </button>
            <button class="btn-reset" :disabled="generating" @click="handleClear">清空</button>
          </div>
        </div>
      </aside>
    </div>

    <div
      v-if="deleteConfirm.show"
      class="delete-popover"
      :style="{ left: deleteConfirm.x + 'px', top: deleteConfirm.y + 'px' }"
    >
      <p class="delete-text">确认删除？</p>
      <div class="delete-actions">
        <button class="dp-btn cancel" @click="deleteConfirm.show = false">取消</button>
        <button class="dp-btn danger" @click="confirmDelete">删除</button>
      </div>
    </div>

    <div v-if="lightboxUrl" class="lightbox-overlay" @click.self="lightboxUrl = ''">
      <img :src="lightboxUrl" alt="超分结果预览" class="lightbox-img" />
      <button class="lightbox-close" @click="lightboxUrl = ''">&times;</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useMessage } from 'naive-ui'
import { upscaleApi, taskApi } from '../../api'
import ImageUploader from '../../components/ImageUploader.vue'
import { useUserStore } from '../../stores/user'
import { buildGalleryGroups } from '../../utils/galleryGroups'
import { downloadImageByProxy, getTaskImageUrl as extractTaskImageUrl, stripFeaturePrefix } from '../../utils/task-history'
import type { Task } from '../../api'

const route = useRoute()
const router = useRouter()
const message = useMessage()
const userStore = useUserStore()

const featureSlug = computed(() => {
  const raw = route.meta.slug
  return typeof raw === 'string' && raw ? raw : 'pattern-upscale'
})
const featureTitle = computed(() => {
  const raw = route.meta.title
  return typeof raw === 'string' && raw ? raw : '图案变清晰'
})
const featurePrefix = computed(() => `[${featureSlug.value}] `)
const featureSubtitle = computed(() =>
  featureSlug.value === 'hd-upscale'
    ? '上传图片，选择模式与倍率，直接调用阿里云超分接口放大'
    : '上传图案图，选择模式和倍率，直接调用阿里云超分接口处理'
)

const historyTasks = ref<Task[]>([])
const activeTaskId = ref<string | null>(null)
const generating = ref(false)
const lightboxUrl = ref('')

const uploadedImage = ref('')
const selectedMode = ref<'base' | 'enhancement'>('base')
const upscaleFactor = ref<2 | 3 | 4>(2)
const outputFormat = ref<'jpg' | 'png'>('jpg')
const outputQuality = ref(95)

const modeOptions = [
  { value: 'base' as const, label: '普通模式', desc: '处理更稳，适合大多数图案与印花稿' },
  { value: 'enhancement' as const, label: '增强模式', desc: '更强调细节锐度，适合纹理与边缘较复杂的图案' },
]
const factorOptions = [2, 3, 4] as const
const formatOptions = [
  { value: 'jpg' as const, label: 'JPG', desc: '体积更小，适合常规交付' },
  { value: 'png' as const, label: 'PNG', desc: '无损输出，适合保留边缘细节' },
]

const uploadImages = computed({
  get: () => (uploadedImage.value ? [uploadedImage.value] : []),
  set: (value: string[]) => {
    uploadedImage.value = value.length > 0 ? value[0] : ''
  }
})

const allGalleryGroups = computed(() => buildGalleryGroups(historyTasks.value))

const galleryTasks = computed(() =>
  historyTasks.value.filter(task => {
    if (task.status !== 'completed') return false
    if (!task.prompt || !task.prompt.startsWith(featurePrefix.value)) return false
    return !!getTaskImageUrl(task)
  })
)

const canGenerate = computed(() => userStore.isLoggedIn && !generating.value && !!uploadedImage.value)

const currentConfigSummary = computed(() => {
  const modeLabel = modeOptions.find(item => item.value === selectedMode.value)?.label || '普通模式'
  return `${modeLabel} · ${upscaleFactor.value}x · ${outputFormat.value.toUpperCase()}${outputFormat.value === 'jpg' ? ` · 质量 ${outputQuality.value}` : ''}`
})

interface ChatMessage {
  id: string
  role: 'user' | 'assistant'
  prompt: string
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
const deleteConfirm = ref<{ show: boolean; x: number; y: number; taskId: string }>({
  show: false,
  x: 0,
  y: 0,
  taskId: '',
})

function stripPrefix(str?: string) {
  return stripFeaturePrefix(str)
}

function getTaskImageUrl(task: Task): string {
  return extractTaskImageUrl(task)
}

function scrollToBottom() {
  nextTick(() => {
    if (chatMessagesRef.value) {
      chatMessagesRef.value.scrollTop = chatMessagesRef.value.scrollHeight
    }
  })
}

async function loadHistory() {
  if (!userStore.isLoggedIn) return
  try {
    const res: any = await taskApi.list({ page: 1, page_size: 200 })
    if (res.code === 0) {
      historyTasks.value = res.data.list || []
      if (messages.value.length === 0) {
        rebuildMessagesFromHistory()
      }
    }
  } catch {}
}

function rebuildMessagesFromHistory() {
  const tasks = historyTasks.value
    .filter(task => task.prompt && task.prompt.startsWith(featurePrefix.value))
    .sort((a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime())
    .slice(-20)

  for (const task of tasks) {
    const imageUrl = getTaskImageUrl(task)
    const time = task.created_at
      ? new Date(task.created_at).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
      : ''

    messages.value.push({
      id: 'hist_user_' + task.task_id,
      role: 'user',
      prompt: stripPrefix(task.prompt) || featureTitle.value,
      time,
    })

    if (task.status === 'completed' && imageUrl) {
      let elapsed: string | undefined
      if (task.completed_at && task.created_at) {
        const duration = new Date(task.updated_at).getTime() - new Date(task.created_at).getTime()
        if (duration > 0) elapsed = (duration / 1000).toFixed(1)
      }
      messages.value.push({
        id: 'hist_ai_' + task.task_id,
        role: 'assistant',
        prompt: task.prompt || '',
        status: 'completed',
        imageUrl,
        taskId: task.task_id,
        elapsed,
      })
    } else if (task.status === 'failed') {
      messages.value.push({
        id: 'hist_ai_' + task.task_id,
        role: 'assistant',
        prompt: task.prompt || '',
        status: 'failed',
        errorMsg: task.error_msg || '处理失败',
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
  const existing = messages.value.find(msg => msg.taskId === task.task_id)
  if (existing) {
    const el = document.getElementById('task-' + task.task_id)
    if (el) {
      el.scrollIntoView({ behavior: 'smooth', block: 'center' })
      el.classList.add('highlight')
      setTimeout(() => el.classList.remove('highlight'), 2000)
    }
    return
  }

  const imageUrl = getTaskImageUrl(task)
  messages.value.push({
    id: 'hist_user_' + task.task_id,
    role: 'user',
    prompt: stripPrefix(task.prompt) || featureTitle.value,
  })
  messages.value.push({
    id: 'hist_ai_' + task.task_id,
    role: 'assistant',
    prompt: task.prompt || '',
    status: imageUrl ? 'completed' : 'failed',
    imageUrl,
    errorMsg: imageUrl ? undefined : task.error_msg || '处理失败',
    taskId: task.task_id,
  })
  scrollToBottom()
}

function showDeleteConfirm(event: MouseEvent, taskId?: string) {
  if (!taskId) return
  deleteConfirm.value = { show: true, x: event.clientX, y: event.clientY, taskId }
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
      historyTasks.value = historyTasks.value.filter(task => task.task_id !== taskId)
      messages.value = messages.value.filter(
        msg => msg.id !== 'hist_user_' + taskId && msg.id !== 'hist_ai_' + taskId && msg.taskId !== taskId
      )
      if (activeTaskId.value === taskId) {
        activeTaskId.value = null
      }
    }
  } catch {}
}

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

  activeTaskId.value = null
  generating.value = true
  const startTime = Date.now()
  const prompt = currentConfigSummary.value
  const time = new Date().toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })

  messages.value.push({
    id: 'msg_' + Date.now(),
    role: 'user',
    prompt,
    refImageUrl: uploadedImage.value,
    time,
  })

  messages.value.push({
    id: 'loading_' + Date.now(),
    role: 'assistant',
    prompt,
    status: 'pending',
  })

  const assistantMsg = messages.value[messages.value.length - 1]
  scrollToBottom()

  try {
    const res: any = await upscaleApi.upscale({
      image_url: uploadedImage.value,
      mode: selectedMode.value,
      upscale_factor: upscaleFactor.value,
      output_format: outputFormat.value,
      output_quality: outputQuality.value,
      feature: featureSlug.value,
    })

    if (res.code === 0) {
      assistantMsg.status = 'completed'
      assistantMsg.imageUrl = res.data.image_url
      assistantMsg.taskId = res.data.task_id
      assistantMsg.elapsed = ((Date.now() - startTime) / 1000).toFixed(1)
      await loadHistory()
    } else {
      assistantMsg.status = 'failed'
      assistantMsg.errorMsg = res.msg || '处理失败'
    }
  } catch (error: any) {
    assistantMsg.status = 'failed'
    assistantMsg.errorMsg = error.msg || error.message || '网络错误，请重试'
  } finally {
    generating.value = false
    scrollToBottom()
  }
}

function handleClear() {
  uploadedImage.value = ''
  messages.value = []
  activeTaskId.value = null
}

function handleImgError(event: Event) {
  ;(event.target as HTMLImageElement).style.background = '#27272a'
}

function downloadImage(url?: string) {
  downloadImageByProxy(url)
}

onMounted(() => {
  loadHistory()
})
</script>

<style scoped>
.home-layout { width: 100%; height: 100%; display: flex; flex-direction: column; background: #111113; overflow: hidden; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif; }
.home-body { flex: 1; display: grid; grid-template-columns: 220px 1fr 380px; min-height: 0; overflow: hidden; }

.history-panel { border-right: 1px solid #222225; background: #111113; display: flex; flex-direction: column; overflow: hidden; min-height: 0; }
.history-header { padding: 14px 16px 10px; font-size: 13px; font-weight: 600; color: #ccc; border-bottom: 1px solid #222225; display: flex; align-items: center; justify-content: space-between; flex-shrink: 0; }
.gallery-count { font-size: 11px; color: #a3e635; background: rgba(163,230,53,0.1); padding: 1px 8px; border-radius: 10px; font-weight: 500; }
.gallery-grid { flex: 1; overflow-y: auto; padding: 8px; column-count: 2; column-gap: 6px; }
.gallery-item { break-inside: avoid; margin-bottom: 6px; border-radius: 8px; overflow: hidden; position: relative; cursor: pointer; background: #1a1a1d; border: 2px solid transparent; transition: border-color 0.2s; }
.gallery-item.active { border-color: #a3e635; }
.gallery-item img { display: block; width: 100%; }
.gallery-item-overlay { position: absolute; bottom: 0; left: 0; right: 0; padding: 4px 6px; background: linear-gradient(transparent, rgba(0,0,0,0.7)); }
.gallery-item-prompt { display: block; font-size: 10px; color: #aaa; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.gallery-delete-btn { position: absolute; top: 4px; right: 4px; width: 20px; height: 20px; border-radius: 50%; background: rgba(0,0,0,0.6); border: none; color: #999; cursor: pointer; display: flex; align-items: center; justify-content: center; opacity: 0; transition: opacity 0.15s; }
.gallery-item:hover .gallery-delete-btn { opacity: 1; }
.history-empty { flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; color: #555; font-size: 12px; gap: 6px; text-align: center; padding: 0 24px; }
.history-empty-tip { color: #3f3f46; font-size: 11px; }

.canvas-area { display: flex; flex-direction: column; overflow: hidden; background: #111113; }
.guest-prompt { flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 8px; color: #888; text-align: center; }
.guest-title { font-size: 15px; font-weight: 600; color: #ccc; }
.guest-desc, .guest-action { font-size: 13px; color: #888; }
.link-highlight { color: #a3e635; text-decoration: none; font-weight: 600; }
.gen-display { flex: 1; overflow-y: auto; padding: 24px 32px; display: flex; flex-direction: column; gap: 16px; }
.chat-empty { flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 8px; text-align: center; }
.empty-icon-wrap { width: 84px; height: 84px; display: flex; align-items: center; justify-content: center; border-radius: 50%; background: rgba(163,230,53,0.05); }
.chat-empty-text { font-size: 20px; font-weight: 700; color: #e4e4e7; }
.chat-empty-sub { max-width: 420px; font-size: 13px; color: #52525b; }

.chat-bubble { display: flex; flex-direction: column; }
.bubble-row { display: flex; flex-direction: column; gap: 4px; }
.bubble-row.user { align-items: flex-end; }
.bubble-row.ai { align-items: flex-start; }
.bubble-body { max-width: 85%; padding: 14px 18px; border-radius: 14px; line-height: 1.6; font-size: 13px; }
.bubble-body.user { background: #1e293b; color: #e2e8f0; border-bottom-right-radius: 4px; }
.bubble-prompt { margin: 0; word-break: break-word; }
.bubble-ref-image { margin-top: 8px; }
.bubble-ref-image img { width: 76px; height: 76px; border-radius: 8px; object-fit: cover; display: block; }
.bubble-time { font-size: 10px; color: #52525b; margin-top: 2px; }

.bubble-body.ai-success { background: #18181b; border: 1px solid #27272a; border-bottom-left-radius: 4px; padding: 0; overflow: hidden; }
.bubble-image-wrap { position: relative; cursor: zoom-in; }
.bubble-image { max-width: 420px; max-height: 520px; display: block; }
.image-zoom-hint { position: absolute; bottom: 8px; right: 8px; width: 32px; height: 32px; display: flex; align-items: center; justify-content: center; background: rgba(0,0,0,0.5); border-radius: 50%; color: #fff; opacity: 0; transition: opacity 0.2s; }
.bubble-image-wrap:hover .image-zoom-hint { opacity: 1; }
.bubble-footer { display: flex; align-items: center; justify-content: space-between; padding: 10px 16px; border-top: 1px solid #27272a; gap: 12px; }
.bubble-meta { font-size: 11px; color: #52525b; }
.result-actions { display: flex; gap: 6px; }
.result-action-btn { padding: 5px 10px; border: 1px solid #27272a; background: #111113; color: #a1a1aa; font-size: 11px; border-radius: 6px; cursor: pointer; transition: all 0.15s; }
.result-action-btn:hover { background: #1e1e22; color: #e4e4e7; }
.result-action-btn.primary { border-color: rgba(163,230,53,0.3); color: #a3e635; }
.result-action-btn.primary:hover { background: rgba(163,230,53,0.1); }
.result-action-btn.danger { border-color: rgba(239,68,68,0.2); color: #ef4444; }
.result-action-btn.danger:hover { background: rgba(239,68,68,0.08); }

.bubble-body.failed { background: rgba(239,68,68,0.06); border: 1px solid rgba(239,68,68,0.2); border-bottom-left-radius: 4px; }
.bubble-failed-row { display: flex; align-items: flex-start; gap: 12px; }
.bubble-failed-icon { width: 28px; height: 28px; display: flex; align-items: center; justify-content: center; background: rgba(239,68,68,0.15); border-radius: 50%; color: #ef4444; font-size: 14px; font-weight: 700; flex-shrink: 0; }
.bubble-failed-title { font-weight: 600; color: #fca5a5; margin-bottom: 2px; }
.bubble-failed-reason { color: #a1a1aa; font-size: 12px; }

.skeleton-wrap { width: 340px; height: 220px; background: #18181b; border-radius: 14px; border: 1px solid #27272a; position: relative; overflow: hidden; }
.skeleton-shimmer { position: absolute; inset: 0; background: linear-gradient(90deg, transparent 0%, rgba(255,255,255,0.04) 50%, transparent 100%); animation: shimmer 1.5s infinite; }
.skeleton-overlay { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 8px; }
.skeleton-spinner { width: 28px; height: 28px; border: 3px solid #27272a; border-top-color: #a3e635; border-radius: 50%; animation: spin 0.8s linear infinite; }
.skeleton-text { color: #a1a1aa; font-size: 13px; font-weight: 500; margin: 0; }
.skeleton-sub { color: #52525b; font-size: 11px; margin: 0; }

.control-panel { border-left: 1px solid #222225; background: #111113; display: flex; flex-direction: column; overflow: hidden; }
.panel-top { padding: 16px 16px 0; }
.panel-tabs { display: flex; gap: 4px; }
.panel-tab { padding: 8px 16px; background: #18181b; border: none; color: #e4e4e7; font-size: 13px; font-weight: 500; border-radius: 8px 8px 0 0; }
.panel-scroll { flex: 1; overflow-y: auto; padding: 16px; display: flex; flex-direction: column; gap: 16px; }
.panel-bottom { padding: 16px; border-top: 1px solid #222225; }

.section-block { display: flex; flex-direction: column; gap: 8px; }
.section-header { display: flex; align-items: center; gap: 6px; }
.section-num { display: flex; align-items: center; justify-content: center; width: 22px; height: 22px; border-radius: 50%; background: rgba(163,230,53,0.12); color: #a3e635; font-size: 11px; font-weight: 700; }
.section-title { font-size: 13px; font-weight: 600; color: #d4d4d8; }
.section-tip { margin: 0; font-size: 11px; color: #52525b; }
.section-tip.muted { color: #3f3f46; }

.mode-grid { display: grid; gap: 8px; }
.mode-card { display: flex; flex-direction: column; gap: 4px; padding: 12px; border: 1px solid #27272a; border-radius: 10px; background: #18181b; color: #a1a1aa; cursor: pointer; text-align: left; transition: all 0.15s; }
.mode-card:hover { border-color: #3f3f46; }
.mode-card.active { border-color: #a3e635; background: rgba(163,230,53,0.06); }
.mode-card-title { font-size: 13px; font-weight: 600; color: #e4e4e7; }
.mode-card.active .mode-card-title { color: #a3e635; }
.mode-card-desc { font-size: 11px; line-height: 1.5; color: #71717a; }

.factor-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: 8px; }
.factor-btn { padding: 10px 0; border: 1px solid #27272a; background: #18181b; color: #a1a1aa; border-radius: 8px; cursor: pointer; font-size: 13px; font-weight: 600; transition: all 0.15s; }
.factor-btn:hover { border-color: #3f3f46; color: #e4e4e7; }
.factor-btn.active { border-color: #a3e635; background: rgba(163,230,53,0.06); color: #a3e635; }

.format-grid { display: grid; grid-template-columns: repeat(2, 1fr); gap: 8px; }
.format-btn { display: flex; flex-direction: column; gap: 4px; padding: 12px; border: 1px solid #27272a; background: #18181b; border-radius: 10px; text-align: left; cursor: pointer; transition: all 0.15s; }
.format-btn:hover { border-color: #3f3f46; }
.format-btn.active { border-color: #a3e635; background: rgba(163,230,53,0.06); }
.format-label { font-size: 13px; font-weight: 600; color: #e4e4e7; }
.format-btn.active .format-label { color: #a3e635; }
.format-desc { font-size: 11px; line-height: 1.5; color: #71717a; }

.quality-row { display: flex; align-items: center; justify-content: space-between; }
.quality-label { font-size: 12px; color: #a1a1aa; }
.quality-value { font-size: 12px; font-weight: 600; color: #e4e4e7; }
.quality-slider { width: 100%; accent-color: #a3e635; }
.quality-slider:disabled { opacity: 0.45; cursor: not-allowed; }
.quality-range { display: flex; justify-content: space-between; font-size: 10px; color: #52525b; }

.summary-card { padding: 12px; border-radius: 10px; background: linear-gradient(180deg, rgba(163,230,53,0.08), rgba(163,230,53,0.02)); border: 1px solid rgba(163,230,53,0.12); }
.summary-label { font-size: 11px; color: #a3e635; margin-bottom: 6px; }
.summary-value { font-size: 13px; color: #e4e4e7; line-height: 1.6; }

.button-group { display: flex; gap: 8px; }
.btn-generate { flex: 1; padding: 12px; background: linear-gradient(135deg, #a3e635, #65a30d); color: #111; font-weight: 700; font-size: 14px; border: none; border-radius: 10px; cursor: pointer; transition: all 0.2s; }
.btn-generate:hover:not(:disabled) { filter: brightness(1.1); transform: translateY(-1px); }
.btn-generate:disabled { opacity: 0.4; cursor: not-allowed; }
.btn-reset { padding: 12px 18px; background: #18181b; border: 1px solid #27272a; color: #a1a1aa; font-size: 13px; border-radius: 10px; cursor: pointer; transition: all 0.2s; }
.btn-reset:hover:not(:disabled) { background: #27272a; color: #e4e4e7; }
.btn-reset:disabled { opacity: 0.5; cursor: not-allowed; }

.delete-popover { position: fixed; z-index: 10000; background: #1e1e22; border: 1px solid #333; border-radius: 10px; padding: 14px 16px; box-shadow: 0 8px 30px rgba(0,0,0,0.4); }
.delete-text { margin: 0 0 10px; color: #e4e4e7; }
.delete-actions { display: flex; justify-content: flex-end; gap: 8px; }
.dp-btn { padding: 5px 14px; border-radius: 6px; font-size: 12px; cursor: pointer; border: 1px solid #333; }
.dp-btn.cancel { background: #27272a; color: #a1a1aa; }
.dp-btn.danger { background: #ef4444; color: #fff; border-color: #ef4444; }

.lightbox-overlay { position: fixed; inset: 0; z-index: 9999; background: rgba(0,0,0,0.85); display: flex; align-items: center; justify-content: center; cursor: zoom-out; }
.lightbox-img { max-width: 90vw; max-height: 90vh; object-fit: contain; border-radius: 8px; }
.lightbox-close { position: absolute; top: 16px; right: 24px; background: none; border: none; color: #fff; font-size: 36px; cursor: pointer; }

.highlight { animation: highlightPulse 2s ease-out; }
@keyframes highlightPulse { 0%, 100% { box-shadow: none; } 30% { box-shadow: 0 0 0 2px rgba(163,230,53,0.4); } }
@keyframes shimmer { 0% { transform: translateX(-100%); } 100% { transform: translateX(100%); } }
@keyframes spin { to { transform: rotate(360deg); } }

@media (max-width: 1280px) {
  .home-body { grid-template-columns: 200px 1fr 340px; }
  .bubble-image { max-width: 360px; }
}
</style>
