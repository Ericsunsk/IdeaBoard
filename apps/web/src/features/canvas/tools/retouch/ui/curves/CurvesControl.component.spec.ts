import { defineComponent, h, nextTick, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { RetouchCurvesParams } from '../../core/model.ts'
import RetouchCurvesControl from './CurvesControl.vue'
import { renderWithUser, screen, waitFor, within } from '../../../../../../test/render'
import { setUserLocale } from '../../../../../../core/i18n'

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
  afterEach(async () => {
    vi.restoreAllMocks()
    await setUserLocale('en')
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

  it('renders preset and channel toolbar selects from one config with stable references', async () => {
    const { params, user } = renderCurvesControl({ channels: {} })

    const presetSelect = screen.getByRole('combobox', { name: 'Curves preset' })
    const channelSelect = screen.getByRole('combobox', { name: 'Curves channel' })

    expect(presetSelect).toHaveAttribute('name', 'retouch-curve-preset')
    expect(channelSelect).toHaveAttribute('name', 'retouch-curve-channel')
    expect(presetSelect.querySelector('option[value="custom"]')).not.toBeNull()
    expect(channelSelect.querySelectorAll('option')).toHaveLength(4)

    await user.selectOptions(channelSelect, 'red')
    await user.click(screen.getByRole('button', { name: 'Curve point In 0 Out 0' }))
    expect(params.value.channels).toEqual({})

    await user.selectOptions(presetSelect, 'brighten')
    expect(params.value.channels.rgb).toEqual([
      { id: 'preset:brighten:midtone', input: 50, output: 63 },
    ])
  })

  it('keeps endpoint input locked while output still moves through one axis handler', async () => {
    const { params, user } = renderCurvesControl({ channels: {} })

    const endpointButton = screen.getByRole('button', { name: 'Curve point In 0 Out 0' })
    await user.click(endpointButton)

    const inputField = screen.getByLabelText('Input') as HTMLInputElement
    const outputField = screen.getByLabelText('Output') as HTMLInputElement

    expect(inputField).toBeDisabled()
    expect(outputField).not.toBeDisabled()

    await user.clear(outputField)
    await user.type(outputField, '20')
    expect(params.value.channels.rgb).toEqual([{ id: 'endpoint:black', input: 0, output: 20 }])
  })

  it('re-translates preset options on locale switch without remount', async () => {
    renderCurvesControl({ channels: {} })

    const presetSelect = screen.getByRole('combobox', { name: 'Curves preset' })
    expect(within(presetSelect).getByRole('option', { name: 'Brighten' })).toBeVisible()

    await setUserLocale('es')
    await nextTick()

    expect(screen.getByRole('combobox', { name: 'Ajuste de curvas' })).toBeVisible()
    expect(within(presetSelect).getByRole('option', { name: 'Aclarar' })).toBeVisible()
  })
})
