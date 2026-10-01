import { defineComponent, h, ref } from 'vue'
import { afterEach, describe, expect, it } from 'vitest'
import { renderWithUser, screen, within } from '../../../../../test/render'
import type { RetouchLayer } from '../model.ts'
import LayersPanel from './LayersPanel.vue'

// The panel only reads id/type/kind/name/visible, but `resolveLayerIcon` throws on
// an unhandled kind, so the fixtures use the real effect kinds.
function makeLayers(): RetouchLayer[] {
  return [
    { id: 'source', name: 'Original', type: 'source', visible: true },
    { id: 'hsl', name: 'Hue', type: 'effect', kind: 'hsl', visible: true },
    { id: 'curves', name: 'Curves', type: 'effect', kind: 'curves', visible: false },
    { id: 'sharpen', name: 'Sharpen', type: 'effect', kind: 'sharpen', visible: true },
  ] as unknown as RetouchLayer[]
}

function renderPanel(selectedId = 'source') {
  const selected = ref(selectedId)
  const selectedIds: string[] = []
  const visibilityChanges: Array<{ id: string; visible: boolean }> = []
  const Harness = defineComponent({
    setup() {
      return () =>
        h(LayersPanel, {
          layers: makeLayers(),
          selectedLayerId: selected.value,
          'onSelect-layer': (id: string) => {
            selectedIds.push(id)
            selected.value = id
          },
          'onSet-layer-visible': (id: string, visible: boolean) => {
            visibilityChanges.push({ id, visible })
          },
        })
    },
  })
  return { user: renderWithUser(Harness).user, selectedIds, visibilityChanges }
}

const options = () => screen.getAllByRole('option')

/** `noUncheckedIndexedAccess` is on, so indexing needs an explicit assertion. */
function optionAt(index: number): HTMLElement {
  const option = options()[index]
  if (!option) throw new Error(`no option at index ${index}`)
  return option
}

describe('Retouch LayersPanel keyboard model', () => {
  afterEach(() => {
    // Each render leaves its own rows behind otherwise.
    document.body.innerHTML = ''
  })

  it('keeps a single tab stop on the active option', () => {
    renderPanel('source')
    const tabbable = [...options(), ...screen.getAllByRole('button')].filter(
      (el) => el.tabIndex >= 0,
    )

    expect(tabbable).toHaveLength(1)
    const [first] = tabbable
    if (!first) throw new Error('expected one tabbable option')
    expect(first.getAttribute('aria-selected')).toBe('true')
  })

  it('keeps visibility buttons keyboard-operable without adding tab stops', async () => {
    const { user, selectedIds, visibilityChanges } = renderPanel('hsl')
    const activeRow = screen.getByRole('option', { name: /Hue/ })
    const visibilityButton = within(activeRow).getByRole('button', { name: 'Hide layer' })

    activeRow.focus()
    await user.keyboard('{ArrowRight}')
    expect(document.activeElement).toBe(visibilityButton)

    await user.keyboard('{ArrowLeft}')
    expect(document.activeElement).toBe(activeRow)

    await user.keyboard('{ArrowRight}')
    expect(document.activeElement).toBe(visibilityButton)

    await user.keyboard('{Space}')

    expect(visibilityChanges).toEqual([{ id: 'hsl', visible: false }])
    expect(selectedIds).toEqual([])
  })

  it('moves selection and focus with the arrow keys', async () => {
    const { user } = renderPanel('source')

    optionAt(0).focus()
    await user.keyboard('{ArrowDown}')
    expect(document.activeElement?.textContent).toContain('Hue')
    expect(screen.getByRole('option', { name: /Hue/ }).getAttribute('aria-selected')).toBe('true')

    await user.keyboard('{ArrowDown}')
    expect(document.activeElement?.textContent).toContain('Curves')

    await user.keyboard('{ArrowUp}')
    expect(document.activeElement?.textContent).toContain('Hue')
  })

  it('jumps to the first and last option with Home and End', async () => {
    const { user } = renderPanel('source')
    optionAt(0).focus()

    await user.keyboard('{End}')
    expect(document.activeElement?.textContent).toContain('Sharpen')

    await user.keyboard('{Home}')
    expect(document.activeElement?.textContent).toContain('Original')
  })

  it('does not wrap past the ends', async () => {
    const { user } = renderPanel('source')
    optionAt(0).focus()

    await user.keyboard('{ArrowUp}')
    expect(document.activeElement?.textContent).toContain('Original')

    await user.keyboard('{End}{ArrowDown}')
    expect(document.activeElement?.textContent).toContain('Sharpen')
  })

  it('still selects with Enter and Space', async () => {
    const { user, selectedIds } = renderPanel('source')

    optionAt(2).focus()
    await user.keyboard('{Enter}')
    await user.keyboard('{ArrowUp}{Space}')

    expect(selectedIds).toEqual(['curves', 'hsl'])
  })
})
