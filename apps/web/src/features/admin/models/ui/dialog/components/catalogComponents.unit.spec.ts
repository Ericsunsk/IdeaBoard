import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/vue'
import { userEvent } from '@testing-library/user-event'
import CatalogIdentityPanel from './CatalogIdentityPanel.vue'
import CatalogSpecsPanel from './CatalogSpecsPanel.vue'
import CatalogGatewayPanel from './CatalogGatewayPanel.vue'
import CatalogPreviewCard from './CatalogPreviewCard.vue'
import type { AdminModelItem } from '../../../core/catalogData'

describe('AdminModelEditDialog subcomponents', () => {
  describe('CatalogIdentityPanel', () => {
    it('renders fields and stores the registry slug on quick-pick', async () => {
      const user = userEvent.setup()
      const onUpdateProvider = vi.fn()
      render(CatalogIdentityPanel, {
        props: {
          modelId: '',
          modelName: 'My Model',
          modelProvider: '',
          modelDescription: 'Test desc',
          isEditing: false,
          providerOptions: [{ slug: 'fal', display_name: 'Fal AI', id_prefix: 'fal_ai/' }],
          'onUpdate:modelProvider': onUpdateProvider,
        },
      })

      expect(screen.getByLabelText(/模型标识/)).toBeInTheDocument()
      await user.click(screen.getByRole('button', { name: 'Fal AI' }))
      expect(onUpdateProvider).toHaveBeenCalledWith('fal')
    })
  })

  describe('CatalogSpecsPanel', () => {
    it('handles resolution presets and adding capabilities', async () => {
      const user = userEvent.setup()
      const onUpdateRes = vi.fn()
      const onUpdateCaps = vi.fn()
      render(CatalogSpecsPanel, {
        props: {
          contextOrResolution: '2048×2048',
          hosting: 'Cloudflare 托管',
          capabilitiesDraft: '垫图生图',
          'onUpdate:contextOrResolution': onUpdateRes,
          'onUpdate:capabilitiesDraft': onUpdateCaps,
        },
      })

      await user.click(screen.getByRole('button', { name: '4096×4096' }))

      expect(onUpdateRes).toHaveBeenCalledWith('4096×4096')

      await user.click(screen.getByRole('button', { name: '+ 透明通道' }))

      expect(onUpdateCaps).toHaveBeenCalledWith('垫图生图, 透明通道')
    })
  })

  describe('CatalogGatewayPanel', () => {
    it('toggles collapsible section and handles JSON formatting', async () => {
      const user = userEvent.setup()
      const onUpdateParams = vi.fn()
      render(CatalogGatewayPanel, {
        props: {
          customPath: '',
          customParams: '{"a":1}',
          initiallyOpen: true,
          'onUpdate:customParams': onUpdateParams,
        },
      })

      expect(screen.getByLabelText(/额外自定义请求参数/)).toBeInTheDocument()
      const formatBtn = screen.getByRole('button', { name: '格式化 JSON' })
      await user.click(formatBtn)
      expect(onUpdateParams).toHaveBeenCalledWith('{\n  "a": 1\n}')
    })
  })

  describe('CatalogPreviewCard', () => {
    it('renders preview model and route preview', () => {
      const sampleModel: AdminModelItem = {
        id: 'fal/flux-dev',
        name: 'FLUX.1 [dev]',
        provider: 'Fal AI',
        description: '优质生图模型',
        contextOrResolution: '2048×2048',
        capabilities: ['垫图生图'],
        hosting: 'Cloudflare 托管',
        termsUrl: '',
        docsUrl: '',
      }

      render(CatalogPreviewCard, {
        props: {
          model: sampleModel,
          modelId: 'fal/flux-dev',
          routePreview: {
            providerSlug: 'fal',
            isFirstParty: true,
            targetPath: 'fal-ai/flux-dev',
            protocol: '原生第一方',
          },
        },
      })

      expect(screen.getByText('卡片效果实时预览')).toBeInTheDocument()
      expect(screen.getByText('ID: fal/flux-dev')).toBeInTheDocument()
      expect(screen.getByText('Cloudflare AI Gateway 路由透视')).toBeInTheDocument()
      expect(screen.getAllByText('原生第一方').length).toBeGreaterThanOrEqual(1)
      expect(screen.getByText('fal-ai/flux-dev')).toBeInTheDocument()
    })
  })

})
