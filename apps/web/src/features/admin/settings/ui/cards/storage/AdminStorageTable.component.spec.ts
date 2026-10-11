import { describe, expect, it } from 'vitest'
import { renderWithUser, screen } from '../../../../../../test/render'
import AdminStorageTable, { type StorageTableRow } from './AdminStorageTable.vue'
import type { StorageCard } from '../../../core/storage/profiles'

function makeStorage(overrides: Partial<StorageCard> & { id: string; name: string }): StorageCard {
  return {
    provider: 's3-compatible',
    enabled: true,
    isCurrent: false,
    builtin: false,
    secretConfigured: true,
    config: {},
    ...overrides,
  }
}

function makeRow(storage: StorageCard): StorageTableRow {
  return {
    id: storage.id,
    isPresetOnly: false,
    storage,
    name: storage.name,
    isCurrent: false,
    enabled: storage.enabled,
    health: 'unverified',
    bucket: 'images',
    endpoint: 's3.example.com',
    supportsTest: false,
    canTest: false,
    canApply: true,
  }
}

const primary = makeStorage({ id: 'storage-1', name: '主存储' })
const backup = makeStorage({ id: 'storage-2', name: '备存储' })

function renderTable(isToggling?: (id: string) => boolean) {
  return renderWithUser(AdminStorageTable, {
    props: {
      rows: [makeRow(primary), makeRow(backup)],
      loading: false,
      applyingId: null,
      testingId: null,
      isToggling,
      activeLogoTargetId: null,
    },
  })
}

describe('AdminStorageTable toggling', () => {
  it('disables only the toggling row instead of the whole column', () => {
    renderTable((id) => id === 'storage-1')

    expect(screen.getByRole('switch', { name: 'Enable 主存储' })).toBeDisabled()
    expect(screen.getByRole('switch', { name: 'Enable 备存储' })).not.toBeDisabled()
  })

  it('disables the row reported by isToggling', () => {
    renderWithUser(AdminStorageTable, {
      props: {
        rows: [makeRow(primary), makeRow(backup)],
        loading: false,
        applyingId: null,
        testingId: null,
        isToggling: (id: string) => id === 'storage-2',
        activeLogoTargetId: null,
      },
    })

    expect(screen.getByRole('switch', { name: 'Enable 主存储' })).not.toBeDisabled()
    expect(screen.getByRole('switch', { name: 'Enable 备存储' })).toBeDisabled()
  })

  it('keeps every switch interactive when idle', () => {
    renderTable()

    expect(screen.getByRole('switch', { name: 'Enable 主存储' })).not.toBeDisabled()
    expect(screen.getByRole('switch', { name: 'Enable 备存储' })).not.toBeDisabled()
  })

  it('still emits toggle events from an idle row', async () => {
    const { user, emitted } = renderTable()

    await user.click(screen.getByRole('switch', { name: 'Enable 备存储' }))
    expect(emitted()['toggle-enabled']).toEqual([[backup, false]])
  })
})
