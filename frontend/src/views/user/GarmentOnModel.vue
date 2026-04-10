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
          <p style="font-size:11px;color:#444;">生成图片后将在这里展示</p>
        </div>
      </aside>

      <!-- ====== 中间：生成区 ====== -->
      <main class="canvas-area">
        <div v-if="!userStore.isLoggedIn" class="guest-prompt">
          <div class="smiley-icon">
            <svg width="120" height="120" viewBox="0 0 120 120" fill="none"><circle cx="60" cy="60" r="56" stroke="#a3e635" stroke-width="4" fill="none"/><circle cx="44" cy="48" r="5" fill="#a3e635"/><circle cx="76" cy="48" r="5" fill="#a3e635"/><path d="M38 72 C38 72, 44 90, 60 90 C76 90, 82 72, 82 72" stroke="#a3e635" stroke-width="4" stroke-linecap="round" fill="none"/></svg>
          </div>
          <p class="guest-title">温馨提示：</p>
          <p class="guest-desc">您当前是游客身份，暂无工具相关权限。</p>
          <p class="guest-action">请先 <router-link to="/user/register" class="link-highlight">注册</router-link> 或 <router-link to="/user/login" class="link-highlight">登录</router-link></p>
        </div>
        <div v-else class="gen-display" ref="chatMessagesRef">
          <div v-if="messages.length === 0" class="chat-empty">
            <div class="empty-icon-wrap">
              <svg width="64" height="64" viewBox="0 0 24 24" fill="none" stroke="#a3e635" stroke-width="1.2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 2L2 7l10 5 10-5-10-5z"/><path d="M2 17l10 5 10-5"/><path d="M2 12l10 5 10-5"/></svg>
            </div>
            <p class="chat-empty-text">款式上身</p>
            <p class="chat-empty-sub">上传款式图片，选择模特和姿势，一键生成上身效果</p>
          </div>
          <div v-for="msg in messages" :key="msg.id" :id="msg.taskId ? 'task-' + msg.taskId : undefined" class="chat-bubble" :class="msg.role">
            <div v-if="msg.role === 'user'" class="bubble-row user">
              <div class="bubble-body user">
                <p class="bubble-prompt">{{ msg.prompt }}</p>
                <div class="bubble-ref-images" v-if="msg.refImageUrls && msg.refImageUrls.length">
                  <img v-for="(url, i) in msg.refImageUrls" :key="i" :src="url" alt="参考图" class="bubble-ref-img" />
                </div>
              </div>
              <span class="bubble-time" v-if="msg.time">{{ msg.time }}</span>
            </div>
            <div v-else class="bubble-row ai">
              <div v-if="msg.status === 'pending'" class="skeleton-wrap"><div class="skeleton-shimmer"></div><div class="skeleton-overlay"><div class="skeleton-spinner"></div><p class="skeleton-text">AI 正在生成中...</p><p class="skeleton-sub">请耐心等待，通常需要 10-30 秒</p></div></div>
              <div v-else-if="msg.status === 'failed'" class="bubble-body failed"><div class="bubble-failed-row"><div class="bubble-failed-icon">✕</div><div class="bubble-failed-info"><div class="bubble-failed-title">生成失败</div><div class="bubble-failed-reason">{{ msg.errorMsg }}</div></div></div></div>
              <template v-else-if="msg.status === 'completed'">
                <div class="bubble-body ai-success">
                  <div v-if="msg.imageUrl" class="bubble-image-wrap" @click="lightboxUrl = msg.imageUrl!">
                    <img :src="msg.imageUrl" :alt="msg.prompt" class="bubble-image" @error="handleImgError" />
                    <div class="image-zoom-hint"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/><line x1="11" y1="8" x2="11" y2="14"/><line x1="8" y1="11" x2="14" y2="11"/></svg></div>
                  </div>
                  <div class="bubble-footer">
                    <span class="bubble-meta" v-if="msg.elapsed">耗时 {{ msg.elapsed }}s</span>
                    <div class="result-actions">
                      <button class="result-action-btn" @click="downloadImage(msg.imageUrl)"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>下载</button>
                      <button class="result-action-btn primary" @click="handleGenerate"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/></svg>再次生成</button>
                      <button class="result-action-btn danger" @click="showDeleteConfirm($event, msg.taskId)"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>删除</button>
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
          <div class="panel-tabs"><button class="panel-tab active">款式上身</button></div>
        </div>
        <div class="panel-scroll">

          <!-- ① 上传款式图片 -->
          <div class="section">
            <div class="section-header-row"><span class="section-num">①</span><span class="section-label">上传款式图片（1-3 张）</span></div>
            <ImageUploader v-model="garmentImages" :gallery-groups="allGalleryGroups" :max="3" :max-size-m-b="20" />
          </div>

          <!-- ② 选择模特 -->
          <div class="section" v-if="garmentImages.length > 0">
            <div class="section-header-row"><span class="section-num">②</span><span class="section-label">选择模特</span></div>
            <div class="model-tabs">
              <button v-for="tab in modelTabs" :key="tab.key" class="model-tab" :class="{ active: activeModelTab === tab.key }" @click="activeModelTab = tab.key">{{ tab.label }}</button>
            </div>

            <!-- 系统模特列表 -->
            <div v-if="activeModelTab !== 'custom'" class="model-grid">
              <div v-if="currentModelList.length === 0" class="model-empty">
                <span>暂无{{ activeModelTab === 'male' ? '男模' : '女模' }}模板</span>
                <span style="font-size:10px;color:#444;">请联系管理员添加</span>
              </div>
              <div v-for="m in currentModelList" :key="m.id" class="model-card" :class="{ active: selectedModelUrl === m.image_url }" @click="selectModel(m.image_url, m.name)">
                <div class="model-card-img">
                  <img :src="m.image_url" :alt="m.name" @error="handleImgError" />
                  <div v-if="selectedModelUrl === m.image_url" class="model-card-check">
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3"><polyline points="20 6 9 17 4 12"/></svg>
                  </div>
                </div>
                <span class="model-card-name">{{ m.name }}</span>
              </div>
            </div>

            <!-- 自定义上传模特 -->
            <div v-else class="custom-model-section">
              <div v-if="customModelUrl" class="custom-model-preview">
                <img :src="customModelUrl" alt="自定义模特" @error="handleImgError" />
                <button class="custom-model-remove" @click="removeCustomModel" title="移除">
                  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
                </button>
              </div>
              <label v-else class="custom-model-dropzone" :class="{ disabled: customModelUploading }">
                <input type="file" accept="image/jpeg,image/png" @change="handleCustomModelUpload" hidden />
                <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="#555" stroke-width="1.5"><rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="8.5" cy="8.5" r="1.5"/><path d="m21 15-5-5L5 21"/></svg>
                <span v-if="customModelUploading">上传中...</span>
                <span v-else>点击上传模特照片</span>
              </label>
            </div>
          </div>

          <!-- ③ 选择姿势 -->
          <div class="section" v-if="finalModelUrl">
            <div class="section-header-row"><span class="section-num">③</span><span class="section-label">选择模特姿势</span></div>
            <div v-if="poseLoading" class="pose-loading">加载中...</div>
            <div v-else-if="poseTemplates.length === 0" class="pose-empty">暂无姿势模板，请联系管理员添加</div>
            <div v-else class="pose-grid">
              <div v-for="p in poseTemplates" :key="p.id" class="pose-card" :class="{ active: selectedPoseUrl === p.image_url }" @click="selectPose(p.image_url, p.name)">
                <div class="pose-card-img">
                  <img :src="p.image_url" :alt="p.name" @error="handleImgError" />
                  <div v-if="selectedPoseUrl === p.image_url" class="pose-card-check">
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3"><polyline points="20 6 9 17 4 12"/></svg>
                  </div>
                </div>
                <span class="pose-card-name">{{ p.name }}</span>
              </div>
            </div>
          </div>

          <!-- ④ 提示词（可选） -->
          <div class="section" v-if="selectedPoseUrl">
            <div class="section-header-row"><span class="section-num">④</span><span class="section-label">提示词（可选）</span></div>
            <textarea v-model="userPrompt" class="prompt-textarea" rows="3" placeholder="描述期望的效果，如：时尚杂志风格、户外自然光..."></textarea>
          </div>

          <!-- 状态提示 -->
          <div class="section status-section">
            <div v-if="canGenerate" class="status-ready">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#a3e635" stroke-width="2"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>
              <span>已就绪，可以开始生成</span>
            </div>
            <div v-else class="status-waiting">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#71717a" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>
              <span v-if="garmentImages.length === 0">请上传款式图片</span>
              <span v-else-if="!finalModelUrl">请选择模特</span>
              <span v-else-if="!selectedPoseUrl">请选择姿势</span>
              <span v-else>准备就绪</span>
            </div>
          </div>
        </div>

        <div class="panel-bottom">
          <div class="button-group">
            <button class="btn-generate" :disabled="!canGenerate || generating" @click="handleGenerate">
              {{ generating ? '生成中...' : '生成图片' }}
            </button>
            <button class="btn-reset" @click="resetAll">重置</button>
          </div>
        </div>
      </aside>

    </div>

    <!-- 灯箱 -->
    <Teleport to="body">
      <div v-if="lightboxUrl" class="lightbox-overlay" @click.self="lightboxUrl = ''">
        <img :src="lightboxUrl" class="lightbox-img" />
        <button class="lightbox-close" @click="lightboxUrl = ''">&times;</button>
      </div>
    </Teleport>

    <!-- 删除确认弹窗 -->
    <Teleport to="body">
      <div v-if="deleteConfirm.show" class="delete-popover" :style="{ left: deleteConfirm.x + 'px', top: deleteConfirm.y + 'px' }">
        <p style="margin:0 0 10px;color:#e4e4e7;">确定删除？</p>
        <div style="display:flex;gap:8px;justify-content:flex-end;">
          <button class="dp-btn cancel" @click="deleteConfirm.show = false">取消</button>
          <button class="dp-btn danger" @click="confirmDelete">删除</button>
        </div>
      </div>
    </Teleport>

  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, nextTick, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { useMessage } from 'naive-ui'
import { useUserStore } from '../../stores/user'
import { taskApi, modelTemplateApi, poseTemplateApi, uploadApi } from '../../api'
import ImageUploader from '../../components/ImageUploader.vue'
import { buildGalleryGroups } from '../../utils/galleryGroups'
import { downloadImageByProxy, getTaskImageUrl as extractTaskImageUrl, stripFeaturePrefix } from '../../utils/task-history'
import type { Task } from '../../api'

const FEATURE_SLUG = 'garment-on-model'
const FEATURE_TITLE = '款式上身'
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
const galleryTasks = computed(() => historyTasks.value.filter(t => { if (t.status !== 'completed') return false; if (!t.prompt || !t.prompt.startsWith(FEATURE_PREFIX)) return false; try { return !!JSON.parse(t.result || '{}').image_url } catch { return false } }))
function getTaskImageUrl(task: Task): string {
  return extractTaskImageUrl(task)
}

async function loadHistory() { try { const res: any = await taskApi.list({ page: 1, page_size: 200 }); if (res.code === 0) { historyTasks.value = res.data.list || []; if (messages.value.length === 0) rebuildMessagesFromHistory() } } catch {} }

function rebuildMessagesFromHistory() {
  const tasks = historyTasks.value.filter(t => t.prompt && t.prompt.startsWith(FEATURE_PREFIX)).sort((a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime()).slice(-20)
  for (const task of tasks) {
    const imgUrl = getTaskImageUrl(task); const timeStr = task.created_at ? new Date(task.created_at).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' }) : ''
    messages.value.push({ id: 'hist_user_' + task.task_id, role: 'user', prompt: stripPrefix(task.prompt) || FEATURE_TITLE, time: timeStr })
    if (task.status === 'completed' && imgUrl) { let elapsed: string | undefined; if (task.completed_at && task.created_at) { const t = new Date(task.updated_at).getTime() - new Date(task.created_at).getTime(); if (t > 0) elapsed = (t / 1000).toFixed(1) }; messages.value.push({ id: 'hist_ai_' + task.task_id, role: 'assistant', prompt: task.prompt || '', status: 'completed', imageUrl: imgUrl, taskId: task.task_id, elapsed }) }
    else if (task.status === 'failed') { messages.value.push({ id: 'hist_ai_' + task.task_id, role: 'assistant', prompt: task.prompt || '', status: 'failed', errorMsg: task.error_msg || '生成失败', taskId: task.task_id }) }
  }
  if (messages.value.length > 0) scrollToBottom()
}

function viewHistoryTask(task: Task) {
  activeTaskId.value = task.task_id; const imgUrl = getTaskImageUrl(task)
  if (imgUrl) { const existing = messages.value.find(m => m.taskId === task.task_id); if (!existing) { messages.value.push({ id: 'hist_user_' + task.task_id, role: 'user', prompt: stripPrefix(task.prompt) || FEATURE_TITLE }); messages.value.push({ id: 'hist_ai_' + task.task_id, role: 'assistant', prompt: task.prompt || '', status: 'completed', imageUrl: imgUrl, taskId: task.task_id }); scrollToBottom() } else { const el = document.getElementById('task-' + task.task_id); if (el) { el.scrollIntoView({ behavior: 'smooth', block: 'center' }); el.classList.add('highlight'); setTimeout(() => el.classList.remove('highlight'), 2000) } } }
}

// ============ 删除 ============
const deleteConfirm = ref<{ show: boolean; x: number; y: number; taskId: string }>({ show: false, x: 0, y: 0, taskId: '' })
function showDeleteConfirm(e: MouseEvent, taskId?: string) { if (!taskId) return; deleteConfirm.value = { show: true, x: e.clientX, y: e.clientY, taskId }; const close = (ev: MouseEvent) => { const el = document.querySelector('.delete-popover'); if (el && !el.contains(ev.target as Node)) { deleteConfirm.value.show = false; document.removeEventListener('mousedown', close) } }; setTimeout(() => document.addEventListener('mousedown', close), 0) }
async function confirmDelete() { const taskId = deleteConfirm.value.taskId; deleteConfirm.value.show = false; if (!taskId) return; try { const res: any = await taskApi.delete(taskId); if (res.code === 0) { historyTasks.value = historyTasks.value.filter(t => t.task_id !== taskId); messages.value = messages.value.filter(m => m.id !== 'hist_user_' + taskId && m.id !== 'hist_ai_' + taskId && m.taskId !== taskId); if (activeTaskId.value === taskId) activeTaskId.value = null } } catch {} }

// ============ ① 款式图片上传 ============
const garmentImages = ref<string[]>([])
const allGalleryGroups = computed(() => buildGalleryGroups(historyTasks.value))

// ============ ② 模特选择 ============
interface ModelTpl { id: number; name: string; image_url: string; category: string }
const modelTemplates = ref<ModelTpl[]>([])
const modelLoading = ref(false)
const activeModelTab = ref('female')
const selectedModelUrl = ref('')
const selectedModelName = ref('')
const customModelUrl = ref('')
const customModelUploading = ref(false)

const modelTabs = [
  { key: 'female', label: '女模' },
  { key: 'male', label: '男模' },
  { key: 'custom', label: '自定义' },
]

const currentModelList = computed(() => modelTemplates.value.filter(m => m.category === activeModelTab.value))

async function loadModelTemplates() {
  modelLoading.value = true
  try { const res: any = await modelTemplateApi.listEnabled(); if (res.code === 0) modelTemplates.value = res.data?.list || [] } catch {}
  modelLoading.value = false
}

function selectModel(url: string, name: string) {
  selectedModelUrl.value = url
  selectedModelName.value = name
  customModelUrl.value = ''
}

async function handleCustomModelUpload(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  input.value = ''
  customModelUploading.value = true
  try {
    const res: any = await uploadApi.upload(file)
    if (res.code === 0 && res.data?.url) {
      customModelUrl.value = res.data.url
      selectedModelUrl.value = res.data.url
      selectedModelName.value = '自定义模特'
      message.success('上传成功')
    } else { message.error(res.msg || '上传失败') }
  } catch (err: any) { message.error('上传失败: ' + (err.message || '网络错误')) }
  customModelUploading.value = false
}

function removeCustomModel() {
  customModelUrl.value = ''
  if (selectedModelUrl.value === customModelUrl.value || !modelTemplates.value.some(m => m.image_url === selectedModelUrl.value)) {
    selectedModelUrl.value = ''
    selectedModelName.value = ''
  }
}

const finalModelUrl = computed(() => selectedModelUrl.value)

// ============ ③ 姿势选择 ============
interface PoseTpl { id: number; name: string; image_url: string; category: string }
const poseTemplates = ref<PoseTpl[]>([])
const poseLoading = ref(false)
const selectedPoseUrl = ref('')
const selectedPoseName = ref('')

async function loadPoseTemplates() {
  poseLoading.value = true
  try { const res: any = await poseTemplateApi.listEnabled(); if (res.code === 0) poseTemplates.value = res.data?.list || [] } catch {}
  poseLoading.value = false
}

function selectPose(url: string, name: string) {
  selectedPoseUrl.value = url
  selectedPoseName.value = name
}

// ============ ④ 提示词 ============
const userPrompt = ref('')

// ============ 生成 ============
const generating = ref(false)
const canGenerate = computed(() => garmentImages.value.length > 0 && !!finalModelUrl.value && !!selectedPoseUrl.value && !generating.value)

interface ChatMessage {
  id: string; role: 'user' | 'assistant'; prompt: string; time?: string
  status?: 'pending' | 'completed' | 'failed'; imageUrl?: string; taskId?: string
  elapsed?: string; errorMsg?: string; refImageUrls?: string[]
}
const messages = ref<ChatMessage[]>([])
const chatMessagesRef = ref<HTMLElement | null>(null)
const lightboxUrl = ref('')

function scrollToBottom() { nextTick(() => { if (chatMessagesRef.value) chatMessagesRef.value.scrollTop = chatMessagesRef.value.scrollHeight }) }

let pollTimer: ReturnType<typeof setTimeout> | null = null

async function handleGenerate() {
  if (!canGenerate.value) return
  generating.value = true

  const promptText = userPrompt.value.trim() || FEATURE_TITLE
  const timeStr = new Date().toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
  const userMsgId = 'user_' + Date.now()
  messages.value.push({ id: userMsgId, role: 'user', prompt: promptText, time: timeStr, refImageUrls: [...garmentImages.value] })
  scrollToBottom()

  try {
    const params: Record<string, any> = {
      image_urls: garmentImages.value,
      model_url: finalModelUrl.value,
      pose_url: selectedPoseUrl.value,
      feature: FEATURE_SLUG,
    }

    const res: any = await taskApi.submit({
      prompt: promptText,
      feature: FEATURE_SLUG,
      params,
    })

    if (res.code !== 0) {
      messages.value.push({ id: 'ai_err_' + Date.now(), role: 'assistant', prompt: promptText, status: 'failed', errorMsg: res.msg || '提交失败' })
      generating.value = false
      scrollToBottom()
      return
    }

    const taskId = res.data.task_id
    const aiMsgId = 'ai_' + taskId
    messages.value.push({ id: aiMsgId, role: 'assistant', prompt: promptText, status: 'pending', taskId })
    scrollToBottom()

    pollTaskResult(taskId, aiMsgId)
  } catch (e: any) {
    messages.value.push({ id: 'ai_err_' + Date.now(), role: 'assistant', prompt: promptText, status: 'failed', errorMsg: e.message || '网络错误' })
    generating.value = false
    scrollToBottom()
  }
}

async function pollTaskResult(taskId: string, aiMsgId: string) {
  const maxPoll = 120
  let count = 0
  const doPoll = async () => {
    count++
    try {
      const res: any = await taskApi.get(taskId)
      if (res.code === 0 && res.data?.task) {
        const task = res.data.task
        const msg = messages.value.find(m => m.id === aiMsgId)
        if (!msg) { generating.value = false; return }
        if (task.status === 'completed') {
          const imgUrl = getTaskImageUrl(task)
          let elapsed: string | undefined
          if (task.completed_at && task.created_at) { const t = new Date(task.updated_at).getTime() - new Date(task.created_at).getTime(); if (t > 0) elapsed = (t / 1000).toFixed(1) }
          msg.status = 'completed'; msg.imageUrl = imgUrl; msg.elapsed = elapsed
          generating.value = false
          loadHistory()
          scrollToBottom()
          return
        }
        if (task.status === 'failed') {
          msg.status = 'failed'; msg.errorMsg = task.error_msg || '生成失败'
          generating.value = false
          scrollToBottom()
          return
        }
      }
    } catch {}
    if (count < maxPoll) { pollTimer = setTimeout(doPoll, 3000) }
    else {
      const msg = messages.value.find(m => m.id === aiMsgId)
      if (msg) { msg.status = 'failed'; msg.errorMsg = '轮询超时，请稍后刷新查看' }
      generating.value = false
    }
  }
  pollTimer = setTimeout(doPoll, 3000)
}

function resetAll() {
  garmentImages.value = []
  selectedModelUrl.value = ''
  selectedModelName.value = ''
  customModelUrl.value = ''
  selectedPoseUrl.value = ''
  selectedPoseName.value = ''
  userPrompt.value = ''
  activeModelTab.value = 'female'
  messages.value = []
  activeTaskId.value = null
}

function handleImgError(e: Event) { (e.target as HTMLImageElement).style.background = '#27272a' }
function downloadImage(url?: string) {
  downloadImageByProxy(url)
}

onMounted(() => {
  if (userStore.isLoggedIn) {
    loadHistory()
    loadModelTemplates()
    loadPoseTemplates()
  }
})
onUnmounted(() => { if (pollTimer) clearTimeout(pollTimer) })
</script>

<style scoped>
/* ====== 布局 ====== */
.home-layout { width: 100%; height: 100%; display: flex; flex-direction: column; background: #111113; overflow: hidden; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif; }
.home-body { flex: 1; display: grid; grid-template-columns: 220px 1fr 380px; min-height: 0; overflow: hidden; }

/* ====== 画廊 ====== */
.history-panel { border-right: 1px solid #222225; background: #111113; display: flex; flex-direction: column; overflow: hidden; min-height: 0; }
.history-header { padding: 16px 16px 12px; font-size: 14px; font-weight: 600; color: #d4d4d8; display: flex; align-items: center; gap: 6px; }
.gallery-count { font-size: 11px; background: rgba(163, 230, 53, 0.15); color: #a3e635; padding: 1px 7px; border-radius: 99px; }
.gallery-grid { flex: 1; overflow-y: auto; padding: 0 10px 10px; display: grid; grid-template-columns: 1fr 1fr; gap: 8px; align-content: start; }
.gallery-item { position: relative; border-radius: 8px; overflow: hidden; cursor: pointer; aspect-ratio: 3/4; background: #18181b; border: 1.5px solid transparent; transition: border-color 0.2s; }
.gallery-item.active { border-color: #a3e635; }
.gallery-item img { width: 100%; height: 100%; object-fit: cover; display: block; }
.gallery-item-overlay { position: absolute; inset: 0; background: linear-gradient(transparent 50%, rgba(0,0,0,0.7)); display: flex; align-items: flex-end; padding: 6px; opacity: 0; transition: opacity 0.2s; }
.gallery-item:hover .gallery-item-overlay { opacity: 1; }
.gallery-item-prompt { font-size: 10px; color: #ddd; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.gallery-delete-btn { position: absolute; top: 4px; right: 4px; background: rgba(0,0,0,0.55); border: none; color: #aaa; width: 22px; height: 22px; border-radius: 50%; display: flex; align-items: center; justify-content: center; cursor: pointer; opacity: 0; transition: opacity 0.2s; }
.gallery-item:hover .gallery-delete-btn { opacity: 1; }
.gallery-delete-btn:hover { background: rgba(239,68,68,0.7); color: #fff; }
.history-empty { flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 6px; color: #333; }

/* ====== 中间区 ====== */
.canvas-area { display: flex; flex-direction: column; overflow: hidden; background: #0c0c0f; }
.gen-display { flex: 1; overflow-y: auto; padding: 24px 32px 120px; display: flex; flex-direction: column; gap: 18px; }
.chat-empty { flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 10px; opacity: 0.7; }
.empty-icon-wrap { width: 80px; height: 80px; display: flex; align-items: center; justify-content: center; border-radius: 50%; background: rgba(163,230,53,0.05); }
.chat-empty-text { font-size: 20px; font-weight: 700; color: #e4e4e7; }
.chat-empty-sub { font-size: 13px; color: #52525b; }

/* chat bubbles */
.chat-bubble { display: flex; flex-direction: column; }
.bubble-row { display: flex; flex-direction: column; gap: 4px; }
.bubble-row.user { align-items: flex-end; }
.bubble-row.ai { align-items: flex-start; }
.bubble-body { max-width: 85%; padding: 14px 18px; border-radius: 14px; line-height: 1.6; font-size: 13px; }
.bubble-body.user { background: #1e293b; color: #e2e8f0; border-bottom-right-radius: 4px; }
.bubble-prompt { margin: 0; word-break: break-word; }
.bubble-ref-images { display: flex; gap: 6px; margin-top: 8px; flex-wrap: wrap; }
.bubble-ref-img { width: 60px; height: 80px; border-radius: 6px; object-fit: cover; }
.bubble-time { font-size: 10px; color: #52525b; margin-top: 2px; }
.bubble-body.ai-success { background: #18181b; border: 1px solid #27272a; border-bottom-left-radius: 4px; padding: 0; overflow: hidden; }
.bubble-image-wrap { position: relative; cursor: zoom-in; }
.bubble-image { max-width: 400px; max-height: 500px; display: block; border-radius: 0; }
.image-zoom-hint { position: absolute; bottom: 8px; right: 8px; width: 32px; height: 32px; display: flex; align-items: center; justify-content: center; background: rgba(0,0,0,0.5); border-radius: 50%; color: #fff; opacity: 0; transition: opacity 0.2s; }
.bubble-image-wrap:hover .image-zoom-hint { opacity: 1; }
.bubble-footer { display: flex; align-items: center; justify-content: space-between; padding: 10px 16px; border-top: 1px solid #27272a; }
.bubble-meta { font-size: 11px; color: #52525b; }
.result-actions { display: flex; gap: 6px; }
.result-action-btn { display: flex; align-items: center; gap: 4px; padding: 5px 10px; border: 1px solid #27272a; background: #111113; color: #a1a1aa; font-size: 11px; border-radius: 6px; cursor: pointer; transition: all 0.15s; }
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

/* skeleton */
.skeleton-wrap { width: 340px; height: 200px; background: #18181b; border-radius: 14px; border: 1px solid #27272a; position: relative; overflow: hidden; }
.skeleton-shimmer { position: absolute; inset: 0; background: linear-gradient(90deg, transparent 0%, rgba(255,255,255,0.04) 50%, transparent 100%); animation: shimmer 1.5s infinite; }
@keyframes shimmer { 0% { transform: translateX(-100%); } 100% { transform: translateX(100%); } }
.skeleton-overlay { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 10px; }
.skeleton-spinner { width: 28px; height: 28px; border: 3px solid #27272a; border-top-color: #a3e635; border-radius: 50%; animation: spin 0.8s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
.skeleton-text { color: #a1a1aa; font-size: 13px; font-weight: 500; }
.skeleton-sub { color: #52525b; font-size: 11px; }

/* ====== 右侧面板 ====== */
.control-panel { border-left: 1px solid #222225; background: #111113; display: flex; flex-direction: column; overflow: hidden; }
.panel-top { padding: 16px 16px 0; }
.panel-tabs { display: flex; gap: 4px; }
.panel-tab { padding: 8px 16px; background: none; border: none; color: #71717a; font-size: 13px; font-weight: 500; cursor: pointer; border-radius: 8px 8px 0 0; transition: all 0.2s; }
.panel-tab.active { background: #18181b; color: #e4e4e7; }
.panel-scroll { flex: 1; overflow-y: auto; padding: 16px; display: flex; flex-direction: column; gap: 16px; }
.panel-bottom { padding: 16px; border-top: 1px solid #222225; }

.section { display: flex; flex-direction: column; gap: 8px; }
.section-header-row { display: flex; align-items: center; gap: 6px; }
.section-num { display: flex; align-items: center; justify-content: center; width: 22px; height: 22px; border-radius: 50%; background: rgba(163,230,53,0.12); color: #a3e635; font-size: 11px; font-weight: 700; }
.section-label { font-size: 13px; font-weight: 600; color: #d4d4d8; }

/* 模特 */
.model-tabs { display: flex; gap: 4px; margin-bottom: 4px; }
.model-tab { padding: 5px 12px; border: 1px solid #27272a; background: transparent; color: #71717a; font-size: 11px; border-radius: 6px; cursor: pointer; transition: all 0.15s; }
.model-tab.active { background: rgba(163,230,53,0.12); color: #a3e635; border-color: rgba(163,230,53,0.3); }
.model-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: 8px; max-height: 300px; overflow-y: auto; }
.model-empty { display: flex; flex-direction: column; align-items: center; gap: 4px; padding: 20px 0; font-size: 12px; color: #52525b; }
.model-card { cursor: pointer; border-radius: 8px; overflow: hidden; border: 1.5px solid transparent; transition: all 0.15s; display: flex; flex-direction: column; align-items: center; gap: 4px; }
.model-card:hover { border-color: #444; }
.model-card.active { border-color: #a3e635; }
.model-card-img { position: relative; width: 100%; aspect-ratio: 3/4; overflow: hidden; border-radius: 6px; background: #27272a; }
.model-card-img img { width: 100%; height: 100%; object-fit: cover; display: block; }
.model-card-check { position: absolute; top: 4px; right: 4px; width: 22px; height: 22px; display: flex; align-items: center; justify-content: center; background: #a3e635; border-radius: 50%; color: #111; }
.model-card-name { font-size: 10px; color: #71717a; text-align: center; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 100%; }
.model-card.active .model-card-name { color: #a3e635; }

/* 自定义模特 */
.custom-model-section { display: flex; flex-direction: column; gap: 8px; }
.custom-model-preview { position: relative; width: 100px; height: 130px; border-radius: 8px; overflow: hidden; border: 1.5px solid #a3e635; }
.custom-model-preview img { width: 100%; height: 100%; object-fit: cover; }
.custom-model-remove { position: absolute; top: 4px; right: 4px; width: 22px; height: 22px; border-radius: 50%; background: rgba(0,0,0,0.6); border: none; color: #fff; cursor: pointer; display: flex; align-items: center; justify-content: center; }
.custom-model-dropzone { display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 6px; padding: 24px; border: 1.5px dashed #333; border-radius: 8px; cursor: pointer; color: #555; font-size: 12px; transition: all 0.15s; }
.custom-model-dropzone:hover { border-color: #a3e635; color: #a3e635; }
.custom-model-dropzone.disabled { pointer-events: none; opacity: 0.5; }

/* 姿势 */
.pose-loading, .pose-empty { font-size: 12px; color: #71717a; padding: 12px 0; text-align: center; }
.pose-grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 8px; }
.pose-card { cursor: pointer; border-radius: 8px; overflow: hidden; border: 1.5px solid transparent; transition: all 0.15s; display: flex; flex-direction: column; align-items: center; gap: 4px; }
.pose-card:hover { border-color: #444; }
.pose-card.active { border-color: #a3e635; }
.pose-card-img { position: relative; width: 100%; aspect-ratio: 3/4; overflow: hidden; border-radius: 6px; background: #27272a; }
.pose-card-img img { width: 100%; height: 100%; object-fit: cover; display: block; }
.pose-card-check { position: absolute; top: 4px; right: 4px; width: 22px; height: 22px; display: flex; align-items: center; justify-content: center; background: #a3e635; border-radius: 50%; color: #111; }
.pose-card-name { font-size: 10px; color: #71717a; text-align: center; }
.pose-card.active .pose-card-name { color: #a3e635; }

/* 提示词 */
.prompt-textarea { width: 100%; padding: 10px 12px; border: 1px solid #27272a; border-radius: 8px; background: #18181b; color: #e4e4e7; font-size: 13px; resize: vertical; outline: none; transition: border-color 0.15s; font-family: inherit; }
.prompt-textarea:focus { border-color: #a3e635; }
.prompt-textarea::placeholder { color: #52525b; }

/* 状态 */
.status-section { margin-top: auto; }
.status-ready, .status-waiting { display: flex; align-items: center; gap: 8px; padding: 10px 12px; border-radius: 8px; font-size: 12px; }
.status-ready { background: rgba(163,230,53,0.06); color: #a3e635; }
.status-waiting { background: rgba(113,113,122,0.08); color: #71717a; }

/* 按钮 */
.button-group { display: flex; gap: 8px; }
.btn-generate { flex: 1; padding: 12px; background: linear-gradient(135deg, #a3e635, #65a30d); color: #111; font-weight: 700; font-size: 14px; border: none; border-radius: 10px; cursor: pointer; transition: all 0.2s; }
.btn-generate:hover:not(:disabled) { filter: brightness(1.1); transform: translateY(-1px); }
.btn-generate:disabled { opacity: 0.4; cursor: not-allowed; }
.btn-reset { padding: 12px 18px; background: #18181b; border: 1px solid #27272a; color: #a1a1aa; font-size: 13px; border-radius: 10px; cursor: pointer; transition: all 0.2s; }
.btn-reset:hover { background: #27272a; color: #e4e4e7; }

/* 灯箱 */
.lightbox-overlay { position: fixed; inset: 0; z-index: 9999; background: rgba(0,0,0,0.85); display: flex; align-items: center; justify-content: center; cursor: zoom-out; }
.lightbox-img { max-width: 90vw; max-height: 90vh; object-fit: contain; border-radius: 8px; }
.lightbox-close { position: absolute; top: 16px; right: 24px; background: none; border: none; color: #fff; font-size: 36px; cursor: pointer; }

/* 删除确认 */
.delete-popover { position: fixed; z-index: 10000; background: #1e1e22; border: 1px solid #333; border-radius: 10px; padding: 14px 16px; box-shadow: 0 8px 30px rgba(0,0,0,0.4); }
.dp-btn { padding: 5px 14px; border-radius: 6px; font-size: 12px; cursor: pointer; border: 1px solid #333; }
.dp-btn.cancel { background: #27272a; color: #a1a1aa; }
.dp-btn.danger { background: #ef4444; color: #fff; border-color: #ef4444; }

/* 游客 */
.guest-prompt { flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; text-align: center; color: #e4e4e7; gap: 8px; }
.smiley-icon { margin-bottom: 12px; }
.guest-title { font-size: 18px; font-weight: 700; color: #a3e635; }
.guest-desc { font-size: 14px; color: #a1a1aa; }
.guest-action { font-size: 14px; color: #a1a1aa; }
.link-highlight { color: #a3e635; text-decoration: underline; }

.highlight { animation: highlightPulse 2s ease-out; }
@keyframes highlightPulse { 0%, 100% { box-shadow: none; } 30% { box-shadow: 0 0 0 2px rgba(163,230,53,0.4); } }
</style>
