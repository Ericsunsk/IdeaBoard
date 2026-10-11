import { describe, expect, it } from 'vitest'
import { renderWithUser, screen } from '../../../../../../test/render'
import AdminStorageEditorDialog from './AdminStorageEditorDialog.vue'

function renderDialog(fetching: boolean) {
  const form = {
    id: 'storage-1',
    name: '生产环境 OSS',
    provider: 's3-compatible',
    enabled: true,
    isCurrent: false,
    secretConfigured: true,
    config: {
      endpoint: 'oss-cn-shanghai.aliyuncs.com',
      bucket_name: 'prod-bucket',
      access_domain: '',
    },
  }
  return renderWithUser(AdminStorageEditorDialog, {
    props: {
      visible: true,
      editorMode: 'edit',
      selectedVendorKey: 'aliyun-oss',
      form,
      selectedProviderMeta: {
        provider: 's3-compatible',
        label: 'S3 兼容存储',
        description: '',
        builtin: false,
        create_allowed: true,
        supports_test: true,
        supports_bucket_info: true,
        fields: [],
      },
      textFields: [
        { key: 'endpoint', label: 'Endpoint', type: 'text', required: true },
        { key: 'bucket_name', label: 'Bucket 名称', type: 'text', required: true },
        { key: 'access_domain', label: '访问域名', type: 'text' },
        { key: 'access_key_id', label: 'Access Key ID', type: 'text', required: true },
      ],
      switchFields: [],
      bucketDomains: [],
      saving: false,
      fetching,
      configTextValue: (key: string) => String(form.config[key as keyof typeof form.config] ?? ''),
      configBoolValue: () => false,
      providerLabel: (provider: string) => provider,
    },
  })
}

describe('AdminStorageEditorDialog domain probe', () => {
  it('keeps the detect button idle without a status spinner when not fetching', async () => {
    const { user } = renderDialog(false)

    const endpointTab = await screen.findByRole('button', { name: '端点与网络' })
    await user.click(endpointTab)

    expect(await screen.findByLabelText('访问域名')).toBeInTheDocument()
    const detectButton = screen.getByRole('button', { name: '一键自动探测域名' })
    expect(detectButton).not.toBeDisabled()
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
    expect(screen.getByLabelText('访问域名')).not.toHaveAttribute('aria-busy')
  })

  it('shows the fetching spinner in the access domain input suffix instead of the detect button', async () => {
    const { user } = renderDialog(true)

    const endpointTab = await screen.findByRole('button', { name: '端点与网络' })
    await user.click(endpointTab)

    // 输入栏尾部出现可访问的 fetching 状态，按钮保持 disabled 但自身无 spinner。
    const status = await screen.findByRole('status')
    expect(status).toHaveTextContent('Fetching…')
    const spinner = status.querySelector('svg')
    expect(spinner?.getAttribute('aria-hidden')).toBe('true')
    const detectButton = screen.getByRole('button', { name: '一键自动探测域名' })
    expect(detectButton).toBeDisabled()
    expect(detectButton.querySelector('svg')).not.toBeInTheDocument()
    // loading 行为：输入栏标记 aria-busy（不测内部 Spinner 实现）。
    expect(screen.getByLabelText('访问域名')).toHaveAttribute('aria-busy', 'true')
  })
})
