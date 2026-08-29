import { defineComponent, h, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { RetouchCurvesParams } from '../types'
import RetouchCurvesControl from './RetouchCurvesControl.vue'
import { renderWithUser, screen, waitFor } from '../../../../test/render'

function renderCurvesControl(initialParams: RetouchCurvesParams) {
  const params = ref(initialParams)
  const Harness = defineComponent({
    setup() {
      return () =>
        h(RetouchCurvesControl, {
          params: params.value,
          'onUpdate:params': (nextParams: RetouchCurvesParams) => {
            params.value = nextParams
          },
        })
    },
  })

  return {
    params,
    ...renderWithUser(Harness),
  }
}

function dispatchDelete(target: Element | null) {
  target?.dispatchEvent(
    new KeyboardEvent('keydown', { key: 'Delete', bubbles: true, cancelable: true }),
  )
}

describe('RetouchCurvesControl', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('keeps keyboard focus on the next curve point after deleting the selected point', async () => {
    const { container, params } = renderCurvesControl({
      channels: {
        rgb: [
          { id: 'shadow', input: 25, output: 20 },
          { id: 'midtone', input: 50, output: 55 },
          { id: 'highlight', input: 75, output: 82 },
        ],
      },
    })

    const bubbledKeydown = vi.fn()
    container.addEventListener('keydown', bubbledKeydown)
    const midtonePoint = screen.getByRole('button', { name: 'Curve point In 50 Out 55' })

    midtonePoint.focus()
    dispatchDelete(midtonePoint)

    const highlightPoint = screen.getByRole('button', { name: 'Curve point In 75 Out 82' })

    await waitFor(() => expect(highlightPoint).toHaveFocus())
    expect(params.value.channels.rgb?.map((point) => point.id)).toEqual(['shadow', 'highlight'])

    dispatchDelete(document.activeElement)

    await waitFor(() => {
      expect(params.value.channels.rgb?.map((point) => point.id)).toEqual(['shadow'])
    })
    expect(bubbledKeydown).not.toHaveBeenCalled()
  })

  it('ignores cleared selected point number inputs instead of moving the point to zero', async () => {
    const { params, user } = renderCurvesControl({
      channels: {
        rgb: [{ id: 'midtone', input: 50, output: 55 }],
      },
    })

    await user.click(screen.getByRole('button', { name: 'Curve point In 50 Out 55' }))

    const inputField = screen.getByLabelText('Input')
    const outputField = screen.getByLabelText('Output')

    await user.clear(inputField)
    await user.clear(outputField)

    expect(params.value.channels.rgb).toEqual([{ id: 'midtone', input: 50, output: 55 }])
  })
})
