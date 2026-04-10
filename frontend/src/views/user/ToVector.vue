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
          <div v-for="ht in galleryTasks" :key="ht.task_id" class="gallery-item" :class="{ active: activeTaskId === ht.task_id }" @click="viewHistoryTask(ht)">
            <img :src="getTaskImageUrl(ht)" alt="" loading="lazy" @error="handleImgError" />
            <div class="gallery-item-overlay"><span class="gallery-item-prompt">{{ stripPrefix(ht.prompt) }}</span></div>
            <button class="gallery-delete-btn" @click.stop="showDeleteConfirm($event, ht.task_id)" title="删除">
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
            </button>
          </div>
        </div>
        <div class="history-empty" v-else>
          <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="#333" stroke-width="1.5"><rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="8.5" cy="8.5" r="1.5"/><path d="m21 15-5-5L5 21"/></svg>
          <p>还没有作品</p>
        </div>
      </aside>

      <!-- ====== 中间：生成区 ====== -->
      <main class="canvas-area">
        <div v-if="!userStore.isLoggedIn" class="guest-prompt">
          <p class="guest-title">温馨提示：</p>
          <p class="guest-desc">您当前是游客身份，暂无工具相关权限。</p>
          <p class="guest-action">请先 <router-link to="/user/register" class="link-highlight">注册</router-link> 或 <router-link to="/user/login" class="link-highlight">登录</router-link></p>
        </div>

        <div v-else class="gen-display" ref="chatMessagesRef">
          <div v-if="messages.length === 0" class="chat-empty">
            <div class="empty-icon-wrap">
              <svg width="64" height="64" viewBox="0 0 24 24" fill="none" stroke="#a3e635" stroke-width="1.2">
                <path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z"/>
              </svg>
            </div>
            <p class="chat-empty-text">转矢量图</p>
            <p class="chat-empty-sub">上传位图，调整参数，一键转换为 SVG 矢量图</p>
          </div>

          <div v-for="msg in messages" :key="msg.id" :id="msg.taskId ? 'task-' + msg.taskId : undefined" class="chat-bubble" :class="msg.role">
            <div v-if="msg.role === 'user'" class="bubble-row user">
              <div class="bubble-body user">
                <p class="bubble-prompt">{{ msg.prompt }}</p>
                <div class="bubble-ref-image" v-if="msg.refImageUrl"><img :src="msg.refImageUrl" alt="原图" /></div>
              </div>
              <span class="bubble-time" v-if="msg.time">{{ msg.time }}</span>
            </div>

            <div v-else class="bubble-row ai">
              <div v-if="msg.status === 'pending'" class="skeleton-wrap">
                <div class="skeleton-shimmer"></div>
                <div class="skeleton-overlay"><div class="skeleton-spinner"></div><p class="skeleton-text">正在转换矢量图...</p></div>
              </div>
              <div v-else-if="msg.status === 'failed'" class="bubble-body failed">
                <div class="bubble-failed-row"><div class="bubble-failed-icon">✕</div><div><div class="bubble-failed-title">转换失败</div><div class="bubble-failed-reason">{{ msg.errorMsg }}</div></div></div>
              </div>
              <template v-else-if="msg.status === 'completed'">
                <div class="bubble-body ai-success">
                  <div v-if="msg.imageUrl" class="bubble-image-wrap" @click="lightboxUrl = msg.imageUrl!">
                    <img :src="msg.imageUrl" class="bubble-image" @error="handleImgError" />
                    <div class="image-zoom-hint"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg></div>
                  </div>
                  <div class="bubble-footer">
                    <span class="bubble-meta" v-if="msg.elapsed">耗时 {{ msg.elapsed }}s</span>
                    <div class="result-actions">
                      <button class="result-action-btn" @click="downloadSVG(msg.imageUrl)">
                        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
                        下载 SVG
                      </button>
                      <button class="result-action-btn primary" @click="handleGenerate">再次生成</button>
                      <button class="result-action-btn danger" @click="showDeleteConfirm($event, msg.taskId)">删除</button>
                    </div>
                  </div>
                </div>
              </template>
            </div>
          </div>
        </div>
      </main>

      <!-- ====== 右侧：参数面板 ====== -->
      <aside class="control-panel">
        <div class="panel-top"><div class="panel-tabs"><button class="panel-tab active">转矢量图</button></div></div>

        <div class="panel-scroll">
          <!-- ① 上传图片 -->
          <div class="section-block">
            <div class="section-header"><span class="section-num">①</span><span class="section-title">上传图片</span></div>
            <ImageUploader v-model="uploadImages" :gallery-groups="allGalleryGroups" :max="1" :max-size-m-b="20" />
            <p class="section-tip">支持 JPG / PNG，建议不超过 5MB</p>
          </div>

          <!-- ② 转换模式 -->
          <div class="section-block" v-if="uploadedImage">
            <div class="section-header"><span class="section-num">②</span><span class="section-title">转换模式</span></div>
            <div class="mode-cards">
              <div class="mode-card" :class="{ active: colorCount === 0 }" @click="colorCount = 0">
                <div class="mode-card-icon">
                  <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><circle cx="12" cy="12" r="10"/><path d="M12 2a10 10 0 0 1 0 20V2z" fill="currentColor"/></svg>
                </div>
                <div class="mode-card-info"><span class="mode-card-name">黑白模式</span><span class="mode-card-desc">单色矢量，适合线稿/简单图形</span></div>
                <div v-if="colorCount === 0" class="mode-card-check"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3"><polyline points="20 6 9 17 4 12"/></svg></div>
              </div>
              <div class="mode-card" :class="{ active: colorCount > 0 }" @click="colorCount = 8">
                <div class="mode-card-icon">
                  <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><circle cx="12" cy="12" r="10"/><path d="M12 2a10 10 0 0 1 0 20" fill="#666"/><path d="M12 2a10 10 0 0 0 0 20" fill="#ccc"/></svg>
                </div>
                <div class="mode-card-info"><span class="mode-card-name">多色模式</span><span class="mode-card-desc">多层灰阶矢量，保留更多细节</span></div>
                <div v-if="colorCount > 0" class="mode-card-check"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3"><polyline points="20 6 9 17 4 12"/></svg></div>
              </div>
            </div>
          </div>

          <!-- 多色模式：颜色数 -->
          <div class="section-block" v-if="uploadedImage && colorCount > 0">
            <div class="param-row">
              <span class="param-label">颜色层数</span>
              <span class="param-value">{{ colorCount }}</span>
            </div>
            <input type="range" class="param-slider" v-model.number="colorCount" min="2" max="32" step="1" />
            <div class="param-range"><span>2</span><span>32</span></div>
          </div>

          <!-- ③ 参数调节 -->
          <div class="section-block" v-if="uploadedImage">
            <div class="section-header"><span class="section-num">③</span><span class="section-title">参数调节</span></div>

            <!-- 亮度阈值 -->
            <div class="param-row">
              <span class="param-label">亮度阈值</span>
              <span class="param-value">{{ threshold.toFixed(2) }}</span>
            </div>
            <input type="range" class="param-slider" v-model.number="threshold" min="0.01" max="1.0" step="0.01" />
            <div class="param-range"><span>0.01 (深)</span><span>1.00 (浅)</span></div>

            <!-- 去噪点 -->
            <div class="param-row">
              <span class="param-label">去噪点 (Turd Size)</span>
              <span class="param-value">{{ turdSize }}</span>
            </div>
            <input type="range" class="param-slider" v-model.number="turdSize" min="0" max="100" step="1" />
            <div class="param-range"><span>0 (保留全部)</span><span>100 (去除大噪点)</span></div>

            <!-- 平滑度 -->
            <div class="param-row">
              <span class="param-label">平滑度 (Alpha Max)</span>
              <span class="param-value">{{ alphaMax.toFixed(2) }}</span>
            </div>
            <input type="range" class="param-slider" v-model.number="alphaMax" min="0" max="1.33" step="0.01" />
            <div class="param-range"><span>0 (尖锐)</span><span>1.33 (平滑)</span></div>

            <!-- 优化容差 -->
            <div class="param-row">
              <span class="param-label">优化容差 (Opt Tolerance)</span>
              <span class="param-value">{{ optTolerance.toFixed(2) }}</span>
            </div>
            <input type="range" class="param-slider" v-model.number="optTolerance" min="0" max="5" step="0.1" />
            <div class="param-range"><span>0 (精确)</span><span>5.0 (简化)</span></div>

            <!-- 转向策略 -->
            <div class="param-row">
              <span class="param-label">转向策略 (Turn Policy)</span>
            </div>
            <div class="policy-grid">
              <button v-for="p in policies" :key="p.value" class="policy-btn" :class="{ active: turnPolicy === p.value }" @click="turnPolicy = p.value">{{ p.label }}</button>
            </div>

            <!-- 反转 -->
            <div class="param-row toggle-row" @click="invert = !invert">
              <span class="param-label">反转黑白</span>
              <div class="toggle-switch" :class="{ on: invert }"><div class="toggle-knob"></div></div>
            </div>
          </div>

          <!-- potrace 状态 -->
          <div class="section-block" v-if="potraceStatus !== null">
            <div class="potrace-status" :class="{ ok: potraceStatus === true, err: potraceStatus === false }">
              <span v-if="potraceStatus === true">✓ potrace {{ potraceVersion }}</span>
              <span v-else>✕ potrace 未安装，请在服务器安装</span>
            </div>
          </div>
        </div>

        <!-- 底部操作 -->
        <div class="panel-bottom">
          <div class="button-group">
            <button class="btn-generate" :disabled="!canGenerate || generating" @click="handleGenerate">
              {{ generating ? '转换中...' : '开始转换' }}
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
          <button class="lightbox-action-btn" @click="downloadSVG(lightboxUrl)">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
            下载 SVG
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, nextTick, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { useMessage } from 'naive-ui'
import { useUserStore } from '../../stores/user'
import { vectorizeApi, taskApi } from '../../api'
import ImageUploader from '../../components/ImageUploader.vue'
import { buildGalleryGroups } from '../../utils/galleryGroups'
import { getTaskImageUrl as extractTaskImageUrl, stripFeaturePrefix } from '../../utils/task-history'
import type { Task } from '../../api'

const FEATURE_SLUG = 'to-vector'
const FEATURE_PREFIX = `[${FEATURE_SLUG}] `

const router = useRouter()
const message = useMessage()
const userStore = useUserStore()

function stripPrefix(str?: string) {
  return stripFeaturePrefix(str)
}

// ============ 参数 ============
const threshold = ref(0.45)
const turdSize = ref(2)
const alphaMax = ref(1.0)
const optTolerance = ref(0.2)
const turnPolicy = ref('minority')
const invert = ref(false)
const colorCount = ref(0) // 0 = B&W, >= 2 = multi-color

const policies = [
  { value: 'minority', label: '少数' },
  { value: 'majority', label: '多数' },
  { value: 'black', label: '黑色' },
  { value: 'white', label: '白色' },
  { value: 'left', label: '左转' },
  { value: 'right', label: '右转' },
  { value: 'random', label: '随机' },
]

// ============ potrace 检测 ============
const potraceStatus = ref<boolean | null>(null)
const potraceVersion = ref('')

async function checkPotrace() {
  try {
    const res: any = await vectorizeApi.checkPotrace()
    if (res.code === 0) {
      potraceStatus.value = true
      potraceVersion.value = res.data.version
    } else {
      potraceStatus.value = false
    }
  } catch {
    potraceStatus.value = false
  }
}

// ============ 画廊 ============
const historyTasks = ref<Task[]>([])
const activeTaskId = ref<string | null>(null)

const galleryTasks = computed(() => {
  return historyTasks.value.filter(t => {
    if (t.status !== 'completed') return false
    if (!t.prompt || !t.prompt.startsWith(FEATURE_PREFIX)) return false
    try {
      const r = JSON.parse(t.result || '{}')
      return !!r.image_url
    } catch { return false }
  })
})

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
    messages.value.push({ id: 'hist_user_' + task.task_id, role: 'user', prompt: stripPrefix(task.prompt) || '转矢量图', time: timeStr })
    if (task.status === 'completed' && imgUrl) {
      let elapsed: string | undefined
      if (task.completed_at && task.created_at) {
        const t = new Date(task.updated_at).getTime() - new Date(task.created_at).getTime()
        if (t > 0) elapsed = (t / 1000).toFixed(1)
      }
      messages.value.push({ id: 'hist_ai_' + task.task_id, role: 'assistant', prompt: task.prompt || '', status: 'completed', imageUrl: imgUrl, taskId: task.task_id, elapsed })
    } else if (task.status === 'failed') {
      messages.value.push({ id: 'hist_ai_' + task.task_id, role: 'assistant', prompt: task.prompt || '', status: 'failed', errorMsg: task.error_msg || '转换失败', taskId: task.task_id })
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
      messages.value.push({ id: 'hist_user_' + task.task_id, role: 'user', prompt: stripPrefix(task.prompt) || '转矢量图' })
      messages.value.push({ id: 'hist_ai_' + task.task_id, role: 'assistant', prompt: task.prompt || '', status: 'completed', imageUrl: imgUrl, taskId: task.task_id })
      scrollToBottom()
    } else {
      const el = document.getElementById('task-' + task.task_id)
      if (el) {
        el.scrollIntoView({ behavior: 'smooth', block: 'center' })
        el.classList.add('highlight')
        setTimeout(() => el.classList.remove('highlight'), 2000)
      }
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
      messages.value = messages.value.filter(m => m.id !== 'hist_user_' + taskId && m.id !== 'hist_ai_' + taskId && m.taskId !== taskId)
      if (activeTaskId.value === taskId) activeTaskId.value = null
    }
  } catch {}
}

// ============ 上传 ============
const uploadedImage = ref('')
const uploadImages = computed({
  get: () => uploadedImage.value ? [uploadedImage.value] : [],
  set: (val: string[]) => { uploadedImage.value = val.length > 0 ? val[0] : '' }
})
const allGalleryGroups = computed(() => buildGalleryGroups(historyTasks.value))

// ============ 消息 ============
interface ChatMessage {
  id: string; role: 'user' | 'assistant'; prompt?: string; refImageUrl?: string
  status?: 'pending' | 'completed' | 'failed'; imageUrl?: string; errorMsg?: string
  taskId?: string; elapsed?: string; time?: string
}
const messages = ref<ChatMessage[]>([])
const chatMessagesRef = ref<HTMLElement | null>(null)

function scrollToBottom() {
  nextTick(() => { if (chatMessagesRef.value) chatMessagesRef.value.scrollTop = chatMessagesRef.value.scrollHeight })
}

// ============ 生成 ============
const generating = ref(false)
const lightboxUrl = ref('')
const canGenerate = computed(() => userStore.isLoggedIn && !generating.value && !!uploadedImage.value)

async function handleGenerate() {
  if (!userStore.isLoggedIn) { message.warning('请先登录'); router.push('/user/login'); return }
  if (!uploadedImage.value) { message.warning('请先上传图片'); return }

  activeTaskId.value = null
  const modeDesc = colorCount.value > 0 ? `${colorCount.value}色模式` : '黑白模式'
  const desc = `${modeDesc} 阈值${threshold.value.toFixed(2)}`
  const now = new Date()
  const timeStr = `${now.getHours().toString().padStart(2, '0')}:${now.getMinutes().toString().padStart(2, '0')}`

  const userMsg: ChatMessage = { id: 'msg_' + Date.now(), role: 'user', prompt: desc, refImageUrl: uploadedImage.value, time: timeStr }
  messages.value.push(userMsg)

  const assistantMsg: ChatMessage = { id: 'loading_' + Date.now(), role: 'assistant', prompt: desc, status: 'pending' }
  messages.value.push(assistantMsg)
  const assistantRef = messages.value[messages.value.length - 1]
  scrollToBottom()

  generating.value = true
  const startTime = Date.now()

  try {
    const res: any = await vectorizeApi.vectorize({
      image_url: uploadedImage.value,
      threshold: threshold.value,
      turd_size: turdSize.value,
      alpha_max: alphaMax.value,
      opt_tolerance: optTolerance.value,
      turn_policy: turnPolicy.value,
      invert: invert.value,
      color_count: colorCount.value,
    })

    if (res.code === 0) {
      const elapsed = ((Date.now() - startTime) / 1000).toFixed(1)
      assistantRef.imageUrl = res.data.svg_url
      assistantRef.taskId = res.data.task_id
      assistantRef.status = 'completed'
      assistantRef.elapsed = elapsed
      loadHistory()
    } else {
      assistantRef.status = 'failed'
      assistantRef.errorMsg = res.msg || '转换失败'
    }
  } catch (e: any) {
    assistantRef.status = 'failed'
    assistantRef.errorMsg = e.msg || e.message || '网络错误，请重试'
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

function handleImgError(e: Event) { (e.target as HTMLImageElement).style.background = '#27272a' }

function downloadSVG(url?: string) {
  if (!url) return
  const a = document.createElement('a')
  a.href = url
  a.download = 'vector.svg'
  a.click()
}

// ============ 生命周期 ============
onMounted(() => {
  if (userStore.isLoggedIn) {
    loadHistory()
    checkPotrace()
  }
})
</script>

<style scoped>
.home-layout { width: 100%; height: 100%; display: flex; flex-direction: column; background: #111113; overflow: hidden; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif; }
.home-body { flex: 1; display: grid; grid-template-columns: 220px 1fr 380px; min-height: 0; overflow: hidden; }

/* 左侧画廊 */
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

/* 中间生成区 */
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

/* 聊天气泡 */
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
.bubble-failed-title { font-size: 13px; color: #f87171; font-weight: 500; }
.bubble-failed-reason { font-size: 12px; color: #888; margin-top: 3px; word-break: break-word; }
.bubble-image-wrap { border-radius: 10px; overflow: hidden; max-width: 400px; cursor: pointer; position: relative; background: #fff; }
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

/* 骨架 */
.skeleton-wrap { width: 240px; aspect-ratio: 1 / 1; border-radius: 12px; background: #1a1a1e; border: 1px solid #27272a; position: relative; overflow: hidden; }
.skeleton-shimmer { position: absolute; inset: 0; background: linear-gradient(110deg, transparent 30%, rgba(163,230,53,0.04) 50%, transparent 70%); background-size: 300% 100%; animation: shimmer 2s ease-in-out infinite; }
@keyframes shimmer { 0% { background-position: 200% 0; } 100% { background-position: -100% 0; } }
.skeleton-overlay { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 12px; }
.skeleton-spinner { width: 32px; height: 32px; border: 3px solid #27272a; border-top-color: #a3e635; border-radius: 50%; animation: spin 1s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
.skeleton-text { font-size: 13px; color: #888; font-weight: 500; }

/* 删除确认 */
.delete-popover { position: fixed; z-index: 1000; background: #27272a; border: 1px solid #3f3f46; border-radius: 10px; padding: 12px 16px; box-shadow: 0 8px 24px rgba(0,0,0,0.4); transform: translate(-50%, -100%); margin-top: -8px; }
.delete-popover p { margin: 0 0 8px; font-size: 13px; color: #e4e4e7; }
.delete-popover-btns { display: flex; gap: 8px; justify-content: flex-end; }
.delete-popover-btns button { padding: 4px 14px; border-radius: 6px; border: 1px solid #3f3f46; background: #18181b; color: #a1a1aa; font-size: 12px; cursor: pointer; }
.delete-popover-btns button.confirm { background: rgba(220,38,38,0.2); border-color: rgba(220,38,38,0.4); color: #ef4444; }

/* lightbox */
.lightbox-overlay { position: fixed; inset: 0; z-index: 999; background: rgba(0,0,0,0.85); display: flex; align-items: center; justify-content: center; }
.lightbox-container { position: relative; max-width: 90vw; max-height: 90vh; }
.lightbox-image { max-width: 100%; max-height: 85vh; border-radius: 8px; background: #fff; }
.lightbox-close { position: absolute; top: -10px; right: -10px; width: 32px; height: 32px; border-radius: 50%; background: #333; border: none; color: #ccc; cursor: pointer; display: flex; align-items: center; justify-content: center; }
.lightbox-actions { display: flex; gap: 8px; justify-content: center; margin-top: 12px; }
.lightbox-action-btn { padding: 8px 16px; border-radius: 8px; border: 1px solid #444; background: #27272a; color: #ccc; font-size: 13px; cursor: pointer; display: flex; align-items: center; gap: 6px; }

/* ============ 右侧面板 ============ */
.control-panel { display: flex; flex-direction: column; border-left: 1px solid #222225; background: #18181b; overflow: hidden; }
.panel-top { padding: 12px 16px 0; flex-shrink: 0; }
.panel-tabs { display: flex; gap: 0; background: #27272a; border-radius: 8px; padding: 3px; }
.panel-tab { flex: 1; padding: 7px 0; border: none; border-radius: 6px; background: transparent; color: #888; font-size: 13px; font-weight: 500; cursor: pointer; transition: all 0.2s; }
.panel-tab.active { background: #3f3f46; color: #e4e4e7; }
.panel-scroll { flex: 1; overflow-y: auto; padding: 16px; }
.panel-bottom { padding: 16px; border-top: 1px solid #27272a; flex-shrink: 0; }
.button-group { display: flex; gap: 10px; }
.btn-generate { flex: 1; padding: 12px 24px; border-radius: 10px; border: none; background: linear-gradient(135deg, #a3e635 0%, #65a30d 100%); color: #111; font-size: 14px; font-weight: 600; cursor: pointer; transition: all 0.2s; }
.btn-generate:hover:not(:disabled) { opacity: 0.9; transform: translateY(-1px); }
.btn-generate:disabled { opacity: 0.4; cursor: not-allowed; }
.btn-clear { padding: 12px 24px; border-radius: 10px; border: 1px solid #333; background: #27272a; color: #aaa; font-size: 14px; cursor: pointer; }
.btn-clear:hover:not(:disabled) { background: #3f3f46; }

/* section */
.section-block { margin-bottom: 20px; }
.section-header { display: flex; align-items: center; gap: 8px; margin-bottom: 10px; }
.section-num { display: inline-flex; align-items: center; justify-content: center; width: 22px; height: 22px; border-radius: 50%; background: rgba(163,230,53,0.15); color: #a3e635; font-size: 12px; font-weight: 700; flex-shrink: 0; }
.section-title { font-size: 13px; font-weight: 600; color: #ccc; }
.section-tip { font-size: 11px; color: #555; margin-top: 6px; }

/* 模式卡片 */
.mode-cards { display: flex; flex-direction: column; gap: 8px; }
.mode-card { display: flex; align-items: center; gap: 12px; padding: 12px; border-radius: 10px; border: 1px solid #333; background: #1a1a1e; cursor: pointer; transition: all 0.2s; }
.mode-card:hover { border-color: #555; }
.mode-card.active { border-color: #a3e635; background: rgba(163,230,53,0.04); }
.mode-card-icon { width: 40px; height: 40px; display: flex; align-items: center; justify-content: center; border-radius: 8px; background: #27272a; color: #aaa; flex-shrink: 0; }
.mode-card.active .mode-card-icon { color: #a3e635; }
.mode-card-info { flex: 1; }
.mode-card-name { display: block; font-size: 13px; font-weight: 600; color: #ccc; }
.mode-card-desc { display: block; font-size: 11px; color: #666; margin-top: 2px; }
.mode-card-check { width: 20px; height: 20px; border-radius: 50%; background: #a3e635; display: flex; align-items: center; justify-content: center; color: #111; flex-shrink: 0; }

/* 参数控件 */
.param-row { display: flex; align-items: center; justify-content: space-between; margin-bottom: 6px; margin-top: 14px; }
.param-row:first-child { margin-top: 0; }
.param-label { font-size: 12px; color: #aaa; font-weight: 500; }
.param-value { font-size: 12px; color: #a3e635; font-weight: 600; font-variant-numeric: tabular-nums; }
.param-slider { width: 100%; height: 4px; border-radius: 2px; -webkit-appearance: none; appearance: none; background: #333; outline: none; }
.param-slider::-webkit-slider-thumb { -webkit-appearance: none; width: 14px; height: 14px; border-radius: 50%; background: #a3e635; cursor: pointer; border: 2px solid #111; }
.param-range { display: flex; justify-content: space-between; font-size: 10px; color: #555; margin-top: 4px; }

/* 转向策略网格 */
.policy-grid { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 6px; }
.policy-btn { padding: 5px 12px; border-radius: 6px; border: 1px solid #333; background: #1a1a1e; color: #888; font-size: 12px; cursor: pointer; transition: all 0.2s; }
.policy-btn:hover { border-color: #555; color: #ccc; }
.policy-btn.active { border-color: #a3e635; background: rgba(163,230,53,0.08); color: #a3e635; }

/* 反转开关 */
.toggle-row { cursor: pointer; padding: 8px 0; }
.toggle-switch { width: 36px; height: 20px; border-radius: 10px; background: #333; position: relative; transition: background 0.2s; }
.toggle-switch.on { background: #a3e635; }
.toggle-knob { width: 16px; height: 16px; border-radius: 50%; background: #fff; position: absolute; top: 2px; left: 2px; transition: transform 0.2s; }
.toggle-switch.on .toggle-knob { transform: translateX(16px); }

/* potrace 状态 */
.potrace-status { font-size: 11px; padding: 8px 12px; border-radius: 8px; }
.potrace-status.ok { background: rgba(163,230,53,0.08); color: #a3e635; border: 1px solid rgba(163,230,53,0.15); }
.potrace-status.err { background: rgba(248,113,113,0.08); color: #f87171; border: 1px solid rgba(248,113,113,0.15); }
</style>
