<template>
  <AdminSettingsTable
    title="网站设置"
    subtitle="站点名称、品牌标识与图片搜索供应商配置"
    table-aria-label="网站设置列表"
    scroll-aria-label="网站设置列表横向滚动区域"
    :rows="settingRows"
  >
    <template #summary="{ data }">
      <span class="font-mono text-admin-meta text-admin-text-2">{{ data.summary }}</span>
    </template>

    <template #expansion="{ data }">
      <div class="border-t border-admin-border-soft bg-admin-bg p-4 md:p-5">
        <SettingEditorCard
          v-if="data.id === 'branding'"
          title="站点设置"
          :status="brandingForm.logo_url ? '自定义 Logo' : '默认标识'"
          :status-severity="brandingForm.logo_url ? 'info' : 'secondary'"
          class="mb-0"
        >
          <div :class="settingCardBodyClass">
            <div class="grid gap-5 md:grid-cols-[140px_minmax(0,1fr)]">
              <div class="flex min-w-0 flex-col gap-2">
                <span class="text-admin-meta font-[680] text-admin-text-2">站点 Logo</span>
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
                      fallback-class="select-none text-center text-xl leading-none font-bold tracking-normal"
                      :class="brandMarkClasses.logo"
                    />
                  </button>

                  <button
                    v-if="brandingForm.logo_url"
                    class="absolute right-1.5 top-1.5 grid size-5 place-items-center rounded-full border border-admin-border-soft bg-white text-admin-text-2 leading-none shadow-xs transition-colors hover:text-admin-danger focus-visible:outline-none disabled:cursor-not-allowed cursor-pointer"
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

                <p class="text-admin-caption text-admin-text-3">支持 PNG、SVG、JPG，透明底为佳</p>

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
                  <AdminInput
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
                  <AdminInput
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
          <template #actions>
            <AdminButton
              :class="adminActionButtonClass"
              :disabled="brandingLoading || brandingSaving || uploading"
              :loading="brandingSaving"
              @click="saveBranding"
            >
              保存
            </AdminButton>
            <AdminButton
              :class="adminActionButtonClass"
              variant="outline"
              :disabled="brandingLoading || brandingSaving || uploading"
              @click="resetBranding"
            >
              重置
            </AdminButton>
          </template>
        </SettingEditorCard>
        <template v-else-if="resolveSearchConfig(data.id)">
          <AdminSearchProviderCard
            :provider="resolveSearchProviderId(data.id)!"
            :config="resolveSearchConfig(data.id)!"
            :resource="searchResource"
            class="mb-0"
            @change="loadSearchProviders"
          />
        </template>
        <AdminAlert v-else-if="searchLoadError" severity="error">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <span>{{ searchLoadError }}</span>
            <AdminButton
              :class="adminActionButtonClass"
              variant="outline"
              size="small"
              @click="loadSearchProviders"
            >
              重试
            </AdminButton>
          </div>
        </AdminAlert>
      </div>
    </template>

    <template #after>
      <AdminLogoCropDialog
        v-model:visible="logoCropDialogVisible"
        :busy="uploading"
        :file="pendingLogoFile"
        @confirm="handleLogoCropConfirm"
      />
    </template>
  </AdminSettingsTable>
</template>

<script setup lang="ts">
import { Globe, Search, Xmark } from '@iconoir/vue'
import { computed, onMounted, ref, watch } from 'vue'
import AdminAlert from '../../shared/components/AdminAlert.vue'
import AdminButton from '../../shared/components/AdminButton.vue'
import AdminFormField from '../../shared/components/AdminFormField.vue'
import AdminInput from '../../shared/components/AdminInput.vue'
import AdminLogoCropDialog from '../../shared/components/AdminLogoCropDialog.vue'
import AdminSearchProviderCard from '../AdminSearchProviderCard.vue'
import AdminSettingsTable, {
  type AdminSettingsTableRow,
} from '../components/AdminSettingsTable.vue'
import SiteBrandLogo from '../../../../shared/components/SiteBrandLogo.vue'
import { brandMarkClasses } from '../../../../shared/lib/brandMark'
import { IMAGE_UPLOAD_ACCEPT } from '../../../../shared/constants/media'
import { prepareImageAssetForUpload } from '../../../../shared/media/imageAssetPipeline'
import { isSupportedUploadImageFile } from '../../../../shared/utils/imageUploadSupport'
import { formatAdminDisplayError } from '../../shared/lib/toast'
import SettingEditorCard from '../SettingEditorCard.vue'
import {
  createAdminSearchProviderResource,
  type AdminSearchProviderResource,
} from '../searchProviderResource'
import {
  SEARCH_PROVIDERS,
  SEARCH_PROVIDER_LABELS,
  type SearchProviderConfig,
  type SearchProviderId,
} from '../searchProviderTypes'
import { useBrandingSettings } from '../useBrandingSettings'
import {
  useAdminBrandingSettingsFeedback,
  useAdminUnsupportedLogoFileFeedback,
} from '../useSettingsFeedback'
import { useAdminConfirm } from '../../shared/composables/useAdminConfirm'
import { adminLogoButtonClass, adminActionButtonClass } from '../../shared/lib/uiClasses'

const settingCardBodyClass = 'flex flex-col gap-5'
const brandingLogoButtonClass = [adminLogoButtonClass, 'size-28 relative'].join(' ')

const branding = useBrandingSettings({
  feedback: useAdminBrandingSettingsFeedback(),
  confirm: useAdminConfirm(),
})
const unsupportedLogoFileFeedback = useAdminUnsupportedLogoFileFeedback()
const logoFileInputRef = ref<HTMLInputElement | null>(null)
const logoCropDialogVisible = ref(false)
const pendingLogoFile = ref<File | null>(null)

const {
  form: brandingForm,
  loading: brandingLoading,
  logoPreviewURL: brandingLogoPreviewURL,
  saving: brandingSaving,
  uploading,
  clearLogo: clearBrandingLogo,
  fallbackText: brandingFallbackText,
  reset: resetBranding,
  save: saveBranding,
  handleUploadLogo: setCustomLogoFile,
} = branding

function buildBrandingRow(form: typeof brandingForm.value): AdminSettingsTableRow {
  const hasCustomLogo = Boolean(form?.logo_url)
  const projectName = form?.project_name || '默认 (IdeaBoard)'
  return {
    id: 'branding',
    title: '站点设置',
    description: '站点名称与主品牌 Logo 标识',
    icon: Globe,
    status: hasCustomLogo ? '自定义 Logo' : '默认标识',
    statusSeverity: hasCustomLogo ? 'info' : 'secondary',
    summary: `名称: ${projectName} · Logo: ${hasCustomLogo ? '已配置' : '默认'}`,
  }
}

const settingRows = computed<AdminSettingsTableRow[]>(() => [
  buildBrandingRow(brandingForm.value),
  ...buildSearchProviderRows(),
])

const searchResource: AdminSearchProviderResource = createAdminSearchProviderResource()
const searchConfigs = ref<SearchProviderConfig[]>([])
const searchLoadError = ref('')

const searchConfigByProvider = computed<Partial<Record<SearchProviderId, SearchProviderConfig>>>(
  () => Object.fromEntries(searchConfigs.value.map((config) => [config.provider, config])),
)

function resolveSearchProviderId(id: string): SearchProviderId | null {
  return (SEARCH_PROVIDERS as readonly string[]).includes(id) ? (id as SearchProviderId) : null
}

function resolveSearchConfig(id: string): SearchProviderConfig | null {
  const provider = resolveSearchProviderId(id)
  return provider ? (searchConfigByProvider.value[provider] ?? null) : null
}

function searchProviderSummary(config: SearchProviderConfig): string {
  if (config.test_state === 'passed') return '连接正常，可启用或停用'
  if (config.test_state === 'failed') return '上次连接测试失败，请检查后重试'
  return '尚未通过连接测试，启用前需先测试'
}

function buildSearchProviderRows(): AdminSettingsTableRow[] {
  return SEARCH_PROVIDERS.map((provider) => {
    const config = searchConfigByProvider.value[provider]
    if (!config) {
      return {
        id: provider,
        title: `${SEARCH_PROVIDER_LABELS[provider]} 图片源`,
        description: `${SEARCH_PROVIDER_LABELS[provider]} 官方 API 接入与可用状态`,
        icon: Search,
        status: searchLoadError.value ? '加载失败' : '加载中',
        statusSeverity: searchLoadError.value ? 'danger' : 'secondary',
        summary: searchLoadError.value ? '读取供应商配置失败' : '正在读取供应商配置',
      }
    }
    if (config.enabled) {
      return {
        id: provider,
        title: `${SEARCH_PROVIDER_LABELS[provider]} 图片源`,
        description: `${SEARCH_PROVIDER_LABELS[provider]} 官方 API 接入与可用状态`,
        icon: Search,
        status: '已启用',
        statusSeverity: 'success',
        summary: searchProviderSummary(config),
      }
    }
    return {
      id: provider,
      title: `${SEARCH_PROVIDER_LABELS[provider]} 图片源`,
      description: `${SEARCH_PROVIDER_LABELS[provider]} 官方 API 接入与可用状态`,
      icon: Search,
      status: config.api_key_configured ? '已停用' : '未配置',
      statusSeverity: config.api_key_configured ? 'secondary' : 'warn',
      summary: config.api_key_configured ? searchProviderSummary(config) : '尚未配置 API Key',
    }
  })
}

async function loadSearchProviders() {
  searchLoadError.value = ''
  try {
    const list = await searchResource.list()
    searchConfigs.value = [...list.providers].sort(
      (left, right) =>
        SEARCH_PROVIDERS.indexOf(left.provider) - SEARCH_PROVIDERS.indexOf(right.provider),
    )
  } catch (err) {
    searchLoadError.value = formatAdminDisplayError(err, '加载图片搜索供应商配置失败')
  }
}

onMounted(() => {
  void loadSearchProviders()
})

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

  const preparedFile = await prepareImageAssetForUpload(file)
  pendingLogoFile.value = preparedFile.uploadFile
  logoCropDialogVisible.value = true
}

async function handleLogoCropConfirm(croppedFile: File) {
  await setCustomLogoFile(croppedFile)
  pendingLogoFile.value = null
}
</script>
