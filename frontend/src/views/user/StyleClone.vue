<template>
  <div class="home-layout">
    <div class="home-body">
      <!-- 左侧画廊 -->
      <aside class="history-panel">
        <div class="history-header"><span>我的画廊</span><span class="gallery-count" v-if="galleryTasks.length">{{ galleryTasks.length }}</span></div>
        <div class="gallery-grid" v-if="galleryTasks.length > 0">
          <div v-for="ht in galleryTasks" :key="ht.task_id" class="gallery-item" :class="{ active: activeTaskId === ht.task_id }" @click="viewHistoryTask(ht)">
            <img :src="getTaskImageUrl(ht)" alt="" loading="lazy" @error="handleImgError" />
            <div class="gallery-item-overlay"><span class="gallery-item-prompt">{{ stripPrefix(ht.prompt) }}</span></div>
            <button class="gallery-delete-btn" @click.stop="showDeleteConfirm($event, ht.task_id)" title="删除"><svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg></button>
          </div>
        </div>
        <div class="history-empty" v-else><svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="#333" stroke-width="1.5"><rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="8.5" cy="8.5" r="1.5"/><path d="m21 15-5-5L5 21"/></svg><p>还没有作品</p></div>
      </aside>

      <!-- 中间生成区 -->
      <main class="canvas-area">
        <div v-if="!userStore.isLoggedIn" class="guest-prompt">
          <p class="guest-title">温馨提示：</p>
          <p class="guest-desc">您当前是游客身份，暂无工具相关权限。</p>
          <p class="guest-action">请先 <router-link to="/user/register" class="link-highlight">注册</router-link> 或 <router-link to="/user/login" class="link-highlight">登录</router-link></p>
        </div>
        <div v-else class="gen-display" ref="chatMessagesRef">
          <div v-if="messages.length === 0" class="chat-empty">
            <div class="empty-icon-wrap"><svg width="64" height="64" viewBox="0 0 24 24" fill="none" stroke="#a3e635" stroke-width="1.2"><path d="M12 2L2 7l10 5 10-5-10-5z"/><path d="M2 17l10 5 10-5"/><path d="M2 12l10 5 10-5"/></svg></div>
            <p class="chat-empty-text">画风复刻</p>
            <p class="chat-empty-sub">上传原图，选择图案风格，输入描述，一键生成复刻效果</p>
          </div>
          <div v-for="msg in messages" :key="msg.id" :id="msg.taskId ? 'task-' + msg.taskId : undefined" class="chat-bubble" :class="msg.role">
            <div v-if="msg.role === 'user'" class="bubble-row user">
              <div class="bubble-body user"><p class="bubble-prompt">{{ msg.prompt }}</p>
                <div class="bubble-ref-images" v-if="msg.refImageUrls && msg.refImageUrls.length"><img v-for="(url, i) in msg.refImageUrls" :key="i" :src="url" class="bubble-ref-img" /></div>
              </div><span class="bubble-time" v-if="msg.time">{{ msg.time }}</span>
            </div>
            <div v-else class="bubble-row ai">
              <div v-if="msg.status === 'pending'" class="skeleton-wrap"><div class="skeleton-shimmer"></div><div class="skeleton-overlay"><div class="skeleton-spinner"></div><p class="skeleton-text">AI 正在生成中...</p></div></div>
              <div v-else-if="msg.status === 'failed'" class="bubble-body failed"><div class="bubble-failed-row"><div class="bubble-failed-icon">✕</div><div><div class="bubble-failed-title">生成失败</div><div class="bubble-failed-reason">{{ msg.errorMsg }}</div></div></div></div>
              <template v-else-if="msg.status === 'completed'">
                <div class="bubble-body ai-success">
                  <div v-if="msg.imageUrl" class="bubble-image-wrap" @click="lightboxUrl = msg.imageUrl!"><img :src="msg.imageUrl" class="bubble-image" @error="handleImgError" /><div class="image-zoom-hint"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg></div></div>
                  <div class="bubble-footer"><span class="bubble-meta" v-if="msg.elapsed">耗时 {{ msg.elapsed }}s</span>
                    <div class="result-actions">
                      <button class="result-action-btn" @click="downloadImage(msg.imageUrl)">下载</button>
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

      <!-- 右侧创作面板 -->
      <aside class="control-panel">
        <div class="panel-top"><div class="panel-tabs"><button class="panel-tab active">画风复刻</button></div></div>
        <div class="panel-scroll">

          <!-- ① 上传原图（必传） -->
          <div class="section-block">
            <div class="section-header"><span class="section-num">①</span><span class="section-title">上传原图</span></div>
            <ImageUploader v-model="sourceImages" :gallery-groups="allGalleryGroups" :max="1" :max-size-m-b="20" />
          </div>

          <!-- ② 选择图案风格 -->
          <div class="section-block" v-if="sourceImageUrl">
            <div class="section-header"><span class="section-num">②</span><span class="section-title">选择图案风格</span></div>
            <div v-if="stylesLoading" class="loading-state"><span style="color:#71717a;font-size:12px;">加载中...</span></div>
            <div v-else-if="patternStyles.length === 0" class="empty-state"><p>暂无图案风格，请联系管理员添加</p></div>
            <div v-else class="style-grid">
              <div v-for="s in patternStyles" :key="s.id" class="style-card" :class="{ selected: selectedStyleId === s.id }" @click="selectStyle(s)">
                <div class="style-card-cover"><img :src="s.image_url" :alt="s.name" loading="lazy" @error="handleImgError" /><div v-if="selectedStyleId === s.id" class="card-check"><svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="white" stroke-width="3"><polyline points="20 6 9 17 4 12"/></svg></div></div>
                <div class="style-card-name">{{ s.name }}</div>
              </div>
            </div>
          </div>

          <!-- ③ 上传参考图（可选） -->
          <div class="section-block" v-if="selectedStyleId">
            <div class="section-header"><span class="section-num">③</span><span class="section-title">上传参考图（可选）</span></div>
            <ImageUploader v-model="refImages" :gallery-groups="allGalleryGroups" :max="1" :max-size-m-b="20" />
          </div>

          <!-- ④ 画面描述 -->
          <div class="section-block" v-if="selectedStyleId">
            <div class="section-header"><span class="section-num">④</span><span class="section-title">画面描述</span></div>
            <textarea v-model="userPrompt" class="prompt-textarea" rows="3" maxlength="500" placeholder="描述你想要的复刻效果，如：保留原图构图，转为水彩画风格..."></textarea>
          </div>

          <!-- ⑤ 图片尺寸 -->
          <div class="section-block" v-if="selectedStyleId">
            <div class="section-header"><span class="section-num">⑤</span><span class="section-title">图片尺寸</span></div>
            <div class="ratio-grid">
              <button v-for="r in ratios" :key="r.value" class="ratio-btn" :class="{ active: selectedRatio === r.value }" @click="selectedRatio = r.value"><div class="ratio-icon" :style="r.iconStyle"></div><span>{{ r.label }}</span></button>
            </div>
          </div>

          <!-- ⑥ 出图张数 -->
          <div class="section-block" v-if="selectedStyleId">
            <div class="section-header"><span class="section-num">⑥</span><span class="section-title">出图张数</span></div>
            <div class="count-grid"><button v-for="n in [1, 2, 4]" :key="n" class="count-btn" :class="{ active: imageCount === n }" @click="imageCount = n">{{ n }} 张</button></div>
          </div>

          <!-- ⑦ 图片清晰度 -->
          <div class="section-block" v-if="selectedStyleId">
            <div class="section-header"><span class="section-num">⑦</span><span class="section-title">图片清晰度</span></div>
            <div class="quality-grid">
              <button v-for="q in qualityOptions" :key="q.value" class="quality-btn" :class="{ active: selectedQuality === q.value }" @click="selectedQuality = q.value"><span class="quality-label">{{ q.label }}</span><span class="quality-desc">{{ q.desc }}</span></button>
            </div>
          </div>

          <!-- 状态提示 -->
          <div class="section-block status-section">
            <div v-if="canGenerate" class="status-ready">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#a3e635" stroke-width="2"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>
              <span>已就绪，可以开始生成</span>
            </div>
            <div v-else class="status-waiting">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#71717a" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>
              <span v-if="!sourceImageUrl">请上传原图</span>
              <span v-else-if="!selectedStyleId">请选择图案风格</span>
              <span v-else-if="!userPrompt.trim()">请输入画面描述</span>
              <span v-else>准备就绪</span>
            </div>
          </div>
        </div>

        <div class="panel-bottom"><div class="button-group">
          <button class="btn-generate" :disabled="!canGenerate || generating" @click="handleGenerate">{{ generating ? '生成中...' : '生成图片' }}</button>
          <button class="btn-reset" @click="resetAll">重置</button>
        </div></div>
      </aside>
    </div>
    <Teleport to="body"><div v-if="lightboxUrl" class="lightbox-overlay" @click.self="lightboxUrl = ''"><img :src="lightboxUrl" class="lightbox-img" /><button class="lightbox-close" @click="lightboxUrl = ''">&times;</button></div></Teleport>
    <Teleport to="body"><div v-if="deleteConfirm.show" class="delete-popover" :style="{ left: deleteConfirm.x + 'px', top: deleteConfirm.y + 'px' }"><p style="margin:0 0 10px;color:#e4e4e7;">确定删除？</p><div style="display:flex;gap:8px;justify-content:flex-end;"><button class="dp-btn cancel" @click="deleteConfirm.show = false">取消</button><button class="dp-btn danger" @click="confirmDelete">删除</button></div></div></Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue'
import { useMessage } from 'naive-ui'
import { useUserStore } from '../../stores/user'
import { taskApi, patternStyleApi } from '../../api'
import ImageUploader from '../../components/ImageUploader.vue'
import { buildGalleryGroups } from '../../utils/galleryGroups'
import { downloadImageByProxy, getTaskImageUrl as extractTaskImageUrl, stripFeaturePrefix } from '../../utils/task-history'
import type { Task } from '../../api'

const FEATURE_SLUG = 'style-clone'
const FEATURE_PREFIX = `[${FEATURE_SLUG}] `

const message = useMessage()
const userStore = useUserStore()
function stripPrefix(str?: string) {
  return stripFeaturePrefix(str)
}

// ============ ① 上传原图 ============
const sourceImages = ref<string[]>([])
const sourceImageUrl = computed(() => sourceImages.value[0] || '')
const allGalleryGroups = computed(() => buildGalleryGroups(historyTasks.value))

// ============ ② 选择图案风格 ============
interface StyleItem { id: number; name: string; image_url: string; category: string }
const patternStyles = ref<StyleItem[]>([])
const stylesLoading = ref(false)
const selectedStyleId = ref<number | null>(null)
const selectedStyleName = ref('')
async function loadStyles() { stylesLoading.value = true; try { const r: any = await patternStyleApi.listEnabled(FEATURE_SLUG); if (r.code === 0) patternStyles.value = r.data?.list || [] } catch {}; stylesLoading.value = false }
function selectStyle(s: StyleItem) { selectedStyleId.value = s.id; selectedStyleName.value = s.name }

// ============ ③ 参考图（可选） ============
const refImages = ref<string[]>([])

// ============ ④ 描述 ============
const userPrompt = ref('')

// ============ ⑤ 尺寸 ============
const ratios = [
  { label: '1:1', value: '1:1', iconStyle: { width: '18px', height: '18px' } },
  { label: '3:4', value: '3:4', iconStyle: { width: '16px', height: '22px' } },
  { label: '4:3', value: '4:3', iconStyle: { width: '22px', height: '16px' } },
  { label: '2:3', value: '2:3', iconStyle: { width: '16px', height: '24px' } },
  { label: '3:2', value: '3:2', iconStyle: { width: '24px', height: '16px' } },
]
const selectedRatio = ref('1:1')

// ============ ⑥ 张数 ============
const imageCount = ref(1)

// ============ ⑦ 清晰度 ============
const qualityOptions = [{ label: '1K', value: '1k', desc: '1024px' }, { label: '2K', value: '2k', desc: '2048px' }, { label: '4K', value: '4k', desc: '4096px' }]
const selectedQuality = ref('1k')

// ============ 画廊 ============
const historyTasks = ref<Task[]>([])
const activeTaskId = ref<string | null>(null)
const galleryTasks = computed(() => historyTasks.value.filter(t => { if (t.status !== 'completed') return false; if (!t.prompt || !t.prompt.startsWith(FEATURE_PREFIX)) return false; try { return !!JSON.parse(t.result || '{}').image_url } catch { return false } }))
function getTaskImageUrl(task: Task): string {
  return extractTaskImageUrl(task)
}
async function loadHistory() { if (!userStore.isLoggedIn) return; try { const r: any = await taskApi.list({ page: 1, page_size: 200 }); if (r.code === 0) { historyTasks.value = r.data.list || []; if (messages.value.length === 0) rebuildMessages() } } catch {} }
function rebuildMessages() {
  const tasks = historyTasks.value.filter(t => t.prompt && t.prompt.startsWith(FEATURE_PREFIX)).sort((a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime()).slice(-20)
  for (const t of tasks) {
    const img = getTaskImageUrl(t); const time = t.created_at ? new Date(t.created_at).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' }) : ''
    messages.value.push({ id: 'hu_' + t.task_id, role: 'user', prompt: stripPrefix(t.prompt) || '画风复刻', time })
    if (t.status === 'completed' && img) { let el: string | undefined; if (t.completed_at && t.created_at) { const d = new Date(t.updated_at).getTime() - new Date(t.created_at).getTime(); if (d > 0) el = (d / 1000).toFixed(1) }; messages.value.push({ id: 'ha_' + t.task_id, role: 'assistant', prompt: t.prompt || '', status: 'completed', imageUrl: img, taskId: t.task_id, elapsed: el }) }
    else if (t.status === 'failed') messages.value.push({ id: 'ha_' + t.task_id, role: 'assistant', prompt: t.prompt || '', status: 'failed', errorMsg: t.error_msg || '生成失败', taskId: t.task_id })
  }
  if (messages.value.length > 0) scrollToBottom()
}
function viewHistoryTask(t: Task) { activeTaskId.value = t.task_id; const el = document.getElementById('task-' + t.task_id); if (el) { el.scrollIntoView({ behavior: 'smooth', block: 'center' }); el.classList.add('highlight'); setTimeout(() => el.classList.remove('highlight'), 2000) } }

// ============ 删除 ============
const deleteConfirm = ref<{ show: boolean; x: number; y: number; taskId: string }>({ show: false, x: 0, y: 0, taskId: '' })
function showDeleteConfirm(e: MouseEvent, taskId?: string) { if (!taskId) return; deleteConfirm.value = { show: true, x: e.clientX, y: e.clientY, taskId } }
async function confirmDelete() { const id = deleteConfirm.value.taskId; deleteConfirm.value.show = false; if (!id) return; try { const r: any = await taskApi.delete(id); if (r.code === 0) { historyTasks.value = historyTasks.value.filter(t => t.task_id !== id); messages.value = messages.value.filter(m => m.id !== 'hu_' + id && m.id !== 'ha_' + id && m.taskId !== id) } } catch {} }

// ============ 生成 ============
interface ChatMessage { id: string; role: 'user' | 'assistant'; prompt: string; time?: string; status?: 'pending' | 'completed' | 'failed'; imageUrl?: string; taskId?: string; elapsed?: string; errorMsg?: string; refImageUrls?: string[] }
const messages = ref<ChatMessage[]>([])
const chatMessagesRef = ref<HTMLElement | null>(null)
const lightboxUrl = ref('')
const generating = ref(false)
function scrollToBottom() { nextTick(() => { if (chatMessagesRef.value) chatMessagesRef.value.scrollTop = chatMessagesRef.value.scrollHeight }) }
const canGenerate = computed(() => userStore.isLoggedIn && !generating.value && !!sourceImageUrl.value && !!selectedStyleId.value && !!userPrompt.value.trim())

async function handleGenerate() {
  if (!canGenerate.value) return
  generating.value = true; activeTaskId.value = null
  const prompt = userPrompt.value.trim()
  const time = new Date().toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
  // 收集所有图片引用：原图 + 可选参考图
  const allImages = [sourceImageUrl.value, ...refImages.value]
  messages.value.push({ id: 'u_' + Date.now(), role: 'user', prompt, time, refImageUrls: [...allImages] })
  const aiRaw: ChatMessage = { id: 'a_' + Date.now(), role: 'assistant', prompt, status: 'pending' }
  messages.value.push(aiRaw); const aMsg = messages.value[messages.value.length - 1]; scrollToBottom()
  try {
    const res: any = await taskApi.submit({
      prompt,
      feature: FEATURE_SLUG,
      image_urls: allImages,
      params: {
        ratio: selectedRatio.value,
        count: imageCount.value,
        quality: selectedQuality.value,
        style_model: selectedStyleName.value,
        feature: FEATURE_SLUG,
      }
    })
    if (res.code === 0) { aMsg.taskId = res.data.task_id; pollTask(res.data.task_id, aMsg) }
    else { aMsg.status = 'failed'; aMsg.errorMsg = res.msg || '提交失败'; generating.value = false }
  } catch (e: any) { aMsg.status = 'failed'; aMsg.errorMsg = e.msg || '网络错误'; generating.value = false }
  scrollToBottom()
}

let pollTimer: ReturnType<typeof setTimeout> | null = null
async function pollTask(taskId: string, aMsg: ChatMessage) {
  if (pollTimer) clearTimeout(pollTimer)
  try {
    const res: any = await taskApi.get(taskId)
    if (res.code === 0) {
      const t = res.data.task
      if (t.status === 'completed') { try { aMsg.imageUrl = JSON.parse(t.result || '{}').image_url || '' } catch {}; if (t.completed_at && t.created_at) { const d = new Date(t.updated_at).getTime() - new Date(t.created_at).getTime(); if (d > 0) aMsg.elapsed = (d / 1000).toFixed(1) }; aMsg.status = 'completed'; generating.value = false; scrollToBottom(); loadHistory(); return }
      if (t.status === 'failed') { aMsg.status = 'failed'; aMsg.errorMsg = t.error_msg || '生成失败'; generating.value = false; scrollToBottom(); return }
      pollTimer = setTimeout(() => pollTask(taskId, aMsg), 2000)
    } else pollTimer = setTimeout(() => pollTask(taskId, aMsg), 3000)
  } catch { pollTimer = setTimeout(() => pollTask(taskId, aMsg), 3000) }
}

function resetAll() { sourceImages.value = []; selectedStyleId.value = null; selectedStyleName.value = ''; refImages.value = []; userPrompt.value = ''; selectedRatio.value = '1:1'; imageCount.value = 1; selectedQuality.value = '1k' }
function handleImgError(e: Event) { (e.target as HTMLImageElement).style.background = '#27272a' }
function downloadImage(url?: string) {
  downloadImageByProxy(url)
}

onMounted(() => { if (userStore.isLoggedIn) { loadHistory(); loadStyles() } })
onUnmounted(() => { if (pollTimer) clearTimeout(pollTimer) })
</script>

<style src="./PatternStudio.css" scoped></style>
