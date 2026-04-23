<template>
  <div class="cover-upload-area">
    <div :class="['cover-preview', previewVariant, previewClass]">
      <img v-if="imageUrl" :src="imageUrl" :alt="alt" @error="hideBrokenImage" />
      <div v-else class="cover-placeholder">{{ emptyText }}</div>
    </div>
    <n-upload
      :action="action"
      :headers="headers"
      :max="1"
      :show-file-list="false"
      :accept="accept"
      @finish="handleFinish"
      @before-upload="handleBeforeUpload"
    >
      <n-button :loading="loading" secondary>
        <template #icon>
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="17 8 12 3 7 8"/><line x1="12" y1="3" x2="12" y2="15"/></svg>
        </template>
        {{ resolvedButtonText }}
      </n-button>
    </n-upload>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { NButton, NUpload, type UploadFileInfo } from 'naive-ui'

const props = withDefaults(defineProps<{
  imageUrl: string
  emptyText?: string
  alt?: string
  accept: string
  loading?: boolean
  action: string
  headers?: Record<string, string>
  previewClass?: string
  previewVariant?: 'default' | 'landscape' | 'portrait' | 'square'
  buttonText?: string
  uploadText?: string
  reuploadText?: string
}>(), {
  emptyText: '暂无图片',
  alt: '图片预览',
  loading: false,
  headers: () => ({}),
  previewClass: '',
  previewVariant: 'default',
  buttonText: '',
  uploadText: '上传图片',
  reuploadText: '重新上传',
})

const emit = defineEmits<{
  (e: 'finish', payload: { file: UploadFileInfo; event?: ProgressEvent }): void
  (e: 'start-upload'): void
}>()

const resolvedButtonText = computed(() => {
  if (props.buttonText) return props.buttonText
  return props.imageUrl ? props.reuploadText : props.uploadText
})

function hideBrokenImage(event: Event) {
  ;(event.target as HTMLImageElement).style.display = 'none'
}

function handleFinish(payload: { file: UploadFileInfo; event?: ProgressEvent }) {
  emit('finish', payload)
}

function handleBeforeUpload() {
  emit('start-upload')
  return true
}
</script>

<style scoped>
.cover-upload-area {
  display: flex;
  align-items: center;
  gap: 16px;
}

.cover-preview {
  overflow: hidden;
  flex-shrink: 0;
}

.cover-preview.default,
.cover-preview.landscape {
  width: 120px;
  height: 80px;
  border-radius: 14px;
  background: var(--admin-preview-bg, #1a1b1e);
  border: 1px solid var(--admin-border, #232326);
}

.cover-preview.square {
  width: 80px;
  height: 80px;
  border-radius: 8px;
  background: var(--admin-preview-bg, #1a1b1e);
  border: 1px solid var(--admin-border, #232326);
}

.cover-preview.portrait {
  width: 80px;
  height: 106px;
  border-radius: 8px;
  background: var(--admin-preview-bg, #1a1b1e);
  border: 1px solid var(--admin-border, #232326);
}

.cover-preview img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.cover-placeholder {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  color: var(--admin-text-3, #6b6b71);
}

.cover-preview.square .cover-placeholder,
.cover-preview.portrait .cover-placeholder {
  color: var(--admin-text-3, #6b6b71);
}
</style>
