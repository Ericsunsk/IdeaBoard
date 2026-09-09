/// <reference types="node" />

import { existsSync } from 'node:fs'
import { readFile } from 'node:fs/promises'
import path from 'node:path'
import { describe, expect, it } from 'vitest'

const featureRoot = path.resolve(process.cwd(), 'src/features')
const accountRoot = path.join(featureRoot, 'account')
const overlaySourcePath = path.join(accountRoot, 'application/overlay/overlay.ts')
const legacyControllerPath = path.join(accountRoot, 'useAccountManagementDialogController.ts')
const userLayoutSourcePath = path.join(featureRoot, 'user/layout/UserLayout.vue')
const userAccountControlsSourcePath = path.join(featureRoot, 'user-shell/useUserAccountControls.ts')

describe('Account overlay ownership contract', () => {
  it('has no compatibility singleton or hidden host-owned controller', () => {
    expect(existsSync(legacyControllerPath)).toBe(false)
  })

  it('is explicitly installed by User Shell and consumed through the feature seam', async () => {
    const [overlaySource, userLayoutSource, userAccountControlsSource] = await Promise.all([
      readFile(overlaySourcePath, 'utf8'),
      readFile(userLayoutSourcePath, 'utf8'),
      readFile(userAccountControlsSourcePath, 'utf8'),
    ])

    expect(overlaySource).toMatch(/export function createAccountOverlay\(/)
    expect(overlaySource).toMatch(/export function provideAccountOverlay\(/)
    expect(overlaySource).toMatch(/export function useAccountOverlay\(/)
    expect(userLayoutSource).toMatch(/provideAccountOverlay\(accountOverlay\)/)
    expect(userAccountControlsSource).toMatch(/useAccountOverlay\(\)/)
    expect(userAccountControlsSource).not.toMatch(/openAccountManagementDialog/)
  })
})
