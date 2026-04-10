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
            <p class="chat-empty-text">模特多角度</p>
            <p class="chat-empty-sub">上传图片，选择角度、动作、表情和场景后生成</p>
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
          <div class="panel-tabs"><button class="panel-tab active">模特多角度</button></div>
        </div>
        <div class="panel-scroll">

          <!-- ① 上传参考图 -->
          <div class="section">
            <div class="section-header-row"><span class="section-num">①</span><span class="section-label">上传参考图片</span><span class="section-hint">最多 3 张</span></div>
            <ImageUploader v-model="refImages" :gallery-groups="allGalleryGroups" :max="3" :max-size-m-b="20" />
          </div>

          <!-- ② 模特角度 -->
          <div class="section" v-if="refImageUrls.length > 0">
            <div class="section-header-row">
              <span class="section-num">②</span><span class="section-label">模特角度</span>
              <span class="section-counter">已选 {{ selectedAngles.length }}/4</span>
            </div>
            <div class="card-grid cols-4">
              <div v-for="item in angleOptions" :key="item.key" class="option-card" :class="{ active: selectedAngles.includes(item.key), disabled: selectedAngles.length >= 4 && !selectedAngles.includes(item.key) }" @click="toggleAngle(item.key)">
                <div class="option-card-icon">{{ item.icon }}</div>
                <span class="option-card-label">{{ item.label }}</span>
                <div v-if="selectedAngles.includes(item.key)" class="option-card-check">
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3"><polyline points="20 6 9 17 4 12"/></svg>
                </div>
              </div>
            </div>
          </div>

          <!-- ③ 模特动作 -->
          <div class="section" v-if="refImageUrls.length > 0">
            <div class="section-header-row"><span class="section-num">③</span><span class="section-label">模特动作</span></div>
            <div class="card-grid cols-4">
              <div v-for="item in poseOptions" :key="item.key" class="option-card" :class="{ active: selectedPose === item.key }" @click="selectedPose = item.key">
                <div class="option-card-icon">{{ item.icon }}</div>
                <span class="option-card-label">{{ item.label }}</span>
              </div>
            </div>
          </div>

          <!-- ④ 面部表情 -->
          <div class="section" v-if="refImageUrls.length > 0">
            <div class="section-header-row"><span class="section-num">④</span><span class="section-label">面部表情</span></div>
            <div class="card-grid cols-4">
              <div v-for="item in expressionOptions" :key="item.key" class="option-card compact" :class="{ active: selectedExpression === item.key }" @click="selectedExpression = item.key">
                <div class="option-card-emoji">{{ item.icon }}</div>
                <span class="option-card-label">{{ item.label }}</span>
              </div>
            </div>
          </div>

          <!-- ⑤ 环境背景 -->
          <div class="section" v-if="refImageUrls.length > 0">
            <div class="section-header-row"><span class="section-num">⑤</span><span class="section-label">环境背景</span></div>
            <div class="card-grid cols-2">
              <div v-for="item in bgOptions" :key="item.key" class="option-card wide" :class="{ active: selectedBg === item.key }" @click="selectedBg = item.key">
                <div class="option-card-icon">{{ item.icon }}</div>
                <div class="option-card-text">
                  <span class="option-card-label">{{ item.label }}</span>
                  <span class="option-card-desc">{{ item.desc }}</span>
                </div>
              </div>
            </div>
          </div>

          <!-- 状态提示 -->
          <div class="section status-section">
            <div v-if="canGenerate" class="status-ready">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#a3e635" stroke-width="2"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>
              <span>已就绪，可以开始生成</span>
            </div>
            <div v-else class="status-waiting">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#71717a" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>
              <span v-if="refImageUrls.length === 0">请上传参考图片</span>
              <span v-else-if="selectedAngles.length === 0">请选择至少一个角度</span>
              <span v-else>请完善参数设置</span>
            </div>
          </div>
        </div>

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

    <!-- 删除确认 -->
    <div v-if="deleteConfirm.show" class="delete-popover" :style="{ left: deleteConfirm.x+'px', top: deleteConfirm.y+'px' }">
      <p>确认删除？</p>
      <div class="delete-popover-btns"><button @click="deleteConfirm.show=false">取消</button><button class="confirm" @click="confirmDelete">删除</button></div>
    </div>

    <!-- 图片放大 -->
    <div v-if="lightboxUrl" class="lightbox-overlay" @click.self="lightboxUrl=''">
      <div class="lightbox-container">
        <img :src="lightboxUrl" class="lightbox-image" />
        <button class="lightbox-close" @click="lightboxUrl=''"><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg></button>
        <div class="lightbox-actions"><button class="lightbox-action-btn" @click="downloadImage(lightboxUrl)"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>下载原图</button></div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, nextTick, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { useMessage } from 'naive-ui'
import { useUserStore } from '../../stores/user'
import { taskApi } from '../../api'
import ImageUploader from '../../components/ImageUploader.vue'
import { buildGalleryGroups } from '../../utils/galleryGroups'
import { downloadImageByProxy, getTaskImageUrl as extractTaskImageUrl, stripFeaturePrefix } from '../../utils/task-history'
import type { Task } from '../../api'

const FEATURE_SLUG = 'multi-angle'
const FEATURE_TITLE = '模特多角度'
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

// ============ 图片上传 ============
const refImageUrls = ref<string[]>([])
const refImages = computed({
  get: () => refImageUrls.value,
  set: (val: string[]) => { refImageUrls.value = val }
})
const allGalleryGroups = computed(() => buildGalleryGroups(historyTasks.value))

// ============ 角度选择（多选，最多4个） ============
const angleOptions = [
  { key: 'front_full',      label: '正面全身', icon: '🧍' },
  { key: 'front_left_45',   label: '前左 45°', icon: '↖' },
  { key: 'front_right_45',  label: '前右 45°', icon: '↗' },
  { key: 'left_90',         label: '左侧 90°', icon: '←' },
  { key: 'right_90',        label: '右侧 90°', icon: '→' },
  { key: 'back_left_45',    label: '后左 45°', icon: '↙' },
  { key: 'back_right_45',   label: '后右 45°', icon: '↘' },
  { key: 'back_full',       label: '背面全身', icon: '🔄' },
  { key: 'front_half',      label: '正面半身', icon: '👤' },
  { key: 'side_half',       label: '侧面半身', icon: '👥' },
  { key: 'portrait',        label: '肖像特写', icon: '📷' },
  { key: 'side_portrait',   label: '侧颜特写', icon: '🎭' },
]
const selectedAngles = ref<string[]>([])

function toggleAngle(key: string) {
  const idx = selectedAngles.value.indexOf(key)
  if (idx >= 0) {
    selectedAngles.value.splice(idx, 1)
  } else if (selectedAngles.value.length < 4) {
    selectedAngles.value.push(key)
  }
}

function getAngleLabel(key: string): string {
  return angleOptions.find(o => o.key === key)?.label || key
}

// ============ 动作选择（单选） ============
const poseOptions = [
  { key: 'runway_stand',     label: '标准T台站姿', icon: '🚶' },
  { key: 'hands_on_waist',   label: '叉腰自信',   icon: '💪' },
  { key: 'walking',          label: '动感漫步',   icon: '🚶‍♀️' },
  { key: 'lean_casual',      label: '随性斜靠',   icon: '😎' },
  { key: 's_curve',          label: 'S型曲线',    icon: '💃' },
  { key: 'power_walk',       label: '气场步伐',   icon: '👠' },
  { key: 'sporty',           label: '活力运动',   icon: '🏃' },
  { key: 'vogue',            label: 'VOGUE 律动', icon: '✨' },
  { key: 'elegant_sit',      label: '优雅坐姿',   icon: '🪑' },
  { key: 'cross_legs',       label: '交叉修长',   icon: '🦵' },
  { key: 'hands_in_pocket',  label: '双手插兜',   icon: '🧥' },
  { key: 'arms_crossed',     label: '抱臂气场',   icon: '🤞' },
]
const selectedPose = ref('runway_stand')

function getPoseLabel(key: string): string {
  return poseOptions.find(o => o.key === key)?.label || key
}

// ============ 表情选择（单选） ============
const expressionOptions = [
  { key: 'cool',    label: '清冷高级', icon: '❄️' },
  { key: 'smile',   label: '自信微笑', icon: '😊' },
  { key: 'fierce',  label: '犀利厌世', icon: '😤' },
  { key: 'soft',    label: '温柔空灵', icon: '🌸' },
]
const selectedExpression = ref('cool')

function getExpressionLabel(key: string): string {
  return expressionOptions.find(o => o.key === key)?.label || key
}

// ============ 环境背景选择（单选） ============
const bgOptions = [
  { key: 'original', label: '原图背景', desc: '保留环境', icon: '🖼️' },
  { key: 'studio',   label: '影棚白底', desc: '纯白干净', icon: '⬜' },
]
const selectedBg = ref('original')

function getBgLabel(key: string): string {
  return bgOptions.find(o => o.key === key)?.label || key
}

// ============ 消息气泡 ============
interface ChatMessage { id: string; role: 'user' | 'assistant'; prompt?: string; refImageUrls?: string[]; status?: 'pending' | 'completed' | 'failed'; imageUrl?: string; errorMsg?: string; taskId?: string; elapsed?: string; time?: string }
const messages = ref<ChatMessage[]>([])
const chatMessagesRef = ref<HTMLElement | null>(null)
function scrollToBottom() { nextTick(() => { if (chatMessagesRef.value) chatMessagesRef.value.scrollTop = chatMessagesRef.value.scrollHeight }) }

// ============ 生成 ============
const generating = ref(false)
const currentTaskId = ref<string | null>(null)
const lightboxUrl = ref('')

const canGenerate = computed(() => {
  if (!userStore.isLoggedIn) return false
  if (generating.value) return false
  if (refImageUrls.value.length === 0) return false
  if (selectedAngles.value.length === 0) return false
  return true
})

async function handleGenerate() {
  if (!userStore.isLoggedIn) { message.warning('请先登录'); router.push('/user/login'); return }
  if (refImageUrls.value.length === 0) { message.warning('请上传参考图片'); return }
  if (selectedAngles.value.length === 0) { message.warning('请选择至少一个角度'); return }

  activeTaskId.value = null
  const now = new Date()
  const timeStr = `${now.getHours().toString().padStart(2, '0')}:${now.getMinutes().toString().padStart(2, '0')}`

  const angleLabels = selectedAngles.value.map(k => getAngleLabel(k)).join('、')
  const poseLabel = getPoseLabel(selectedPose.value)
  const exprLabel = getExpressionLabel(selectedExpression.value)
  const bgLabel = getBgLabel(selectedBg.value)
  const displayPrompt = `角度：${angleLabels}，动作：${poseLabel}，表情：${exprLabel}，背景：${bgLabel}`

  const userMsg: ChatMessage = {
    id: 'msg_' + Date.now(), role: 'user',
    prompt: displayPrompt,
    refImageUrls: [...refImageUrls.value],
    time: timeStr,
  }
  messages.value.push(userMsg)

  const assistantMsg: ChatMessage = { id: 'loading_' + Date.now(), role: 'assistant', prompt: displayPrompt, status: 'pending' }
  messages.value.push(assistantMsg)
  const assistantRef = messages.value[messages.value.length - 1]
  scrollToBottom()

  generating.value = true
  currentTaskId.value = null

  try {
    const res: any = await taskApi.submit({
      prompt: FEATURE_PREFIX + displayPrompt,
      params: {
        image_urls: [...refImageUrls.value],
        feature: FEATURE_SLUG,
        angles: selectedAngles.value.join(','),
        pose: selectedPose.value,
        expression: selectedExpression.value,
        background: selectedBg.value,
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
  refImageUrls.value = []
  selectedAngles.value = []
  selectedPose.value = 'runway_stand'
  selectedExpression.value = 'cool'
  selectedBg.value = 'original'
  messages.value = []
  activeTaskId.value = null
}

function handleImgError(e: Event) { (e.target as HTMLImageElement).style.background = '#27272a' }
function downloadImage(url?: string) {
  downloadImageByProxy(url)
}

onMounted(() => { if (userStore.isLoggedIn) loadHistory() })
onUnmounted(() => { if (pollTimer) clearTimeout(pollTimer) })
</script>

<style scoped>
/* ====== 布局 ====== */
.home-layout { width: 100%; height: 100%; display: flex; flex-direction: column; background: #111113; overflow: hidden; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif; }
.home-body { flex: 1; display: grid; grid-template-columns: 220px 1fr 380px; min-height: 0; overflow: hidden; }

/* ====== 画廊 ====== */
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

/* ====== 中间生成区 ====== */
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

/* ====== 气泡 ====== */
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
.bubble-ref-images { display: flex; gap: 8px; margin-top: 8px; flex-wrap: wrap; }
.bubble-ref-img { width: 80px; height: 80px; object-fit: cover; border-radius: 8px; border: 1px solid #333; }
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

/* ====== 骨架 ====== */
.skeleton-wrap { width: 240px; aspect-ratio: 3 / 4; border-radius: 12px; background: #1a1a1e; border: 1px solid #27272a; position: relative; overflow: hidden; }
.skeleton-shimmer { position: absolute; inset: 0; background: linear-gradient(110deg, transparent 30%, rgba(163,230,53,0.04) 50%, transparent 70%); background-size: 300% 100%; animation: shimmer 2s ease-in-out infinite; }
@keyframes shimmer { 0% { background-position: 200% 0; } 100% { background-position: -100% 0; } }
.skeleton-overlay { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 12px; }
.skeleton-spinner { width: 32px; height: 32px; border: 3px solid #27272a; border-top-color: #a3e635; border-radius: 50%; animation: spin 1s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
.skeleton-text { font-size: 13px; color: #888; font-weight: 500; }
.skeleton-sub { font-size: 11px; color: #555; }

/* ====== 右侧面板 ====== */
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
.section-hint { font-size: 11px; color: #555; margin-left: auto; }
.section-counter { font-size: 11px; color: #a3e635; margin-left: auto; font-weight: 600; font-variant-numeric: tabular-nums; }

/* ====== 卡片网格 ====== */
.card-grid { display: grid; gap: 8px; }
.card-grid.cols-4 { grid-template-columns: repeat(4, 1fr); }
.card-grid.cols-2 { grid-template-columns: repeat(2, 1fr); }

.option-card {
  position: relative;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 12px 4px 10px;
  border-radius: 10px;
  border: 1.5px solid #27272a;
  background: #1a1a1e;
  cursor: pointer;
  transition: all 0.2s;
  user-select: none;
}
.option-card:hover { border-color: #444; background: #222226; }
.option-card.active { border-color: #a3e635; background: rgba(163,230,53,0.06); }
.option-card.disabled { opacity: 0.35; cursor: not-allowed; pointer-events: none; }
.option-card-icon { font-size: 22px; line-height: 1; }
.option-card-emoji { font-size: 26px; line-height: 1; }
.option-card-label { font-size: 11px; color: #a1a1aa; text-align: center; line-height: 1.3; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 100%; }
.option-card.active .option-card-label { color: #a3e635; font-weight: 600; }
.option-card-check { position: absolute; top: 4px; right: 4px; width: 18px; height: 18px; border-radius: 50%; background: #a3e635; display: flex; align-items: center; justify-content: center; color: #111; }

/* 宽卡片 */
.option-card.wide {
  flex-direction: row;
  justify-content: flex-start;
  gap: 12px;
  padding: 14px 16px;
}
.option-card.wide .option-card-icon { font-size: 28px; flex-shrink: 0; }
.option-card-text { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
.option-card-desc { font-size: 10px; color: #555; }
.option-card.active .option-card-desc { color: rgba(163,230,53,0.6); }

/* 紧凑卡片 */
.option-card.compact { padding: 10px 4px 8px; }

/* ====== 状态 ====== */
.status-section { padding: 12px 14px; border-radius: 10px; border: 1px solid #27272a; background: #1a1a1e; }
.status-ready, .status-waiting { display: flex; align-items: center; gap: 10px; font-size: 12px; }
.status-ready { color: #a3e635; }
.status-waiting { color: #71717a; }

/* ====== 底部 ====== */
.panel-bottom { padding: 16px; border-top: 1px solid #27272a; flex-shrink: 0; }
.button-group { display: flex; gap: 10px; }
.btn-generate { flex: 1; padding: 12px 24px; border-radius: 10px; border: none; background: linear-gradient(135deg, #a3e635 0%, #65a30d 100%); color: #111; font-size: 14px; font-weight: 600; cursor: pointer; transition: all 0.2s; }
.btn-generate:hover:not(:disabled) { opacity: 0.9; transform: translateY(-1px); }
.btn-generate:disabled { opacity: 0.4; cursor: not-allowed; }
.btn-clear { padding: 12px 24px; border-radius: 10px; border: 1px solid #333; background: #27272a; color: #aaa; font-size: 14px; cursor: pointer; }
.btn-clear:hover:not(:disabled) { background: #3f3f46; }

/* ====== 弹层 ====== */
.delete-popover { position: fixed; z-index: 1000; background: #27272a; border: 1px solid #3f3f46; border-radius: 10px; padding: 12px 16px; box-shadow: 0 8px 24px rgba(0,0,0,0.4); transform: translate(-50%, -100%); margin-top: -8px; }
.delete-popover p { margin: 0 0 8px; font-size: 13px; color: #e4e4e7; }
.delete-popover-btns { display: flex; gap: 8px; justify-content: flex-end; }
.delete-popover-btns button { padding: 4px 14px; border-radius: 6px; border: 1px solid #3f3f46; background: #18181b; color: #a1a1aa; font-size: 12px; cursor: pointer; }
.delete-popover-btns button.confirm { background: rgba(220,38,38,0.2); border-color: rgba(220,38,38,0.4); color: #ef4444; }

/* ====== lightbox ====== */
.lightbox-overlay { position: fixed; inset: 0; z-index: 999; background: rgba(0,0,0,0.85); display: flex; align-items: center; justify-content: center; }
.lightbox-container { position: relative; max-width: 90vw; max-height: 90vh; }
.lightbox-image { max-width: 100%; max-height: 85vh; border-radius: 8px; }
.lightbox-close { position: absolute; top: -10px; right: -10px; width: 32px; height: 32px; border-radius: 50%; background: #333; border: none; color: #ccc; cursor: pointer; display: flex; align-items: center; justify-content: center; }
.lightbox-actions { display: flex; gap: 8px; justify-content: center; margin-top: 12px; }
.lightbox-action-btn { padding: 8px 16px; border-radius: 8px; border: 1px solid #444; background: #27272a; color: #ccc; font-size: 13px; cursor: pointer; display: flex; align-items: center; gap: 6px; }
</style>
