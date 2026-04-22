<template>
  <div class="img-uploader">
    <!-- 已上传图片预览 -->
    <div class="uploader-preview-grid" v-if="modelValue.length > 0" :class="{ single: max === 1 }">
      <div v-for="(img, idx) in modelValue" :key="idx" class="uploader-preview-item" :class="{ single: max === 1 }">
        <img :src="img" alt="" />
        <button class="uploader-preview-remove" @click="removeImage(idx)">
          <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
        </button>
      </div>
    </div>

    <!-- 上传区域 -->
    <div
      v-if="modelValue.length < max"
      class="uploader-dropzone"
      :class="{ dragover: isDragOver }"
      @click="triggerFileInput"
      @dragover.prevent="isDragOver = true"
      @dragleave.prevent="isDragOver = false"
      @drop.prevent="handleFileDrop"
    >
      <template v-if="uploadingCount > 0">
        <div class="uploader-spinner"></div>
        <p class="uploader-hint-main">上传中...</p>
      </template>
      <template v-else>
        <svg width="36" height="36" viewBox="0 0 24 24" fill="none" stroke="#666" stroke-width="1.5"><path d="M12 16V8m0 0l-3 3m3-3l3 3"/><path d="M20 16.7V19a2 2 0 01-2 2H6a2 2 0 01-2-2v-2.3"/><path d="M4 12V5a2 2 0 012-2h12a2 2 0 012 2v7"/></svg>
        <p class="uploader-hint-main">点击或将图片拖拽到这里上传</p>
        <p class="uploader-hint-sub">(支持 JPG / PNG，最大 {{ maxSizeMB }}MB{{ max > 1 ? `，最多 ${max} 张` : '' }})</p>
      </template>
    </div>
    <input ref="fileInputRef" type="file" accept=".jpg,.jpeg,.png" :multiple="max > 1" style="display:none" @change="handleFileSelect" />

    <!-- 从作品选择按钮 -->
    <button
      v-if="totalGalleryCount > 0 && modelValue.length < max"
      class="uploader-gallery-btn"
      @click="showPicker = true"
    >从作品选择</button>

    <!-- 从作品选择弹窗 -->
    <n-modal v-model:show="showPicker" preset="card" title="从作品选择" :style="{ maxWidth: '640px', maxHeight: '80vh' }" :scrollable="true">
      <!-- 分类标签 -->
      <div class="picker-category-bar">
        <button
          v-for="(group, gIdx) in visibleGroups"
          :key="group.label"
          class="picker-category-pill"
          :class="{ active: activeCategoryIdx === gIdx }"
          @click="activeCategoryIdx = gIdx"
        >{{ group.label }}<span class="pill-count">{{ group.images.length }}</span></button>
      </div>

      <!-- 当前分类的图片网格 -->
      <div class="picker-grid-wrap">
        <div v-if="activeGroupImages.length > 0" class="picker-grid">
          <div
            v-for="(img, idx) in activeGroupImages"
            :key="idx"
            class="picker-item"
            :class="{ selected: pickerSelected.includes(img) }"
            @click="togglePickerSelect(img)"
          >
            <img :src="img" alt="" loading="lazy" />
            <div v-if="pickerSelected.includes(img)" class="picker-check">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="white" stroke-width="3"><polyline points="20 6 9 17 4 12"/></svg>
            </div>
          </div>
        </div>
        <div v-else class="picker-empty"><p>暂无历史作品</p></div>
      </div>

      <template #footer>
        <div class="picker-footer">
          <span class="picker-count">已选 {{ pickerSelected.length }} 张</span>
          <n-button type="primary" :disabled="pickerSelected.length === 0" @click="confirmPicker">确定</n-button>
        </div>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { NModal, NButton, useMessage } from 'naive-ui'
import { uploadApi } from '../api'
import type { GalleryGroup } from '../utils/galleryGroups'

const props = withDefaults(defineProps<{
  modelValue: string[]
  galleryGroups?: GalleryGroup[]
  max?: number
  maxSizeMB?: number
}>(), {
  galleryGroups: () => [],
  max: 1,
  maxSizeMB: 20,
})

const totalGalleryCount = computed(() =>
  props.galleryGroups.reduce((sum, g) => sum + g.images.length, 0)
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: string[]): void
}>()

const message = useMessage()

// ============ 上传 ============
const isDragOver = ref(false)
const uploadingCount = ref(0)
const fileInputRef = ref<HTMLInputElement | null>(null)

function triggerFileInput() {
  fileInputRef.value?.click()
}

async function handleFileSelect(e: Event) {
  const files = (e.target as HTMLInputElement).files
  if (!files || files.length === 0) return
  await uploadFiles(Array.from(files))
  if (fileInputRef.value) fileInputRef.value.value = ''
}

async function handleFileDrop(e: DragEvent) {
  isDragOver.value = false
  const files = e.dataTransfer?.files
  if (!files || files.length === 0) return
  await uploadFiles(Array.from(files))
}

async function uploadFiles(files: File[]) {
  const remaining = props.max - props.modelValue.length
  const toUpload = files.slice(0, remaining)
  for (const file of toUpload) {
    if (file.size > props.maxSizeMB * 1024 * 1024) {
      message.warning(`文件 ${file.name} 超过 ${props.maxSizeMB}MB`)
      continue
    }
    const ext = file.name.toLowerCase().split('.').pop()
    if (!['jpg', 'jpeg', 'png'].includes(ext || '')) {
      message.warning('仅支持 JPG 和 PNG 格式')
      continue
    }
    uploadingCount.value++
    try {
      const res: any = await uploadApi.upload(file)
      if (res.code === 0 && res.data?.url) {
        const newVal = [...props.modelValue, res.data.url]
        emit('update:modelValue', newVal.slice(0, props.max))
      } else {
        message.error(res.msg || '上传失败')
      }
    } catch {
      message.error('上传失败')
    } finally {
      uploadingCount.value--
    }
  }
}

function removeImage(idx: number) {
  const newVal = [...props.modelValue]
  newVal.splice(idx, 1)
  emit('update:modelValue', newVal)
}

// ============ 弹窗（从作品选择） ============
const showPicker = ref(false)
const pickerSelected = ref<string[]>([])
const activeCategoryIdx = ref(0)

const visibleGroups = computed(() =>
  props.galleryGroups.filter(g => g.images.length > 0)
)

const activeGroupImages = computed(() => {
  const groups = visibleGroups.value
  if (groups.length === 0) return []
  const idx = Math.min(activeCategoryIdx.value, groups.length - 1)
  return groups[idx]?.images || []
})

function togglePickerSelect(url: string) {
  const idx = pickerSelected.value.indexOf(url)
  if (idx >= 0) {
    pickerSelected.value.splice(idx, 1)
  } else {
    const remaining = props.max - props.modelValue.length
    if (pickerSelected.value.length >= remaining) {
      message.warning(`最多还能选择 ${remaining} 张`)
      return
    }
    pickerSelected.value.push(url)
  }
}

function confirmPicker() {
  const newVal = [...props.modelValue]
  for (const url of pickerSelected.value) {
    if (newVal.length < props.max && !newVal.includes(url)) {
      newVal.push(url)
    }
  }
  emit('update:modelValue', newVal)
  pickerSelected.value = []
  showPicker.value = false
}
</script>

<style scoped>
/* ============ 预览网格 ============ */
.uploader-preview-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 6px;
  margin-bottom: 8px;
}
.uploader-preview-grid.single {
  grid-template-columns: 1fr;
}
.uploader-preview-item {
  position: relative;
  border-radius: 8px;
  overflow: hidden;
  aspect-ratio: 1;
  background: #1a1a1e;
}
.uploader-preview-item.single {
  aspect-ratio: auto;
}
.uploader-preview-item img { width: 100%; height: 100%; object-fit: cover; display: block; }
.uploader-preview-item.single img { height: auto; object-fit: contain; }
.uploader-preview-remove {
  position: absolute;
  top: 4px;
  right: 4px;
  width: 20px;
  height: 20px;
  border-radius: 50%;
  background: rgba(0,0,0,0.7);
  border: none;
  color: #ccc;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  opacity: 0;
  transition: opacity 0.2s;
}
.uploader-preview-item:hover .uploader-preview-remove { opacity: 1; }
.uploader-preview-remove:hover { background: rgba(220,38,38,0.8); color: #fff; }

/* ============ 上传拖拽区 ============ */
.uploader-dropzone {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 24px 16px;
  border: 2px dashed #2a2a2e;
  border-radius: 10px;
  cursor: pointer;
  transition: all 0.2s;
  text-align: center;
}
.uploader-dropzone:hover, .uploader-dropzone.dragover {
  border-color: #a3e635;
  background: rgba(163,230,53,0.04);
}
.uploader-dropzone.large {
  padding: 48px 16px;
  min-height: 200px;
}
.uploader-hint-main { font-size: 13px; color: #ccc; font-weight: 500; margin: 0; }
.uploader-hint-sub { font-size: 11px; color: #666; margin: 0; }
.uploader-spinner {
  width: 24px;
  height: 24px;
  border: 3px solid #333;
  border-top-color: #a3e635;
  border-radius: 50%;
  animation: uploaderSpin 0.8s linear infinite;
}
@keyframes uploaderSpin { to { transform: rotate(360deg); } }

/* ============ 从作品选择按钮 ============ */
.uploader-gallery-btn {
  width: 100%;
  margin-top: 8px;
  padding: 8px 0;
  border: 1px solid rgba(163,230,53,0.3);
  border-radius: 8px;
  background: rgba(163,230,53,0.08);
  color: #a3e635;
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s;
}
.uploader-gallery-btn:hover { background: rgba(163,230,53,0.15); }

/* ============ 分类标签栏 ============ */
.picker-category-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 14px;
}
.picker-category-pill {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 5px 14px;
  border-radius: 20px;
  border: 1px solid #333;
  background: #27272a;
  color: #aaa;
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.15s;
  white-space: nowrap;
}
.picker-category-pill:hover { border-color: #555; color: #ddd; }
.picker-category-pill.active {
  background: rgba(163,230,53,0.12);
  border-color: #a3e635;
  color: #a3e635;
}
.pill-count {
  font-size: 11px;
  opacity: 0.6;
}

/* ============ 作品网格容器 ============ */
.picker-grid-wrap {
  max-height: 48vh;
  overflow-y: auto;
}
.picker-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 8px;
}
.picker-item {
  position: relative;
  border-radius: 8px;
  overflow: hidden;
  aspect-ratio: 1;
  cursor: pointer;
  border: 2px solid transparent;
  transition: border-color 0.2s;
}
.picker-item img { width: 100%; height: 100%; object-fit: cover; display: block; }
.picker-item:hover { border-color: #444; }
.picker-item.selected { border-color: #a3e635; }
.picker-check {
  position: absolute;
  top: 4px;
  right: 4px;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: #a3e635;
  display: flex;
  align-items: center;
  justify-content: center;
}
.picker-empty { text-align: center; color: #555; font-size: 13px; padding: 32px 0; }

/* ============ 弹窗底部 ============ */
.picker-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.picker-count { font-size: 12px; color: #888; }
</style>
