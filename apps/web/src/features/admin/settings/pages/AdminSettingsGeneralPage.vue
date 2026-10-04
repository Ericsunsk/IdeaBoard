<template>
  <AdminSettingsTable
    :title="SETTINGS_MESSAGES.siteTitle"
    :subtitle="SETTINGS_MESSAGES.siteSubtitle"
    :table-aria-label="SETTINGS_MESSAGES.siteTableAria"
    :scroll-aria-label="SETTINGS_MESSAGES.siteTableScrollAria"
    :rows="settingRows"
  >
    <template #summary="{ data }">
      <span class="font-mono text-admin-meta text-muted-foreground">{{ data.summary }}</span>
    </template>

    <template #expansion="{ data }">
      <div class="border-t border-border bg-card p-4 md:p-5">
        <SettingEditorCard
          v-if="data.id === 'branding'"
          :title="SETTINGS_MESSAGES.brandingTitle"
          :status="
            brandingForm.logo_url ? SETTINGS_MESSAGES.customLogo : SETTINGS_MESSAGES.defaultBrand
          "
          :status-severity="brandingForm.logo_url ? 'info' : 'secondary'"
          class="mb-0"
        >
          <div :class="settingCardBodyClass">
            <div class="grid gap-5 md:grid-cols-[140px_minmax(0,1fr)]">
              <div class="flex min-w-0 flex-col gap-2">
                <span class="text-admin-meta font-[680] text-muted-foreground">{{
                  SETTINGS_MESSAGES.logoLabel
                }}</span>
                <div class="group/logo relative size-28">
                  <button
                    :class="brandingLogoButtonClass"
                    type="button"
                    :disabled="brandingLoading || brandingSaving || uploading"
                    :aria-label="SETTINGS_MESSAGES.uploadLogo"
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
                    class="absolute right-1.5 top-1.5 grid size-5 place-items-center rounded-full border border-border bg-card text-muted-foreground leading-none shadow-xs transition-colors hover:text-destructive focus-visible:outline-none disabled:cursor-not-allowed cursor-pointer"
                    :class="
                      brandingLoading || brandingSaving || uploading
                        ? 'opacity-40'
                        : 'opacity-0 group-hover/logo:opacity-100 group-focus-within/logo:opacity-100 focus-visible:opacity-100'
                    "
                    type="button"
                    :title="SETTINGS_MESSAGES.clearLogo"
                    :aria-label="SETTINGS_MESSAGES.clearLogo"
                    :disabled="brandingLoading || brandingSaving || uploading"
                    @mousedown.stop
                    @pointerdown.stop
                    @click.stop="clearBrandingLogo"
                  >
                    <Xmark class="size-2.5" />
                  </button>
                </div>

                <p class="text-admin-caption text-muted-foreground">{{ SETTINGS_MESSAGES.logoHint }}</p>

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
                    :disabled="brandingLoading || brandingSaving"
                    @keyup.enter="saveBranding()"
                  />
                </AdminFormField>

                <AdminFormField
                  :label="SETTINGS_MESSAGES.logoUrlLabel"
                  for="admin-branding-logo-url"
                >
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
              {{ SETTINGS_MESSAGES.save }}
            </AdminButton>
            <AdminButton
              :class="adminActionButtonClass"
              variant="outline"
              :disabled="brandingLoading || brandingSaving || uploading"
              @click="resetBranding"
            >
              {{ SETTINGS_MESSAGES.reset }}
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
              {{ SETTINGS_MESSAGES.retry }}
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
import { i18n } from '../../../../shared/i18n'
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
import { SETTINGS_MESSAGES } from './messages'

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
  const projectName = form?.project_name || SETTINGS_MESSAGES.defaultProjectName
  return {
    id: 'branding',
    title: SETTINGS_MESSAGES.brandingTitle,
    description: SETTINGS_MESSAGES.brandingDescription,
    icon: Globe,
    status: hasCustomLogo ? SETTINGS_MESSAGES.customLogo : SETTINGS_MESSAGES.defaultBrand,
    statusSeverity: hasCustomLogo ? 'info' : 'secondary',
    summary: SETTINGS_MESSAGES.brandingSummary(
      projectName,
      hasCustomLogo ? SETTINGS_MESSAGES.logoConfigured : SETTINGS_MESSAGES.logoUnset,
    ),
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
  if (config.test_state === 'passed') return SETTINGS_MESSAGES.searchPassed
  if (config.test_state === 'failed') return SETTINGS_MESSAGES.searchFailedHint
  return SETTINGS_MESSAGES.searchUntested
}

function buildSearchProviderRows(): AdminSettingsTableRow[] {
  return SEARCH_PROVIDERS.map((provider) => {
    const config = searchConfigByProvider.value[provider]
    if (!config) {
      return {
        id: provider,
        title: SETTINGS_MESSAGES.imageSourceTitle(SEARCH_PROVIDER_LABELS[provider]),
        description: SETTINGS_MESSAGES.imageSourceDescription(SEARCH_PROVIDER_LABELS[provider]),
        icon: Search,
        status: searchLoadError.value
          ? SETTINGS_MESSAGES.providerLoadFailed
          : SETTINGS_MESSAGES.providerLoading,
        statusSeverity: searchLoadError.value ? 'danger' : 'secondary',
        summary: searchLoadError.value
          ? SETTINGS_MESSAGES.providerConfigLoadFailed
          : SETTINGS_MESSAGES.providerConfigLoading,
      }
    }
    if (config.enabled) {
      return {
        id: provider,
        title: SETTINGS_MESSAGES.imageSourceTitle(SEARCH_PROVIDER_LABELS[provider]),
        description: SETTINGS_MESSAGES.imageSourceDescription(SEARCH_PROVIDER_LABELS[provider]),
        icon: Search,
        status: SETTINGS_MESSAGES.providerEnabled,
        statusSeverity: 'success',
        summary: searchProviderSummary(config),
      }
    }
    return {
      id: provider,
      title: SETTINGS_MESSAGES.imageSourceTitle(SEARCH_PROVIDER_LABELS[provider]),
      description: SETTINGS_MESSAGES.imageSourceDescription(SEARCH_PROVIDER_LABELS[provider]),
      icon: Search,
      status: config.api_key_configured
        ? SETTINGS_MESSAGES.providerDisabled
        : SETTINGS_MESSAGES.providerUnconfigured,
      statusSeverity: config.api_key_configured ? 'secondary' : 'warn',
      summary: config.api_key_configured
        ? searchProviderSummary(config)
        : SETTINGS_MESSAGES.apiKeyMissing,
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
    searchLoadError.value = formatAdminDisplayError(
      err,
      SETTINGS_MESSAGES.searchProvidersLoadFailed,
    )
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

  // Warning copy is unchanged (same keys, now resolved by the caller).
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
