import { defineComponent, h, ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import type { ImageEditorSharpenParams } from '../types'
import ImageEditorSharpenControl from './ImageEditorSharpenControl.vue'
import { renderWithUser, screen, waitFor } from '../../../../test/render'

function renderSharpenControl(initialParams: ImageEditorSharpenParams) {
  const params = ref(initialParams)
  const events = {
    updateParams: vi.fn((nextParams: ImageEditorSharpenParams) => {
      params.value = nextParams
    }),
  }
  const Harness = defineComponent({
    setup() {
      return () =>
        h(ImageEditorSharpenControl, {
          params: params.value,
          'onUpdate:params': events.updateParams,
        })
    },
  })
  return {
    events,
    params,
    ...renderWithUser(Harness),
  }
}

function updateInputValue(target: HTMLElement, value: string) {
  const input = target as HTMLInputElement
  input.value = value
  input.dispatchEvent(new Event('input', { bubbles: true }))
}

describe('ImageEditorSharpenControl', () => {
  it('keeps radius and threshold tracks visibly styled', () => {
    renderSharpenControl({ amount: 0, radius: 1, threshold: 4, protectColor: true })

    expect(screen.getByLabelText('Radius slider')).toHaveClass(
      'editing-controls-slider-row__range--tonal',
    )
    expect(screen.getByLabelText('Threshold slider')).toHaveClass(
      'editing-controls-slider-row__range--tonal',
    )
  })

  it('updates sharpen amount, decimal radius, threshold, and color protection', async () => {
    const { events, user } = renderSharpenControl({
      amount: 0,
      radius: 1,
      threshold: 4,
      protectColor: true,
    })

    updateInputValue(screen.getByLabelText('Amount slider'), '120')

    expect(events.updateParams).toHaveBeenCalledWith({
      amount: 120,
      radius: 1,
      threshold: 4,
      protectColor: true,
    })

    await waitFor(() => expect(screen.getByLabelText('Amount value')).toHaveValue(120))
    updateInputValue(screen.getByLabelText('Radius value'), '1.7')

    expect(events.updateParams).toHaveBeenCalledWith({
      amount: 120,
      radius: 1.7,
      threshold: 4,
      protectColor: true,
    })

    await waitFor(() => expect(screen.getByLabelText('Radius value')).toHaveValue(1.7))
    updateInputValue(screen.getByLabelText('Threshold value'), '60')

    expect(events.updateParams).toHaveBeenCalledWith({
      amount: 120,
      radius: 1.7,
      threshold: 50,
      protectColor: true,
    })

    await waitFor(() => expect(screen.getByLabelText('Threshold value')).toHaveValue(50))
    const protectColorButton = screen.getByRole('button', { name: 'Protect sharpen colors' })

    expect(protectColorButton).toHaveAttribute('aria-pressed', 'true')
    await user.click(protectColorButton)

    expect(events.updateParams).toHaveBeenCalledWith({
      amount: 120,
      radius: 1.7,
      threshold: 50,
      protectColor: false,
    })
  })
})
