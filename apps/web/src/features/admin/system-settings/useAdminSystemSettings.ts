import { useAdminAccessSettings } from './useAdminAccessSettings'
import { useAdminBrandingSettings } from './useAdminBrandingSettings'
import { useAdminMediaUploadSettings } from './useAdminMediaUploadSettings'
import {
  useAdminAccessSettingsFeedback,
  useAdminBrandingSettingsFeedback,
  useAdminMediaUploadSettingsFeedback,
  useAdminUnsupportedLogoFileFeedback,
} from './useAdminSystemSettingsFeedback'

export function useAdminSystemSettings() {
  const branding = useAdminBrandingSettings({ feedback: useAdminBrandingSettingsFeedback() })
  const access = useAdminAccessSettings({ feedback: useAdminAccessSettingsFeedback() })
  const mediaUpload = useAdminMediaUploadSettings({
    feedback: useAdminMediaUploadSettingsFeedback(),
  })

  return {
    access,
    branding,
    mediaUpload,
    unsupportedLogoFileFeedback: useAdminUnsupportedLogoFileFeedback(),
  }
}
