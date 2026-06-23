<template>
  <AdminManagementFrame :content-class="systemSettingsContentClass">
    <div class="grid w-full items-start gap-5 lg:grid-cols-2">
      <Card class="!rounded-lg" :pt="adminCardPt">
        <template #title>
          <div class="flex min-w-0 items-center justify-between gap-3">
            <span class="truncate text-admin-title-sm font-bold text-admin-title-color">
              项目品牌
            </span>
            <Tag
              :value="brandingForm.logo_url ? '已配置 Logo' : '默认标识'"
              severity="secondary"
              rounded
            />
          </div>
        </template>

        <template #content>
          <div class="grid gap-5 md:grid-cols-[148px_minmax(0,1fr)]">
            <div class="flex min-w-0 flex-col gap-3">
              <div
                class="flex size-28 items-center justify-center overflow-hidden rounded-xl border border-black bg-white text-admin-accent-pressed"
              >
                <img
                  v-if="brandingForm.logo_url"
                  :src="brandingForm.logo_url"
                  class="h-full w-full object-contain p-2"
                  alt="项目 Logo"
                />
                <span v-else class="text-xl font-[850] tracking-normal">
                  {{ brandingFallbackText }}
                </span>
              </div>

              <input
                ref="logoFileInputRef"
                class="hidden"
                type="file"
                :accept="IMAGE_UPLOAD_ACCEPT"
                @change="handleLogoInputChange"
              />

              <Button
                class="!h-9 !rounded-lg"
                label="上传 Logo"
                severity="contrast"
                :disabled="brandingLoading || brandingSaving || uploading"
                :loading="uploading"
                @click="openLogoPicker"
              >
                <template #icon>
                  <Upload class="size-4" />
                </template>
              </Button>
              <Button
                v-if="brandingForm.logo_url"
                class="!h-9 !rounded-lg"
                label="清除 Logo"
                severity="contrast"
                text
                :disabled="brandingLoading || brandingSaving || uploading"
                @click="clearBrandingLogo"
              >
                <template #icon>
                  <Trash class="size-4" />
                </template>
              </Button>
            </div>

            <div class="flex min-w-0 flex-col gap-4">
              <label class="flex min-w-0 flex-col gap-1.5">
                <span class="text-admin-meta font-[680] text-admin-text-2">项目名称</span>
                <InputText
                  v-model="brandingForm.project_name"
                  autocomplete="off"
                  maxlength="40"
                  placeholder="IdeaBoard"
                  :disabled="brandingLoading || brandingSaving"
                  @keyup.enter="handleSaveBranding()"
                />
              </label>

              <label class="flex min-w-0 flex-col gap-1.5">
                <span class="text-admin-meta font-[680] text-admin-text-2">Logo 地址</span>
                <InputText
                  v-model="brandingForm.logo_url"
                  autocomplete="off"
                  maxlength="2048"
                  placeholder="/uploads/admin/logo.png"
                  :disabled="brandingLoading || brandingSaving || uploading"
                  @keyup.enter="handleSaveBranding()"
                />
              </label>

              <div class="flex flex-wrap items-center gap-2 pt-1">
                <Button
                  class="!h-9 !rounded-lg"
                  label="保存"
                  severity="contrast"
                  :disabled="brandingLoading || brandingSaving || uploading"
                  :loading="brandingSaving"
                  @click="handleSaveBranding"
                >
                  <template #icon>
                    <Check class="size-4" />
                  </template>
                </Button>
              </div>
            </div>
          </div>
        </template>
      </Card>

      <Card class="!rounded-lg" :pt="adminCardPt">
        <template #title>
          <div class="flex min-w-0 items-center justify-between gap-3">
            <span class="truncate text-admin-title-sm font-bold text-admin-title-color">
              注册与 API 限流
            </span>
            <Tag :value="registrationEnabled ? '开放注册' : '关闭注册'" severity="secondary" rounded />
          </div>
        </template>

        <template #content>
          <div class="flex flex-col gap-5">
            <div
              class="flex items-center justify-between gap-4 rounded-lg border border-admin-border bg-admin-surface-muted px-3.5 py-3"
            >
              <span class="text-admin-body font-bold text-admin-text-1">开放注册</span>
              <ToggleSwitch
                v-model="registrationEnabled"
                :disabled="registrationSettingLoading"
                aria-label="开放注册"
                @update:model-value="handleToggleRegistration"
              />
            </div>

            <div class="grid gap-4 md:grid-cols-2">
              <label class="flex min-w-0 flex-col gap-1.5">
                <span class="text-admin-meta font-[680] text-admin-text-2">最大请求 (次)</span>
                <InputNumber
                  v-model="rateLimitForm.max_requests"
                  :min="1"
                  :max="RATE_LIMIT_MAX_REQUESTS"
                  fluid
                  :disabled="rateLimitLoading || rateLimitSaving"
                />
              </label>

              <label class="flex min-w-0 flex-col gap-1.5">
                <span class="text-admin-meta font-[680] text-admin-text-2">窗口周期 (秒)</span>
                <InputNumber
                  v-model="rateLimitForm.window_seconds"
                  :min="1"
                  :max="RATE_LIMIT_MAX_WINDOW_SECONDS"
                  fluid
                  :disabled="rateLimitLoading || rateLimitSaving"
                />
              </label>
            </div>

            <div class="flex flex-wrap items-center gap-2">
              <Button
                class="!h-9 !rounded-lg"
                label="保存访问限流"
                severity="contrast"
                :disabled="rateLimitLoading || rateLimitSaving"
                :loading="rateLimitSaving"
                @click="handleSaveRateLimit"
              >
                <template #icon>
                  <Check class="size-4" />
                </template>
              </Button>
            </div>
          </div>
        </template>
      </Card>

      <Card class="!rounded-lg" :pt="adminCardPt">
        <template #title>
          <div class="flex min-w-0 items-center justify-between gap-3">
            <span class="truncate text-admin-title-sm font-bold text-admin-title-color">
              任务监控展示
            </span>
            <Tag
              :value="taskMonitorForm.auto_refresh_enabled ? '自动刷新' : '手动刷新'"
              severity="secondary"
              rounded
            />
          </div>
        </template>

        <template #content>
          <div class="grid gap-5">
            <div class="grid gap-4 sm:grid-cols-2">
              <label class="flex min-w-0 flex-col gap-1.5">
                <span class="text-admin-meta font-[680] text-admin-text-2">
                  任务列表每页数量
                </span>
                <InputNumber
                  v-model="taskMonitorForm.list_page_size"
                  :min="TASK_MONITOR_MIN_PAGE_SIZE"
                  :max="TASK_MONITOR_MAX_PAGE_SIZE"
                  fluid
                  :disabled="taskMonitorLoading || taskMonitorSaving"
                />
              </label>

              <label class="flex min-w-0 flex-col gap-1.5">
                <span class="text-admin-meta font-[680] text-admin-text-2">刷新间隔 (秒)</span>
                <InputNumber
                  v-model="taskMonitorForm.refresh_interval_seconds"
                  :min="TASK_MONITOR_MIN_REFRESH_INTERVAL_SECONDS"
                  :max="TASK_MONITOR_MAX_REFRESH_INTERVAL_SECONDS"
                  fluid
                  :disabled="taskMonitorLoading || taskMonitorSaving"
                />
              </label>
            </div>

            <div class="grid gap-3">
              <div
                class="flex items-center justify-between gap-4 rounded-lg border border-admin-border bg-admin-surface-muted px-3.5 py-3"
              >
                <span class="text-admin-body font-bold text-admin-text-1">自动刷新</span>
                <ToggleSwitch
                  v-model="taskMonitorForm.auto_refresh_enabled"
                  :disabled="taskMonitorLoading || taskMonitorSaving"
                  aria-label="自动刷新"
                />
              </div>

              <div
                class="flex items-center justify-between gap-4 rounded-lg border border-admin-border bg-admin-surface-muted px-3.5 py-3"
              >
                <span class="text-admin-body font-bold text-admin-text-1">结果预览</span>
                <ToggleSwitch
                  v-model="taskMonitorForm.result_preview_enabled"
                  :disabled="taskMonitorLoading || taskMonitorSaving"
                  aria-label="结果预览"
                />
              </div>

              <Button
                class="!h-9 !rounded-lg"
                label="保存任务设置"
                severity="contrast"
                :disabled="taskMonitorLoading"
                :loading="taskMonitorSaving"
                @click="handleSaveTaskMonitorSettings"
              >
                <template #icon>
                  <Check class="size-4" />
                </template>
              </Button>
            </div>
          </div>
        </template>
      </Card>

      <Card class="!rounded-lg" :pt="adminCardPt">
        <template #title>
          <div class="flex min-w-0 items-center justify-between gap-3">
            <span class="truncate text-admin-title-sm font-bold text-admin-title-color">
              链接与域名
            </span>
            <Tag :value="publicURLStatusLabel" severity="secondary" rounded />
          </div>
        </template>

        <template #content>
          <div class="grid gap-5">
            <label class="flex min-w-0 flex-col gap-1.5">
              <span class="text-admin-meta font-[680] text-admin-text-2">前端访问地址</span>
              <InputText
                v-model="publicURLForm.public_base_url"
                autocomplete="off"
                maxlength="2048"
                placeholder="https://app.example.com"
                :disabled="publicURLLoading || publicURLSaving"
                @keyup.enter="handleSavePublicURLSettings()"
              />
            </label>

            <label class="flex min-w-0 flex-col gap-1.5">
              <span class="text-admin-meta font-[680] text-admin-text-2">素材访问地址</span>
              <InputText
                v-model="publicURLForm.public_asset_base_url"
                autocomplete="off"
                maxlength="2048"
                placeholder="https://api.example.com"
                :disabled="publicURLLoading || publicURLSaving"
                @keyup.enter="handleSavePublicURLSettings()"
              />
            </label>

            <div class="flex flex-wrap items-center gap-2">
              <Button
                class="!h-9 !rounded-lg"
                label="保存链接"
                severity="contrast"
                :disabled="publicURLLoading"
                :loading="publicURLSaving"
                @click="handleSavePublicURLSettings"
              >
                <template #icon>
                  <Check class="size-4" />
                </template>
              </Button>
            </div>
          </div>
        </template>
      </Card>

      <Card class="!rounded-lg" :pt="adminCardPt">
        <template #title>
          <div class="flex min-w-0 items-center justify-between gap-3">
            <span class="truncate text-admin-title-sm font-bold text-admin-title-color">
              上传并发
            </span>
            <Tag :value="`并发 ${mediaUploadForm.upload_max_concurrent}`" severity="secondary" rounded />
          </div>
        </template>

        <template #content>
          <div class="grid gap-5">
            <label class="flex min-w-0 flex-col gap-1.5">
              <span class="text-admin-meta font-[680] text-admin-text-2">上传并发上限</span>
              <InputNumber
                v-model="mediaUploadForm.upload_max_concurrent"
                :min="MEDIA_UPLOAD_MIN_CONCURRENT"
                :max="MEDIA_UPLOAD_MAX_CONCURRENT"
                fluid
                :disabled="mediaUploadLoading || mediaUploadSaving"
              />
            </label>

            <div class="flex flex-wrap items-center gap-2">
              <Button
                class="!h-9 !rounded-lg"
                label="保存上传设置"
                severity="contrast"
                :disabled="mediaUploadLoading"
                :loading="mediaUploadSaving"
                @click="handleSaveMediaUploadSettings"
              >
                <template #icon>
                  <Check class="size-4" />
                </template>
              </Button>
            </div>
          </div>
        </template>
      </Card>
    </div>
  </AdminManagementFrame>
</template>

<script setup lang="ts">
import { Check, Trash, Upload } from '@iconoir/vue'
import Button from 'primevue/button'
import Card from 'primevue/card'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import { IMAGE_UPLOAD_ACCEPT } from '../../../../shared/constants/media'
import { useAppToast } from '../../../../shared/composables/useAppToast'
import AdminManagementFrame from '../../shared/components/AdminManagementFrame.vue'
import {
  RATE_LIMIT_MAX_REQUESTS,
  RATE_LIMIT_MAX_WINDOW_SECONDS,
  useAdminAccessSettings,
} from '../../shared/composables/useAdminAccessSettings'
import { useAdminBrandingSettings } from '../../shared/composables/useAdminBrandingSettings'
import {
  MEDIA_UPLOAD_MAX_CONCURRENT,
  MEDIA_UPLOAD_MIN_CONCURRENT,
  useAdminMediaUploadSettings,
} from '../../shared/composables/useAdminMediaUploadSettings'
import {
  TASK_MONITOR_MAX_PAGE_SIZE,
  TASK_MONITOR_MAX_REFRESH_INTERVAL_SECONDS,
  TASK_MONITOR_MIN_PAGE_SIZE,
  TASK_MONITOR_MIN_REFRESH_INTERVAL_SECONDS,
  useAdminTaskMonitorSettings,
} from '../../shared/composables/useAdminTaskMonitorSettings'
import { useAdminPublicURLSettings } from '../../shared/composables/useAdminPublicURLSettings'
import { adminCardPt } from '../../shared/lib/adminCardPt'

const systemSettingsContentClass = 'p-6'

const appToast = useAppToast()
const logoFileInputRef = ref<HTMLInputElement | null>(null)
const {
  clearLogo: clearBrandingLogo,
  fallbackText: brandingFallbackText,
  form: brandingForm,
  handleSaveBranding,
  handleUploadLogo,
  loading: brandingLoading,
  saving: brandingSaving,
  uploading,
} = useAdminBrandingSettings({ message: appToast })
const {
  handleSaveRateLimit,
  handleToggleRegistration,
  rateLimitLoading,
  rateLimitForm,
  rateLimitSaving,
  registrationEnabled,
  registrationSettingLoading,
} = useAdminAccessSettings({ message: appToast })
const {
  form: taskMonitorForm,
  handleSaveTaskMonitorSettings,
  loading: taskMonitorLoading,
  saving: taskMonitorSaving,
} = useAdminTaskMonitorSettings({ message: appToast })
const {
  form: publicURLForm,
  handleSavePublicURLSettings,
  loading: publicURLLoading,
  saving: publicURLSaving,
} = useAdminPublicURLSettings({ message: appToast })
const {
  form: mediaUploadForm,
  handleSaveMediaUploadSettings,
  loading: mediaUploadLoading,
  saving: mediaUploadSaving,
} = useAdminMediaUploadSettings({ message: appToast })

const publicURLStatusLabel = computed(() => {
  const hasPublicBaseURL = Boolean(publicURLForm.value.public_base_url.trim())
  const hasPublicAssetBaseURL = Boolean(publicURLForm.value.public_asset_base_url.trim())
  if (hasPublicBaseURL && hasPublicAssetBaseURL) return '已配置'
  if (hasPublicBaseURL || hasPublicAssetBaseURL) return '部分配置'
  return '使用默认'
})

function openLogoPicker() {
  logoFileInputRef.value?.click()
}

async function handleLogoInputChange(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  await handleUploadLogo(file)
}
</script>
