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
          <div v-if="messages.length === 0" class="chat-empty">
            <div class="empty-icon-wrap">
              <svg width="64" height="64" viewBox="0 0 24 24" fill="none" stroke="#a3e635" stroke-width="1.2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M12 2L2 7l10 5 10-5-10-5z"/><path d="M2 17l10 5 10-5"/><path d="M2 12l10 5 10-5"/>
              </svg>
            </div>
            <p class="chat-empty-text">系列配色</p>
            <p class="chat-empty-sub">在右侧上传图片并选择配色类型开始创作</p>
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
              <div v-if="msg.status === 'pending'" class="skeleton-wrap">
                <div class="skeleton-shimmer"></div>
                <div class="skeleton-overlay">
                  <div class="skeleton-spinner"></div>
                  <p class="skeleton-text">AI 正在生成配色...</p>
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
            <button class="panel-tab active">系列配色</button>
          </div>
        </div>

        <div class="panel-scroll">
          <!-- ① 上传图片 -->
          <div class="section">
            <div class="step-header">
              <span class="step-num">①</span>
              <span class="step-label">上传配色图</span>
            </div>
            <ImageUploader
              v-model="mainImages"
              :gallery-groups="allGalleryGroups"
              :max="1"
              :max-size-m-b="20"
            />
          </div>

          <!-- ② 配色类型 -->
          <div class="section">
            <div class="step-header">
              <span class="step-num">②</span>
              <span class="step-label">选择配色类型</span>
            </div>
            <div class="color-type-grid">
              <button
                v-for="ct in colorTypeOptions"
                :key="ct.value"
                class="color-type-card"
                :class="{ active: colorType === ct.value }"
                @click="selectColorType(ct.value)"
              >
                <div class="color-type-icon" v-html="ct.icon"></div>
                <span class="color-type-name">{{ ct.label }}</span>
              </button>
            </div>
          </div>

          <!-- 预设配色面板（女装/男装/童装展开时） -->
          <div class="section" v-if="colorType && colorType !== 'reference'">
            <div class="preset-panel">
              <div class="preset-grid">
                <button
                  v-for="(palette, pi) in currentPresets"
                  :key="pi"
                  class="preset-card"
                  :class="{ active: selectedPresetIdx === pi }"
                  @click="selectPreset(pi)"
                >
                  <span
                    v-for="(c, ci) in palette"
                    :key="ci"
                    class="preset-dot"
                    :style="{ background: c }"
                  ></span>
                </button>
              </div>
              <button class="custom-color-btn" @click="showColorPicker = !showColorPicker">
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
                自定义颜色
              </button>
              <div v-if="showColorPicker" class="color-picker-wrap">
                <div class="sv-canvas-wrap">
                  <canvas ref="svCanvasRef" class="sv-canvas" width="260" height="160"
                    @mousedown="onSvMouseDown" @touchstart.prevent="onSvTouchStart"></canvas>
                  <div class="sv-cursor" :style="{ left: svCursor.x + 'px', top: svCursor.y + 'px', background: pickerColor }"></div>
                </div>
                <div class="hue-bar-wrap">
                  <canvas ref="hueCanvasRef" class="hue-bar" width="260" height="14"
                    @mousedown="onHueMouseDown" @touchstart.prevent="onHueTouchStart"></canvas>
                  <div class="hue-cursor" :style="{ left: hueCursorX + 'px' }"></div>
                </div>
                <div class="picker-inputs-row">
                  <span class="picker-preview" :style="{ background: pickerColor }"></span>
                  <div class="picker-field"><label>HEX</label><input class="picker-input hex" v-model="pickerHexInput" maxlength="7" @change="onHexInput" /></div>
                  <div class="picker-field"><label>R</label><input class="picker-input" type="number" min="0" max="255" :value="pickerRgb.r" @input="onRgbInput('r', $event)" /></div>
                  <div class="picker-field"><label>G</label><input class="picker-input" type="number" min="0" max="255" :value="pickerRgb.g" @input="onRgbInput('g', $event)" /></div>
                  <div class="picker-field"><label>B</label><input class="picker-input" type="number" min="0" max="255" :value="pickerRgb.b" @input="onRgbInput('b', $event)" /></div>
                </div>
                <button class="picker-add-btn full" @click="addCustomColor">添加此颜色</button>
              </div>
            </div>
          </div>

          <!-- ③ 参考图（仅参考图配色时显示） -->
          <div class="section" v-if="colorType === 'reference'">
            <div class="step-header">
              <span class="step-num">③</span>
              <span class="step-label">上传参考图</span>
            </div>
            <ImageUploader
              v-model="refImages"
              :gallery-groups="allGalleryGroups"
              :max="1"
              :max-size-m-b="20"
            />
            <div v-if="extractingColors" class="extracting-hint">
              <div class="mini-spinner"></div>
              <span>正在分析参考图配色...</span>
            </div>
          </div>

          <!-- 当前选择颜色 -->
          <div class="section" v-if="selectedColors.length > 0">
            <div class="selected-colors-label">当前选择颜色</div>
            <div class="selected-colors-bar">
              <div
                v-for="(c, i) in selectedColors"
                :key="i"
                class="selected-color-dot-wrap"
              >
                <span class="selected-color-dot" :style="{ background: c }"></span>
                <button class="selected-color-remove" @click="removeSelectedColor(i)">
                  <svg width="8" height="8" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
                </button>
              </div>
              <button v-if="selectedColors.length < 8" class="add-color-placeholder" @click="showColorPicker = true; colorType = colorType || 'women'">
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
              </button>
              <button v-if="selectedColors.length < 8" class="add-color-placeholder" @click="showColorPicker = true; colorType = colorType || 'women'">
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
              </button>
            </div>
          </div>

          <!-- 品类选择 -->
          <div class="section">
            <div class="step-header">
              <span class="step-num">◎</span>
              <span class="step-label">选择品类</span>
            </div>
            <div class="category-selector" @click="showCategoryPicker = true">
              <span v-if="selectedCategory" class="category-selected">{{ selectedCategory }}</span>
              <span v-else class="category-placeholder">点击选择品类（可选）</span>
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="6 9 12 15 18 9"/></svg>
            </div>
          </div>

          <!-- 状态提示 -->
          <div class="section status-section">
            <div v-if="canGenerate" class="status-ready">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#a3e635" stroke-width="2"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>
              <span>已就绪，点击下方按钮开始生成</span>
            </div>
            <div v-else class="status-waiting">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#71717a" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>
              <span v-if="!mainImageUrl">请先上传配色图</span>
              <span v-else-if="!colorType">请选择配色类型</span>
              <span v-else-if="colorType === 'reference' && !refImageUrl">请上传参考图</span>
              <span v-else-if="selectedColors.length === 0">请选择配色方案或自定义颜色</span>
            </div>
          </div>
        </div>

        <!-- 底部操作 -->
        <div class="panel-bottom">
          <div class="button-group">
            <button class="btn-generate" :disabled="!canGenerate || generating" @click="handleGenerate">
              {{ generating ? '生成中...' : '生成配色' }}
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

    <!-- 品类选择弹窗 -->
    <div v-if="showCategoryPicker" class="category-modal-overlay" @click.self="showCategoryPicker = false">
      <div class="category-modal">
        <div class="category-modal-header">
          <span class="category-modal-title">选择品类</span>
          <button class="category-modal-close" @click="showCategoryPicker = false">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
          </button>
        </div>
        <div class="category-modal-body">
          <div v-for="group in categoryGroups" :key="group.label" class="category-group">
            <div class="category-group-label">{{ group.label }}</div>
            <div class="category-group-items">
              <button
                v-for="item in group.items"
                :key="item"
                class="category-item"
                :class="{ active: selectedCategory === item }"
                @click="pickCategory(item)"
              >{{ item }}</button>
            </div>
          </div>
        </div>
        <div class="category-modal-footer">
          <button class="category-clear-btn" @click="selectedCategory = ''; showCategoryPicker = false">清除选择</button>
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
import { ref, computed, watch, onMounted, nextTick, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { useMessage } from 'naive-ui'
import { useUserStore } from '../../stores/user'
import { taskApi, segApi } from '../../api'
import ImageUploader from '../../components/ImageUploader.vue'
import { buildGalleryGroups } from '../../utils/galleryGroups'
import type { Task } from '../../api'

const FEATURE_SLUG = 'series-color'
const FEATURE_TITLE = '系列配色'
const FEATURE_PREFIX = `[${FEATURE_SLUG}] `

const router = useRouter()
const message = useMessage()
const userStore = useUserStore()

function stripPrefix(str?: string) {
  if (!str) return ''
  if (str.startsWith('[')) {
    const idx = str.indexOf('] ')
    if (idx > 0) return str.slice(idx + 2)
  }
  return str
}

// ============ 配色类型 ============
type ColorTypeValue = 'women' | 'men' | 'kids' | 'reference'

const colorTypeOptions: { value: ColorTypeValue; label: string; icon: string }[] = [
  { value: 'women', label: '女装配色', icon: '<svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M12 2a4 4 0 0 0-4 4c0 2 1.5 3.5 4 5 2.5-1.5 4-3 4-5a4 4 0 0 0-4-4z"/><path d="M8.5 11L4 22h16l-4.5-11"/><line x1="12" y1="11" x2="12" y2="22"/></svg>' },
  { value: 'men', label: '男装配色', icon: '<svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><rect x="4" y="2" width="16" height="20" rx="2"/><line x1="12" y1="6" x2="12" y2="22"/><polyline points="9 2 12 6 15 2"/></svg>' },
  { value: 'kids', label: '童装配色', icon: '<svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><circle cx="12" cy="5" r="3"/><path d="M7 11h10l-1 11H8L7 11z"/><path d="M9 11l3-3 3 3"/></svg>' },
  { value: 'reference', label: '参考图配色', icon: '<svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="8.5" cy="8.5" r="1.5"/><path d="m21 15-5-5L5 21"/></svg>' },
]

const colorType = ref<ColorTypeValue | ''>('')
const selectedPresetIdx = ref<number | null>(null)
const selectedColors = ref<string[]>([])
const showColorPicker = ref(false)
const extractingColors = ref(false)

// ---------- HSV 颜色选择器 ----------
const svCanvasRef = ref<HTMLCanvasElement | null>(null)
const hueCanvasRef = ref<HTMLCanvasElement | null>(null)
const pickerHue = ref(0)
const pickerSat = ref(1)
const pickerVal = ref(1)
const svCursor = computed(() => {
  const w = 260, h = 160
  return { x: pickerSat.value * w, y: (1 - pickerVal.value) * h }
})
const hueCursorX = computed(() => (pickerHue.value / 360) * 260)

function hsvToRgb(h: number, s: number, v: number): [number, number, number] {
  let r = 0, g = 0, b = 0
  const i = Math.floor(h / 60) % 6
  const f = h / 60 - Math.floor(h / 60)
  const p = v * (1 - s), q = v * (1 - f * s), t = v * (1 - (1 - f) * s)
  switch (i) {
    case 0: r = v; g = t; b = p; break
    case 1: r = q; g = v; b = p; break
    case 2: r = p; g = v; b = t; break
    case 3: r = p; g = q; b = v; break
    case 4: r = t; g = p; b = v; break
    case 5: r = v; g = p; b = q; break
  }
  return [Math.round(r * 255), Math.round(g * 255), Math.round(b * 255)]
}

function rgbToHsv(r: number, g: number, b: number): [number, number, number] {
  r /= 255; g /= 255; b /= 255
  const max = Math.max(r, g, b), min = Math.min(r, g, b), d = max - min
  let h = 0
  if (d !== 0) {
    if (max === r) h = ((g - b) / d + 6) % 6 * 60
    else if (max === g) h = ((b - r) / d + 2) * 60
    else h = ((r - g) / d + 4) * 60
  }
  const s = max === 0 ? 0 : d / max
  return [h, s, max]
}

const pickerRgb = computed(() => {
  const [r, g, b] = hsvToRgb(pickerHue.value, pickerSat.value, pickerVal.value)
  return { r, g, b }
})

const pickerColor = computed(() => {
  const { r, g, b } = pickerRgb.value
  return '#' + [r, g, b].map(v => v.toString(16).padStart(2, '0')).join('')
})

const pickerHexInput = ref('#ff0000')
watch(pickerColor, (v) => { pickerHexInput.value = v })

function drawSvCanvas() {
  const canvas = svCanvasRef.value
  if (!canvas) return
  const ctx = canvas.getContext('2d')!
  const w = canvas.width, h = canvas.height
  const hue = pickerHue.value
  // base hue color
  const [hr, hg, hb] = hsvToRgb(hue, 1, 1)
  ctx.fillStyle = `rgb(${hr},${hg},${hb})`
  ctx.fillRect(0, 0, w, h)
  // white gradient left to right
  const gWhite = ctx.createLinearGradient(0, 0, w, 0)
  gWhite.addColorStop(0, 'rgba(255,255,255,1)')
  gWhite.addColorStop(1, 'rgba(255,255,255,0)')
  ctx.fillStyle = gWhite
  ctx.fillRect(0, 0, w, h)
  // black gradient top to bottom
  const gBlack = ctx.createLinearGradient(0, 0, 0, h)
  gBlack.addColorStop(0, 'rgba(0,0,0,0)')
  gBlack.addColorStop(1, 'rgba(0,0,0,1)')
  ctx.fillStyle = gBlack
  ctx.fillRect(0, 0, w, h)
}

function drawHueCanvas() {
  const canvas = hueCanvasRef.value
  if (!canvas) return
  const ctx = canvas.getContext('2d')!
  const w = canvas.width, h = canvas.height
  const grad = ctx.createLinearGradient(0, 0, w, 0)
  for (let i = 0; i <= 6; i++) {
    const [r, g, b] = hsvToRgb(i * 60, 1, 1)
    grad.addColorStop(i / 6, `rgb(${r},${g},${b})`)
  }
  ctx.fillStyle = grad
  ctx.fillRect(0, 0, w, h)
}

watch(pickerHue, () => drawSvCanvas())
watch(showColorPicker, (v) => { if (v) nextTick(() => { drawSvCanvas(); drawHueCanvas() }) })

function onSvMouseDown(e: MouseEvent) {
  updateSv(e); const move = (ev: MouseEvent) => updateSv(ev); const up = () => { document.removeEventListener('mousemove', move); document.removeEventListener('mouseup', up) }
  document.addEventListener('mousemove', move); document.addEventListener('mouseup', up)
}
function onSvTouchStart(e: TouchEvent) {
  updateSvTouch(e); const move = (ev: TouchEvent) => { ev.preventDefault(); updateSvTouch(ev) }; const up = () => { document.removeEventListener('touchmove', move); document.removeEventListener('touchend', up) }
  document.addEventListener('touchmove', move, { passive: false }); document.addEventListener('touchend', up)
}
function updateSv(e: MouseEvent) {
  const rect = svCanvasRef.value!.getBoundingClientRect()
  pickerSat.value = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width))
  pickerVal.value = Math.max(0, Math.min(1, 1 - (e.clientY - rect.top) / rect.height))
}
function updateSvTouch(e: TouchEvent) {
  const t = e.touches[0]; const rect = svCanvasRef.value!.getBoundingClientRect()
  pickerSat.value = Math.max(0, Math.min(1, (t.clientX - rect.left) / rect.width))
  pickerVal.value = Math.max(0, Math.min(1, 1 - (t.clientY - rect.top) / rect.height))
}

function onHueMouseDown(e: MouseEvent) {
  updateHue(e); const move = (ev: MouseEvent) => updateHue(ev); const up = () => { document.removeEventListener('mousemove', move); document.removeEventListener('mouseup', up) }
  document.addEventListener('mousemove', move); document.addEventListener('mouseup', up)
}
function onHueTouchStart(e: TouchEvent) {
  updateHueTouch(e); const move = (ev: TouchEvent) => { ev.preventDefault(); updateHueTouch(ev) }; const up = () => { document.removeEventListener('touchmove', move); document.removeEventListener('touchend', up) }
  document.addEventListener('touchmove', move, { passive: false }); document.addEventListener('touchend', up)
}
function updateHue(e: MouseEvent) {
  const rect = hueCanvasRef.value!.getBoundingClientRect()
  pickerHue.value = Math.max(0, Math.min(360, ((e.clientX - rect.left) / rect.width) * 360))
}
function updateHueTouch(e: TouchEvent) {
  const t = e.touches[0]; const rect = hueCanvasRef.value!.getBoundingClientRect()
  pickerHue.value = Math.max(0, Math.min(360, ((t.clientX - rect.left) / rect.width) * 360))
}

function onHexInput() {
  const hex = pickerHexInput.value.trim()
  if (/^#[0-9a-fA-F]{6}$/.test(hex)) {
    const r = parseInt(hex.slice(1, 3), 16), g = parseInt(hex.slice(3, 5), 16), b = parseInt(hex.slice(5, 7), 16)
    const [h, s, v] = rgbToHsv(r, g, b)
    pickerHue.value = h; pickerSat.value = s; pickerVal.value = v
  }
}

function onRgbInput(channel: 'r' | 'g' | 'b', e: Event) {
  const val = Math.max(0, Math.min(255, parseInt((e.target as HTMLInputElement).value) || 0))
  const rgb = { ...pickerRgb.value, [channel]: val }
  const [h, s, v] = rgbToHsv(rgb.r, rgb.g, rgb.b)
  pickerHue.value = h; pickerSat.value = s; pickerVal.value = v
}

const colorTypeLabel = computed(() => {
  const ct = colorTypeOptions.find(c => c.value === colorType.value)
  return ct ? ct.label : ''
})

// ============ 品类选择 ============
const showCategoryPicker = ref(false)
const selectedCategory = ref('')

const categoryGroups = [
  { label: '上装', items: ['T恤', '衬衫', '针织衫', '毛衣', '卫衣', '马甲', '背心', 'Polo衫', '打底衫', '雪纺衫', '蕾丝衫', '短上衣'] },
  { label: '外套', items: ['西装外套', '风衣', '大衣', '羽绒服', '棉服', '皮草', '皮衣', '牛仔外套', '夹克', '斗篷', '披肩', '短外套'] },
  { label: '裙装', items: ['连衣裙', '半身裙', 'A字裙', '百褶裙', '包臀裙', '鱼尾裙', '蓬蓬裙', '蛋糕裙', '旗袍', '吊带裙', '衬衫裙', '针织裙'] },
  { label: '裤装', items: ['牛仔裤', '休闲裤', '阔腿裤', '西裤', '铅笔裤', '喇叭裤', '工装裤', '运动裤', '打底裤', '短裤', '连体裤', '背带裤'] },
  { label: '套装 / 其他', items: ['套装', '礼服', '婚纱', '旗袍套装', '运动套装', '家居服', '睡衣', '泳装', '汉服', '民族服饰', '制服', 'Lolita'] },
]

function pickCategory(item: string) {
  selectedCategory.value = item
  showCategoryPicker.value = false
}

// ---------- 预设配色方案 ----------
const womenPresets: string[][] = [
  ['#8b3a3a', '#6b2d3e'],
  ['#6b6b5e', '#4b5e4b'],
  ['#b08050', '#c85040', '#a03030'],
  ['#cc7744', '#b09080', '#a08060'],
  ['#d08050', '#e0b898', '#3d4560'],
  ['#cc7744', '#55665e', '#996070'],
  ['#e8c840', '#707870', '#c8a0b8', '#a08850'],
  ['#e09090', '#b09080', '#8888a0'],
  ['#f0e8d0', '#7a5540', '#708068', '#507068'],
  ['#cc6050', '#505858', '#c06080'],
  ['#c86858', '#505048', '#b8a8a8'],
  ['#9080a0', '#884040', '#e8e0b0'],
  ['#3040a0', '#cc3030', '#b09878', '#d8dce0'],
]
const menPresets: string[][] = [
  ['#6088a8', '#a08088', '#b83858'],
  ['#cc7840', '#384880', '#886840'],
  ['#808088', '#a8a8a0', '#606870'],
  ['#907868', '#a89058', '#987858'],
  ['#e0d090', '#a08858', '#907060'],
  ['#9878a0', '#808888', '#7898b0'],
  ['#b84858', '#707070', '#a8bcc8', '#586068'],
  ['#384880', '#486858', '#b83848', '#787878'],
  ['#808078', '#c08888', '#90b0c8', '#b8a068'],
  ['#908090', '#b87878', '#d8c898'],
]
const kidsPresets: string[][] = [
  ['#a08878', '#6080a8', '#c86040', '#b84050'],
  ['#808880', '#808070', '#e0d8c8', '#808880'],
  ['#987058', '#80b0a0', '#c87840', '#d8b8d0'],
  ['#808080', '#a89060', '#e89878', '#809868'],
  ['#a88880', '#e0d0a0', '#a8b898', '#a08898'],
  ['#588078', '#a07848', '#385888', '#e8dcc0'],
  ['#886888', '#489060', '#c8c090', '#788898'],
  ['#d8a8a0', '#e8e0c8', '#98a868', '#8890a0'],
  ['#787878', '#e87040', '#606060', '#808888'],
  ['#607060', '#c8c090', '#cc8848', '#707878'],
]

const currentPresets = computed(() => {
  if (colorType.value === 'women') return womenPresets
  if (colorType.value === 'men') return menPresets
  if (colorType.value === 'kids') return kidsPresets
  return []
})

function selectColorType(val: ColorTypeValue) {
  if (colorType.value === val) return
  colorType.value = val
  selectedPresetIdx.value = null
  selectedColors.value = []
  showColorPicker.value = false
}

function selectPreset(idx: number) {
  selectedPresetIdx.value = idx
  selectedColors.value = [...currentPresets.value[idx]]
}

function addCustomColor() {
  if (selectedColors.value.length >= 8) { message.warning('最多选择 8 种颜色'); return }
  selectedColors.value.push(pickerColor.value)
  selectedPresetIdx.value = null
}

function removeSelectedColor(idx: number) {
  selectedColors.value.splice(idx, 1)
  selectedPresetIdx.value = null
}

function extractDominantColors(imageUrl: string, maxColors: number): Promise<string[]> {
  return new Promise((resolve, reject) => {
    const img = new Image()
    img.crossOrigin = 'anonymous'
    img.onload = () => {
      const canvas = document.createElement('canvas')
      const size = 150
      canvas.width = size; canvas.height = size
      const ctx = canvas.getContext('2d')!
      ctx.drawImage(img, 0, 0, size, size)
      const data = ctx.getImageData(0, 0, size, size).data
      const buckets = new Map<string, number>()
      for (let i = 0; i < data.length; i += 4) {
        const r = data[i], g = data[i + 1], b = data[i + 2], a = data[i + 3]
        if (a < 128) continue
        if (r > 240 && g > 240 && b > 240) continue
        if (r < 15 && g < 15 && b < 15) continue
        const qr = Math.round(r / 32) * 32
        const qg = Math.round(g / 32) * 32
        const qb = Math.round(b / 32) * 32
        const key = `${qr},${qg},${qb}`
        buckets.set(key, (buckets.get(key) || 0) + 1)
      }
      const sorted = [...buckets.entries()].sort((a, b) => b[1] - a[1])
      const result: string[] = []
      for (const [key] of sorted) {
        if (result.length >= maxColors) break
        const [r, g, b] = key.split(',').map(Number)
        const hex = '#' + [r, g, b].map(v => v.toString(16).padStart(2, '0')).join('')
        const tooClose = result.some(existing => {
          const er = parseInt(existing.slice(1, 3), 16)
          const eg = parseInt(existing.slice(3, 5), 16)
          const eb = parseInt(existing.slice(5, 7), 16)
          return Math.abs(r - er) + Math.abs(g - eg) + Math.abs(b - eb) < 80
        })
        if (!tooClose) result.push(hex)
      }
      resolve(result)
    }
    img.onerror = () => reject(new Error('Failed to load image'))
    img.src = imageUrl
  })
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
  try { return JSON.parse(task.result || '{}').image_url || '' } catch { return '' }
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
    messages.value.push({ id: 'hist_user_' + task.task_id, role: 'user', prompt: stripPrefix(task.prompt) || FEATURE_TITLE, time: timeStr })
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
const mainImageUrl = ref('')
const refImageUrl = ref('')

const mainImages = computed({
  get: () => mainImageUrl.value ? [mainImageUrl.value] : [],
  set: (val: string[]) => { mainImageUrl.value = val.length > 0 ? val[0] : '' }
})

const refImages = computed({
  get: () => refImageUrl.value ? [refImageUrl.value] : [],
  set: (val: string[]) => { refImageUrl.value = val.length > 0 ? val[0] : '' }
})

// ---------- 参考图自动提取配色 ----------
watch(refImageUrl, async (url) => {
  if (!url || colorType.value !== 'reference') return
  extractingColors.value = true
  try {
    const segRes: any = await segApi.segment({ image_url: url, seg_type: 'clothing', return_form: 'mask' })
    const segUrl = segRes?.data?.result_url || segRes?.result_url || url
    const colors = await extractDominantColors(segUrl, 5)
    selectedColors.value = colors
    selectedPresetIdx.value = null
  } catch (e: any) {
    console.warn('服饰分割/取色失败，回退到原图取色', e)
    try {
      const colors = await extractDominantColors(url, 5)
      selectedColors.value = colors
    } catch { /* ignore */ }
  } finally {
    extractingColors.value = false
  }
})

const allGalleryGroups = computed(() => buildGalleryGroups(historyTasks.value))

// ============ 消息气泡 ============
interface ChatMessage {
  id: string
  role: 'user' | 'assistant'
  prompt?: string
  refImageUrls?: string[]
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
  nextTick(() => { if (chatMessagesRef.value) chatMessagesRef.value.scrollTop = chatMessagesRef.value.scrollHeight })
}

// ============ 生成 ============
const generating = ref(false)
const currentTaskId = ref<string | null>(null)
const lightboxUrl = ref('')

const canGenerate = computed(() => {
  if (!userStore.isLoggedIn || generating.value) return false
  if (!mainImageUrl.value) return false
  if (!colorType.value) return false
  if (colorType.value === 'reference' && !refImageUrl.value) return false
  if (selectedColors.value.length === 0) return false
  return true
})

async function handleGenerate() {
  if (!userStore.isLoggedIn) { message.warning('请先登录'); router.push('/user/login'); return }
  if (!mainImageUrl.value) { message.warning('请先上传配色图'); return }
  if (!colorType.value) { message.warning('请选择配色类型'); return }
  if (colorType.value === 'reference' && !refImageUrl.value) { message.warning('请上传参考图'); return }
  if (selectedColors.value.length === 0) { message.warning('请选择配色方案'); return }

  activeTaskId.value = null
  const now = new Date()
  const timeStr = `${now.getHours().toString().padStart(2, '0')}:${now.getMinutes().toString().padStart(2, '0')}`

  const colorsStr = selectedColors.value.join(', ')
  const catLabel = selectedCategory.value ? `品类：${selectedCategory.value}` : ''
  const displayPrompt = `${FEATURE_TITLE}（${colorTypeLabel.value}）${catLabel ? ' ' + catLabel : ''} 配色：${colorsStr}`

  // 构建图片列表：主图 + 可选参考图
  const imageUrls = [mainImageUrl.value]
  if (colorType.value === 'reference' && refImageUrl.value) {
    imageUrls.push(refImageUrl.value)
  }

  const userMsg: ChatMessage = {
    id: 'msg_' + Date.now(), role: 'user',
    prompt: displayPrompt,
    refImageUrls: imageUrls,
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
    const promptParts = [FEATURE_TITLE, `配色类型：${colorTypeLabel.value}`, `目标配色：${colorsStr}`]
    if (selectedCategory.value) promptParts.push(`目标品类：${selectedCategory.value}`)
    const res: any = await taskApi.submit({
      prompt: FEATURE_PREFIX + promptParts.join('，'),
      params: {
        image_urls: imageUrls,
        feature: FEATURE_SLUG,
        color_type: colorType.value,
        colors: selectedColors.value,
        ...(selectedCategory.value ? { category: selectedCategory.value } : {}),
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
          if (task.completed_at) {
            const t = new Date(task.updated_at).getTime() - new Date(task.created_at).getTime()
            assistantMsg.elapsed = (t / 1000).toFixed(1)
          }
        } catch { assistantMsg.imageUrl = '' }
        assistantMsg.status = 'completed'
        generating.value = false; scrollToBottom(); loadHistory(); return
      } else if (task.status === 'failed') {
        assistantMsg.status = 'failed'
        assistantMsg.errorMsg = task.error_msg || '生成失败'
        generating.value = false; scrollToBottom(); loadHistory(); return
      }
    }
  } catch {}
  pollTimer = window.setTimeout(() => pollTaskStatus(taskId, assistantMsg), 3000)
}

function handleClear() {
  mainImageUrl.value = ''
  refImageUrl.value = ''
  colorType.value = ''
  selectedPresetIdx.value = null
  selectedColors.value = []
  showColorPicker.value = false
  selectedCategory.value = ''
  messages.value = []
  activeTaskId.value = null
}

function handleImgError(e: Event) {
  const img = e.target as HTMLImageElement
  img.style.background = '#27272a'
}

function downloadImage(url?: string) {
  if (!url) return
  const proxyUrl = '/api/user/download?url=' + encodeURIComponent(url)
  const a = document.createElement('a')
  a.href = proxyUrl; a.download = 'image.png'; a.click()
}

// ============ 生命周期 ============
onMounted(() => { if (userStore.isLoggedIn) loadHistory() })
onUnmounted(() => { if (pollTimer) clearTimeout(pollTimer) })
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
.bubble-ref-images { display: flex; gap: 8px; margin-top: 8px; }
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

/* 步骤标题 */
.step-header { display: flex; align-items: center; gap: 8px; margin-bottom: 10px; }
.step-num { width: 22px; height: 22px; border-radius: 50%; background: rgba(163,230,53,0.12); color: #a3e635; font-size: 12px; font-weight: 700; display: flex; align-items: center; justify-content: center; flex-shrink: 0; }
.step-label { font-size: 13px; font-weight: 600; color: #d4d4d8; }

/* ============ 配色类型卡片 ============ */
.color-type-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
}
.color-type-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 16px 8px;
  border-radius: 12px;
  border: 2px solid #27272a;
  background: #111113;
  cursor: pointer;
  transition: all 0.2s;
  color: #888;
}
.color-type-card:hover {
  border-color: #3f3f46;
  background: #1a1a1e;
  color: #ccc;
}
.color-type-card.active {
  border-color: #a3e635;
  background: rgba(163,230,53,0.06);
  color: #a3e635;
}
.color-type-icon {
  width: 44px;
  height: 44px;
  border-radius: 10px;
  background: rgba(255,255,255,0.03);
  display: flex;
  align-items: center;
  justify-content: center;
}
.color-type-card.active .color-type-icon {
  background: rgba(163,230,53,0.1);
}
.color-type-name {
  font-size: 12px;
  font-weight: 500;
}

/* ============ 预设配色面板 ============ */
.preset-panel { display: flex; flex-direction: column; gap: 10px; }
.preset-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 8px; }
.preset-card {
  display: flex; align-items: center; gap: 6px;
  padding: 10px 12px; border-radius: 10px;
  border: 2px solid #27272a; background: #111113;
  cursor: pointer; transition: all 0.2s;
}
.preset-card:hover { border-color: #3f3f46; background: #1a1a1e; }
.preset-card.active { border-color: #a3e635; background: rgba(163,230,53,0.06); }
.preset-dot {
  width: 28px; height: 28px; border-radius: 50%; flex-shrink: 0;
  border: 2px solid rgba(255,255,255,0.08);
}

/* 自定义颜色 */
.custom-color-btn {
  display: flex; align-items: center; justify-content: center; gap: 6px;
  padding: 8px; border-radius: 8px; border: 1px dashed #3f3f46;
  background: transparent; color: #71717a; font-size: 12px;
  cursor: pointer; transition: all 0.2s; width: 100%;
}
.custom-color-btn:hover { border-color: #a3e635; color: #a3e635; }
.color-picker-wrap {
  display: flex; flex-direction: column; gap: 10px;
  padding: 12px; border-radius: 10px; background: #1a1a1e;
  border: 1px solid #27272a;
}
/* SV canvas */
.sv-canvas-wrap { position: relative; width: 260px; height: 160px; border-radius: 6px; overflow: hidden; cursor: crosshair; }
.sv-canvas { display: block; width: 260px; height: 160px; }
.sv-cursor {
  position: absolute; width: 14px; height: 14px; border-radius: 50%;
  border: 2px solid #fff; box-shadow: 0 0 2px rgba(0,0,0,0.6);
  pointer-events: none; transform: translate(-50%, -50%);
}
/* Hue bar */
.hue-bar-wrap { position: relative; width: 260px; height: 14px; border-radius: 7px; overflow: hidden; cursor: pointer; }
.hue-bar { display: block; width: 260px; height: 14px; border-radius: 7px; }
.hue-cursor {
  position: absolute; top: -1px; width: 6px; height: 16px; border-radius: 3px;
  border: 2px solid #fff; box-shadow: 0 0 2px rgba(0,0,0,0.5);
  pointer-events: none; transform: translateX(-50%);
}
/* Inputs row */
.picker-inputs-row { display: flex; align-items: flex-end; gap: 6px; }
.picker-preview { width: 28px; height: 28px; border-radius: 50%; flex-shrink: 0; border: 2px solid rgba(255,255,255,0.1); }
.picker-field { display: flex; flex-direction: column; gap: 2px; flex: 1; }
.picker-field label { font-size: 9px; color: #71717a; text-transform: uppercase; }
.picker-input {
  width: 100%; padding: 4px 5px; border-radius: 4px; border: 1px solid #3f3f46;
  background: #27272a; color: #e4e4e7; font-size: 11px; font-family: monospace;
  outline: none; text-align: center;
}
.picker-input:focus { border-color: #a3e635; }
.picker-input.hex { text-align: left; }
.picker-input::-webkit-inner-spin-button, .picker-input::-webkit-outer-spin-button { -webkit-appearance: none; margin: 0; }
.picker-input[type=number] { -moz-appearance: textfield; }
.picker-add-btn {
  padding: 7px 14px; border-radius: 6px; border: none;
  background: rgba(163,230,53,0.15); color: #a3e635;
  font-size: 12px; font-weight: 500; cursor: pointer;
  transition: background 0.15s;
}
.picker-add-btn:hover { background: rgba(163,230,53,0.25); }
.picker-add-btn.full { width: 100%; }

/* ============ 当前选择颜色栏 ============ */
.selected-colors-label { font-size: 12px; font-weight: 600; color: #a3e635; margin-bottom: 8px; }
.selected-colors-bar {
  display: flex; align-items: center; gap: 8px;
  padding: 10px 16px; border-radius: 12px;
  background: #27272a;
}
.selected-color-dot-wrap { position: relative; }
.selected-color-dot {
  width: 32px; height: 32px; border-radius: 50%;
  border: 2px solid rgba(255,255,255,0.1); display: block;
}
.selected-color-remove {
  position: absolute; top: -4px; right: -4px;
  width: 16px; height: 16px; border-radius: 50%;
  background: rgba(0,0,0,0.7); border: 1px solid #555;
  color: #aaa; cursor: pointer;
  display: flex; align-items: center; justify-content: center;
  opacity: 0; transition: opacity 0.15s;
}
.selected-color-dot-wrap:hover .selected-color-remove { opacity: 1; }
.add-color-placeholder {
  width: 32px; height: 32px; border-radius: 50%;
  border: 2px dashed #555; background: transparent;
  color: #555; cursor: pointer;
  display: flex; align-items: center; justify-content: center;
  transition: all 0.15s;
}
.add-color-placeholder:hover { border-color: #888; color: #888; }

/* 提取中提示 */
.extracting-hint {
  display: flex; align-items: center; gap: 8px;
  margin-top: 10px; font-size: 12px; color: #a3e635;
}
.mini-spinner {
  width: 16px; height: 16px; border: 2px solid #27272a;
  border-top-color: #a3e635; border-radius: 50%;
  animation: spin 1s linear infinite;
}

/* 状态提示 */
.status-section {
  padding: 12px 14px;
  border-radius: 10px;
  border: 1px solid #27272a;
  background: #1a1a1e;
}
.status-ready, .status-waiting {
  display: flex; align-items: center; gap: 10px; font-size: 12px;
}
.status-ready { color: #a3e635; }
.status-waiting { color: #71717a; }

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

/* ============ 品类选择 ============ */
.category-selector {
  display: flex; align-items: center; justify-content: space-between;
  padding: 10px 14px; border-radius: 10px; border: 1px solid #27272a;
  background: #1a1a1e; cursor: pointer; transition: border-color 0.2s;
}
.category-selector:hover { border-color: #3f3f46; }
.category-selected { font-size: 13px; color: #e4e4e7; font-weight: 500; }
.category-placeholder { font-size: 13px; color: #555; }
.category-modal-overlay {
  position: fixed; inset: 0; z-index: 1000;
  background: rgba(0,0,0,0.6); display: flex; align-items: center; justify-content: center;
}
.category-modal {
  background: #1a1a1e; border: 1px solid #27272a; border-radius: 14px;
  width: 520px; max-width: 92vw; max-height: 80vh; display: flex; flex-direction: column;
  box-shadow: 0 16px 48px rgba(0,0,0,0.5);
}
.category-modal-header {
  display: flex; align-items: center; justify-content: space-between;
  padding: 16px 20px; border-bottom: 1px solid #27272a; flex-shrink: 0;
}
.category-modal-title { font-size: 15px; font-weight: 600; color: #e4e4e7; }
.category-modal-close { background: none; border: none; color: #71717a; cursor: pointer; padding: 4px; border-radius: 6px; }
.category-modal-close:hover { background: #27272a; color: #e4e4e7; }
.category-modal-body { flex: 1; overflow-y: auto; padding: 16px 20px; }
.category-group { margin-bottom: 16px; }
.category-group-label { font-size: 12px; font-weight: 600; color: #71717a; margin-bottom: 8px; }
.category-group-items { display: flex; flex-wrap: wrap; gap: 8px; }
.category-item {
  padding: 6px 14px; border-radius: 8px; border: 1px solid #27272a;
  background: #111113; color: #a1a1aa; font-size: 12px; cursor: pointer; transition: all 0.15s;
}
.category-item:hover { border-color: #3f3f46; color: #e4e4e7; }
.category-item.active { border-color: #a3e635; background: rgba(163,230,53,0.08); color: #a3e635; }
.category-modal-footer {
  padding: 12px 20px; border-top: 1px solid #27272a; display: flex; justify-content: flex-end; flex-shrink: 0;
}
.category-clear-btn {
  padding: 6px 16px; border-radius: 6px; border: 1px solid #3f3f46;
  background: transparent; color: #71717a; font-size: 12px; cursor: pointer;
}
.category-clear-btn:hover { color: #e4e4e7; border-color: #52525b; }
</style>
