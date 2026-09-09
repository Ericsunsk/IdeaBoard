import { useAccessSettings } from './useAccessSettings'
import { useBrandingSettings } from './useBrandingSettings'
import { useMediaUploadSettings } from './useMediaUploadSettings'
import {
  useAdminAccessSettingsFeedback,
  useAdminBrandingSettingsFeedback,
  useAdminMediaUploadSettingsFeedback,
  useAdminUnsupportedLogoFileFeedback,
} from './useSystemSettingsFeedback'

export function useSystemSettings() {
  const branding = useBrandingSettings({ feedback: useAdminBrandingSettingsFeedback() })
  const access = useAccessSettings({ feedback: useAdminAccessSettingsFeedback() })
  const mediaUpload = useMediaUploadSettings({
    feedback: useAdminMediaUploadSettingsFeedback(),
  })

  return {
    access,
    branding,
    mediaUpload,
    unsupportedLogoFileFeedback: useAdminUnsupportedLogoFileFeedback(),
  }
}
