import { defineComponent, ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent } from '@testing-library/vue'
import { renderWithUser, screen } from '../../../../../test/render'
import AdminLogoPickerPopover from './AdminLogoPickerPopover.vue'
import {
  getAdminBrandLogoOverride,
  replaceAdminBrandLogoOverrides,
} from '../../session/brand/brandLogos'

const logoResource = vi.hoisted(() => ({
  save: vi.fn().mockResolvedValue(undefined),
  remove: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('../../session/brand/brandLogoResource', async () => {
  const logos = await import('../../session/brand/brandLogos')
  return {
    useAdminBrandLogoResource: () => ({
      save: async (scope: 'provider-endpoint', target: number, key: string) => {
        await logoResource.save(scope, target, key)
        logos.replaceAdminBrandLogoOverrides([
          { scope, targetKey: String(target), logoKey: key as never },
        ])
      },
      remove: async (scope: 'provider-endpoint', target: number) => {
        await logoResource.remove(scope, target)
        logos.replaceAdminBrandLogoOverrides([])
      },
    }),
  }
})

type LogoPickerPopoverRef = {
  toggle: (event: Event) => void
}

const Harness = defineComponent({
  components: { AdminLogoPickerPopover },
  setup() {
    const popoverRef = ref<LogoPickerPopoverRef | null>(null)

    function open(event: Event) {
      popoverRef.value?.toggle(event)
    }

    return { open, popoverRef }
  },
  template: `
    <button type="button" @click="open">打开 Logo 选择器</button>
    <AdminLogoPickerPopover
      ref="popoverRef"
      scope="provider-endpoint"
      :target-id="101"
      target-name="Custom Endpoint"
    />
  `,
})

describe('AdminLogoPickerPopover', () => {
  beforeEach(() => {
    logoResource.save.mockReset()
    logoResource.remove.mockReset()
    logoResource.save.mockResolvedValue(undefined)
    logoResource.remove.mockResolvedValue(undefined)
    replaceAdminBrandLogoOverrides([])
  })

  it('renders brand logos, supports category filtering and logo picking', async () => {
    const { user } = renderWithUser(Harness, {})

    await user.click(screen.getByRole('button', { name: '打开 Logo 选择器' }))

    expect(await screen.findByText('选择品牌 Logo')).toBeInTheDocument()
    expect(screen.getByText('智能识别')).toBeInTheDocument()

    expect(screen.getByRole('button', { name: '全部' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '生图' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '国内' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '存储' })).toBeInTheDocument()

    // Filter by '国内'
    await user.click(screen.getByRole('button', { name: '国内' }))
    expect(screen.getByRole('button', { name: '智谱 AI (GLM / CogView)' })).toBeInTheDocument()

    // Click on Zhipu logo
    const zhipuBtn = screen.getByRole('button', { name: '智谱 AI (GLM / CogView)' })
    await user.click(zhipuBtn)

    expect(getAdminBrandLogoOverride('provider-endpoint', 101)).toBe('zhipu')
    expect(logoResource.save).toHaveBeenCalledWith('provider-endpoint', 101, 'zhipu')

    // Reopen and reset to auto
    await user.click(screen.getByRole('button', { name: '打开 Logo 选择器' }))
    const autoBtn = await screen.findByText('智能识别')
    await user.click(autoBtn)

    expect(getAdminBrandLogoOverride('provider-endpoint', 101)).toBeUndefined()
    expect(logoResource.remove).toHaveBeenCalledWith('provider-endpoint', 101)
  })

  it('falls back to a placeholder when a brand thumbnail fails to load', async () => {
    const { user } = renderWithUser(Harness, {})

    await user.click(screen.getByRole('button', { name: '打开 Logo 选择器' }))
    const fluxThumb = await screen.findByRole('img', { name: 'Flux (Black Forest Labs)' })
    await fireEvent.error(fluxThumb)

    // 破图缩略图隐藏回落占位，选择按钮仍在可继续点选。
    expect(screen.queryByRole('img', { name: 'Flux (Black Forest Labs)' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Flux (Black Forest Labs)' })).toBeInTheDocument()
  })

  it('keeps the current logo when the server mutation fails', async () => {
    logoResource.save.mockRejectedValueOnce(new Error('保存失败'))
    const { user } = renderWithUser(Harness, {})

    await user.click(screen.getByRole('button', { name: '打开 Logo 选择器' }))
    await user.click(screen.getByRole('button', { name: 'Flux (Black Forest Labs)' }))

    expect(getAdminBrandLogoOverride('provider-endpoint', 101)).toBeUndefined()
  })
})
