<template>
  <AdminResourceGridFrame
    empty-title="暂无系统配置"
    :grid-class="adminTwoColumnMasonryGridClass"
    :is-empty="false"
    :is-loading="false"
  >
    <SettingEditorCard
      title="站点设置"
      :status="brandingForm.logo_url ? '自定义 Logo' : '默认标识'"
    >
      <template #default>
        <div :class="settingCardBodyClass">
          <div class="grid gap-5 xl:grid-cols-[136px_minmax(0,1fr)]">
            <div class="flex min-w-0 flex-col gap-3">
              <div class="group/logo relative size-28">
                <button
                  :class="brandingLogoButtonClass"
                  type="button"
                  :disabled="brandingLoading || brandingSaving || uploading"
                  aria-label="上传站点 Logo"
                  @click="openLogoPicker"
                >
                  <SiteBrandLogo
                    :src="brandingLogoPreviewURL"
                    :fallback-text="brandingFallbackText"
                    fallback-class="select-none text-center text-xl leading-none font-[850] tracking-normal"
                    :class="brandMarkClasses.logo"
                  />
                </button>

                <button
                  v-if="brandingForm.logo_url"
                  class="absolute right-1.5 top-1.5 grid size-5 place-items-center rounded-full border-0 bg-white/90 p-0 text-admin-text-2 leading-none transition-opacity hover:bg-white hover:text-admin-text-1 focus-visible:bg-white focus-visible:text-admin-text-1 focus-visible:outline-none disabled:cursor-not-allowed"
                  :class="
                    brandingLoading || brandingSaving || uploading
                      ? 'opacity-40'
                      : 'opacity-0 group-hover/logo:opacity-100 group-focus-within/logo:opacity-100 focus-visible:opacity-100'
                  "
                  type="button"
                  title="清除 Logo"
                  aria-label="清除 Logo"
                  :disabled="brandingLoading || brandingSaving || uploading"
                  @mousedown.stop
                  @pointerdown.stop
                  @click.stop="clearBrandingLogo"
                >
                  <Xmark class="size-2.5" />
                </button>
              </div>

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

            <div class="grid content-start gap-4">
              <AdminFormField label="站点名称" for="admin-branding-project-name">
                <InputText
                  id="admin-branding-project-name"
                  v-model="brandingForm.project_name"
                  autocomplete="off"
                  maxlength="40"
                  placeholder="IdeaBoard"
                  :disabled="brandingLoading || brandingSaving"
                  @keyup.enter="saveBranding()"
                />
              </AdminFormField>

              <AdminFormField label="Logo 地址" for="admin-branding-logo-url">
                <InputText
                  id="admin-branding-logo-url"
                  v-model="brandingForm.logo_url"
                  autocomplete="off"
                  maxlength="2048"
                  placeholder="/uploads/site/assets/logo.png"
                  :disabled="brandingLoading || brandingSaving || uploading"
                  @keyup.enter="saveBranding()"
                />
              </AdminFormField>
            </div>
          </div>
        </div>
      </template>
      <template #actions>
        <Button
          :class="adminActionButtonClass"
          label="保存"
          severity="contrast"
          :disabled="brandingLoading || brandingSaving || uploading"
          :loading="brandingSaving"
          @click="saveBranding"
        />
        <Button
          :class="adminActionButtonClass"
          label="重置"
          severity="secondary"
          outlined
          :disabled="brandingLoading || brandingSaving || uploading"
          @click="resetBranding"
        />
      </template>
    </SettingEditorCard>

    <SettingEditorCard
      title="注册与 API 限流"
      :status="registrationEnabled ? '开放注册' : '关闭注册'"
    >
      <template #default>
        <div :class="settingCardBodyClass">
          <div :class="settingToggleRowClass">
            <span class="text-admin-body font-bold text-admin-text-1">开放注册</span>
            <ToggleSwitch
              v-model="registrationEnabled"
              input-id="admin-registration-enabled"
              :disabled="registrationSettingLoading"
              aria-label="开放注册"
              @update:model-value="handleToggleRegistration"
            />
          </div>

          <div class="grid gap-4 md:grid-cols-2">
            <AdminFormField label="最大请求 (次)" for="admin-rate-limit-max-requests">
              <InputNumber
                v-model="rateLimitForm.max_requests"
                input-id="admin-rate-limit-max-requests"
                :min="1"
                :max="RATE_LIMIT_MAX_REQUESTS"
                fluid
                :disabled="rateLimitLoading || rateLimitSaving"
              />
            </AdminFormField>

            <AdminFormField label="窗口周期 (秒)" for="admin-rate-limit-window-seconds">
              <InputNumber
                v-model="rateLimitForm.window_seconds"
                input-id="admin-rate-limit-window-seconds"
                :min="1"
                :max="RATE_LIMIT_MAX_WINDOW_SECONDS"
                fluid
                :disabled="rateLimitLoading || rateLimitSaving"
              />
            </AdminFormField>
          </div>
        </div>
      </template>
      <template #actions>
        <Button
          :class="adminActionButtonClass"
          label="保存"
          severity="contrast"
          :disabled="rateLimitLoading || rateLimitSaving"
          :loading="rateLimitSaving"
          @click="saveRateLimit"
        />
        <Button
          :class="adminActionButtonClass"
          label="重置"
          severity="secondary"
          outlined
          :disabled="rateLimitLoading || rateLimitSaving"
          @click="resetRateLimit"
        />
      </template>
    </SettingEditorCard>

    <SettingEditorCard title="上传并发" :status="`并发 ${mediaUploadForm.upload_max_concurrent}`">
      <template #default>
        <div :class="settingCardBodyClass">
          <AdminFormField label="上传并发上限" for="admin-media-upload-max-concurrent">
            <InputNumber
              v-model="mediaUploadForm.upload_max_concurrent"
              input-id="admin-media-upload-max-concurrent"
              :min="MEDIA_UPLOAD_MIN_CONCURRENT"
              :max="MEDIA_UPLOAD_MAX_CONCURRENT"
              fluid
              :disabled="mediaUploadLoading || mediaUploadSaving"
            />
          </AdminFormField>
        </div>
      </template>
      <template #actions>
        <Button
          :class="adminActionButtonClass"
          label="保存"
          severity="contrast"
          :disabled="mediaUploadLoading || mediaUploadSaving"
          :loading="mediaUploadSaving"
          @click="saveMediaUpload"
        />
        <Button
          :class="adminActionButtonClass"
          label="重置"
          severity="secondary"
          outlined
          :disabled="mediaUploadLoading || mediaUploadSaving"
          @click="resetMediaUpload"
        />
      </template>
    </SettingEditorCard>

    <AdminOutboundPolicyCard />
    <AdminMailConfigCard />
  </AdminResourceGridFrame>

  <AdminLogoCropDialog
    v-model:visible="logoCropDialogVisible"
    :busy="uploading"
    :file="pendingLogoFile"
    @confirm="handleLogoCropConfirm"
  />
</template>

<script setup lang="ts">
import { Xmark } from '@iconoir/vue'
import { ref, watch } from 'vue'
import Button from 'primevue/button'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import ToggleSwitch from 'primevue/toggleswitch'
import { IMAGE_UPLOAD_ACCEPT } from '../../../../shared/constants/media'
import { prepareImageAssetForUpload } from '../../../../shared/media/imageAssetPipeline'
import { isSupportedUploadImageFile } from '../../../../shared/utils/imageUploadSupport'
import AdminOutboundPolicyCard from '../AdminOutboundPolicyCard.vue'
import AdminMailConfigCard from '../AdminMailConfigCard.vue'
import SettingEditorCard from '../SettingEditorCard.vue'
import AdminFormField from '../../shared/components/AdminFormField.vue'
import AdminLogoCropDialog from '../../shared/components/AdminLogoCropDialog.vue'
import AdminResourceGridFrame from '../../shared/components/AdminResourceGridFrame.vue'
import {
  RATE_LIMIT_MAX_REQUESTS,
  RATE_LIMIT_MAX_WINDOW_SECONDS,
} from '../useAccessSettings'
import {
  MEDIA_UPLOAD_MAX_CONCURRENT,
  MEDIA_UPLOAD_MIN_CONCURRENT,
} from '../useMediaUploadSettings'
import { useSystemSettings } from '../useSystemSettings'
import {
  adminActionButtonClass,
  adminTwoColumnMasonryGridClass,
} from '../../shared/lib/uiClasses'
import SiteBrandLogo from '../../../../shared/components/SiteBrandLogo.vue'
import { brandMarkClasses, createBrandMarkClass } from '../../../../shared/lib/brandMark'

const settingCardBodyClass = 'flex flex-col gap-5'
const settingToggleRowClass =
  'flex min-h-12 items-center justify-between gap-4 rounded-lg border border-admin-border bg-admin-surface-muted px-3.5 py-3'
const brandingLogoButtonClass = createBrandMarkClass(
  brandMarkClasses.sizes.fill,
  brandMarkClasses.states.fallbackSoft,
  'hover:border-admin-border-hover hover:bg-admin-surface-muted focus-visible:outline-none focus-visible:shadow-[0_0_0_3px_var(--color-admin-focus-ring)] disabled:cursor-not-allowed disabled:opacity-60',
)

const { access, branding, mediaUpload, unsupportedLogoFileFeedback } = useSystemSettings()
const logoFileInputRef = ref<HTMLInputElement | null>(null)
const logoCropDialogVisible = ref(false)
const pendingLogoFile = ref<File | null>(null)
const {
  clearLogo: clearBrandingLogo,
  fallbackText: brandingFallbackText,
  form: brandingForm,
  save: saveBranding,
  handleUploadLogo,
  loading: brandingLoading,
  logoPreviewURL: brandingLogoPreviewURL,
  reset: resetBranding,
  saving: brandingSaving,
  uploading,
} = branding
const {
  handleToggleRegistration,
  rateLimitLoading,
  rateLimitForm,
  rateLimitSaving,
  registrationEnabled,
  registrationSettingLoading,
  resetRateLimit,
  saveRateLimit,
} = access
const {
  form: mediaUploadForm,
  save: saveMediaUpload,
  loading: mediaUploadLoading,
  reset: resetMediaUpload,
  saving: mediaUploadSaving,
} = mediaUpload
watch(logoCropDialogVisible, (visible) => {
  if (!visible) {
    pendingLogoFile.value = null
  }
})

function openLogoPicker() {
  logoFileInputRef.value?.click()
}

async function handleLogoInputChange(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  if (!isSupportedUploadImageFile(file)) {
    unsupportedLogoFileFeedback()
    return
  }
  try {
    const prepared = await prepareImageAssetForUpload(file, { compressionPolicy: 'none' })
    pendingLogoFile.value = prepared.uploadFile
    logoCropDialogVisible.value = true
  } catch (error) {
    unsupportedLogoFileFeedback(error)
  }
}

async function handleLogoCropConfirm(file: File) {
  const uploaded = await handleUploadLogo(file)
  if (!uploaded) return
  logoCropDialogVisible.value = false
  pendingLogoFile.value = null
}
</script>
