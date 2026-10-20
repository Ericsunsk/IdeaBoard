import { describe, expect, test, vi } from 'vitest'
import type { User } from '../../../../api/sessionUserType'
import { makeSessionUser } from '../../../../test/factories'
import { useDisplayNameRename, type DisplayNameRenameDependencies } from './useDisplayNameRename'

describe('useDisplayNameRename', () => {
  test.each(['logout', 'identity switch', 'same-user new session'])(
    'discards an old rename response after %s',
    async (transition) => {
      let resolveRename!: (user: User) => void
      const deps = createDeps({
        rename: vi.fn(
          () =>
            new Promise<User>((resolve) => {
              resolveRename = resolve
            }),
        ),
      })
      const workflow = useDisplayNameRename(deps)
      workflow.startRename()
      workflow.draftName.value = 'Old session name'
      const pending = workflow.commitRename()

      deps.isCurrentSession.mockReturnValue(false)
      deps.userStore.user =
        transition === 'logout'
          ? null
          : makeSessionUser({
              public_id:
                transition === 'identity switch' ? 'other-user' : deps.userStore.user!.public_id,
              display_name: 'Current session name',
            })
      resolveRename(makeSessionUser({ display_name: 'Old session name' }))
      await pending

      expect(deps.userStore.setAuth).not.toHaveBeenCalled()
      expect(deps.profileFeedback.displayNameRename.success).not.toHaveBeenCalled()
      expect(deps.profileFeedback.displayNameRename.error).not.toHaveBeenCalled()
      expect(workflow.draftName.value).toBe(deps.userStore.user?.display_name ?? '')
      expect(workflow.isEditing.value).toBe(false)
      expect(workflow.isRenaming.value).toBe(false)
    },
  )

  test('ignores failure feedback from an expired rename session', async () => {
    let rejectRename!: (error: Error) => void
    const deps = createDeps({
      rename: vi.fn(
        () =>
          new Promise<User>((_resolve, reject) => {
            rejectRename = reject
          }),
      ),
    })
    const workflow = useDisplayNameRename(deps)
    workflow.startRename()
    workflow.draftName.value = 'New'
    const pending = workflow.commitRename()
    deps.isCurrentSession.mockReturnValue(false)
    deps.userStore.user = null
    rejectRename(new Error('expired'))
    await pending

    expect(deps.profileFeedback.displayNameRename.error).not.toHaveBeenCalled()
    expect(deps.userStore.setAuth).not.toHaveBeenCalled()
    expect(workflow.isEditing.value).toBe(false)
  })

  test('renames successfully, refreshes the session user, and closes the editor', async () => {
    const deps = createDeps({
      rename: vi.fn(async () => makeSessionUser({ display_name: 'New' })),
    })
    const workflow = useDisplayNameRename(deps)

    workflow.startRename()
    expect(workflow.isEditing.value).toBe(true)
    workflow.draftName.value = '  New  '
    await workflow.commitRename()

    expect(deps.rename).toHaveBeenCalledWith('New')
    expect(deps.userStore.setAuth).toHaveBeenCalledWith(
      expect.objectContaining({ display_name: 'New' }),
    )
    expect(deps.profileFeedback.displayNameRename.success).toHaveBeenCalled()
    expect(workflow.isEditing.value).toBe(false)
    expect(workflow.isRenaming.value).toBe(false)
  })

  test('rejects an empty name without touching the transport', async () => {
    const deps = createDeps()
    const workflow = useDisplayNameRename(deps)

    workflow.startRename()
    workflow.draftName.value = '   '
    await workflow.commitRename()

    expect(deps.rename).not.toHaveBeenCalled()
    expect(deps.profileFeedback.invalidDisplayName).toHaveBeenCalledWith(
      'Display name cannot be empty.',
    )
    expect(deps.userStore.setAuth).not.toHaveBeenCalled()
    expect(workflow.isEditing.value).toBe(true)
  })

  test('saves the full 100-code-point name', async () => {
    const name = '😀'.repeat(100)
    const deps = createDeps({
      rename: vi.fn(async () => makeSessionUser({ display_name: name })),
    })
    const workflow = useDisplayNameRename(deps)
    workflow.startRename()
    workflow.draftName.value = name

    await workflow.commitRename()

    expect(deps.rename).toHaveBeenCalledWith(name)
    expect(deps.userStore.setAuth).toHaveBeenCalledWith(
      expect.objectContaining({ display_name: name }),
    )
    expect(workflow.isEditing.value).toBe(false)
  })

  test('rejects 101 code points before submitting without truncating the draft', async () => {
    const name = '😀'.repeat(101)
    const deps = createDeps()
    const workflow = useDisplayNameRename(deps)
    workflow.startRename()
    workflow.draftName.value = name

    await workflow.commitRename()

    expect(deps.rename).not.toHaveBeenCalled()
    expect(deps.profileFeedback.invalidDisplayName).toHaveBeenCalledWith(
      'Display name must be no more than 100 characters.',
    )
    expect(workflow.draftName.value).toBe(name)
    expect(workflow.isEditing.value).toBe(true)
  })

  test('rolls the draft back to the stored name when the rename fails', async () => {
    const failure = new Error('boom')
    const deps = createDeps({ rename: vi.fn(async () => Promise.reject(failure)) })
    const workflow = useDisplayNameRename(deps)

    workflow.startRename()
    workflow.draftName.value = 'New'
    await workflow.commitRename()

    expect(deps.rename).toHaveBeenCalledWith('New')
    expect(deps.profileFeedback.displayNameRename.error).toHaveBeenCalledWith(failure)
    expect(deps.userStore.setAuth).not.toHaveBeenCalled()
    expect(workflow.draftName.value).toBe('Ada')
    expect(workflow.isEditing.value).toBe(true)
    expect(workflow.isRenaming.value).toBe(false)
  })

  test('closes the editor without a rename when the name is unchanged', async () => {
    const deps = createDeps()
    const workflow = useDisplayNameRename(deps)

    workflow.startRename()
    workflow.draftName.value = 'Ada'
    await workflow.commitRename()

    expect(deps.rename).not.toHaveBeenCalled()
    expect(workflow.isEditing.value).toBe(false)
  })
})

type TestDeps = DisplayNameRenameDependencies
type TestUserStore = NonNullable<TestDeps['userStore']>
type TestProfileFeedback = NonNullable<TestDeps['profileFeedback']>

function createDeps(
  overrides: Partial<{
    rename: NonNullable<TestDeps['rename']>
    displayName: string
    setAuth: TestUserStore['setAuth']
    invalidDisplayName: TestProfileFeedback['invalidDisplayName']
    renameSuccess: NonNullable<TestProfileFeedback['displayNameRename']['success']>
    renameError: NonNullable<TestProfileFeedback['displayNameRename']['error']>
  }> = {},
): {
  rename: NonNullable<TestDeps['rename']>
  userStore: TestUserStore
  profileFeedback: TestProfileFeedback
  isCurrentSession: ReturnType<typeof vi.fn<() => boolean>>
} {
  const currentUser = makeSessionUser({ display_name: overrides.displayName ?? 'Ada' })
  const isCurrentSession = vi.fn(() => true)
  return {
    isCurrentSession,
    rename: overrides.rename ?? vi.fn(async () => currentUser),
    userStore: {
      user: currentUser,
      captureSession: () => isCurrentSession,
      setAuth: overrides.setAuth ?? vi.fn(),
    },
    profileFeedback: {
      invalidDisplayName: overrides.invalidDisplayName ?? vi.fn(),
      displayNameRename: {
        success: overrides.renameSuccess ?? vi.fn(),
        error: overrides.renameError ?? vi.fn(),
      },
    },
  }
}
