import { useMutation } from '@tanstack/vue-query'
import { computed } from 'vue'
import { uploadAdminAsset } from '../../../shared/api/generated-query/admin-assets/admin-assets'
import {
  getGetAdminBrandingSettingQueryKey,
  useGetAdminBrandingSetting,
} from '../../../shared/api/generated-query/admin-settings/admin-settings'
import type { BrandingSetting } from '../../../shared/api/generated-query/model'
import { requireApiData } from '../../../shared/api/apiResponse'
import type { AdminBrandingSettingsFeedback } from '../feedback/useAdminFeedback'
import {
  isSupportedUploadImageFile,
  unsupportedUploadImageMessage,
} from '../../../shared/utils/imageUploadSupport'
import {
  DEFAULT_SITE_BRANDING_SETTING,
  SITE_BRANDING_QUERY_KEY,
  getSiteBrandFallbackText,
  normalizeSiteBrandingSetting,
  resolveSiteBrandingLogoURL,
  validateSiteBrandingSetting,
  writeSiteBrandingQueryCache,
} from '../../../shared/composables/useSiteBranding'
import { updateBrandingSetting } from './systemSettingsCommands'
import { useSettingResourceLifecycle } from './useSettingFormLifecycle'

export function useAdminBrandingSettings(options: { feedback: AdminBrandingSettingsFeedback }) {
  const brandingQuery = useGetAdminBrandingSetting()
  const sourceSetting = computed(() => brandingQuery.data.value?.data)
  const resource = useSettingResourceLifecycle<BrandingSetting>({
    defaultSetting: DEFAULT_SITE_BRANDING_SETTING,
    extraInvalidateQueryKeys: [SITE_BRANDING_QUERY_KEY],
    feedback: options.feedback,
    isSame: isSameBrandingSetting,
    normalize: normalizeSiteBrandingSetting,
    query: brandingQuery,
    queryKey: getGetAdminBrandingSettingQueryKey(),
    save: updateBrandingSetting,
    source: sourceSetting,
    validate: (setting) => validateSiteBrandingSetting(normalizeSiteBrandingSetting(setting)),
    writeAdditionalQueryCache: writeSiteBrandingQueryCache,
  })

  const uploadLogoMutation = useMutation({
    mutationFn: uploadBrandingLogoFile,
    onSuccess: (logoURL) => {
      resource.form.value.logo_url = normalizeSiteBrandingSetting({
        ...resource.form.value,
        logo_url: logoURL,
      }).logo_url
    },
  })

  const uploading = computed(() => uploadLogoMutation.isPending.value)
  const fallbackText = computed(() => getSiteBrandFallbackText(resource.form.value.project_name))
  const logoPreviewURL = computed(() => resolveSiteBrandingLogoURL(resource.form.value.logo_url))

  async function handleSaveBranding() {
    await resource.save()
  }

  async function handleUploadLogo(file: File) {
    try {
      await uploadLogoMutation.mutateAsync(file)
      options.feedback.logoUploaded()
      return true
    } catch (error) {
      options.feedback.logoUploadFailed(error)
      return false
    }
  }

  function clearLogo() {
    resource.form.value.logo_url = ''
  }

  function resetBrandingForm() {
    resource.reset()
  }

  return {
    clearLogo,
    fallbackText,
    form: resource.form,
    handleSaveBranding,
    handleUploadLogo,
    loading: resource.loading,
    logoPreviewURL,
    resetBrandingForm,
    saving: resource.saving,
    uploading,
  }
}

function isSameBrandingSetting(current: BrandingSetting, synced: BrandingSetting): boolean {
  return current.project_name === synced.project_name && current.logo_url === synced.logo_url
}

async function uploadBrandingLogoFile(file: File): Promise<string> {
  if (!isSupportedUploadImageFile(file)) {
    throw new Error(unsupportedUploadImageMessage())
  }

  const data = requireApiData(await uploadAdminAsset({ file }), '上传 Logo 失败')
  if (!data.url) throw new Error('上传 Logo 失败')
  return data.url
}
