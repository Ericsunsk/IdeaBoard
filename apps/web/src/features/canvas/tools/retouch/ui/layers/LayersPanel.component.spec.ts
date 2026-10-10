import { defineComponent, h, nextTick, ref } from 'vue'
import { afterEach, describe, expect, it } from 'vitest'
import { renderWithUser, screen, within } from '../../../../../../test/render'
import type { RetouchLayer } from '../../core/model.ts'
import {
  applyLayerCommand,
  createDefaultImageEffectLayer,
  createInitialLayerStack,
} from '../../core/layers/layerStack.ts'
import { setUserLocale } from '../../../../../../core/i18n'
import LayersPanel from './LayersPanel.vue'

// The panel only reads id/type/kind/name/visible. Unknown effect kinds
// degrade to the generic thumb through the retouch view model (no throw),
// so the fallback fixture below can use a forward-compat kind.
function makeLayers(): RetouchLayer[] {
  return [
    { id: 'source', name: 'Original', type: 'source', visible: true },
    { id: 'hsl', name: 'Hue', type: 'effect', kind: 'hsl', visible: true },
    { id: 'curves', name: 'Curves', type: 'effect', kind: 'curves', visible: false },
    { id: 'sharpen', name: 'Sharpen', type: 'effect', kind: 'sharpen', visible: true },
  ] as unknown as RetouchLayer[]
}

function renderPanel(selectedId = 'source', layers: RetouchLayer[] = makeLayers()) {
  const selected = ref(selectedId)
  const selectedIds: string[] = []
  const visibilityChanges: Array<{ id: string; visible: boolean }> = []
  const Harness = defineComponent({
    setup() {
      return () =>
        h(LayersPanel, {
          layers,
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
  afterEach(async () => {
    // Each render leaves its own rows behind otherwise.
    document.body.innerHTML = ''
    await setUserLocale('en')
  })

  it('translates generated source, effects, paint, merges and copies when the locale changes', async () => {
    let stack = applyLayerCommand(createInitialLayerStack(), { type: 'add-paint' })
    stack = applyLayerCommand(stack, { type: 'add-paint' })
    stack = applyLayerCommand(stack, { type: 'merge-selected-down' })
    stack = applyLayerCommand(stack, { type: 'copy-selected' })
    stack = applyLayerCommand(stack, { type: 'add-paint' })
    renderPanel(stack.selectedLayerId, [
      ...stack.layers,
      ...(['hsl', 'threshold', 'curves', 'levels', 'sharpen'] as const).map((kind) =>
        createDefaultImageEffectLayer(kind),
      ),
    ])
    for (const name of [
      'Original',
      'Layer 3',
      'Merged layer 1',
      'Merged layer 1 copy',
      'Sharpen 1',
    ]) {
      expect(screen.getByText(name)).toBeTruthy()
    }

    await setUserLocale('zh-CN')
    await nextTick()

    for (const name of [
      '原图',
      '图层 3',
      '合并图层 1',
      '合并图层 1 副本',
      '色相/饱和度 1',
      '阈值 1',
      '曲线 1',
      '色阶 1',
      '锐化 1',
    ]) {
      expect(screen.getByText(name)).toBeTruthy()
    }
  })

  it('preserves custom names and translates only generated copy suffixes', async () => {
    let stack = applyLayerCommand(createInitialLayerStack(), { type: 'add-paint' })
    const original = stack.layers[0]!
    original.name = 'My custom layer'
    stack = applyLayerCommand(stack, { type: 'copy-selected' })
    stack = applyLayerCommand(stack, { type: 'copy-selected' })
    renderPanel(stack.selectedLayerId, stack.layers)

    await setUserLocale('zh-CN')
    await nextTick()

    expect(screen.getByText('My custom layer')).toBeTruthy()
    expect(screen.getByText('My custom layer 副本')).toBeTruthy()
    expect(screen.getByText('My custom layer 副本 副本')).toBeTruthy()
  })

  it('keeps paint numbers and copy collision suffixes unique across locale switches', async () => {
    let stack = applyLayerCommand(createInitialLayerStack(), { type: 'add-paint' })
    const originalId = stack.selectedLayerId
    stack = applyLayerCommand(stack, { type: 'copy-selected' })
    const { rerender } = renderWithUser(LayersPanel, {
      props: { layers: stack.layers, selectedLayerId: stack.selectedLayerId },
    })

    await setUserLocale('zh-CN')
    await nextTick()
    stack = applyLayerCommand(stack, { type: 'select-layer', layerId: originalId })
    stack = applyLayerCommand(stack, { type: 'copy-selected' })
    stack = applyLayerCommand(stack, { type: 'add-paint' })
    await rerender({ layers: stack.layers, selectedLayerId: stack.selectedLayerId })

    for (const name of ['图层 1', '图层 1 副本', '图层 1 副本 2', '图层 2']) {
      expect(screen.getByText(name)).toBeTruthy()
    }
    await setUserLocale('en')
    await nextTick()
    for (const name of ['Layer 1', 'Layer 1 copy', 'Layer 1 copy 2', 'Layer 2']) {
      expect(screen.getByText(name)).toBeTruthy()
    }
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

  it('degrades unknown effect kinds to the generic thumb instead of throwing', () => {
    const layers = [
      { id: 'source', name: 'Original', type: 'source', visible: true },
      {
        id: 'mystery',
        name: 'Mystery',
        type: 'effect',
        kind: 'sepia',
        visible: true,
        params: {},
      },
    ] as unknown as RetouchLayer[]
    renderPanel('mystery', layers)

    expect(screen.getByRole('option', { name: /Mystery/ })).toBeTruthy()
    expect(screen.getByRole('option', { name: /Original/ })).toBeTruthy()
  })
})
