import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import {
  MediaWorkspaceUnavailableError,
  requireMediaWorkspaceId,
  resolveMediaAssetDerivativeURL,
  resolveMediaAssetOriginalURL,
  resolveWorkspaceMediaAssetDerivativeURL,
  resolveWorkspaceMediaAssetOriginalURL,
} from './mediaAssetUrls'
import {
  resetMediaWorkspaceIdResolver,
  setMediaWorkspaceIdResolver,
} from './mediaWorkspaceScope'

const WORKSPACE_ID = 'ws_test123'
const ASSET_ID = 'asset_uid42'

describe('mediaAssetUrls', () => {
  beforeEach(() => {
    setMediaWorkspaceIdResolver(() => WORKSPACE_ID)
  })

  afterEach(() => {
    resetMediaWorkspaceIdResolver()
  })

  it('resolves a stable v1 content URL for an asset', () => {
    expect(resolveMediaAssetOriginalURL(ASSET_ID)).toBe(
      `/api/v1/workspaces/${WORKSPACE_ID}/media-assets/${ASSET_ID}/content`,
    )
  })

  it('resolves a format-agnostic v1 derivative URL per tier', () => {
    expect(resolveMediaAssetDerivativeURL(ASSET_ID, 'preview')).toBe(
      `/api/v1/workspaces/${WORKSPACE_ID}/media-assets/${ASSET_ID}/derivatives/preview`,
    )
    expect(resolveMediaAssetDerivativeURL(ASSET_ID)).toBe(
      `/api/v1/workspaces/${WORKSPACE_ID}/media-assets/${ASSET_ID}/derivatives/large-preview`,
    )
  })

  it('resolves explicit-workspace URLs without the selection store', () => {
    expect(resolveWorkspaceMediaAssetOriginalURL(WORKSPACE_ID, ASSET_ID)).toBe(
      `/api/v1/workspaces/${WORKSPACE_ID}/media-assets/${ASSET_ID}/content`,
    )
    expect(resolveWorkspaceMediaAssetDerivativeURL(WORKSPACE_ID, ASSET_ID, 'thumb')).toBe(
      `/api/v1/workspaces/${WORKSPACE_ID}/media-assets/${ASSET_ID}/derivatives/thumb`,
    )
  })

  it('fails URL builders closed without a workspace selection', () => {
    setMediaWorkspaceIdResolver(() => null)
    expect(() => resolveMediaAssetOriginalURL(ASSET_ID)).toThrow(MediaWorkspaceUnavailableError)
    expect(() => resolveMediaAssetDerivativeURL(ASSET_ID)).toThrow(MediaWorkspaceUnavailableError)
    expect(() => requireMediaWorkspaceId()).toThrow(MediaWorkspaceUnavailableError)
  })

  it('prefers an explicit workspace id over the selection store', () => {
    expect(requireMediaWorkspaceId('ws_explicit')).toBe('ws_explicit')
  })
})
