import '@testing-library/jest-dom/vitest'

// Retouch focus-ring computed-style browser contract.
//
// Observable replacement for the retired
// `retouchShared.contract.spec.ts` "keeps focus rings on motion-safe outline
// tokens without layout shift" source-string assertions
// (`computed-style/3a1e8303`, 5 IDs): every retired constraint needs an
// effective computed-style replacement passing first. Follows the
// board/search P3 precedent (`boardCardMotion.browser.spec.ts`,
// `searchChromeStyle.browser.spec.ts`): assert rendered output — computed
// outline paint, live token provenance and no layout shift — never file text.
//
// Coverage map (ledger
// `src/features/canvas/tools/retouch/ui/retouchShared.contract.spec.ts`,
// scenario "keeps focus rings ...", lines 44-50):
// - histogramPlot --focus-outline-width (source-23d8ff29ef0cb426-1): marker
//   ::after width rides the width token live; button + range-input variants.
// - histogramPlot outline-offset (source-454c4ea7e9100077-1): marker ::after
//   offset lands at 2px on focus with no box change.
// - curves --focus-outline-width (source-9390812c7634177b-1): point width
//   rides the width token live.
// - curves --focus-outline-color (source-4aca935a1c8dc57c-1): point color
//   rides the color token live.
// - layersPanel --focus-outline-offset (source-0b046b21a0683a40-1): row
//   offset rides the offset token live with no box change.
// Reduced motion: outline rings carry no animation, so they still render at
// full width under `prefers-reduced-motion: reduce` while the motion tokens
// collapse to 0ms.

import { commands, page } from 'vitest/browser'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { renderWithUser, screen } from '../../../../../test/render'
import { cleanup } from '@testing-library/vue'
import { i18n } from '../../../../../core/i18n'
import '../../../../../app/style.css'
import './shared/styles/retouch-histogram-plot.css'
import './shared/styles/retouch-curves.css'
import HistogramPlot from './histogram/HistogramPlot.vue'
import CurveGraph from './curves/CurveGraph.vue'
import LayersPanel from './layers/LayersPanel.vue'
import { normalizeCurveChannelPoints } from '../core/kinds/curves'
import type { RetouchLayer } from '../core/model'

declare module 'vitest/internal/browser' {
  interface BrowserCommands {
    emulateMotion: (reducedMotion: 'reduce' | 'no-preference') => Promise<void>
  }
}

function styleOf(element: Element): CSSStyleDeclaration {
  return window.getComputedStyle(element)
}

function afterStyleOf(element: Element): CSSStyleDeclaration {
  return window.getComputedStyle(element, '::after')
}

async function setMotion(isReduced: boolean) {
  await commands.emulateMotion(isReduced ? 'reduce' : 'no-preference')
}

function makeLayers(): RetouchLayer[] {
  return [
    { id: 'source', name: 'Original', type: 'source', visible: true },
    { id: 'hsl', name: 'Hue', type: 'effect', kind: 'hsl', visible: true },
  ] as unknown as RetouchLayer[]
}

describe('Retouch focus-ring browser contract', () => {
  beforeEach(async () => {
    await page.viewport(1280, 800)
    document.documentElement.classList.remove('dark')
    i18n.global.locale.value = 'en'
    await setMotion(false)
  })

  afterEach(async () => {
    cleanup()
    document.body.innerHTML = ''
    await setMotion(false)
  })

  it('keeps histogram marker rings on the shared outline tokens without layout shift', async () => {
    const buttonRendered = renderWithUser(HistogramPlot, {
      props: {
        histogramPath: 'M 0 100 L 50 0 L 100 100 Z',
        markers: [{ id: 'marker', position: 50, ariaLabel: 'Histogram marker' }],
        ariaLabel: 'Histogram controls',
      },
    })
    try {
      const marker = screen.getByRole('slider', { name: 'Histogram marker' }) as HTMLElement
      // Default: no ring on the painted dot.
      expect(afterStyleOf(marker).outlineStyle).toBe('none')
      const restWidth = marker.offsetWidth
      const restDot = Number.parseFloat(afterStyleOf(marker).width)
      marker.focus()
      expect(marker.matches(':focus-visible')).toBe(true)
      // Focus lands the shared ring paint on ::after (the painted dot keeps
      // its 12px box; the transparent button box stays at the AA floor).
      expect(afterStyleOf(marker).outlineStyle).toBe('solid')
      expect(afterStyleOf(marker).outlineWidth).toBe('2px')
      expect(afterStyleOf(marker).outlineOffset).toBe('2px')
      expect(marker.offsetWidth).toBe(restWidth)
      expect(Number.parseFloat(afterStyleOf(marker).width)).toBe(restDot)
      // Provenance: the ring rides the shared tokens live — moving them moves
      // the paint, so a hardcoded marker-side value cannot satisfy this.
      const colorProbe = document.createElement('div')
      colorProbe.style.outlineStyle = 'solid'
      colorProbe.style.outlineColor = 'var(--focus-outline-color)'
      document.body.append(colorProbe)
      try {
        expect(afterStyleOf(marker).outlineColor).toBe(styleOf(colorProbe).outlineColor)
        document.documentElement.style.setProperty('--focus-outline-color', 'rgb(255, 0, 0)')
        try {
          await expect.poll(() => afterStyleOf(marker).outlineColor).toBe('rgb(255, 0, 0)')
        } finally {
          document.documentElement.style.removeProperty('--focus-outline-color')
        }
        await expect
          .poll(() => afterStyleOf(marker).outlineColor)
          .toBe(styleOf(colorProbe).outlineColor)
        document.documentElement.style.setProperty('--focus-outline-width', '5px')
        try {
          await expect.poll(() => afterStyleOf(marker).outlineWidth).toBe('5px')
        } finally {
          document.documentElement.style.removeProperty('--focus-outline-width')
        }
        await expect.poll(() => afterStyleOf(marker).outlineWidth).toBe('2px')
      } finally {
        colorProbe.remove()
      }
    } finally {
      buttonRendered.unmount()
    }
    cleanup()
    document.body.innerHTML = ''
    // The range-input variant mirrors the button ring through the sibling
    // marker (the hidden input itself stays outline-free).
    const rangeRendered = renderWithUser(HistogramPlot, {
      props: {
        histogramPath: 'M 0 100 L 50 0 L 100 100 Z',
        markers: [
          {
            id: 'range',
            position: 50,
            ariaLabel: 'Range marker',
            ariaValueMin: 0,
            ariaValueMax: 100,
            ariaValueNow: 50,
          },
        ],
        markerMode: 'range',
        ariaLabel: 'Histogram controls',
      },
    })
    try {
      const input = document.querySelector('.retouch-histogram-plot__range-input') as HTMLElement
      const dot = document.querySelector('.retouch-histogram-plot__marker--range') as HTMLElement
      expect(input).toBeTruthy()
      expect(afterStyleOf(dot).outlineStyle).toBe('none')
      input.focus()
      expect(input.matches(':focus-visible')).toBe(true)
      // The hidden input keeps no solid outline of its own; the visible ring
      // lives on the sibling marker ::after.
      expect(styleOf(input).outlineStyle).not.toBe('solid')
      expect(afterStyleOf(dot).outlineStyle).toBe('solid')
      expect(afterStyleOf(dot).outlineWidth).toBe('2px')
      expect(afterStyleOf(dot).outlineOffset).toBe('2px')
    } finally {
      rangeRendered.unmount()
    }
  })

  it('keeps curve point rings on the shared outline tokens without layout shift', async () => {
    const points = normalizeCurveChannelPoints([{ id: 'mid', input: 50, output: 50 }])
    const rendered = renderWithUser(CurveGraph, {
      props: {
        activeCurvePath: 'M 0 100 L 100 0',
        ghostCurvePaths: [],
        histogramPath: '',
        points,
        selectedPointId: 'mid',
      },
    })
    try {
      const point = screen.getByRole('button', {
        name: 'Curve point In 50 Out 50',
      }) as HTMLElement
      expect(styleOf(point).outlineStyle).toBe('none')
      const restBorder = styleOf(point).borderWidth
      point.focus()
      expect(point.matches(':focus-visible')).toBe(true)
      expect(styleOf(point).outlineStyle).toBe('solid')
      expect(styleOf(point).outlineWidth).toBe('2px')
      expect(styleOf(point).outlineOffset).toBe('2px')
      // Outline never moves the box: the border and the handle box stay put.
      expect(styleOf(point).borderWidth).toBe(restBorder)
      const colorProbe = document.createElement('div')
      colorProbe.style.outlineStyle = 'solid'
      colorProbe.style.outlineColor = 'var(--focus-outline-color)'
      document.body.append(colorProbe)
      try {
        expect(styleOf(point).outlineColor).toBe(styleOf(colorProbe).outlineColor)
        document.documentElement.style.setProperty('--focus-outline-color', 'rgb(255, 0, 0)')
        try {
          await expect.poll(() => styleOf(point).outlineColor).toBe('rgb(255, 0, 0)')
        } finally {
          document.documentElement.style.removeProperty('--focus-outline-color')
        }
        await expect.poll(() => styleOf(point).outlineColor).toBe(styleOf(colorProbe).outlineColor)
        document.documentElement.style.setProperty('--focus-outline-width', '5px')
        try {
          await expect.poll(() => styleOf(point).outlineWidth).toBe('5px')
        } finally {
          document.documentElement.style.removeProperty('--focus-outline-width')
        }
        await expect.poll(() => styleOf(point).outlineWidth).toBe('2px')
      } finally {
        colorProbe.remove()
      }
    } finally {
      rendered.unmount()
    }
  })

  it('keeps layer row rings on the shared outline tokens without layout shift', async () => {
    const rendered = renderWithUser(LayersPanel, {
      props: { layers: makeLayers(), selectedLayerId: 'source' },
    })
    try {
      const row = screen.getByRole('option', { name: /Original/ }) as HTMLElement
      expect(styleOf(row).outlineStyle).toBe('none')
      const restBorder = styleOf(row).borderWidth
      const restWidth = row.offsetWidth
      row.focus()
      expect(row.matches(':focus-visible')).toBe(true)
      expect(styleOf(row).outlineStyle).toBe('solid')
      expect(styleOf(row).outlineWidth).toBe('2px')
      expect(styleOf(row).outlineOffset).toBe('2px')
      expect(styleOf(row).borderWidth).toBe(restBorder)
      expect(row.offsetWidth).toBe(restWidth)
      const offsetProbe = document.createElement('div')
      offsetProbe.style.outlineStyle = 'solid'
      offsetProbe.style.outlineOffset = 'var(--focus-outline-offset)'
      document.body.append(offsetProbe)
      try {
        expect(styleOf(row).outlineOffset).toBe(styleOf(offsetProbe).outlineOffset)
        document.documentElement.style.setProperty('--focus-outline-offset', '7px')
        try {
          await expect.poll(() => styleOf(row).outlineOffset).toBe('7px')
        } finally {
          document.documentElement.style.removeProperty('--focus-outline-offset')
        }
        await expect.poll(() => styleOf(row).outlineOffset).toBe('2px')
      } finally {
        offsetProbe.remove()
      }
      const colorProbe = document.createElement('div')
      colorProbe.style.outlineStyle = 'solid'
      colorProbe.style.outlineColor = 'var(--focus-outline-color)'
      document.body.append(colorProbe)
      try {
        expect(styleOf(row).outlineColor).toBe(styleOf(colorProbe).outlineColor)
        document.documentElement.style.setProperty('--focus-outline-color', 'rgb(255, 0, 0)')
        try {
          await expect.poll(() => styleOf(row).outlineColor).toBe('rgb(255, 0, 0)')
        } finally {
          document.documentElement.style.removeProperty('--focus-outline-color')
        }
        await expect.poll(() => styleOf(row).outlineColor).toBe(styleOf(colorProbe).outlineColor)
      } finally {
        colorProbe.remove()
      }
    } finally {
      rendered.unmount()
    }
  })

  it('keeps retouch rings under reduced motion while motion tokens collapse', async () => {
    await setMotion(true)
    expect(matchMedia('(prefers-reduced-motion: reduce)').matches).toBe(true)
    expect(
      styleOf(document.documentElement).getPropertyValue('--motion-control-duration').trim(),
    ).toBe('0ms')
    const points = normalizeCurveChannelPoints([{ id: 'mid', input: 50, output: 50 }])
    const rendered = renderWithUser(CurveGraph, {
      props: {
        activeCurvePath: 'M 0 100 L 100 0',
        ghostCurvePaths: [],
        histogramPath: '',
        points,
        selectedPointId: 'mid',
      },
    })
    try {
      const point = screen.getByRole('button', {
        name: 'Curve point In 50 Out 50',
      }) as HTMLElement
      point.focus()
      expect(point.matches(':focus-visible')).toBe(true)
      // The ring carries no animation: it still renders at full width even
      // when the motion policy zeroes every interaction token.
      expect(styleOf(point).outlineStyle).toBe('solid')
      expect(styleOf(point).outlineWidth).toBe('2px')
      expect(styleOf(point).outlineOffset).toBe('2px')
    } finally {
      rendered.unmount()
    }
  })
})
