<template>
  <AdminResourceGridFrame
    empty-description="系统配置加载完成后会显示在这里。"
    empty-title="暂无系统配置"
    :grid-class="adminTwoColumnMasonryGridClass"
    :is-empty="false"
    :is-loading="false"
    :is-scrolling="isSystemSettingsPanelScrolling"
    @scroll="handleSystemSettingsPanelScroll"
  >
    <Card :class="adminMasonryCardClass" :pt="adminCardPt">
      <template #title>
        <div class="flex min-w-0 items-center justify-between gap-3">
          <span class="truncate text-admin-title-sm font-bold text-admin-title-color">
            站点设置
          </span>
          <Tag
            :value="brandingForm.logo_url ? '自定义 Logo' : '默认标识'"
            class="shrink-0"
            severity="secondary"
            rounded
          />
        </div>
      </template>

      <template #content>
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
                ref="logoFileInputRef"
                class="hidden"
                type="file"
                :accept="IMAGE_UPLOAD_ACCEPT"
                @change="handleLogoInputChange"
              />
            </div>

            <div class="grid content-start gap-4">
              <AdminFormField label="站点名称">
                <InputText
                  v-model="brandingForm.project_name"
                  autocomplete="off"
                  maxlength="40"
                  placeholder="IdeaBoard"
                  :disabled="brandingLoading || brandingSaving"
                  @keyup.enter="handleSaveBranding()"
                />
              </AdminFormField>

              <AdminFormField label="Logo 地址">
                <InputText
                  v-model="brandingForm.logo_url"
                  autocomplete="off"
                  maxlength="2048"
                  placeholder="/uploads/site/assets/logo.png"
                  :disabled="brandingLoading || brandingSaving || uploading"
                  @keyup.enter="handleSaveBranding()"
                />
              </AdminFormField>
            </div>
          </div>

          <div :class="settingActionRowClass">
            <Button
              :class="adminActionButtonClass"
              label="保存"
              severity="contrast"
              :disabled="brandingLoading || brandingSaving || uploading"
              :loading="brandingSaving"
              @click="handleSaveBranding"
            />
            <Button
              :class="adminActionButtonClass"
              label="重置"
              severity="secondary"
              outlined
              :disabled="brandingLoading || brandingSaving || uploading"
              @click="resetBrandingForm"
            />
          </div>
        </div>
      </template>
    </Card>

    <Card :class="adminMasonryCardClass" :pt="adminCardPt">
      <template #title>
        <div class="flex min-w-0 items-center justify-between gap-3">
          <span class="truncate text-admin-title-sm font-bold text-admin-title-color">
            注册与 API 限流
          </span>
          <Tag
            :value="registrationEnabled ? '开放注册' : '关闭注册'"
            class="shrink-0"
            severity="secondary"
            rounded
          />
        </div>
      </template>

      <template #content>
        <div :class="settingCardBodyClass">
          <div :class="settingToggleRowClass">
            <span class="text-admin-body font-bold text-admin-text-1">开放注册</span>
            <ToggleSwitch
              v-model="registrationEnabled"
              :disabled="registrationSettingLoading"
              aria-label="开放注册"
              @update:model-value="handleToggleRegistration"
            />
          </div>

          <div class="grid gap-4 md:grid-cols-2">
            <AdminFormField label="最大请求 (次)">
              <InputNumber
                v-model="rateLimitForm.max_requests"
                :min="1"
                :max="RATE_LIMIT_MAX_REQUESTS"
                fluid
                :disabled="rateLimitLoading || rateLimitSaving"
              />
            </AdminFormField>

            <AdminFormField label="窗口周期 (秒)">
              <InputNumber
                v-model="rateLimitForm.window_seconds"
                :min="1"
                :max="RATE_LIMIT_MAX_WINDOW_SECONDS"
                fluid
                :disabled="rateLimitLoading || rateLimitSaving"
              />
            </AdminFormField>
          </div>

          <div :class="settingActionRowClass">
            <Button
              :class="adminActionButtonClass"
              label="保存"
              severity="contrast"
              :disabled="rateLimitLoading || rateLimitSaving"
              :loading="rateLimitSaving"
              @click="handleSaveRateLimit"
            />
            <Button
              :class="adminActionButtonClass"
              label="重置"
              severity="secondary"
              outlined
              :disabled="rateLimitLoading || rateLimitSaving"
              @click="resetRateLimitForm"
            />
          </div>
        </div>
      </template>
    </Card>

    <Card :class="adminMasonryCardClass" :pt="adminCardPt">
      <template #title>
        <div class="flex min-w-0 items-center justify-between gap-3">
          <span class="truncate text-admin-title-sm font-bold text-admin-title-color">
            任务监控展示
          </span>
          <Tag
            :value="taskMonitorForm.auto_refresh_enabled ? '自动刷新' : '手动刷新'"
            class="shrink-0"
            severity="secondary"
            rounded
          />
        </div>
      </template>

      <template #content>
        <div :class="settingCardBodyClass">
          <div class="grid gap-4 sm:grid-cols-2">
            <AdminFormField label="任务列表每页数量">
              <InputNumber
                v-model="taskMonitorForm.list_page_size"
                :min="TASK_MONITOR_MIN_PAGE_SIZE"
                :max="TASK_MONITOR_MAX_PAGE_SIZE"
                fluid
                :disabled="taskMonitorLoading || taskMonitorSaving"
              />
            </AdminFormField>

            <AdminFormField label="刷新间隔 (秒)">
              <InputNumber
                v-model="taskMonitorForm.refresh_interval_seconds"
                :min="TASK_MONITOR_MIN_REFRESH_INTERVAL_SECONDS"
                :max="TASK_MONITOR_MAX_REFRESH_INTERVAL_SECONDS"
                fluid
                :disabled="taskMonitorLoading || taskMonitorSaving"
              />
            </AdminFormField>
          </div>

          <div class="grid gap-3 sm:grid-cols-2">
            <div :class="settingToggleRowClass">
              <span class="text-admin-body font-bold text-admin-text-1">自动刷新</span>
              <ToggleSwitch
                v-model="taskMonitorForm.auto_refresh_enabled"
                :disabled="taskMonitorLoading || taskMonitorSaving"
                aria-label="自动刷新"
              />
            </div>

            <div :class="settingToggleRowClass">
              <span class="text-admin-body font-bold text-admin-text-1">结果预览</span>
              <ToggleSwitch
                v-model="taskMonitorForm.result_preview_enabled"
                :disabled="taskMonitorLoading || taskMonitorSaving"
                aria-label="结果预览"
              />
            </div>
          </div>

          <div :class="settingActionRowClass">
            <Button
              :class="adminActionButtonClass"
              label="保存"
              severity="contrast"
              :disabled="taskMonitorLoading || taskMonitorSaving"
              :loading="taskMonitorSaving"
              @click="handleSaveTaskMonitorSettings"
            />
            <Button
              :class="adminActionButtonClass"
              label="重置"
              severity="secondary"
              outlined
              :disabled="taskMonitorLoading || taskMonitorSaving"
              @click="resetTaskMonitorForm"
            />
          </div>
        </div>
      </template>
    </Card>

    <Card :class="adminMasonryCardClass" :pt="adminCardPt">
      <template #title>
        <div class="flex min-w-0 items-center justify-between gap-3">
          <span class="truncate text-admin-title-sm font-bold text-admin-title-color">
            链接与域名
          </span>
          <Tag :value="publicURLStatusLabel" class="shrink-0" severity="secondary" rounded />
        </div>
      </template>

      <template #content>
        <div :class="settingCardBodyClass">
          <div class="grid gap-4">
            <AdminFormField label="前端访问地址">
              <InputText
                v-model="publicURLForm.public_base_url"
                autocomplete="off"
                maxlength="2048"
                placeholder="https://app.example.com"
                :disabled="publicURLLoading || publicURLSaving"
                @keyup.enter="handleSavePublicURLSettings()"
              />
            </AdminFormField>

            <AdminFormField label="素材访问地址">
              <InputText
                v-model="publicURLForm.public_asset_base_url"
                autocomplete="off"
                maxlength="2048"
                placeholder="https://api.example.com"
                :disabled="publicURLLoading || publicURLSaving"
                @keyup.enter="handleSavePublicURLSettings()"
              />
            </AdminFormField>
          </div>

          <div :class="settingActionRowClass">
            <Button
              :class="adminActionButtonClass"
              label="保存"
              severity="contrast"
              :disabled="publicURLLoading || publicURLSaving"
              :loading="publicURLSaving"
              @click="handleSavePublicURLSettings"
            />
            <Button
              :class="adminActionButtonClass"
              label="重置"
              severity="secondary"
              outlined
              :disabled="publicURLLoading || publicURLSaving"
              @click="resetPublicURLForm"
            />
          </div>
        </div>
      </template>
    </Card>

    <Card :class="adminMasonryCardClass" :pt="adminCardPt">
      <template #title>
        <div class="flex min-w-0 items-center justify-between gap-3">
          <span class="truncate text-admin-title-sm font-bold text-admin-title-color">
            上传并发
          </span>
          <Tag
            :value="`并发 ${mediaUploadForm.upload_max_concurrent}`"
            class="shrink-0"
            severity="secondary"
            rounded
          />
        </div>
      </template>

      <template #content>
        <div :class="settingCardBodyClass">
          <AdminFormField label="上传并发上限">
            <InputNumber
              v-model="mediaUploadForm.upload_max_concurrent"
              :min="MEDIA_UPLOAD_MIN_CONCURRENT"
              :max="MEDIA_UPLOAD_MAX_CONCURRENT"
              fluid
              :disabled="mediaUploadLoading || mediaUploadSaving"
            />
          </AdminFormField>

          <div :class="settingActionRowClass">
            <Button
              :class="adminActionButtonClass"
              label="保存"
              severity="contrast"
              :disabled="mediaUploadLoading || mediaUploadSaving"
              :loading="mediaUploadSaving"
              @click="handleSaveMediaUploadSettings"
            />
            <Button
              :class="adminActionButtonClass"
              label="重置"
              severity="secondary"
              outlined
              :disabled="mediaUploadLoading || mediaUploadSaving"
              @click="resetMediaUploadForm"
            />
          </div>
        </div>
      </template>
    </Card>
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
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import Card from 'primevue/card'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import { IMAGE_UPLOAD_ACCEPT } from '../../../../shared/constants/media'
import { isSupportedUploadImageFile } from '../../../../shared/utils/imageUploadSupport'
import { useAdminFeedback } from '../../feedback/useAdminFeedback'
import AdminFormField from '../../shared/components/AdminFormField.vue'
import AdminLogoCropDialog from '../../shared/components/AdminLogoCropDialog.vue'
import AdminResourceGridFrame from '../../shared/components/AdminResourceGridFrame.vue'
import {
  RATE_LIMIT_MAX_REQUESTS,
  RATE_LIMIT_MAX_WINDOW_SECONDS,
  useAdminAccessSettings,
} from '../../system-settings/useAdminAccessSettings'
import { useAdminBrandingSettings } from '../../system-settings/useAdminBrandingSettings'
import {
  MEDIA_UPLOAD_MAX_CONCURRENT,
  MEDIA_UPLOAD_MIN_CONCURRENT,
  useAdminMediaUploadSettings,
} from '../../system-settings/useAdminMediaUploadSettings'
import {
  TASK_MONITOR_MAX_PAGE_SIZE,
  TASK_MONITOR_MAX_REFRESH_INTERVAL_SECONDS,
  TASK_MONITOR_MIN_PAGE_SIZE,
  TASK_MONITOR_MIN_REFRESH_INTERVAL_SECONDS,
} from '../../system-settings/taskMonitorSettingsResource'
import { useAdminTaskMonitorSettings } from '../../system-settings/useAdminTaskMonitorSettings'
import { useAdminPublicURLSettings } from '../../system-settings/useAdminPublicURLSettings'
import { useTransientScrolling } from '../../shared/composables/useTransientScrolling'
import { adminCardPt } from '../../shared/lib/adminCardPt'
import {
  adminActionButtonClass,
  adminMasonryCardClass,
  adminTwoColumnMasonryGridClass,
} from '../../shared/lib/adminUiClasses'
import SiteBrandLogo from '../../../../shared/components/SiteBrandLogo.vue'
import { brandMarkClasses, createBrandMarkClass } from '../../../../shared/lib/brandMark'

const settingCardBodyClass = 'flex flex-col gap-5'
const settingActionRowClass = 'flex flex-wrap items-center gap-2 pt-1'
const settingToggleRowClass =
  'flex min-h-12 items-center justify-between gap-4 rounded-lg border border-admin-border bg-admin-surface-muted px-3.5 py-3'
const brandingLogoButtonClass = createBrandMarkClass(
  brandMarkClasses.sizes.fill,
  brandMarkClasses.states.fallbackSoft,
  'hover:border-admin-border-hover hover:bg-admin-surface-muted focus-visible:outline-none focus-visible:shadow-[0_0_0_3px_var(--color-admin-focus-ring)] disabled:cursor-not-allowed disabled:opacity-60',
)

const adminFeedback = useAdminFeedback()
const {
  isScrolling: isSystemSettingsPanelScrolling,
  handleScroll: handleSystemSettingsPanelScroll,
} = useTransientScrolling()
const logoFileInputRef = ref<HTMLInputElement | null>(null)
const logoCropDialogVisible = ref(false)
const pendingLogoFile = ref<File | null>(null)
const {
  clearLogo: clearBrandingLogo,
  fallbackText: brandingFallbackText,
  form: brandingForm,
  handleSaveBranding,
  handleUploadLogo,
  loading: brandingLoading,
  logoPreviewURL: brandingLogoPreviewURL,
  resetBrandingForm,
  saving: brandingSaving,
  uploading,
} = useAdminBrandingSettings({ feedback: adminFeedback.systemSettings.branding })
const {
  handleSaveRateLimit,
  handleToggleRegistration,
  rateLimitLoading,
  rateLimitForm,
  rateLimitSaving,
  registrationEnabled,
  registrationSettingLoading,
  resetRateLimitForm,
} = useAdminAccessSettings({ feedback: adminFeedback.systemSettings.access })
const {
  form: taskMonitorForm,
  handleSaveTaskMonitorSettings,
  loading: taskMonitorLoading,
  resetTaskMonitorForm,
  saving: taskMonitorSaving,
} = useAdminTaskMonitorSettings({ feedback: adminFeedback.systemSettings.taskMonitor })
const {
  form: publicURLForm,
  handleSavePublicURLSettings,
  loading: publicURLLoading,
  resetPublicURLForm,
  saving: publicURLSaving,
} = useAdminPublicURLSettings({ feedback: adminFeedback.systemSettings.publicURL })
const {
  form: mediaUploadForm,
  handleSaveMediaUploadSettings,
  loading: mediaUploadLoading,
  resetMediaUploadForm,
  saving: mediaUploadSaving,
} = useAdminMediaUploadSettings({ feedback: adminFeedback.systemSettings.mediaUpload })

const publicURLStatusLabel = computed(() => {
  const hasPublicBaseURL = Boolean(publicURLForm.value.public_base_url.trim())
  const hasPublicAssetBaseURL = Boolean(publicURLForm.value.public_asset_base_url.trim())
  if (hasPublicBaseURL && hasPublicAssetBaseURL) return '已配置'
  if (hasPublicBaseURL || hasPublicAssetBaseURL) return '部分配置'
  return '使用默认'
})

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
    adminFeedback.systemSettings.unsupportedLogoFile()
    return
  }
  pendingLogoFile.value = file
  logoCropDialogVisible.value = true
}

async function handleLogoCropConfirm(file: File) {
  const uploaded = await handleUploadLogo(file)
  if (!uploaded) return
  logoCropDialogVisible.value = false
  pendingLogoFile.value = null
}
</script>
