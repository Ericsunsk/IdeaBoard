import { effectScope } from 'vue'
import { describe, expect, it } from 'vitest'
import { useCanvasLibraryIsland } from './useCanvasLibraryIsland'

describe('useCanvasLibraryIsland', () => {
  it('starts closed and keeps its mounted Library session across close and reopen', () => {
    const scope = effectScope()
    const island = scope.run(useCanvasLibraryIsland)
    if (!island) throw new Error('Failed to create Canvas Library island')

    expect(island.view.open.value).toBe(false)
    expect(island.view.mounted.value).toBe(false)

    island.commands.toggle()
    expect(island.view.open.value).toBe(true)
    expect(island.view.mounted.value).toBe(true)

    island.commands.close()
    expect(island.view.open.value).toBe(false)
    expect(island.view.mounted.value).toBe(true)

    island.commands.setFocusWithin(true)
    expect(island.view.focusWithin.value).toBe(true)
    island.commands.close()
    expect(island.view.focusWithin.value).toBe(false)

    island.commands.toggle()
    expect(island.view.open.value).toBe(true)
    expect(island.view.mounted.value).toBe(true)

    scope.stop()
  })
})
