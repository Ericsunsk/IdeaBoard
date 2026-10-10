import { afterEach, describe, expect, it } from 'vitest'
import { i18n } from '../../../core/i18n'
import { SERVICE_SLUGS, type Service } from '../../../core/contracts/imageServices'
import { setUserLocale, type SupportedLocale } from '../../../core/i18n/userLocale'
import { serviceLabelKey } from '../core/nodes/serviceLabelKey'
import { getServiceImageActionText } from './serviceImageActionText'

const labels: Record<SupportedLocale, Record<Service, string>> = {
  en: {
    generation: 'Prompt to Image',
    inpaint: 'Inpaint',
    cutout: 'Cutout',
    fusion: 'Image Fusion',
    render: 'Render',
    variations: 'Variations',
    upscale: 'Upscale',
    outpaint: 'Outpaint',
  },
  'zh-CN': {
    generation: '文生图',
    inpaint: '局部重绘',
    cutout: '抠图',
    fusion: '图像融合',
    render: '渲染',
    variations: '变化衍生',
    upscale: '超分辨率',
    outpaint: '画面扩展',
  },
  es: {
    generation: 'Texto a imagen',
    inpaint: 'Pintar dentro',
    cutout: 'Recorte',
    fusion: 'Fusión de imágenes',
    render: 'Renderizar',
    variations: 'Variaciones',
    upscale: 'Escalar',
    outpaint: 'Extender lienzo',
  },
  ko: {
    generation: '텍스트 투 이미지',
    inpaint: '인페인팅',
    cutout: '컷아웃',
    fusion: '이미지 퓨전',
    render: '렌더',
    variations: '변형',
    upscale: '업스케일',
    outpaint: '아웃페인트',
  },
}

afterEach(async () => {
  await setUserLocale('en')
  localStorage.clear()
})

describe('relocated service labels at call time (i18n/four-locale-service-copy)', () => {
  it('resolves all eight services through the real session seam after every locale switch', async () => {
    expect(SERVICE_SLUGS).toHaveLength(8)
    const snapshots: Record<string, string[]> = {}
    for (const locale of ['en', 'zh-CN', 'es', 'ko', 'en'] as const) {
      await expect(setUserLocale(locale)).resolves.toBe(true)
      snapshots[locale] = SERVICE_SLUGS.map((slug) => {
        expect(serviceLabelKey(slug)).toBe(`canvas.features.${slug}`)
        const text = getServiceImageActionText(slug)
        expect(text).toEqual({ label: labels[locale][slug], title: labels[locale][slug] })
        return text.label
      })
      expect(i18n.global.locale.value).toBe(locale)
    }
    expect(snapshots['en']).not.toEqual(snapshots['zh-CN'])
    expect(snapshots['en']).not.toEqual(snapshots['es'])
    expect(snapshots['en']).not.toEqual(snapshots['ko'])
  })
})
