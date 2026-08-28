import { describe, expect, it, vi } from 'vitest'
import ImageEditorLayerActions from './ImageEditorLayerActions.vue'
import { renderWithUser, screen } from '../../../../test/render'

function renderLayerActions(
  options: {
    canCopyLayer?: boolean
    canMergeLayer?: boolean
    canRemoveLayer?: boolean
    applying?: boolean
    applyDisabled?: boolean
    onAddLayer?: () => void
    onCopyLayer?: () => void
    onMergeLayer?: () => void
    onRemoveLayer?: () => void
    onApplyAdjustments?: () => void
  } = {},
) {
  return renderWithUser(
    ImageEditorLayerActions,
    {
      props: {
        canCopyLayer: options.canCopyLayer ?? true,
        canMergeLayer: options.canMergeLayer ?? true,
        canRemoveLayer: options.canRemoveLayer ?? true,
        applying: options.applying ?? false,
        applyDisabled: options.applyDisabled ?? options.applying ?? false,
        'onAdd-layer': options.onAddLayer,
        'onCopy-layer': options.onCopyLayer,
        'onMerge-layer': options.onMergeLayer,
        'onRemove-layer': options.onRemoveLayer,
        'onApply-adjustments': options.onApplyAdjustments,
      },
    },
  )
}

describe('ImageEditorLayerActions', () => {
  it('renders all layer action buttons and emits actions on click', async () => {
    const onAddLayer = vi.fn()
    const onCopyLayer = vi.fn()
    const onMergeLayer = vi.fn()
    const onRemoveLayer = vi.fn()
    const onApplyAdjustments = vi.fn()

    const { user } = renderLayerActions({
      onAddLayer,
      onCopyLayer,
      onMergeLayer,
      onRemoveLayer,
      onApplyAdjustments,
    })

    const addBtn = screen.getByRole('button', { name: 'Add layer' })
    const copyBtn = screen.getByRole('button', { name: 'Duplicate layer' })
    const mergeBtn = screen.getByRole('button', { name: 'Merge layers' })
    const removeBtn = screen.getByRole('button', { name: 'Delete layer' })
    const applyBtn = screen.getByRole('button', { name: 'Apply as new node' })

    expect(addBtn).toBeInTheDocument()
    expect(copyBtn).toBeInTheDocument()
    expect(mergeBtn).toBeInTheDocument()
    expect(removeBtn).toBeInTheDocument()
    expect(applyBtn).toBeInTheDocument()

    await user.click(addBtn)
    expect(onAddLayer).toHaveBeenCalledTimes(1)

    await user.click(copyBtn)
    expect(onCopyLayer).toHaveBeenCalledTimes(1)

    await user.click(mergeBtn)
    expect(onMergeLayer).toHaveBeenCalledTimes(1)

    await user.click(removeBtn)
    expect(onRemoveLayer).toHaveBeenCalledTimes(1)

    await user.click(applyBtn)
    expect(onApplyAdjustments).toHaveBeenCalledTimes(1)
  })

  it('disables actions when capability props are false', () => {
    renderLayerActions({
      canCopyLayer: false,
      canMergeLayer: false,
      canRemoveLayer: false,
      applyDisabled: true,
    })

    expect(screen.getByRole('button', { name: 'Duplicate layer' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Merge layers' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Delete layer' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Apply as new node' })).toBeDisabled()
  })

  it('shows applying label when applying is active', () => {
    renderLayerActions({
      applying: true,
      applyDisabled: true,
    })

    expect(screen.getByRole('button', { name: 'Applying…' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Applying…' })).toBeDisabled()
  })

})
