/// <reference types="node" />

import { existsSync } from 'node:fs'
import { readFile } from 'node:fs/promises'
import path from 'node:path'
import { describe, expect, it } from 'vitest'

const featureRoot = path.resolve(process.cwd(), 'src/features')
const accountRoot = path.join(featureRoot, 'account')
const overlaySourcePath = path.join(accountRoot, 'application/overlay.ts')
const legacyControllerPath = path.join(accountRoot, 'useAccountManagementDialogController.ts')
const workspaceLayoutSourcePath = path.join(featureRoot, 'workspace/layout/WorkspaceLayout.vue')
const workspaceAccountControlsSourcePath = path.join(
  featureRoot,
  'workspace/shell/useWorkspaceAccountControls.ts',
)

describe('Account overlay ownership contract', () => {
  it('has no compatibility singleton or hidden host-owned controller', () => {
    expect(existsSync(legacyControllerPath)).toBe(false)
  })

  it('is explicitly installed by Workspace Layout and consumed through the feature seam', async () => {
    const [overlaySource, workspaceLayoutSource, workspaceAccountControlsSource] =
      await Promise.all([
        readFile(overlaySourcePath, 'utf8'),
        readFile(workspaceLayoutSourcePath, 'utf8'),
        readFile(workspaceAccountControlsSourcePath, 'utf8'),
      ])

    expect(overlaySource).toMatch(/export function createAccountOverlay\(/)
    expect(overlaySource).toMatch(/export function provideAccountOverlay\(/)
    expect(overlaySource).toMatch(/export function useAccountOverlay\(/)
    expect(workspaceLayoutSource).toMatch(/provideAccountOverlay\(accountOverlay\)/)
    expect(workspaceAccountControlsSource).toMatch(/useAccountOverlay\(\)/)
    expect(workspaceAccountControlsSource).not.toMatch(/openAccountManagementDialog/)
  })
})
