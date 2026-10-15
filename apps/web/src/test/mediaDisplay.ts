import type { MediaDisplay } from '../core/media/mediaDisplay'

export function mediaDisplayFixture(key = 'fixture'): MediaDisplay {
  return {
    variants: [
      { tier: 'thumb', max_side: 256, url: `/api/media/previews/${key}/thumb` },
      { tier: 'preview', max_side: 640, url: `/api/media/previews/${key}/preview` },
      { tier: 'large-preview', max_side: 1280, url: `/api/media/previews/${key}/large-preview` },
    ],
  }
}
