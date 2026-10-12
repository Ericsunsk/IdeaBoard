<template>
  <SettingEditorCard
    :icon="SETTINGS_SECTIONS.branding.icon"
    :title="SETTINGS_SECTIONS.branding.title"
    :description="SETTINGS_MESSAGES.brandingDescription"
    :plain="props.plain"
    :class="props.class"
  >
    <div :class="settingCardBodyClass">
      <div class="flex flex-col gap-5 sm:flex-row sm:items-start">
        <!-- 紧凑 Logo 选择区 (尺寸与比例自适应) -->
        <div class="group/logo relative shrink-0">
          <button
            :class="brandingLogoButtonClass"
            type="button"
            :disabled="isBrandingLoading || isBrandingSaving || uploading"
            :aria-label="SETTINGS_MESSAGES.uploadLogo"
            :title="SETTINGS_MESSAGES.logoHint"
            @click="openLogoPicker"
          >
            <SiteBrandLogo
              :src="brandingLogoPreviewURL"
              :fallback-text="brandingFallbackText"
              fallback-class="select-none text-center text-lg leading-none font-bold tracking-normal"
              :class="[brandMarkClasses.logo, 'flex size-full items-center justify-center']"
            />
          </button>

          <button
            v-if="brandingForm.logo_url"
            class="absolute -top-1.5 -right-1.5 z-admin-sticky grid size-5 place-items-center rounded-full border border-border bg-card text-muted-foreground leading-none shadow-xs transition-colors hover:text-destructive focus-visible:outline-none disabled:cursor-not-allowed cursor-pointer"
            :class="
              isBrandingLoading || isBrandingSaving || uploading
                ? 'opacity-40'
                : 'opacity-0 group-hover/logo:opacity-100 group-focus-within/logo:opacity-100 focus-visible:opacity-100'
            "
            type="button"
            :title="SETTINGS_MESSAGES.clearLogo"
            :aria-label="SETTINGS_MESSAGES.clearLogo"
            :disabled="isBrandingLoading || isBrandingSaving || uploading"
            @mousedown.stop
            @pointerdown.stop
            @click.stop="clearBrandingLogo"
          >
            <Xmark class="size-2.5" />
          </button>

          <input
            id="admin-branding-logo-file"
            ref="logoFileInputRef"
            class="hidden"
            type="file"
            name="branding-logo-file"
            :accept="IMAGE_UPLOAD_ACCEPT"
            @change="handleLogoInputChange"
          />
        </div>

        <!-- 站点信息输入 (站点名称与 Logo 地址分两行) -->
        <div class="flex flex-1 flex-col gap-3.5">
          <AdminFormField
            :label="SETTINGS_MESSAGES.projectNameLabel"
            for="admin-branding-project-name"
          >
            <AdminInput
              id="admin-branding-project-name"
              v-model="brandingForm.project_name"
              autocomplete="off"
              maxlength="40"
              placeholder="IdeaBoard"
              :disabled="isBrandingLoading || isBrandingSaving"
              @keyup.enter="saveBranding()"
            />
          </AdminFormField>

          <AdminFormField :label="SETTINGS_MESSAGES.logoUrlLabel" for="admin-branding-logo-url">
            <AdminInput
              id="admin-branding-logo-url"
              v-model="brandingForm.logo_url"
              autocomplete="off"
              maxlength="2048"
              placeholder="/uploads/site/assets/logo.png"
              :disabled="isBrandingLoading || isBrandingSaving || uploading"
              @keyup.enter="saveBranding()"
            />
          </AdminFormField>
        </div>
      </div>
    </div>

    <template #actions>
      <AdminEditorActions
        :save-disabled="isBrandingLoading || isBrandingSaving || uploading"
        :reset-disabled="isBrandingLoading || isBrandingSaving || uploading"
        :is-saving="isBrandingSaving"
        @save="saveBranding"
        @reset="resetBranding"
      >
        <template #leading>
          <span class="text-admin-caption text-muted-foreground">
            {{ logoHint }}
          </span>
        </template>
      </AdminEditorActions>
    </template>
  </SettingEditorCard>

  <AdminLogoCropDialog
    v-model:visible="logoCropDialogVisible"
    :busy="uploading"
    :file="pendingLogoFile"
    @confirm="handleLogoCropConfirm"
  />
</template>

<script setup lang="ts">
import { Xmark } from '@iconoir/vue'
import { ref, watch, type HTMLAttributes } from 'vue'
import { AdminEditorActions } from '../../../../shared/index'
import { AdminFormField } from '../../../../shared/index'
import { AdminInput } from '../../../../shared/index'
import { AdminLogoCropDialog } from '../../../../shared/index'
import SiteBrandLogo from '../../../../../../core/components/SiteBrandLogo.vue'
import { brandMarkClasses } from '../../../../../../core/lib/brandMark'
import { IMAGE_UPLOAD_ACCEPT } from '../../../../../../core/constants/media'
import { prepareImageAssetForUpload } from '../../../../../../core/media/imageAssetPipeline'
import { i18n } from '../../../../../../core/i18n/index'
import { isSupportedUploadImageFile } from '../../../../../../core/utils/imageUploadSupport'
import SettingEditorCard from '../SettingEditorCard.vue'
import { SETTINGS_SECTIONS } from '../../pages/sections'
import { useBrandingSettings } from '../../../session/branding/useBrandingSettings'
import {
  useAdminBrandingSettingsFeedback,
  useAdminUnsupportedLogoFileFeedback,
} from '../../../session/useSettingsFeedback'
import { adminLogoButtonClass } from '../../../../shared/index'
import { SETTINGS_MESSAGES } from '../../pages/messages'

const props = withDefaults(
  defineProps<{
    class?: HTMLAttributes['class'] | undefined
    plain?: boolean
  }>(),
  { class: undefined, plain: true },
)

const settingCardBodyClass = 'flex flex-col gap-5'
const logoHint = SETTINGS_MESSAGES.logoHint
const brandingLogoButtonClass = [
  adminLogoButtonClass,
  'relative h-28 sm:h-[126px] min-w-28 sm:min-w-[126px] max-w-[220px] w-auto p-2.5',
].join(' ')

const branding = useBrandingSettings({
  feedback: useAdminBrandingSettingsFeedback(),
})
const unsupportedLogoFileFeedback = useAdminUnsupportedLogoFileFeedback()
const logoFileInputRef = ref<HTMLInputElement | null>(null)
const logoCropDialogVisible = ref(false)
const pendingLogoFile = ref<File | null>(null)

const {
  form: brandingForm,
  isLoading: isBrandingLoading,
  logoPreviewURL: brandingLogoPreviewURL,
  isSaving: isBrandingSaving,
  uploading,
  clearLogo: clearBrandingLogo,
  fallbackText: brandingFallbackText,
  reset: resetBranding,
  save: saveBranding,
  handleUploadLogo: setCustomLogoFile,
} = branding

watch(
  () => brandingForm.value.logo_url,
  () => {
    if (logoFileInputRef.value) {
      logoFileInputRef.value.value = ''
    }
  },
)

function openLogoPicker() {
  logoFileInputRef.value?.click()
}

async function handleLogoInputChange(event: Event) {
  const input = event.target as HTMLInputElement | null
  const file = input?.files?.[0]
  if (!file) {
    return
  }

  if (!isSupportedUploadImageFile(file)) {
    unsupportedLogoFileFeedback(file)
    if (input) {
      input.value = ''
    }
    return
  }

  const preparedFile = await prepareImageAssetForUpload(file, {
    messages: {
      unknownError: i18n.global.t('canvas.session.unknownError'),
      readImageSizeSkipCompression: i18n.global.t('canvas.session.readImageSizeSkipCompression'),
      compressFallbackOriginal: i18n.global.t('canvas.session.compressFallbackOriginal'),
    },
  })
  pendingLogoFile.value = preparedFile.uploadFile
  logoCropDialogVisible.value = true
}

async function handleLogoCropConfirm(croppedFile: File) {
  await setCustomLogoFile(croppedFile)
  pendingLogoFile.value = null
}
</script>
