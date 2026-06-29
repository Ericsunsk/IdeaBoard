import { describe, expect, it, vi } from 'vitest'
import {
  useAvatarUploadWorkflow,
  type AvatarUploadWorkflowDependencies,
} from './useAvatarUploadWorkflow'

describe('useAvatarUploadWorkflow', () => {
  it('clears the input value and returns when no file is selected', async () => {
    const deps = createDeps()
    const workflow = useAvatarUploadWorkflow(deps)
    const input = createFileInput()

    await workflow.handleFileChange({ target: input } as unknown as Event)

    expect(input.value).toBe('')
    expect(deps.uploadAvatar).not.toHaveBeenCalled()
  })

  it('warns for non-image files without uploading', async () => {
    const deps = createDeps()
    const workflow = useAvatarUploadWorkflow(deps)

    await workflow.selectFile(new File(['x'], 'note.txt', { type: 'text/plain' }))

    expect(deps.appToast.warning).toHaveBeenCalledWith('仅支持上传图片文件')
    expect(deps.uploadAvatar).not.toHaveBeenCalled()
  })

  it('throws through runAsyncAction when uploaded avatar URL is empty after trim', async () => {
    const capturedErrors: string[] = []
    const deps = createDeps({
      uploadAvatar: vi.fn(async () => ({ code: 0, msg: 'ok', data: { avatar: '   ' } })),
      runAsyncAction: async (action) => {
        try {
          return await action()
        } catch (error) {
          capturedErrors.push(error instanceof Error ? error.message : String(error))
          return undefined
        }
      },
    })
    const workflow = useAvatarUploadWorkflow(deps)

    await workflow.selectFile(new File(['image'], 'avatar.png', { type: 'image/png' }))

    expect(capturedErrors).toEqual(['头像上传失败，请稍后重试'])
    expect(deps.userStore.setAvatarLocal).not.toHaveBeenCalled()
  })

  it('uploads images, trims avatar URL, updates local user, and shows success', async () => {
    const deps = createDeps({
      uploadAvatar: vi.fn(async () => ({
        code: 0,
        msg: 'ok',
        data: { avatar: ' https://cdn.example/avatar.png ' },
      })),
    })
    const workflow = useAvatarUploadWorkflow(deps)
    const file = new File(['image'], 'avatar.png', { type: 'image/png' })

    await workflow.selectFile(file)

    expect(deps.uploadAvatar).toHaveBeenCalledWith(file)
    expect(deps.userStore.setAvatarLocal).toHaveBeenCalledWith('https://cdn.example/avatar.png')
    expect(deps.appToast.success).toHaveBeenCalledWith('头像已更新')
    expect(workflow.uploading.value).toBe(false)
  })
})

type TestDeps = AvatarUploadWorkflowDependencies
type TestUserStore = NonNullable<TestDeps['userStore']>
type TestToast = NonNullable<TestDeps['appToast']>

function createDeps(overrides: Partial<{
  uploadAvatar: NonNullable<TestDeps['uploadAvatar']>
  setAvatarLocal: TestUserStore['setAvatarLocal']
  success: TestToast['success']
  warning: TestToast['warning']
  runAsyncAction: NonNullable<TestDeps['runAsyncAction']>
}> = {}): Required<TestDeps> {
  const defaultRunAsyncAction: NonNullable<TestDeps['runAsyncAction']> = async (
    action,
    options = {},
  ) => {
    if (options.loading) options.loading.value = true
    try {
      return await action()
    } finally {
      if (options.loading) options.loading.value = false
    }
  }

  return {
    uploadAvatar:
      overrides.uploadAvatar ??
      vi.fn(async () => ({ code: 0, msg: 'ok', data: { avatar: 'https://cdn.example/a.png' } })),
    userStore: {
      setAvatarLocal: overrides.setAvatarLocal ?? vi.fn(),
    },
    appToast: {
      success: overrides.success ?? vi.fn(),
      warning: overrides.warning ?? vi.fn(),
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
