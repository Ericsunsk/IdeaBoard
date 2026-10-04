<template>
  <AdminDialogShell
    :visible="visible"
    description="调整裁剪区域后确认使用。"
    size="medium"
    content-layout="scrollable"
    :busy="isProcessing"
    @update:visible="handleVisibleUpdate"
  >
    <template #header>
      <h2 :class="adminDialogTitleClass">裁剪 Logo</h2>
    </template>

    <div class="flex flex-col gap-3.5">
      <div class="relative overflow-hidden rounded-xl bg-muted p-2">
        <Cropper
          v-if="sourceURL"
          ref="cropperRef"
          class="h-[340px] max-h-[52vh] min-h-[260px] w-full overflow-hidden rounded-lg bg-white"
          boundaries-class="overflow-hidden rounded-lg"
          :canvas="true"
          :src="sourceURL"
          :stencil-props="logoStencilProps"
          :auto-zoom="true"
          :transitions="true"
          image-restriction="stencil"
          @error="handleCropperError"
        />
        <div
          class="absolute bottom-3 left-3 flex items-center gap-1 rounded-md border border-border/80 bg-white/90 p-1 backdrop-blur"
        >
          <AdminButton
            variant="ghost"
            size="icon"
            :class="[adminTextButtonClass, '!size-8 !rounded-md !p-0']"
            aria-label="缩小"
            title="缩小"
            :disabled="isProcessing"
            @click="zoomLogo(0.86)"
          >
            <template #icon>
              <Minus class="size-4" />
            </template>
          </AdminButton>
          <AdminButton
            variant="ghost"
            size="icon"
            :class="[adminTextButtonClass, '!size-8 !rounded-md !p-0']"
            aria-label="放大"
            title="放大"
            :disabled="isProcessing"
            @click="zoomLogo(1.16)"
          >
            <template #icon>
              <Plus class="size-4" />
            </template>
          </AdminButton>
          <AdminButton
            variant="ghost"
            size="icon"
            :class="[adminTextButtonClass, '!size-8 !rounded-md !p-0']"
            aria-label="向左旋转"
            title="向左旋转"
            :disabled="isProcessing"
            @click="rotateLogo(-90)"
          >
            <template #icon>
              <RotateCameraLeft class="size-4" />
            </template>
          </AdminButton>
          <AdminButton
            variant="ghost"
            size="icon"
            :class="[adminTextButtonClass, '!size-8 !rounded-md !p-0']"
            aria-label="重置"
            title="重置"
            :disabled="isProcessing"
            @click="resetLogoCrop"
          >
            <template #icon>
              <Refresh class="size-4" />
            </template>
          </AdminButton>
        </div>
      </div>

      <p v-if="cropError" class="min-w-0 text-admin-caption font-[650] text-destructive">
        {{ cropError }}
      </p>
    </div>

    <template #footer>
      <AdminButton
        variant="ghost"
        :class="[adminTextButtonClass, adminActionButtonClass]"
        label="取消"
        :disabled="isProcessing"
        @click="closeDialog"
      >
        <template #icon>
          <Xmark class="size-4" />
        </template>
      </AdminButton>
      <AdminButton
        :class="adminActionButtonClass"
        label="确认上传"
        :disabled="isProcessing || !sourceURL"
        :loading="isProcessing"
        @click="confirmCrop"
      >
        <template #icon>
          <Check class="size-4" />
        </template>
      </AdminButton>
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { Check, Minus, Plus, Refresh, RotateCameraLeft, Xmark } from '@iconoir/vue'
import { computed, onUnmounted, ref, watch } from 'vue'
import { Cropper } from 'vue-advanced-cropper'
import 'vue-advanced-cropper/dist/style.css'
import AdminButton from './AdminButton.vue'
import AdminDialogShell from './AdminDialogShell.vue'
import {
  adminActionButtonClass,
  adminDialogTitleClass,
  adminTextButtonClass,
} from '../lib/uiClasses'

type CropperInstance = InstanceType<typeof Cropper>

const props = defineProps<{
  busy?: boolean
  file: File | null
  visible: boolean
}>()

const emit = defineEmits<{
  confirm: [file: File]
  'update:visible': [visible: boolean]
}>()

const CROPPED_LOGO_SIZE = 512
const CROPPED_LOGO_MIME = 'image/png'

const logoStencilProps = {
  aspectRatio: 1,
} as const

const cropperRef = ref<CropperInstance | null>(null)
const sourceURL = ref('')
const cropError = ref('')
const exporting = ref(false)

const isProcessing = computed(() => Boolean(props.busy) || exporting.value)

watch(
  () => props.file,
  (file) => {
    revokeSourceURL()
    cropError.value = ''
    if (file) {
      sourceURL.value = URL.createObjectURL(file)
    }
  },
  { immediate: true },
)

onUnmounted(() => {
  revokeSourceURL()
})

function revokeSourceURL() {
  if (!sourceURL.value) return
  URL.revokeObjectURL(sourceURL.value)
  sourceURL.value = ''
}

function handleVisibleUpdate(visible: boolean) {
  emit('update:visible', visible)
}

function closeDialog() {
  emit('update:visible', false)
}

function zoomLogo(factor: number) {
  cropperRef.value?.zoom(factor)
}

function rotateLogo(angle: number) {
  cropperRef.value?.rotate(angle)
}

function resetLogoCrop() {
  cropError.value = ''
  cropperRef.value?.reset()
}

function handleCropperError() {
  cropError.value = '图片加载失败，请换一张图片'
}

async function confirmCrop() {
  if (isProcessing.value) return
  exporting.value = true
  cropError.value = ''
  try {
    const canvas = cropperRef.value?.getResult().canvas
    if (!canvas) {
      cropError.value = '裁剪结果为空，请重新选择图片'
      return
    }

    const outputCanvas = document.createElement('canvas')
    outputCanvas.width = CROPPED_LOGO_SIZE
    outputCanvas.height = CROPPED_LOGO_SIZE
    const context = outputCanvas.getContext('2d')
    if (!context) {
      cropError.value = '浏览器不支持图片裁剪'
      return
    }

    context.clearRect(0, 0, CROPPED_LOGO_SIZE, CROPPED_LOGO_SIZE)
    context.imageSmoothingEnabled = true
    context.imageSmoothingQuality = 'high'
    context.drawImage(canvas, 0, 0, CROPPED_LOGO_SIZE, CROPPED_LOGO_SIZE)

    const blob = await canvasToBlob(outputCanvas)
    if (!blob) {
      cropError.value = '生成裁剪图片失败'
      return
    }

    emit(
      'confirm',
      new File([blob], buildCroppedLogoName(props.file?.name), {
        lastModified: Date.now(),
        type: CROPPED_LOGO_MIME,
      }),
    )
  } finally {
    exporting.value = false
  }
}

function canvasToBlob(canvas: HTMLCanvasElement): Promise<Blob | null> {
  return new Promise((resolve) => {
    canvas.toBlob(resolve, CROPPED_LOGO_MIME)
  })
}

function buildCroppedLogoName(name: string | undefined): string {
  const baseName = name?.trim().replace(/\.[^.]+$/, '') || 'site-logo'
  return `${baseName}-cropped.png`
}
</script>
