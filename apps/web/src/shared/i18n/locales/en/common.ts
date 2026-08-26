/**
 * Shared English copy used across User-facing surfaces.
 * Keys are stable identifiers; never use sentences as keys.
 */
export default {
  actions: {
    cancel: 'Cancel',
    delete: 'Delete',
    retry: 'Retry',
    loading: 'Loading…',
  },
  states: {
    noData: 'No data',
    empty: 'Nothing here yet',
  },
  errors: {
    imageLoadFailed: 'The image could not be loaded. Try again.',
    requestIncomplete: 'The request could not be completed.',
    sessionLoadFailed: 'Could not load your session. Please sign in again.',
    taskSubmitFailed: 'Could not submit the task. Please try again.',
    taskLoadFailed: 'Could not load the task.',
    taskCancelFailed: 'Could not cancel the task.',
    billingInfoUnavailable: 'Billing information is unavailable right now.',
    uploadFailed: 'Upload failed. Please try again.',
    uploadProcessingFailed: 'The uploaded image could not be processed. Try again.',
    avatarUploadFailed: 'Avatar upload failed. Please try again.',
    unsupportedImageFormat: 'That image format is not supported.',
    mediaInvalid: 'That media reference is invalid.',
    mediaInfoLoadFailed: 'Could not load media info.',
    actionUnavailable: 'This action is unavailable right now.',
    readImageSizeFailed: 'Could not read the image size.',
    parseImageFailed: 'The image content could not be parsed.',
    compressInvalidOutput: 'Compression produced invalid output.',
  },
  a11y: {
    chartTimeRange: 'Chart time range',
  },
  impersonation: {
    activeBanner: 'Impersonation preview active',
    exit: 'Exit impersonation',
  },
} as const
