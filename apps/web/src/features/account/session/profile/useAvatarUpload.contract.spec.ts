import { describe, expect, it, vi } from 'vitest'
import { useAvatarUpload, type AvatarUploadDependencies } from './useAvatarUpload'

describe('useAvatarUpload', () => {
  it('clears the input value and returns when no file is selected', async () => {
    const deps = createDeps()
    const workflow = useAvatarUpload(deps)
    const input = createFileInput()

    await workflow.handleFileChange({ target: input } as unknown as Event)

    expect(input.value).toBe('')
    expect(deps.uploadAvatar).not.toHaveBeenCalled()
  })

  it('warns for non-image files without uploading', async () => {
    const deps = createDeps()
    const workflow = useAvatarUpload(deps)

    await workflow.selectFile(new File(['x'], 'note.txt', { type: 'text/plain' }))

    expect(deps.profileFeedback.invalidAvatarFile).toHaveBeenCalled()
    expect(deps.uploadAvatar).not.toHaveBeenCalled()
  })

  it('uploads images, trims avatar URL, updates local user, and shows success', async () => {
    const deps = createDeps({
      uploadAvatar: vi.fn(async () => ({
        avatar: ' https://cdn.example/avatar.png ',
      })),
    })
    const workflow = useAvatarUpload(deps)
    const file = new File(['image'], 'avatar.png', { type: 'image/png' })

    await workflow.selectFile(file)

    expect(deps.uploadAvatar).toHaveBeenCalledWith(file)
    expect(deps.userStore.setAvatarLocal).toHaveBeenCalledWith('https://cdn.example/avatar.png')
    expect(deps.profileFeedback.avatarUpload.success).toHaveBeenCalled()
    expect(workflow.isUploading.value).toBe(false)
  })
})

type TestDeps = AvatarUploadDependencies
type TestUserStore = NonNullable<TestDeps['userStore']>
type TestProfileFeedback = NonNullable<TestDeps['profileFeedback']>

function createDeps(
  overrides: Partial<{
    uploadAvatar: NonNullable<TestDeps['uploadAvatar']>
    setAvatarLocal: TestUserStore['setAvatarLocal']
    avatarUploadSuccess: NonNullable<TestProfileFeedback['avatarUpload']['success']>
    avatarUploadError: NonNullable<TestProfileFeedback['avatarUpload']['error']>
    invalidAvatarFile: TestProfileFeedback['invalidAvatarFile']
    runAsyncAction: NonNullable<TestDeps['runAsyncAction']>
  }> = {},
): Required<TestDeps> {
  const defaultRunAsyncAction: NonNullable<TestDeps['runAsyncAction']> = async (
    action,
    options = {},
  ) => {
    if (options.loading) options.loading.value = true
    try {
      const result = await action()
      options.feedback?.success?.(result)
      return result
    } catch (error) {
      options.feedback?.error?.(error)
      return undefined
    } finally {
      if (options.loading) options.loading.value = false
    }
  }

  return {
    uploadAvatar:
      overrides.uploadAvatar ?? vi.fn(async () => ({ avatar: 'https://cdn.example/a.png' })),
    userStore: {
      setAvatarLocal: overrides.setAvatarLocal ?? vi.fn(),
    },
    profileFeedback: {
      invalidAvatarFile: overrides.invalidAvatarFile ?? vi.fn(),
      avatarUpload: {
        success: overrides.avatarUploadSuccess ?? vi.fn(),
        error: overrides.avatarUploadError ?? vi.fn(),
      },
    },
    runAsyncAction: overrides.runAsyncAction ?? defaultRunAsyncAction,
  }
}

function createFileInput() {
  return {
    files: [],
    value: 'selected',
  }
}
