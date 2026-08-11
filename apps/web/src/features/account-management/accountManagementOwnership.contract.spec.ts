/// <reference types="node" />

import { existsSync } from 'node:fs'
import { readFile } from 'node:fs/promises'
import path from 'node:path'
import { describe, expect, it } from 'vitest'

const featureRoot = path.resolve(process.cwd(), 'src/features')
const accountManagementRoot = path.join(featureRoot, 'account-management')
const overlaySourcePath = path.join(accountManagementRoot, 'accountManagementOverlay.ts')
const legacyControllerPath = path.join(
  accountManagementRoot,
  'useAccountManagementDialogController.ts',
)
const userLayoutSourcePath = path.join(featureRoot, 'user/layout/UserLayout.vue')
const userAccountControlsSourcePath = path.join(featureRoot, 'user-shell/useUserAccountControls.ts')

describe('Account Management overlay ownership contract', () => {
  it('has no compatibility singleton or hidden host-owned controller', () => {
    expect(existsSync(legacyControllerPath)).toBe(false)
  })

  it('is explicitly installed by User Shell and consumed through the feature seam', async () => {
    const [overlaySource, userLayoutSource, userAccountControlsSource] = await Promise.all([
      readFile(overlaySourcePath, 'utf8'),
      readFile(userLayoutSourcePath, 'utf8'),
      readFile(userAccountControlsSourcePath, 'utf8'),
    ])

    expect(overlaySource).toMatch(/export function createAccountManagementOverlay\(/)
    expect(overlaySource).toMatch(/export function provideAccountManagementOverlay\(/)
    expect(overlaySource).toMatch(/export function useAccountManagementOverlay\(/)
    expect(userLayoutSource).toMatch(/provideAccountManagementOverlay\(accountManagementOverlay\)/)
    expect(userAccountControlsSource).toMatch(/useAccountManagementOverlay\(\)/)
    expect(userAccountControlsSource).not.toMatch(/openAccountManagementDialog/)
  })
})
